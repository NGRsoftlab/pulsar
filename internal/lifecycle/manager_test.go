package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/nashabanov/ueba-event-generator/internal/logger"
)

// --- Mocks ---

type mockPipeline struct {
	startErr error
	stopErr  error
	started  bool
	stopped  bool
	mu       sync.Mutex
}

func (p *mockPipeline) Start(ctx context.Context) error {
	p.mu.Lock()
	p.started = true
	p.mu.Unlock()

	if p.startErr != nil {
		return p.startErr
	}

	<-ctx.Done()
	return nil
}

func (p *mockPipeline) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopped = true
	return p.stopErr
}

func (p *mockPipeline) WasStarted() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.started
}

func (p *mockPipeline) WasStopped() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stopped
}

type mockMonitor struct {
	started bool
	stopped bool
	mu      sync.Mutex
}

func (m *mockMonitor) Start(_ context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
}

func (m *mockMonitor) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	return nil
}

func (m *mockMonitor) WasStarted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.started
}

func (m *mockMonitor) WasStopped() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopped
}

type mockMonitorWithError struct {
	started bool
	stopped bool
	stopErr error
	mu      sync.Mutex
}

func (m *mockMonitorWithError) Start(_ context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
}

func (m *mockMonitorWithError) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	return m.stopErr
}

func (m *mockMonitorWithError) WasStarted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.started
}

func (m *mockMonitorWithError) WasStopped() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopped
}

// --- Tests ---

func TestManager_Run_Success(t *testing.T) {
	p := &mockPipeline{}
	m := &mockMonitor{}
	log := logger.NewStdLogger()

	manager := NewManager(p, m, log)

	// Запускаем на 10 мс — этого достаточно для graceful shutdown
	err := manager.Run(10 * time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Дать немного времени на завершение (на случай, если таймер чуть отстаёт)
	time.Sleep(1 * time.Millisecond)

	if !p.WasStarted() {
		t.Error("pipeline.Start was not called")
	}
	if !p.WasStopped() {
		t.Error("pipeline.Stop was not called")
	}
	if !m.WasStarted() {
		t.Error("monitor.Start was not called")
	}
	if !m.WasStopped() {
		t.Error("monitor.Stop was not called")
	}
}

func TestManager_Run_PipelineError(t *testing.T) {
	expectedErr := errors.New("pipeline failed")
	p := &mockPipeline{startErr: expectedErr}
	m := &mockMonitor{}
	log := logger.NewStdLogger()

	manager := NewManager(p, m, log)

	err := manager.Run(0)
	if err == nil || err.Error() != expectedErr.Error() {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}

	if !p.WasStopped() {
		t.Error("pipeline.Stop should be called even on error")
	}
	if !m.WasStopped() {
		t.Error("monitor.Stop should be called even on error")
	}
}

func TestManager_Run_DurationTimeout(t *testing.T) {
	p := &mockPipeline{}
	m := &mockMonitor{}
	log := logger.NewStdLogger()

	manager := NewManager(p, m, log)

	err := manager.Run(10 * time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	if !p.WasStarted() {
		t.Error("pipeline should have started")
	}
	if !p.WasStopped() {
		t.Error("pipeline should have been stopped after timeout")
	}
	if !m.WasStopped() {
		t.Error("monitor should have been stopped after timeout")
	}
}

func TestManager_Stop_ExternalCancel(t *testing.T) {
	p := &mockPipeline{}
	m := &mockMonitor{}
	log := logger.NewStdLogger()

	manager := NewManager(p, m, log)

	var runErr error
	done := make(chan struct{})

	go func() {
		runErr = manager.Run(0)
		close(done)
	}()

	time.Sleep(1 * time.Millisecond)

	manager.Stop()

	<-done

	if runErr != nil {
		t.Errorf("unexpected error from Run: %v", runErr)
	}
	if !p.WasStopped() {
		t.Error("pipeline should be stopped after external Stop()")
	}
	if !m.WasStopped() {
		t.Error("monitor should be stopped after external Stop()")
	}
}

func TestManager_Run_PipelineStopErrorPriority(t *testing.T) {
	pipelineStopErr := errors.New("pipeline stop failed")
	monitorStopErr := errors.New("monitor stop failed")

	p := &mockPipeline{stopErr: pipelineStopErr}
	mon := &mockMonitorWithError{stopErr: monitorStopErr}
	log := logger.NewStdLogger()

	manager := NewManager(p, mon, log)

	err := manager.Run(50 * time.Millisecond)
	if err == nil || err.Error() != pipelineStopErr.Error() {
		t.Errorf("expected pipeline stop error, got %v", err)
	}
}

func TestManager_Run_MonitorStopErrorWhenPipelineOK(t *testing.T) {
	monitorStopErr := errors.New("monitor stop failed")

	p := &mockPipeline{}
	m := &mockMonitorWithError{stopErr: monitorStopErr}
	log := logger.NewStdLogger()

	manager := NewManager(p, m, log)

	err := manager.Run(50 * time.Millisecond)
	if err == nil || err.Error() != monitorStopErr.Error() {
		t.Errorf("expected monitor stop error, got %v", err)
	}
}
