// Package workers управляет очередью и пулом воркеров
package workers

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NGRsoftlab/pulsar/internal/types"
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
		head := q.head.Load()
		tail := q.tail.Load()

		// Очередь полна?
		if tail-head == q.size {
			return false
		}

		if q.tail.CompareAndSwap(tail, tail+1) {
			idx := tail % q.size
			q.buffer[idx].Store(job)

			select {
			case q.notify <- struct{}{}:
			default:
			}
			return true
		}
	}
}

// TryPop попытается достать задачу из очереди без ожидания
func (q *LockFreeQueue) TryPop() types.JobBatch {
	for {
		head := q.head.Load()
		tail := q.tail.Load()

		if head == tail {
			return nil
		}

		if q.head.CompareAndSwap(head, head+1) {
			idx := head % q.size
			val := q.buffer[idx].Load()
			if val != nil {
				return val.(types.JobBatch)
			}
			return nil
		}
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
		// Простая проверка в начале
		if wp.quit.Load() {
			// Простой drain без retry
			for {
				job := wp.queue.TryPop()
				if job == nil {
					return
				}
				start := time.Now()
				_ = job.ExecuteBatch()
				localCompleted++

				if localCompleted%100 == 0 {
					duration := time.Since(start)
					select {
					case wp.metricsChan <- func() {
						wp.metrics.IncrementCompletedJobs()
						wp.metrics.RecordProcessingTime(duration)
					}:
					default:
					}
					localCompleted = 0
				}
			}
		}

		job := wp.queue.TryPop()
		if job == nil {
			job = wp.queue.WaitPop(ctx)
			if job == nil {
				continue
			}
		}

		start := time.Now()
		_ = job.ExecuteBatch()
		localCompleted++

		if localCompleted%100 == 0 {
			duration := time.Since(start)
			select {
			case wp.metricsChan <- func() {
				wp.metrics.IncrementCompletedJobs()
				wp.metrics.RecordProcessingTime(duration)
			}:
			default:
			}
			localCompleted = 0
		}
	}
}

// Submit отправляет задачу в очередь
func (wp *WorkerPool) Submit(job types.JobBatch) bool {
	if job == nil {
		select {
		case wp.metricsChan <- func() {
			wp.metrics.IncrementRejectedJobs()
		}:
		default:
		}
		return false
	}

	if !wp.queue.TryPush(job) {
		wp.metricsChan <- func() {
			wp.metrics.IncrementRejectedJobs()
		}
		return false
	}

	select {
	case wp.metricsChan <- func() {
		head := wp.queue.head.Load()
		tail := wp.queue.tail.Load()
		wp.metrics.SetQueuedJobs(tail - head)
	}:
	default:
	}
	return true
}

// Stop останавливает пул воркеров
func (wp *WorkerPool) Stop() {
	wp.quit.Store(true)
	wp.wg.Wait()
	time.Sleep(5 * time.Millisecond)
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
			for {
				select {
				case f, ok := <-wp.metricsChan:
					if !ok {
						return
					}
					f()
				default:
					return
				}
			}
		}
	}
}
