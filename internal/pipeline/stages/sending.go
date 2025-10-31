package stages

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/nashabanov/ueba-event-generator/internal/domain/event"
	"github.com/nashabanov/ueba-event-generator/internal/metrics"
	"github.com/nashabanov/ueba-event-generator/internal/network"
	"github.com/nashabanov/ueba-event-generator/internal/workers"
)

// NetworkSendingStage реализует SendingStage для отправки по сети
type NetworkSendingStage struct {
	destinations []string // ["127.0.0.1:514", "10.0.0.1:514"]
	protocol     string   // "udp" или "tcp"
	timeout      time.Duration

	tcpPool    *network.TCPConnectionPool
	workerPool *workers.WorkerPool
	udpConn    net.Conn
	metrics    *metrics.PerformanceMetrics
	input      chan event.Event
}

func NewNetworkSendingStage() *NetworkSendingStage {
	workerPool := workers.NewWorkerPool(0, 5000, func() workers.JobBatch {
		return &NetworkSendJobBatch{
			data: make([]*SerializedData, 0, 50), // предварительная ёмкость
		}
	})
	workerPool.SetPoolType("network")
	return &NetworkSendingStage{
		destinations: []string{"127.0.0.1:514"},
		protocol:     "udp",
		timeout:      5 * time.Second,
		workerPool:   workerPool,
		metrics:      metrics.NewPerformanceMetrics(),
		input:        make(chan event.Event, 1000),
	}
}

type NetworkSendJobBatch struct {
	stage *NetworkSendingStage
	data  []*SerializedData
}

func (jb *NetworkSendJobBatch) ExecuteBatch() error {
	if jb == nil {
		log.Printf("❌ CRITICAL: NetworkSendJobBatch is nil!")
		return fmt.Errorf("job batch is nil")
	}

	for _, d := range jb.data {
		if err := jb.stage.SendData(d); err != nil {
			// Логируем ошибку (опционально — можно сделать через debug-флаг)
			// log.Printf("Failed to send event %s: %v", d.ID, err)

			// Ошибки уже учитываются внутри SendData(),
			// поэтому здесь ничего дополнительно делать не нужно
		}
	}

	// Важно: очищаем слайс, но сохраняем ёмкость для переиспользования
	jb.data = jb.data[:0]

	return nil
}

func (s *NetworkSendingStage) Run(ctx context.Context, in <-chan *SerializedData, out chan<- *SerializedData, ready chan<- bool) error {
	// Инициализация соединений
	var err error
	if s.protocol == "tcp" && len(s.destinations) > 0 {
		poolSize := 12
		s.tcpPool, err = network.NewTCPConnectionPool(s.destinations[0], poolSize)
		if err != nil {
			return fmt.Errorf("failed to create TCP connection pool: %w", err)
		}
		log.Printf("✅ TCP connection pool created: %d connections to %s", poolSize, s.destinations[0])
	} else if s.protocol == "udp" && len(s.destinations) > 0 {
		s.udpConn, err = net.Dial("udp", s.destinations[0])
		if err != nil {
			return fmt.Errorf("failed to create UDP socket to %s: %w", s.destinations[0], err)
		}
		s.udpConn.SetWriteDeadline(time.Now().Add(s.timeout))
		log.Printf("✅ UDP socket created to %s", s.destinations[0])
	}

	// Запускаем worker pool
	s.workerPool.Start(ctx)

	// 🔥 Сигнализируем готовность ДО начала обработки данных
	if ready != nil {
		close(ready)
	}

	defer s.workerPool.Stop()
	defer s.closeConnections()

	const batchSize = 50
	const batchTimeout = 5 * time.Millisecond

	var (
		currentBatch *NetworkSendJobBatch
		timer        *time.Timer
		timerC       <-chan time.Time
	)

	for {
		select {
		case serializedData, ok := <-in:
			if !ok {
				// Канал закрыт — завершаем
				if currentBatch != nil && len(currentBatch.data) > 0 {
					s.workerPool.Submit(currentBatch)
				}
				// Статистика...
				return nil
			}

			if currentBatch == nil {
				currentBatch = s.workerPool.GetJob().(*NetworkSendJobBatch)
				currentBatch.stage = s
				currentBatch.data = currentBatch.data[:0]
				timer = time.NewTimer(batchTimeout)
				timerC = timer.C
			}

			currentBatch.data = append(currentBatch.data, serializedData)

			if len(currentBatch.data) >= batchSize {
				if !s.workerPool.Submit(currentBatch) {
					metrics.GetGlobalMetrics().IncrementDropped()
				}
				currentBatch = nil
				if timer != nil {
					timer.Stop()
					timer = nil
					timerC = nil
				}
			}

		case <-timerC:
			if currentBatch != nil && len(currentBatch.data) > 0 {
				if !s.workerPool.Submit(currentBatch) {
					metrics.GetGlobalMetrics().IncrementDropped()
				}
				currentBatch = nil
			}
			timer = nil
			timerC = nil

		case <-ctx.Done():
			if currentBatch != nil && len(currentBatch.data) > 0 {
				s.workerPool.Submit(currentBatch)
			}
			// Статистика...
			return ctx.Err()
		}
	}
}

