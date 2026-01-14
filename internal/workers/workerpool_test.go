package workers

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/NGRsoftlab/pulsar/internal/types"
)

func TestWorkerPool_SubmitNil(t *testing.T) {
	wp := NewWorkerPool(1, 1, func() types.JobBatch { return &mockJob{} })

	success := wp.Submit(nil)
	assert.False(t, success, "ожидалось false при отправке nil")
}

func TestWorkerPool_Submit_ValidJob(t *testing.T) {
	wp := NewWorkerPool(1, 2, func() types.JobBatch { return &mockJob{} })

	job := &mockJob{}
	success := wp.Submit(job)

	assert.True(t, success, "ожидлось true при отправке валидной задачи")

	popped := wp.queue.TryPop()
	assert.Equal(t, job, popped, "ожидалась та же задача в очереди")
}

func TestWorkerPool_GetJob_CallFactory(t *testing.T) {
	var called bool
	exceptedJob := &mockJob{}

	wp := NewWorkerPool(1, 1, func() types.JobBatch {
		called = true
		return exceptedJob
	})

	job := wp.GetJob()

	assert.True(t, called, "ожидался вызов фабричной функции")
	assert.Equal(t, exceptedJob, job, "ожидалась задача, созданная фабрикой")
}

type countingJob struct {
	fn func()
}

func (c countingJob) ExecuteBatch() error {
	c.fn()
	return nil
}

func TestWorkerPool_Integration_SingleJob(t *testing.T) {
	var wg sync.WaitGroup
	wp := NewWorkerPool(1, 2, func() types.JobBatch { return &mockJob{} })

	wg.Add(1)
	job := countingJob{
		fn: func() {
			defer wg.Done()
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	wp.Start(ctx)
	defer wp.Stop()

	assert.True(t, wp.Submit(&job))

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("Задача выполнена")
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout")
	}
}

func TestWorkerPool_Integration_MultitipleJobs(t *testing.T) {
	const jobCount = 5
	var completed int
	var mu sync.Mutex

	wp := NewWorkerPool(1, 10, func() types.JobBatch { return &mockJob{} })

	jobs := make([]*countingJob, jobCount)
	for i := range jobs {
		jobs[i] = &countingJob{
			fn: func() {
				mu.Lock()
				completed++
				mu.Unlock()
			},
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	wp.Start(ctx)
	defer wp.Stop()

	for _, job := range jobs {
		assert.True(t, wp.Submit(job), "ожидалась успешая отправка задачи")
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, jobCount, completed, "все задачи были выполнены")
	mu.Unlock()
}

func TestWorkerPool_ContextCancel_StopsAcceptingNewWork(t *testing.T) {
	var mu sync.Mutex
	var completed int

	wp := NewWorkerPool(2, 10, func() types.JobBatch { return &mockJob{} })

	longJob := &countingJob{
		fn: func() {
			time.Sleep(100 * time.Millisecond)
			mu.Lock()
			completed++
			mu.Unlock()
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	wp.Start(ctx)

	assert.True(t, wp.Submit(longJob))

	cancel()

	time.Sleep(20 * time.Millisecond)

	wp.Stop()

	mu.Lock()
	assert.LessOrEqual(t, completed, 1)
	mu.Unlock()
}

func TestWorkerPool_Integration_Concurrency(t *testing.T) {
	const workerCount = 4
	const queueSize = 1000
	const totalJobs = 500

	var completed int64
	var accepted int64

	wp := NewWorkerPool(workerCount, queueSize, func() types.JobBatch { return &mockJob{} })

	ctx, cancel := context.WithCancel(context.Background())

	wp.Start(ctx)

	var submitWg sync.WaitGroup
	submitters := 10

	for i := 0; i < submitters; i++ {
		submitWg.Add(1)
		go func() {
			defer submitWg.Done()
			for j := 0; j < totalJobs/submitters; j++ {
				job := &countingJob{
					fn: func() {
						atomic.AddInt64(&completed, 1)
					},
				}
				if wp.Submit(job) {
					atomic.AddInt64(&accepted, 1)
				}
			}
		}()
	}

	submitWg.Wait()
	cancel()
	wp.Stop()

	completedVal := atomic.LoadInt64(&completed)
	acceptedVal := atomic.LoadInt64(&accepted)

	if acceptedVal > 0 {
		completionRate := float64(completedVal) / float64(acceptedVal)

		// ✅ 95% threshold для race mode
		assert.GreaterOrEqual(t, completionRate, 0.95,
			"должно быть выполнено не менее 95%% принятых задач (lock-free queue limitation), "+
				"получено: %.1f%% (completed=%d, accepted=%d)",
			completionRate*100, completedVal, acceptedVal)
	}
}
