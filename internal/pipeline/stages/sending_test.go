package stages

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NGRsoftlab/pulsar/internal/domain/event"
	"github.com/NGRsoftlab/pulsar/internal/types"
)

// ================
// Моки
// ================

// mockSender
type mockSender struct {
	mu        sync.Mutex
	sent      []mockSentItem
	failSend  bool
	healthy   bool
	timeout   time.Duration
	closeCall bool
}

type mockSentItem struct {
	destination string
	data        []byte
}

func newMockSender() *mockSender {
	return &mockSender{
		healthy: true,
	}
}

func (s *mockSender) Send(destination string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.failSend {
		return errors.New("simulated send failure")
	}

	s.sent = append(s.sent, mockSentItem{destination, data})
	return nil
}

func (s *mockSender) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeCall = true
	return nil
}

func (s *mockSender) IsHealthy() (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.healthy {
		return true, "ok"
	}
	return false, "mock unhealthy"
}

func (s *mockSender) SetTimeout(timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.timeout = timeout
}

func (s *mockSender) GetSent() []mockSentItem {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := make([]mockSentItem, len(s.sent))
	copy(copied, s.sent)
	return copied
}

func (s *mockSender) SetFailSend(fail bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failSend = fail
}

func (s *mockSender) SetHealthy(healthy bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.healthy = healthy
}

// mockWorkerPoolForNetwork — простой синхронный мок, как обсуждали
type mockWorkerPoolForNetwork struct{}

func newMockWorkerPoolForNetwork() *mockWorkerPoolForNetwork {
	return &mockWorkerPoolForNetwork{}
}

func (wp *mockWorkerPoolForNetwork) Start(context.Context) {}
func (wp *mockWorkerPoolForNetwork) Stop()                 {}
func (wp *mockWorkerPoolForNetwork) Submit(job types.JobBatch) bool {
	if job != nil {
		job.ExecuteBatch()
	}
	return true
}

func (wp *mockWorkerPoolForNetwork) GetJob() types.JobBatch {
	return NewNetworkSendJobBatch(50)
}

// ================
// Вспомогательные функции
// ================

func newTestSerializedData(dest string, data []byte) *SerializedData {
	return &SerializedData{
		Data:              data,
		EventType:         event.EventTypeNetflow,
		EventID:           "test-id",
		Destination:       dest,
		SerializationMode: SerializationModeBinary,
	}
}

// ================
// Тесты
// ================

func TestNewNetworkSendingStage(t *testing.T) {
	wp := newMockWorkerPoolForNetwork()
	sender := newMockSender()

	stage := NewNetworkSendingStage(wp, sender)

	assert.NotNil(t, stage)
	assert.Equal(t, []string{"127.0.0.1:514"}, stage.destinations)
	assert.Equal(t, "udp", stage.protocol)
	assert.Equal(t, 5*time.Second, stage.timeout)
	assert.NotNil(t, stage.input)
}

func TestNetworkSendJobBatch_ExecuteBatch_Nil(t *testing.T) {
	var batch *NetworkSendJobBatch
	err := batch.ExecuteBatch()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "job batch is nil")
}

func TestNetworkSendJobBatch_ExecuteBatch(t *testing.T) {
	sender := newMockSender()
	stage := &NetworkSendingStage{
		destinations: []string{"127.0.0.1:9999"},
		sender:       sender,
		protocol:     "udp",
	}

	data1 := newTestSerializedData("", []byte("data1"))
	data2 := newTestSerializedData("192.0.2.1:1234", []byte("data2"))

	batch := &NetworkSendJobBatch{
		stage: stage,
		data:  []*SerializedData{data1, data2},
	}

	err := batch.ExecuteBatch()
	require.NoError(t, err)
	assert.Empty(t, batch.data)

	sent := sender.GetSent()
	assert.Len(t, sent, 2)
	assert.Equal(t, "127.0.0.1:9999", sent[0].destination) // из destinations
	assert.Equal(t, "192.0.2.1:1234", sent[1].destination) // из data.Destination
}

func TestNetworkSendJobBatch_ExecuteBatch_SendFailure(t *testing.T) {
	sender := newMockSender()
	sender.SetFailSend(true)
	stage := &NetworkSendingStage{
		destinations: []string{"127.0.0.1:9999"},
		sender:       sender,
		protocol:     "udp",
	}

	data := newTestSerializedData("", []byte("data"))

	batch := &NetworkSendJobBatch{
		stage: stage,
		data:  []*SerializedData{data},
	}

	err := batch.ExecuteBatch()
	require.NoError(t, err) // ошибка отправки не прерывает batch
}

