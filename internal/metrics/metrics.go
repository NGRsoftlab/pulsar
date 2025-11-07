// Package metrics сбор и подсчет метрик пайплайна
package metrics

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"time"
)

// PerformanceMetrics - полная система метрик для UEBA генератора
type PerformanceMetrics struct {
	// === СЧЕТЧИКИ СОБЫТИЙ ===
	generatedEvents uint64 // Сколько событий сгенерировано
	sentEvents      uint64 // Сколько событий отправлено
	failedEvents    uint64 // Сколько событий завершилось ошибкой
	droppedEvents   uint64 // Сколько событий отброшено (queue full)

	// === СЕТЕВЫЕ МЕТРИКИ ===
	networkConnections uint64 // Количество активных соединений
	networkReconnects  uint64 // Количество переподключений
	networkTimeouts    uint64 // Количество таймаутов

	// === WORKER POOL МЕТРИКИ ===
	activeWorkers uint64 // Активные worker'ы
	queuedJobs    uint64 // Задач в очереди
	completedJobs uint64 // Выполненных задач
	rejectedJobs  uint64 // Отклоненных задач (queue full)

	// === ВРЕМЕННЫЕ МЕТРИКИ ===
	startTime          time.Time
	lastResetTime      time.Time
	lastEpsCalculation int64
	lastGeneratedCount uint64 // НОВОЕ ПОЛЕ для расчета текущего EPS

	// === ПРОИЗВОДИТЕЛЬНОСТЬ ===
	totalProcessingTime uint64 // Общее время обработки в наносекундах
	maxProcessingTime   uint64 // Максимальное время обработки события
	minProcessingTime   uint64 // Минимальное время обработки события

	// === MEMORY METRICS ===
	// totalAllocations   uint64 // Общее количество аллокаций
	// currentMemoryUsage uint64 // Текущее потребление памяти
}

func NewPerformanceMetrics() *PerformanceMetrics {
	now := time.Now()
	nowNano := time.Now().UnixNano()
	return &PerformanceMetrics{
		startTime:          now,
		lastResetTime:      now,
		lastEpsCalculation: nowNano,
		minProcessingTime:  ^uint64(0), // Максимальное значение uint64
	}
}

// === МЕТОДЫ ДЛЯ СОБЫТИЙ ===

func (m *PerformanceMetrics) IncrementGenerated() {
	atomic.AddUint64(&m.generatedEvents, 1)
}

func (m *PerformanceMetrics) IncrementSent() {
	atomic.AddUint64(&m.sentEvents, 1)
}

func (m *PerformanceMetrics) IncrementFailed() {
	atomic.AddUint64(&m.failedEvents, 1)
}

func (m *PerformanceMetrics) IncrementDropped() {
	atomic.AddUint64(&m.droppedEvents, 1)
}

// === МЕТОДЫ ДЛЯ СЕТИ ===

func (m *PerformanceMetrics) IncrementConnections() {
	atomic.AddUint64(&m.networkConnections, 1)
}

func (m *PerformanceMetrics) DecrementConnections() {
	atomic.AddUint64(&m.networkConnections, ^uint64(0)) // -1 в unsigned арифметике
}

func (m *PerformanceMetrics) IncrementReconnects() {
	atomic.AddUint64(&m.networkReconnects, 1)
}

func (m *PerformanceMetrics) IncrementTimeouts() {
	atomic.AddUint64(&m.networkTimeouts, 1)
}

// === МЕТОДЫ ДЛЯ WORKER POOL ===

func (m *PerformanceMetrics) IncrementActiveWorkers() {
	atomic.AddUint64(&m.activeWorkers, 1)
}

func (m *PerformanceMetrics) DecrementActiveWorkers() {
	atomic.AddUint64(&m.activeWorkers, ^uint64(0))
}

func (m *PerformanceMetrics) SetQueuedJobs(count uint64) {
	atomic.StoreUint64(&m.queuedJobs, count)
}

func (m *PerformanceMetrics) IncrementCompletedJobs() {
	atomic.AddUint64(&m.completedJobs, 1)
}

func (m *PerformanceMetrics) IncrementRejectedJobs() {
	atomic.AddUint64(&m.rejectedJobs, 1)
}

// === МЕТОДЫ ДЛЯ ПРОИЗВОДИТЕЛЬНОСТИ ===

