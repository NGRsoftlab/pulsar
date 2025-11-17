package monitoring_test

import (
	"context"
	"testing"
	"time"

	"github.com/NGRsoftlab/pulsar/internal/monitoring"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockLogger реализует мок логгера
type MockLogger struct {
	mock.Mock
}

func (m *MockLogger) Debug(msg string, args ...any) {
	m.Called(append([]any{msg}, args...)...)
}

func (m *MockLogger) Info(msg string, args ...any) {
	m.Called(append([]any{msg}, args...)...)
}

func (m *MockLogger) Warn(msg string, args ...any) {
	m.Called(append([]any{msg}, args...)...)
}

func (m *MockLogger) Error(msg string, args ...any) {
	m.Called(append([]any{msg}, args...)...)
}

func TestMonitor_StartAndStopWithContext(t *testing.T) {
	log := new(MockLogger)
	monitor := monitoring.NewMonitor(50*time.Millisecond, log)

	// ожидаем старт
	log.On("Info", mock.Anything, mock.Anything).Return().Maybe()
	log.On("Info", mock.Anything).Return().Maybe()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()

	go monitor.Start(ctx)

	// ждём завершения контекста
	<-ctx.Done()

	// Stop можно вызвать, но он не должен паниковать
	require.NoError(t, monitor.Stop())

	log.AssertExpectations(t)
}

func TestMonitor_StopSignal(t *testing.T) {
	log := new(MockLogger)
	monitor := monitoring.NewMonitor(50*time.Millisecond, log)

	log.On("Info", mock.Anything, mock.Anything).Return().Maybe()
	log.On("Info", mock.Anything).Return().Maybe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go monitor.Start(ctx)

	// подождём немного, чтобы тикер успел один раз отработать
	time.Sleep(70 * time.Millisecond)

	// теперь остановим через Stop
	require.NoError(t, monitor.Stop())

	// дадим горутине завершиться
	time.Sleep(30 * time.Millisecond)

	log.AssertExpectations(t)
}

func TestMonitor_StopIdempotent(t *testing.T) {
	log := new(MockLogger)
	monitor := monitoring.NewMonitor(100*time.Millisecond, log)

	// несколько вызовов Stop не должны паниковать
	require.NoError(t, monitor.Stop())
	require.NoError(t, monitor.Stop())
}