func TestNetworkSendingStage_Run(t *testing.T) {
	wp := newMockWorkerPoolForNetwork()
	sender := newMockSender()

	stage := NewNetworkSendingStage(wp, sender)
	stage.SetDestinations([]string{"127.0.0.1:1234"})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	in := make(chan *SerializedData, 10)
	out := make(chan *SerializedData, 10)

	go func() {
		for range out {
		}
	}()

	ready := make(chan bool)
	errCh := make(chan error, 1)
	go func() {
		errCh <- stage.Run(ctx, in, out, ready)
	}()

	<-ready

	in <- newTestSerializedData("", []byte("event1"))
	in <- newTestSerializedData("", []byte("event2"))

	close(in) // сигнализируем завершение

	select {
	case err := <-errCh:
		if err != nil && err != context.DeadlineExceeded {
			t.Fatalf("Unexpected error: %v", err)
		}
	case <-time.After(150 * time.Millisecond):
		t.Fatal("stage did not finish in time")
	}

	sent := sender.GetSent()
	assert.Len(t, sent, 2)
	assert.Equal(t, "127.0.0.1:1234", sent[0].destination)
}

func TestNetworkSendingStage_Run_CancelWithPendingBatch(t *testing.T) {
	wp := newMockWorkerPoolForNetwork()
	sender := newMockSender()

	stage := NewNetworkSendingStage(wp, sender)
	stage.SetDestinations([]string{"127.0.0.1:1234"})

	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan *SerializedData, 10)
	out := make(chan *SerializedData, 10)

	go func() {
		for range out {
		}
	}()

	ready := make(chan bool)
	errCh := make(chan error, 1)
	go func() {
		errCh <- stage.Run(ctx, in, out, ready)
	}()

	<-ready

	in <- newTestSerializedData("", []byte("event1"))
	in <- newTestSerializedData("", []byte("event2"))
	in <- newTestSerializedData("", []byte("event3"))

	time.Sleep(10 * time.Millisecond) // дать обработаться
	cancel()

	select {
	case err := <-errCh:
		assert.Equal(t, context.Canceled, err)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("stage did not handle cancellation")
	}

	sent := sender.GetSent()
	assert.Len(t, sent, 3)
}

func TestNetworkSendingStage_SetDestinations(t *testing.T) {
	stage := NewNetworkSendingStage(newMockWorkerPoolForNetwork(), newMockSender())

	err := stage.SetDestinations([]string{"192.168.1.1:514", "10.0.0.1:514"})
	require.NoError(t, err)
	assert.Equal(t, []string{"192.168.1.1:514", "10.0.0.1:514"}, stage.destinations)

	err = stage.SetDestinations([]string{})
	assert.Error(t, err)

	err = stage.SetDestinations([]string{"invalid"})
	assert.Error(t, err)
}

func TestNetworkSendingStage_SetProtocol(t *testing.T) {
	stage := NewNetworkSendingStage(newMockWorkerPoolForNetwork(), newMockSender())

	err := stage.SetProtocol("tcp")
	require.NoError(t, err)
	assert.Equal(t, "tcp", stage.protocol)

	err = stage.SetProtocol("udp")
	require.NoError(t, err)
	assert.Equal(t, "udp", stage.protocol)

	err = stage.SetProtocol("http")
	assert.Error(t, err)
}

func TestNetworkSendingStage_ResizeConnectionPool(t *testing.T) {
	stage := NewNetworkSendingStage(newMockWorkerPoolForNetwork(), newMockSender())

	err := stage.ResizeConnectionPool(10)
	assert.Error(t, err)

	stage.SetProtocol("tcp")
	err = stage.ResizeConnectionPool(10)
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "only supported for TCP")
}

func TestNetworkSendingStage_IsHealthy(t *testing.T) {
	sender := newMockSender()
	stage := NewNetworkSendingStage(newMockWorkerPoolForNetwork(), sender)

	healthy, msg := stage.IsHealthy()
	assert.True(t, healthy)
	assert.Equal(t, "ok", msg)

	sender.SetHealthy(false)
	healthy, msg = stage.IsHealthy()
	assert.False(t, healthy)
	assert.Equal(t, "mock unhealthy", msg)
}
