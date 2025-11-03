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

type Sender interface {
	// Send отправляет данные по указанному destination
	// формат destination "host:port"
	Send(destination string, data []byte) error

	// Close закрывает все соединения
	Close() error

	// IsHealthy возвращает true, если Sender способен работать
	// Второе значение для диагностики
	IsHealthy() (bool, string)

	// SetTimeout устанавливает таймаут на операции записи
	SetTimeout(timeout time.Duration)
}

// NetworkSendingStage реализует SendingStage для отправки по сети
type NetworkSendingStage struct {
	destinations []string
	protocol     string
	timeout      time.Duration

	sender     Sender
	workerPool *workers.WorkerPool
	metrics    *metrics.PerformanceMetrics
	input      chan event.Event
}

func NewNetworkSendingStage() *NetworkSendingStage {
	sender, err := network.NewUDPSender("127.0.0.1:514", 5*time.Second)
	if err != nil {
		log.Fatalf("❌ Failed to create default UDP sender: %v", err)
	}

	workerPool := workers.NewWorkerPool(0, 5000, func() workers.JobBatch {
		return &NetworkSendJobBatch{
			data: make([]*SerializedData, 0, 50),
		}
	})
	workerPool.SetPoolType("network")

	return &NetworkSendingStage{
		destinations: []string{"127.0.0.1:514"},
		protocol:     "udp",
		timeout:      5 * time.Second,
		sender:       sender,
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
			log.Printf("Failed to send data to %s: %v", d.Destination, err)
		}
	}

	jb.data = jb.data[:0]
	return nil
}

func (s *NetworkSendingStage) Run(ctx context.Context, in <-chan *SerializedData, out chan<- *SerializedData, ready chan<- bool) error {
	s.workerPool.Start(ctx)

	if ready != nil {
		close(ready)
	}

	defer s.workerPool.Stop()
	defer s.sender.Close()
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
				if currentBatch != nil && len(currentBatch.data) > 0 {
					s.workerPool.Submit(currentBatch)
				}
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
			return ctx.Err()
		}
	}
}

// SendData — теперь делегирует всё sender'у
func (s *NetworkSendingStage) SendData(data *SerializedData) error {
	destination := data.Destination
	if destination == "" && len(s.destinations) > 0 {
		destination = s.destinations[0]
	}

	// Протокол теперь фиксирован на этапе (sender знает свой протокол)
	// Если нужно динамическое переключение — sender должен поддерживать мульти-протокол,
	// но пока оставляем как есть (как в оригинале)

	err := s.sender.Send(destination, data.Data)
	if err != nil {
		metrics.GetGlobalMetrics().IncrementFailed()
		return err
	}

	metrics.GetGlobalMetrics().IncrementSent()
	return nil
}

func (s *NetworkSendingStage) SetDestinations(destinations []string) error {
	if len(destinations) == 0 {
		return fmt.Errorf("destinations cannot be empty")
	}

	for _, dest := range destinations {
		if _, _, err := net.SplitHostPort(dest); err != nil {
			return fmt.Errorf("invalid destination address %s: %w", dest, err)
		}
	}

	newSender, err := s.createSender(destinations[0])
	if err != nil {
		return err
	}

	s.sender.Close()
	s.sender = newSender
	s.destinations = destinations
	return nil
}

func (s *NetworkSendingStage) SetProtocol(protocol string) error {
	if protocol != "udp" && protocol != "tcp" {
		return fmt.Errorf("unsupported protocol: %s (supported: udp, tcp)", protocol)
	}

	if s.protocol == protocol {
		return nil
	}

	newSender, err := s.createSender(s.destinations[0])
	if err != nil {
		return err
	}

	s.sender.Close()
	s.sender = newSender
	s.protocol = protocol
	log.Printf("🔄 Protocol changed to %s", protocol)
	return nil
}

func (s *NetworkSendingStage) createSender(dest string) (Sender, error) {
	switch s.protocol {
	case "udp":
		return network.NewUDPSender(dest, s.timeout)
	case "tcp":
		return network.NewTCPSender(dest, 12, s.timeout)
	default:
		return nil, fmt.Errorf("unknown protocol: %s", s.protocol)
	}
}

// ResizeConnectionPool и RecreateUnhealthyConnections
// теперь работают через sender (если он поддерживает)
// Для простоты оставим их как есть, но они будут возвращать ошибку,
// если sender не TCP
func (s *NetworkSendingStage) ResizeConnectionPool(newSize int) error {
	if s.protocol != "tcp" {
		return fmt.Errorf("pool resizing only supported for TCP")
	}
	return fmt.Errorf("pool resizing not implemented")
}

func (s *NetworkSendingStage) RecreateUnhealthyConnections() int {
	return 0
}

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

	return stats
}

func (s *NetworkSendingStage) IsHealthy() (bool, string) {
	if s.workerPool == nil {
		return false, "worker pool not initialized"
	}
	return s.sender.IsHealthy()
}

func (s *NetworkSendingStage) GetOptimizationRecommendations() []string {
	var recommendations []string
	_, sent, failed, dropped := metrics.GetGlobalMetrics().GetStats()

	if dropped > 0 {
		dropRate := float64(dropped) / (float64(sent) + float64(failed) + float64(dropped)) * 100.0
		if dropRate > 1.0 {
			recommendations = append(recommendations,
				fmt.Sprintf("High drop rate (%.1f%%) - consider increasing worker pool queue size", dropRate))
		}
	}

	if failed > 0 {
		failRate := float64(failed) / float64(sent+failed) * 100
		if failRate > 5.0 {
			recommendations = append(recommendations,
				fmt.Sprintf("High failure rate (%.1f%%) - check network connectivity", failRate))
		}
	}

	if len(recommendations) == 0 {
		recommendations = append(recommendations, "Stage operating optimally - no recommendations")
	}

	return recommendations
}
