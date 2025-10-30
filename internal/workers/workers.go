package workers

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nashabanov/ueba-event-generator/internal/metrics"
)

// JobBatch — интерфейс для пакетной обработки задач
type JobBatch interface {
	ExecuteBatch() error
}

// LockFreeQueue + event notification
type LockFreeQueue struct {
	buffer []JobBatch
	size   uint64
	head   uint64
	tail   uint64
	notify chan struct{} // сигнал для ожидания новых элементов
}

func NewLockFreeQueue(capacity int) *LockFreeQueue {
	return &LockFreeQueue{
		buffer: make([]JobBatch, capacity),
		size:   uint64(capacity),
		notify: make(chan struct{}, 1),
	}
}

func (q *LockFreeQueue) TryPush(job JobBatch) bool {
	if job == nil {
		return false
	}

	tail := atomic.LoadUint64(&q.tail)
	head := atomic.LoadUint64(&q.head)

	if (tail+1)%q.size == head%q.size {
		return false // full
	}

	q.buffer[tail%q.size] = job
	atomic.StoreUint64(&q.tail, tail+1)

	select {
	case q.notify <- struct{}{}:
	default:
	}
	return true
}

func (q *LockFreeQueue) TryPop() JobBatch {
	head := atomic.LoadUint64(&q.head)
	tail := atomic.LoadUint64(&q.tail)

	if head%q.size == tail%q.size {
		return nil // empty
	}

	job := q.buffer[head%q.size]
	q.buffer[head%q.size] = nil
	atomic.StoreUint64(&q.head, head+1)
	return job
}

func (q *LockFreeQueue) WaitPop(ctx context.Context) JobBatch {
	for {
		job := q.TryPop()
		if job != nil {
			return job
		}
		select {
		case <-q.notify:
			continue
		case <-ctx.Done():
			return nil // Возвращает nil при отмене контекста
		}
	}
}

// WorkerPool
type WorkerPool struct {
	workerCount int
	queue       *LockFreeQueue
	quit        chan struct{}
	wg          sync.WaitGroup
	poolType    string
	newJobFunc  func() JobBatch
	metricsChan chan func()
}

// NewWorkerPool создаёт пул с пользовательской фабрикой задач
func NewWorkerPool(workerCount, queueSize int, newJobFunc func() JobBatch) *WorkerPool {
	if workerCount <= 0 {
		workerCount = runtime.NumCPU() * 2
	}

	wp := &WorkerPool{
		workerCount: workerCount,
		queue:       NewLockFreeQueue(queueSize),
		quit:        make(chan struct{}),
		newJobFunc:  newJobFunc,
		metricsChan: make(chan func(), 1000),
	}

	return wp
}

func (wp *WorkerPool) SetPoolType(poolType string) {
	wp.poolType = poolType
}

func (wp *WorkerPool) Start(ctx context.Context) {
	wp.wg.Add(1)
	go wp.metricsWorker(ctx)

	for i := 0; i < wp.workerCount; i++ {
		wp.wg.Add(1)
		go wp.worker(i, ctx)
	}
}

func (wp *WorkerPool) worker(id int, ctx context.Context) {
	defer wp.wg.Done()
	globalMetrics := metrics.GetGlobalMetrics()
	globalMetrics.IncrementActiveWorkers()
	defer globalMetrics.DecrementActiveWorkers()

	localCompleted := uint64(0)

	for {
		select {
		case <-wp.quit:
			return
		default:
			job := wp.queue.WaitPop(ctx)
			// WaitPop возвращает nil при отмене контекста или пустой очереди
			if job == nil {
				return
			}

			start := time.Now()
			// ExecuteBatch может вернуть ошибку, но мы её игнорируем
			_ = job.ExecuteBatch()
			localCompleted++

			if localCompleted%100 == 0 {
				// Асинхронное обновление метрик
				wp.metricsChan <- func() {
					globalMetrics.IncrementCompletedJobs()
					globalMetrics.RecordProcessingTime(time.Since(start))
				}
				localCompleted = 0
			}
		}
	}
}

func (wp *WorkerPool) Submit(job JobBatch) bool {
	globalMetrics := metrics.GetGlobalMetrics()

	if job == nil {
		wp.metricsChan <- func() {
			globalMetrics.IncrementRejectedJobs()
		}
		return false
	}

	if wp.queue.TryPush(job) {
		wp.metricsChan <- func() {
			globalMetrics.SetQueuedJobs(atomic.LoadUint64(&wp.queue.tail) - atomic.LoadUint64(&wp.queue.head))
		}
		return true
	}

	wp.metricsChan <- func() {
		globalMetrics.IncrementRejectedJobs()
	}
	return false
}

func (wp *WorkerPool) Stop() {
	close(wp.quit)
	wp.wg.Wait()
	close(wp.metricsChan)
}

func (wp *WorkerPool) GetJob() JobBatch {
	return wp.newJobFunc()
}

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