func (m *PerformanceMetrics) RecordProcessingTime(duration time.Duration) {
	nanos := uint64(duration.Nanoseconds())

	// Проверяем валидность времени
	if nanos == 0 || duration < 0 {
		return // Игнорируем некорректные значения
	}

	// Обновляем общее время
	atomic.AddUint64(&m.totalProcessingTime, nanos)

	// Обновляем максимум
	for {
		current := atomic.LoadUint64(&m.maxProcessingTime)
		if nanos <= current {
			break
		}
		if atomic.CompareAndSwapUint64(&m.maxProcessingTime, current, nanos) {
			break
		}
	}

	for {
		current := atomic.LoadUint64(&m.minProcessingTime)
		// Если текущий минимум еще не установлен (максимальное значение uint64)
		if current == ^uint64(0) {
			if atomic.CompareAndSwapUint64(&m.minProcessingTime, current, nanos) {
				break
			}
			continue
		}
		// Если новое значение меньше текущего минимума
		if nanos >= current {
			break
		}
		if atomic.CompareAndSwapUint64(&m.minProcessingTime, current, nanos) {
			break
		}
	}
}

// calculateCurrentEPS рассчитывает EPS за последний период
func (m *PerformanceMetrics) calculateCurrentEPS() float64 {
	now := time.Now()
	nowNano := now.UnixNano()

	// Атомарно загружаем предыдущие значения
	lastCalcNano := atomic.LoadInt64(&m.lastEpsCalculation)
	lastCount := atomic.LoadUint64(&m.lastGeneratedCount)

	currentGenerated := atomic.LoadUint64(&m.generatedEvents)

	// Первый вызов?
	if lastCount == 0 && lastCalcNano == 0 {
		atomic.StoreUint64(&m.lastGeneratedCount, currentGenerated)
		atomic.StoreInt64(&m.lastEpsCalculation, nowNano)
		return 0.0
	}

	durationSec := float64(nowNano-lastCalcNano) / 1e9
	if durationSec < 1.0 {
		return 0.0
	}

	eventsDiff := currentGenerated - lastCount
	currentEPS := float64(eventsDiff) / durationSec

	atomic.StoreUint64(&m.lastGeneratedCount, currentGenerated)
	atomic.StoreInt64(&m.lastEpsCalculation, nowNano)

	return currentEPS
}

// === МЕТОДЫ ПОЛУЧЕНИЯ СТАТИСТИКИ ===

func (m *PerformanceMetrics) GetStats() (generated, sent, failed uint64, eps float64) {
	generated = atomic.LoadUint64(&m.generatedEvents)
	sent = atomic.LoadUint64(&m.sentEvents)
	failed = atomic.LoadUint64(&m.failedEvents)

	duration := time.Since(m.startTime).Seconds()
	if duration > 0 {
		eps = float64(generated) / duration
	}

	return generated, sent, failed, eps
}

// DetailedStats возвращает подробную статистику
type DetailedStats struct {
	// События
	Generated uint64 `json:"generated"`
	Sent      uint64 `json:"sent"`
	Failed    uint64 `json:"failed"`
	Dropped   uint64 `json:"dropped"`

	// Производительность
	EPS               float64       `json:"eps"`
	CurrentEPS        float64       `json:"current_eps"`
	AvgProcessingTime time.Duration `json:"avg_processing_time"`
	MaxProcessingTime time.Duration `json:"max_processing_time"`
	MinProcessingTime time.Duration `json:"min_processing_time"`

	// Сеть
	NetworkConnections uint64 `json:"network_connections"`
	NetworkReconnects  uint64 `json:"network_reconnects"`
	NetworkTimeouts    uint64 `json:"network_timeouts"`

	// Worker Pool
	ActiveWorkers uint64 `json:"active_workers"`
	QueuedJobs    uint64 `json:"queued_jobs"`
	CompletedJobs uint64 `json:"completed_jobs"`
	RejectedJobs  uint64 `json:"rejected_jobs"`

	// Система
	Runtime     time.Duration `json:"runtime"`
	MemoryUsage uint64        `json:"memory_usage_mb"`
	Goroutines  int           `json:"goroutines"`
	GCPauses    uint32        `json:"gc_pauses"`
}

