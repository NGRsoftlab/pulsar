// Package stages contains implementations of pipeline processing stages,
// including event generation and network sending,
// each conforming to the coordinator.Stage interface.
package stages

import (
	"context"
	"time"

	"github.com/NGRsoftlab/pulsar/internal/types"
)

// StageConfig базовая конфигурация для всех стадий
type StageConfig struct {
	Name        string `yaml:"name" json:"name"`
	WorkerCount int    `yaml:"worker_count" json:"worker_count"`
	BufferSize  int    `yaml:"buffer_size" json:"buffer_size"`
}

// StageMetrics базовые метрики стадии
type StageMetrics struct {
	Name            string    `json:"name"`
	ProcessedEvents uint64    `json:"processed_events"`
	ErrorCount      uint64    `json:"error_count"`
	LastActivity    time.Time `json:"last_activity"`
}

type MetricsCollector interface {
	IncrementGenerated()
	IncrementSent()
	IncrementFailed()
	IncrementDropped()
	GetStats() (generated, sent, failed uint64, dropped float64)
}

type Pool interface {
	Start(ctx context.Context)
	Stop()
	Submit(job types.JobBatch) bool
	GetJob() types.JobBatch
}
