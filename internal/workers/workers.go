// Package workers управляет очередью и пулом воркеров
package workers

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/NGRsoftlab/pulsar/internal/metrics"
	"github.com/NGRsoftlab/pulsar/internal/types"
)

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
	queue       *LockFreeQueue
	wg          sync.WaitGroup
	poolType    string
	newJobFunc  func() types.JobBatch
	started     atomic.Bool
	cancel      context.CancelFunc
}

// NewWorkerPool создаёт пул с пользовательской фабрикой задач
func NewWorkerPool(workerCount, queueSize int, newJobFunc func() types.JobBatch) *WorkerPool {
	if workerCount <= 0 {
		workerCount = runtime.NumCPU() * 2
	}

	wp := &WorkerPool{
		workerCount: workerCount,
		queue:       NewLockFreeQueue(queueSize),
		newJobFunc:  newJobFunc,
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

	ctx, wp.cancel = context.WithCancel(ctx)

	for i := 0; i < wp.workerCount; i++ {
		wp.wg.Add(1)
		go wp.worker(ctx)
	}
}

// worker — рабочая горутина
func (wp *WorkerPool) worker(ctx context.Context) {
	defer wp.wg.Done()
	metrics.ActiveWorkers.Inc()
	defer metrics.ActiveWorkers.Dec()

	localCompleted := uint64(0)

	for {
		job := wp.queue.TryPop()
		if job == nil {
			job = wp.queue.WaitPop(ctx)
			if job == nil {
				break
			}
		}

		_ = job.ExecuteBatch()
		localCompleted++

		if localCompleted%100 == 0 {
			metrics.CompletedJobs.Add(100)
			localCompleted = 0
		}
	}

	for {
		job := wp.queue.TryPop()
		if job == nil {
			break
		}
		_ = job.ExecuteBatch()
		localCompleted++
		if localCompleted%100 == 0 {
			metrics.CompletedJobs.Add(100)
			localCompleted = 0
		}
	}

	if localCompleted > 0 {
		metrics.CompletedJobs.Add(float64(localCompleted))
	}
}

// Submit отправляет задачу в очередь
func (wp *WorkerPool) Submit(job types.JobBatch) bool {
	if job == nil {
		metrics.RejectedJobs.Inc()
		return false
	}

	if !wp.queue.TryPush(job) {
		metrics.RejectedJobs.Inc()
		return false
	}

	head := wp.queue.head.Load()
	tail := wp.queue.tail.Load()
	metrics.QueuedJobs.Set(float64(tail - head))

	return true
}

// Stop останавливает пул воркеров
func (wp *WorkerPool) Stop() {
	if wp.cancel != nil {
		wp.cancel()
	}

	wp.wg.Wait()
}

// GetJob возвращает новую задачу от фабрики
func (wp *WorkerPool) GetJob() types.JobBatch {
	return wp.newJobFunc()
}