// SendData отправляет данные по сети
func (s *NetworkSendingStage) SendData(data *SerializedData) error {
	globalMetrics := metrics.GetGlobalMetrics()

	destination := data.Destination
	if destination == "" && len(s.destinations) > 0 {
		destination = s.destinations[0]
	}

	protocol := data.Protocol
	if protocol == "" {
		protocol = s.protocol
	}

	var err error
	switch protocol {
	case "udp":
		err = s.sendUDP(destination, data.Data)
	case "tcp":
		err = s.sendTCP(destination, data.Data)
	default:
		err = fmt.Errorf("unsupported protocol: %s", protocol)
	}

	if err != nil {
		globalMetrics.IncrementFailed()
		return err
	}

	globalMetrics.IncrementSent()
	return nil
}

// sendUDP отправляет данные через UDP (stateless) - без изменений
func (s *NetworkSendingStage) sendUDP(destination string, data []byte) error {
	if s.udpConn == nil {
		return fmt.Errorf("UDP socket not initialized")
	}

	globalMetrics := metrics.GetGlobalMetrics()
	globalMetrics.IncrementConnections() // можно оставить для совместимости метрик
	defer globalMetrics.DecrementConnections()

	// Обновляем таймаут на каждый вызов (на случай долгой работы)
	s.udpConn.SetWriteDeadline(time.Now().Add(s.timeout))

	_, err := s.udpConn.Write(data)
	if err != nil {
		globalMetrics.IncrementTimeouts()
		return fmt.Errorf("failed to write UDP data to %s: %w", destination, err)
	}

	return nil
}

func (s *NetworkSendingStage) sendTCP(destination string, data []byte) error {
	if s.tcpPool == nil {
		return fmt.Errorf("TCP connection pool not initialized for destination %s", destination)
	}

	globalMetrics := metrics.GetGlobalMetrics()

	conn := s.tcpPool.GetConnection()
	if conn == nil {
		globalMetrics.IncrementTimeouts()
		return fmt.Errorf("no healthy TCP connections available to %s", destination)
	}

	dataWithNewline := append(data, '\n')

	written := 0
	for written < len(dataWithNewline) {
		n, err := conn.Write(dataWithNewline[written:])
		if err != nil {
			conn.MarkUnhealthy()
			globalMetrics.IncrementTimeouts()
			return fmt.Errorf("failed to write TCP data to %s: %w", destination, err)
		}
		written += n
	}

	return nil
}

func (s *NetworkSendingStage) closeConnections() {
	if s.tcpPool != nil {
		s.tcpPool.Close()
		log.Printf("TCP connection pool closed")
		s.tcpPool = nil
	}

	if s.udpConn != nil {
		s.udpConn.Close()
		log.Printf("UDP socket closed")
		s.udpConn = nil
	}
}

func (s *NetworkSendingStage) GetConnectionStats() (total, healthy int) {
	if s.tcpPool != nil {
		return s.tcpPool.GetStats()
	}
	return 0, 0
}

func (s *NetworkSendingStage) GetConnectionPoolInfo() string {
	if s.tcpPool == nil {
		return "TCP pool: not initialized"
	}

	total, healthy := s.tcpPool.GetStats()
	return fmt.Sprintf("TCP pool: %d/%d healthy connections", healthy, total)
}

// Конфигурационные методы
func (s *NetworkSendingStage) SetDestinations(destinations []string) error {
	if len(destinations) == 0 {
		return fmt.Errorf("destinations cannot be empty")
	}

	for _, dest := range destinations {
		if _, _, err := net.SplitHostPort(dest); err != nil {
			return fmt.Errorf("invalid destination address %s: %w", dest, err)
		}
	}

	if s.tcpPool != nil && len(destinations) > 0 && destinations[0] != s.destinations[0] {
		log.Printf("🔄 Destination changed from %s to %s, TCP pool will be recreated on next run",
			s.destinations[0], destinations[0])
		s.closeConnections() // Закрываем старый pool
	}

	s.destinations = destinations
	return nil
}

