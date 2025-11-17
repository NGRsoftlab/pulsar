package workers

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NGRsoftlab/pulsar/internal/types"
)

// ===== ОЧЕРЕДЬ =====

func BenchmarkLockFreeQueue_PushPop(b *testing.B) {
	q := NewLockFreeQueue(1000)
	job := &mockJob{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.TryPush(job)
		q.TryPop()
	}
}

func BenchmarkLockFreeQueue_Concurrent(b *testing.B) {
	q := NewLockFreeQueue(100000)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		job := &mockJob{}
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				q.TryPush(job)
			} else {
				q.TryPop()
			}
			i++
		}
	})
}

// ===== WORKERPOOL =====

func BenchmarkWorkerPool_Submit(b *testing.B) {
	wp := NewWorkerPool(8, 1024, func() types.JobBatch { return &mockJob{} }, &mockWorkerMetrics{})
	ctx, cancel := context.WithCancel(context.Background())
	wp.Start(ctx)
	defer func() {
		cancel()
		wp.Stop()
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			wp.Submit(&mockJob{})
		}
	})
}

func BenchmarkWorkerPool_Throughput(b *testing.B) {
	wp := NewWorkerPool(8, 1024, func() types.JobBatch { return &mockJob{} }, &mockWorkerMetrics{})
	ctx, cancel := context.WithCancel(context.Background())
	wp.Start(ctx)
	defer func() {
		cancel()
		wp.Stop()
	}()

	var completed int64

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		job := &countingJob{
			fn: func() {
				atomic.AddInt64(&completed, 1)
			},
		}
		wp.Submit(job)
	}
	b.StopTimer()

	time.Sleep(10 * time.Millisecond)
}

func BenchmarkWorkerPool_HighContention(b *testing.B) {
	wp := NewWorkerPool(2, 256, func() types.JobBatch { return &mockJob{} }, &mockWorkerMetrics{})
	ctx, cancel := context.WithCancel(context.Background())
	wp.Start(ctx)
	defer func() {
		cancel()
		wp.Stop()
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			wp.Submit(&mockJob{})
		}
	})
}