func (m *PerformanceMetrics) GetDetailedStats() DetailedStats {
	generated := atomic.LoadUint64(&m.generatedEvents)
	sent := atomic.LoadUint64(&m.sentEvents)
	failed := atomic.LoadUint64(&m.failedEvents)
	dropped := atomic.LoadUint64(&m.droppedEvents)

	totalProcessing := atomic.LoadUint64(&m.totalProcessingTime)
	maxProcessing := atomic.LoadUint64(&m.maxProcessingTime)
	minProcessing := atomic.LoadUint64(&m.minProcessingTime)

	// ИСПРАВЛЕНИЕ: переименовываем переменную
	uptime := time.Since(m.startTime) // Было: runtime := time.Since(m.startTime)
	uptimeSeconds := uptime.Seconds() // Было: runtimeSeconds := runtime.Seconds()

	// Текущий EPS (за последние 10 секунд)
	currentEPS := m.calculateCurrentEPS()

	// Средняя EPS за все время
	var avgEPS float64
	if uptimeSeconds > 0 {
		avgEPS = float64(generated) / uptimeSeconds
	}

	// Среднее время обработки
	var avgProcessingTime time.Duration
	if generated > 0 {
		avgProcessingTime = time.Duration(totalProcessing / generated)
	}

	// ИСПРАВЛЕНИЕ: правильное использование пакета runtime
	var memStats runtime.MemStats // Было: runtime.memStats
	runtime.ReadMemStats(&memStats)

	return DetailedStats{
		Generated:          generated,
		Sent:               sent,
		Failed:             failed,
		Dropped:            dropped,
		EPS:                avgEPS,
		CurrentEPS:         currentEPS,
		AvgProcessingTime:  avgProcessingTime,
		MaxProcessingTime:  time.Duration(maxProcessing),
		MinProcessingTime:  time.Duration(minProcessing),
		NetworkConnections: atomic.LoadUint64(&m.networkConnections),
		NetworkReconnects:  atomic.LoadUint64(&m.networkReconnects),
		NetworkTimeouts:    atomic.LoadUint64(&m.networkTimeouts),
		ActiveWorkers:      atomic.LoadUint64(&m.activeWorkers),
		QueuedJobs:         atomic.LoadUint64(&m.queuedJobs),
		CompletedJobs:      atomic.LoadUint64(&m.completedJobs),
		RejectedJobs:       atomic.LoadUint64(&m.rejectedJobs),
		Runtime:            uptime,                       // Было: Runtime: runtime
		MemoryUsage:        memStats.Alloc / 1024 / 1024, // MB
		Goroutines:         runtime.NumGoroutine(),
		GCPauses:           memStats.NumGC,
	}
}

// String тоже нужно обновить
func (m *PerformanceMetrics) String() string {
	stats := m.GetDetailedStats()

	return fmt.Sprintf(`
📊 === UEBA GENERATOR METRICS ===
📈 Events: Generated=%d, Sent=%d, Failed=%d, Dropped=%d
⚡ Performance: %.1f EPS (avg), %.1f EPS (current)
⏱️  Processing: avg=%v, max=%v, min=%v
🌐 Network: Connections=%d, Reconnects=%d, Timeouts=%d
👷 Workers: Active=%d, Queued=%d, Completed=%d, Rejected=%d
💻 System: Uptime=%v, Memory=%dMB, Goroutines=%d, GC=%d
`,
		stats.Generated, stats.Sent, stats.Failed, stats.Dropped,
		stats.EPS, stats.CurrentEPS,
		stats.AvgProcessingTime, stats.MaxProcessingTime, stats.MinProcessingTime,
		stats.NetworkConnections, stats.NetworkReconnects, stats.NetworkTimeouts,
		stats.ActiveWorkers, stats.QueuedJobs, stats.CompletedJobs, stats.RejectedJobs,
		stats.Runtime.Truncate(time.Second), stats.MemoryUsage, stats.Goroutines, stats.GCPauses,
	)
}

// === ГЛОБАЛЬНЫЙ РЕЕСТР МЕТРИК ===

var globalMetrics *PerformanceMetrics

// GetGlobalMetrics возвращает глобальный экземпляр метрик
func GetGlobalMetrics() *PerformanceMetrics {
	if globalMetrics == nil {
		globalMetrics = NewPerformanceMetrics()
	}
	return globalMetrics
}
