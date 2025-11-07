// Package workers управляет очередью и пулом воркеров
package workers

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nashabanov/ueba-event-generator/internal/types"
)

type WorkerMetrics interface {
	IncrementActiveWorkers()
	DecrementActiveWorkers()
	IncrementCompletedJobs()
	IncrementRejectedJobs()
	SetQueuedJobs(count uint64)
	RecordProcessingTime(duration time.Duration)
}

// LockFreeQueue использует sync/atomic.Value для безопасного доступа к буферу
// Это избегает race condition, которую видит race detector
type LockFreeQueue struct {
	// Массив хранит слайсы с types.JobBatch для минимизации конкуренции
	// Вместо прямого доступа к buffer[index], используем atomic.Value
	buffer []atomic.Value // каждый элемент - atomic.Value с *types.JobBatch
	size   uint64
	head   atomic.Uint64
	tail   atomic.Uint64
	notify chan struct{}
}

func NewLockFreeQueue(capacity int) *LockFreeQueue {
	buffer := make([]atomic.Value, capacity)
	return &LockFreeQueue{
		buffer: buffer,
		size:   uint64(capacity),
		notify: make(chan struct{}, 1),
	}
}

// TryPush попытается поместить задачу в очередь
func (q *LockFreeQueue) TryPush(job types.JobBatch) bool {
	if job == nil {
		return false
	}

	for {
		tail := q.tail.Load()
		head := q.head.Load()

		if (tail+1)%q.size == head%q.size {
			return false
		}

		if !q.tail.CompareAndSwap(tail, tail+1) {
			continue
		}

		idx := tail % q.size
		q.buffer[idx].Store(job)

		select {
		case q.notify <- struct{}{}:
		default:
		}
		return true
	}
}

// TryPop попытается достать задачу из очереди без ожидания
func (q *LockFreeQueue) TryPop() types.JobBatch {
	for {
		head := q.head.Load()
		tail := q.tail.Load()

		if head%q.size == tail%q.size {
			return nil
		}

		if !q.head.CompareAndSwap(head, head+1) {
			continue
		}

		idx := head % q.size
		jobVal := q.buffer[idx].Load()

		if jobVal != nil {
			return jobVal.(types.JobBatch)
		}
		return nil
	}
}

// WaitPop ждёт элемента с контекстом
func (q *LockFreeQueue) WaitPop(ctx context.Context) types.JobBatch {
	for {
		job := q.TryPop()
		if job != nil {
			return job
		}
		select {
		case <-q.notify:
			continue
		case <-ctx.Done():
			return nil
		}
	}
}

// WorkerPool управляет пулом воркеров с lock-free очередью
type WorkerPool struct {
	workerCount int
	metrics     WorkerMetrics
	queue       *LockFreeQueue
	quit        atomic.Bool
	wg          sync.WaitGroup
	poolType    string
	newJobFunc  func() types.JobBatch
	metricsChan chan func()
	started     atomic.Bool
}

// NewWorkerPool создаёт пул с пользовательской фабрикой задач
func NewWorkerPool(workerCount, queueSize int, newJobFunc func() types.JobBatch, metrics WorkerMetrics) *WorkerPool {
	if workerCount <= 0 {
		workerCount = runtime.NumCPU() * 2
	}

	wp := &WorkerPool{
		workerCount: workerCount,
		queue:       NewLockFreeQueue(queueSize),
		newJobFunc:  newJobFunc,
		metricsChan: make(chan func(), 1000),
		metrics:     metrics,
	}

	return wp
}

func (wp *WorkerPool) SetPoolType(poolType string) {
	wp.poolType = poolType
}

// Start запускает пул воркеров
func (wp *WorkerPool) Start(ctx context.Context) {
	if wp.started.Swap(true) {
		return
	}

	wp.wg.Add(1)
	go wp.metricsWorker(ctx)

	for i := 0; i < wp.workerCount; i++ {
		wp.wg.Add(1)
		go wp.worker(ctx)
	}
}

// worker — рабочая горутина
func (wp *WorkerPool) worker(ctx context.Context) {
	defer wp.wg.Done()
	wp.metrics.IncrementActiveWorkers()
	defer wp.metrics.DecrementActiveWorkers()

	localCompleted := uint64(0)

	for {
		// Проверка принудительного завершения
		if wp.quit.Load() {
			return
		}

		// Сначала пробуем получить задачу без блокировки
		job := wp.queue.TryPop()
		if job == nil {
			// Если нет задач — ждём с учётом контекста
			job = wp.queue.WaitPop(ctx)
			if job == nil {
				// WaitPop вернул nil только если контекст отменён или quit=true
				if wp.quit.Load() {
					return
				}
				// Контекст отменён, но quit=false — всё равно завершаем
				return
			}
		}

		// Выполняем задачу
		start := time.Now()
		_ = job.ExecuteBatch()
		localCompleted++

		// Отправляем метрики каждые 100 задач
		if localCompleted%100 == 0 {
			duration := time.Since(start)
			wp.metricsChan <- func() {
				wp.metrics.IncrementCompletedJobs()
				wp.metrics.RecordProcessingTime(duration)
			}
			localCompleted = 0
		}
		// Цикл продолжится: на следующей итерации снова попробуем TryPop,
		// что позволит обработать все накопившиеся задачи без лишних пробуждений
	}
}

// Submit отправляет задачу в очередь
func (wp *WorkerPool) Submit(job types.JobBatch) bool {
	if job == nil {
		wp.metricsChan <- func() {
			wp.metrics.IncrementRejectedJobs()
		}
		return false
	}

	if wp.quit.Load() {
		wp.metricsChan <- func() {
			wp.metrics.IncrementRejectedJobs()
		}
		return false
	}

	if wp.queue.TryPush(job) {
		wp.metricsChan <- func() {
			head := wp.queue.head.Load()
			tail := wp.queue.tail.Load()
			wp.metrics.SetQueuedJobs(tail - head)
		}
		return true
	}

	wp.metricsChan <- func() {
		wp.metrics.IncrementRejectedJobs()
	}
	return false
}

// Stop останавливает пул воркеров
func (wp *WorkerPool) Stop() {
	wp.quit.Store(true)
	time.Sleep(100 * time.Millisecond)
	wp.wg.Wait()
	close(wp.metricsChan)
}

// GetJob возвращает новую задачу от фабрики
func (wp *WorkerPool) GetJob() types.JobBatch {
	return wp.newJobFunc()
}

// metricsWorker обрабатывает метрики
func (wp *WorkerPool) metricsWorker(ctx context.Context) {
	defer wp.wg.Done()
	for {
		select {
		case f, ok := <-wp.metricsChan:
			if !ok {
				return
			}
			f()
		case <-ctx.Done():
			return
		}
	}
}
