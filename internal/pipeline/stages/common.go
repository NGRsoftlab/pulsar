// Package stages contains implementations of pipeline processing stages,
// including event generation and network sending,
// each conforming to the coordinator.Stage interface.
package stages

import (
	"context"

	"github.com/NGRsoftlab/pulsar/internal/types"
)

// StageConfig базовая конфигурация для всех стадий
type StageConfig struct {
	Name        string `yaml:"name" json:"name"`
	WorkerCount int    `yaml:"worker_count" json:"worker_count"`
	BufferSize  int    `yaml:"buffer_size" json:"buffer_size"`
}

type Pool interface {
	Start(ctx context.Context)
	Stop()
	Submit(job types.JobBatch) bool
	GetJob() types.JobBatch
}