func (s *NetworkSendingStage) SetProtocol(protocol string) error {
	if protocol != "udp" && protocol != "tcp" {
		return fmt.Errorf("unsupported protocol: %s (supported: udp, tcp)", protocol)
	}

	// ✅ ЕСЛИ меняем протокол - закрываем TCP pool
	if s.protocol == "tcp" && protocol != "tcp" {
		s.closeConnections()
		log.Printf("🔄 Protocol changed from TCP to %s, connection pool closed", protocol)
	}

	s.protocol = protocol
	return nil
}

func (s *NetworkSendingStage) ResizeConnectionPool(newSize int) error {
	if s.tcpPool == nil {
		return fmt.Errorf("TCP connection pool not initialized")
	}

	if newSize < 1 || newSize > 50 {
		return fmt.Errorf("invalid pool size %d, must be between 1 and 50", newSize)
	}

	total, healthy := s.tcpPool.GetStats()
	log.Printf("🔧 TCP pool resize requested: current=%d healthy/%d total, requested=%d",
		healthy, total, newSize)
	log.Printf("⚠️ Pool resizing not implemented - requires restart to change size")

	return fmt.Errorf("pool resizing not implemented - restart application with new configuration")
}

func (s *NetworkSendingStage) RecreateUnhealthyConnections() int {
	if s.tcpPool == nil {
		return 0
	}

	total, healthy := s.tcpPool.GetStats()
	unhealthy := total - healthy

	if unhealthy > 0 {
		log.Printf("🔄 Attempting to recreate %d unhealthy connections", unhealthy)
		return unhealthy
	}

	return 0
}

// Методы метрик
func (s *NetworkSendingStage) GetSentCount() uint64 {
	_, sent, _, _ := metrics.GetGlobalMetrics().GetStats()
	return sent
}

func (s *NetworkSendingStage) GetFailedCount() uint64 {
	_, _, failed, _ := metrics.GetGlobalMetrics().GetStats()
	return failed
}

func (s *NetworkSendingStage) GetStageStats() map[string]any {
	_, sent, failed, dropped := metrics.GetGlobalMetrics().GetStats()

	stats := map[string]any{
		"protocol":            s.protocol,
		"destinations":        len(s.destinations),
		"events_sent":         sent,
		"events_failed":       failed,
		"events_dropped":      dropped,
		"worker_pool_healthy": s.workerPool != nil,
	}

	if s.tcpPool != nil {
		total, healthy := s.tcpPool.GetStats()
		stats["tcp_connections_total"] = total
		stats["tcp_connections_healthy"] = healthy
		stats["tcp_pool_efficiency"] = float64(healthy) / float64(total) * 100.0
	}

	return stats
}

func (s *NetworkSendingStage) IsHealthy() (bool, string) {
	if s.workerPool == nil {
		return false, "worker pool not initialized"
	}

	if s.protocol == "tcp" {
		if s.tcpPool == nil {
			return false, "TCP protocol selected but connection pool not initialized"
		}

		total, healthy := s.tcpPool.GetStats()
		if healthy == 0 {
			return false, fmt.Sprintf("no healthy TCP connections (0/%d)", total)
		}

		efficiency := float64(healthy) / float64(total)
		if efficiency < 0.5 {
			return false, fmt.Sprintf("TCP connection pool degraded: %d/%d healthy (%.1f%%)",
				healthy, total, efficiency*100)
		}
	}

	return true, "stage healthy"
}

func (s *NetworkSendingStage) GetOptimizationRecommendations() []string {
	var recommendations []string

	_, sent, failed, dropped := metrics.GetGlobalMetrics().GetStats()

	if dropped > 0 {
		dropRate := float64(dropped) / float64(float64(sent)+float64(failed)+float64(dropped)) * 100
		if dropRate > 1.0 {
			recommendations = append(recommendations,
				fmt.Sprintf("High drop rate (%.1f%%) - consider increasing worker pool queue size", dropRate))
		}
	}

	if failed > 0 {
		failRate := float64(failed) / float64(sent+failed) * 100
		if failRate > 5.0 {
			recommendations = append(recommendations,
				fmt.Sprintf("High failure rate (%.1f%%) - check network connectivity and destination availability", failRate))
		}
	}

	if s.protocol == "tcp" && s.tcpPool != nil {
		total, healthy := s.tcpPool.GetStats()
		if healthy < total {
			recommendations = append(recommendations,
				fmt.Sprintf("TCP connections degraded (%d/%d) - consider connection timeout tuning", healthy, total))
		}

		if total < 5 && sent > 50000 {
			recommendations = append(recommendations,
				"High load with few TCP connections - consider increasing connection pool size")
		}
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Stage operating optimally - no recommendations")
	}

	return recommendations
}
