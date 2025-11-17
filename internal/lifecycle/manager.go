// Package lifecycle отвечает за управление жизненным циклом и сборкой приложения
package lifecycle

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/NGRsoftlab/pulsar/internal/logger"
)

type pipeline interface {
	Start(ctx context.Context) error
	Stop() error
}

type monitor interface {
	Start(ctx context.Context)
	Stop() error
}

type Manager struct {
	pipeline pipeline
	monitor  monitor
	logger   logger.Logger
	cancel   context.CancelFunc
	mu       sync.Mutex
}

func NewManager(p pipeline, m monitor, log logger.Logger) *Manager {
	return &Manager{pipeline: p, monitor: m, logger: log}
}

func (m *Manager) Run(duration time.Duration) error {
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.cancel = cancel
	m.mu.Unlock()
	defer cancel()

	m.setupSignalHandling(cancel)

	if duration > 0 {
		m.setupTimerDuration(ctx, cancel, duration)
	}

	errCh := make(chan error, 1)

	go m.monitor.Start(ctx)

	m.logger.Info("Starting pipeline...")

	go func() {
		if err := m.pipeline.Start(ctx); err != nil {
			m.logger.Error("Pipeline error: %v", err)
			errCh <- err
			cancel()
		}
	}()

	var runErr error
	select {
	case <-ctx.Done():
		m.logger.Info("Context canceled: %v", ctx.Err())
	case err := <-errCh:
		runErr = err
	}

	m.logger.Info("Stopping pipeline...")
	if err := m.pipeline.Stop(); err != nil {
		m.logger.Error("Error stopping pipeline: %v", err)
		if runErr == nil {
			runErr = err
		}
	}

	m.logger.Info("Stopping monitor...")
	if err := m.monitor.Stop(); err != nil {
		m.logger.Error("Error stopping monitor: %v", err)
		if runErr == nil {
			runErr = err
		}
	}

	return runErr
}

func (m *Manager) Stop() {
	defer m.mu.Unlock()
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
}

func (m *Manager) setupSignalHandling(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		m.logger.Info("Received signal: %v, initiating graceful shutdown...", sig)
		cancel()
	}()
}

func (m *Manager) setupTimerDuration(ctx context.Context, cancel context.CancelFunc, duration time.Duration) {
	m.logger.Info("Application will run for: %v", duration)

	go func() {
		timer := time.NewTimer(duration)
		defer timer.Stop()

		select {
		case <-timer.C:
			m.logger.Info("Duration reached, initiating shutdowm...")
			cancel()
		case <-ctx.Done():
			// контекст отменен в другом месте, ничего не делаем
		}
	}()
}
