package stages

import (
	"context"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NGRsoftlab/pulsar/internal/domain/event"
	"github.com/NGRsoftlab/pulsar/internal/types"
)

// mockWorkerPool имитирует простой WorkerPool без реальной параллельности (для тестов)
type mockWorkerPool struct {
	mu     sync.Mutex
	jobs   []types.JobBatch
	closed bool
}

func newMockWorkerPool() *mockWorkerPool {
	return &mockWorkerPool{
		jobs: make([]types.JobBatch, 0),
	}
}

func (wp *mockWorkerPool) Start(_ context.Context) {}

func (wp *mockWorkerPool) Stop() {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.closed = true
	wp.jobs = nil
}

func (wp *mockWorkerPool) Submit(job types.JobBatch) bool {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if wp.closed {
		return false
	}
	wp.jobs = append(wp.jobs, job)
	job.ExecuteBatch()
	return true
}

func (wp *mockWorkerPool) GetJob() types.JobBatch {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if len(wp.jobs) == 0 {
		return nil
	}
	job := wp.jobs[0]
	wp.jobs = wp.jobs[1:]
	return job
}

// Тест создания нового батча
func TestNewEventGenerationJobBatch(t *testing.T) {
	batch := NewEventGenerationJobBatch(10)
	assert.NotNil(t, batch)
	assert.Empty(t, batch.events)
	assert.Equal(t, 0, len(batch.events))
	assert.Equal(t, 10, cap(batch.events))
}

// Тест ExecuteBatch с Nil
func TestEventGenerationJobBatch_ExecuteBatch_Nil(t *testing.T) {
	var batch *EventGenerationJobBatch
	err := batch.ExecuteBatch()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "job batch is nil")
}

// Тест ExecuteBatch с событием NetflowEvent
func TestEventGenerationJobBatch_ExecuteBatch_Netflow(t *testing.T) {
	stage := &EventGenerationStage{
		serializationMode: SerializationModeBinary,
		packetMode:        false,
	}

	out := make(chan *SerializedData, 10)
	evt := event.NewNetflowEvent()
	_ = evt.GenerateRandomTrafficData()

	batch := &EventGenerationJobBatch{
		stage:  stage,
		out:    out,
		events: []event.Event{evt},
	}

	err := batch.ExecuteBatch()
	require.NoError(t, err)
	assert.Empty(t, batch.events) // должен быть очищен

	select {
	case data := <-out:
		assert.NotNil(t, data)
		assert.Equal(t, event.EventTypeNetflow, data.EventType)
	default:
		assert.Fail(t, "expected serialized data in output channel")
	}
}

// fakeEvent — минимальная реализация event.Event для тестов
type fakeEvent struct{}

func (f *fakeEvent) Type() event.Type {
	return event.EventTypeUnknown
}

func (f *fakeEvent) Timestamp() time.Time {
	return time.Unix(0, 0) // или time.Now(), но фиксированное значение лучше для тестов
}

func (f *fakeEvent) Size() int {
	return 0
}

func (f *fakeEvent) Validate() error {
	return nil // или возвращайте ошибку, если нужно тестировать ошибки
}

func (f *fakeEvent) GetID() string {
	return "fake-id"
}

func (f *fakeEvent) GetSourceIP() netip.Addr {
	addr, _ := netip.ParseAddr("192.0.2.1") // TEST-NET-1 (RFC 5737)
	return addr
}

func (f *fakeEvent) GetDestinationIP() netip.Addr {
	addr, _ := netip.ParseAddr("192.0.2.2")
	return addr
}

// Тест генерации NetflowEvent
func TestEventGenerationStage_generateEvent(t *testing.T) {
	stage := &EventGenerationStage{
		eventType:         event.EventTypeNetflow,
		serializationMode: SerializationModeBinary,
		packetMode:        false,
	}

	evt, err := stage.generateEvent()
	require.NoError(t, err)
	assert.NotNil(t, evt)
	assert.Equal(t, event.EventTypeNetflow, evt.Type())
}

// Тест Run с ограничением скорости и обработкой через mock pool
func TestEventGenerationStage_Run(t *testing.T) {
	wp := newMockWorkerPool()
	defer wp.Stop()

	stage := NewEventGenerationStage(
		event.EventTypeNetflow,
		100,
		SerializationModeBinary,
		true, // packetMode
		wp,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	out := make(chan *SerializedData, 1000) // большой буфер

	// Запускаем consumer, чтобы избежать блокировки канала
	var received int
	receivedCh := make(chan int)
	go func() {
		for range out {
			received++
		}
		receivedCh <- received
	}()

	ready := make(chan bool)
	errCh := make(chan error, 1)
	go func() {
		errCh <- stage.Run(ctx, nil, out, ready)
	}()

	<-ready

	select {
	case err := <-errCh:
		if err != nil &&
			err != context.DeadlineExceeded &&
			err.Error() != "rate: Wait(n=1) would exceed context deadline" {
			t.Fatalf("Unexpected error: %v", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("stage did not finish in time")
	}

	// Закрываем out, чтобы завершить consumer
	close(out)
	received = <-receivedCh

	assert.GreaterOrEqual(t, received, 1)
}

// Тест остановки по контексту и отправки оставшегося батча
func TestEventGenerationStage_Run_CancelWithPendingBatch(t *testing.T) {
	wp := newMockWorkerPool()
	defer wp.Stop()

	stage := NewEventGenerationStage(
		event.EventTypeNetflow,
		1000, // высокая скорость
		SerializationModeBinary,
		true, // packetMode = true → использует ToBinaryNetFlow()
		wp,
	)

	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan *SerializedData, 10)

	// Запускаем потребителя, чтобы избежать блокировки канала
	done := make(chan struct{})
	go func() {
		for range out {
			// просто читаем
		}
		close(done)
	}()

	ready := make(chan bool)
	errCh := make(chan error, 1)
	go func() {
		errCh <- stage.Run(ctx, nil, out, ready)
	}()

	<-ready

	time.Sleep(5 * time.Millisecond) // при 1000/s — должно создаться несколько событий
	cancel()

	select {
	case err := <-errCh:
		assert.Equal(t, context.Canceled, err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("stage did not handle cancellation")
	}

	// Закрываем out, чтобы завершить consumer
	close(out)
	<-done
}
