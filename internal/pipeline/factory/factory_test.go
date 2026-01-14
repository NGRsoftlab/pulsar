package factory

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/NGRsoftlab/pulsar/internal/config"
)

func TestPipelineFactory_ParseEventTypes(t *testing.T) {
	tests := []struct {
		name        string
		eventTypes  []string
		expectError bool
	}{
		{
			name:        "valid netflow",
			eventTypes:  []string{"netflow"},
			expectError: false,
		},
		{
			name:        "unsupported syslog",
			eventTypes:  []string{"syslog"},
			expectError: true,
		},
		{
			name:        "unknown type",
			eventTypes:  []string{"unknown"},
			expectError: true,
		},
		{
			name:        "empty",
			eventTypes:  []string{},
			expectError: false,
		},
		{
			name:        "mixed",
			eventTypes:  []string{"syslog", "netflow"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Generator: config.GeneratorConfig{
					EventTypes: tt.eventTypes,
				},
			}
			factory := NewPipelineFactory(cfg)

			_, err := factory.ParseEventTypes()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPipelineFactory_createGenerationStage(t *testing.T) {
	// t.Run("success with netflow", func(t *testing.T) {
	cfg := &config.Config{
		Generator: config.GeneratorConfig{
			EventsPerSecond: 100,
			EventTypes:      []string{"netflow"},
		},
	}
	factory := NewPipelineFactory(cfg)

	stage, err := factory.createGenerationStage()
	assert.NoError(t, err)
	assert.NotNil(t, stage)
	//})
}

func TestPipelineFactory_createSendingStage(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		cfg := &config.Config{
			Sender: config.SenderConfig{
				Destinations: []string{"127.0.0.1:8080"},
				Protocol:     "udp",
			},
		}
		factory := NewPipelineFactory(cfg)

		stage, err := factory.createSendingStage()
		assert.NoError(t, err)
		assert.NotNil(t, stage)
	})
	t.Run("", func(t *testing.T) {
		cfg := &config.Config{
			Sender: config.SenderConfig{
				Destinations: []string{},
				Protocol:     "tcp",
			},
		}
		factory := NewPipelineFactory(cfg)

		stage, err := factory.createSendingStage()
		assert.Error(t, err)
		assert.Nil(t, stage)
	})
}

func TestPipelineFactory_bufferSizeCalculation(t *testing.T) {
	tests := []struct {
		name               string
		bufferSize         int
		eventsPerSecond    int
		expectedBufferSize int
	}{
		{"explicit buffer", 500, 100, 500},
		{"auto: low EPS", 0, 100, 1000},
		{"auto: medium EPS", 0, 5000, 15000},
		{"auto: high EPS", 0, 100000, 200000},
		{"auto: zero EPS", 0, 0, 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Pipeline: config.PipelineConfig{
					BufferSize: tt.bufferSize,
				},
				Generator: config.GeneratorConfig{
					EventsPerSecond: tt.eventsPerSecond,
					EventTypes:      []string{"netflow"},
				},
				Sender: config.SenderConfig{
					Destinations: []string{"127.0.0.1:9999"},
					Protocol:     "udp",
				},
			}
			factory := NewPipelineFactory(cfg)

			pipeline, err := factory.CreatePipeline()
			assert.NoError(t, err)
			assert.NotNil(t, pipeline)
		})
	}
}

func TestPipelineFactory_CreatePipeline(t *testing.T) {
	// t.Run("success with valid config", func(t *testing.T) {
	cfg := &config.Config{
		Pipeline: config.PipelineConfig{
			BufferSize: 0, // авто
		},
		Generator: config.GeneratorConfig{
			EventsPerSecond: 500,
			EventTypes:      []string{"netflow"},
		},
		Sender: config.SenderConfig{
			Destinations: []string{"localhost:1234"},
			Protocol:     "udp",
		},
	}

	factory := NewPipelineFactory(cfg)
	pipeline, err := factory.CreatePipeline()

	assert.NoError(t, err)
	assert.NotNil(t, pipeline)
	//})
}

func TestPipelineFactory_calculateBufferSize(t *testing.T) {
	tests := []struct {
		name               string
		pipelineBufferSize int
		eventsPerSecond    int
		expectedBufferSize int
	}{
		{
			name:               "explicit buffer size is used",
			pipelineBufferSize: 5000,
			eventsPerSecond:    100,
			expectedBufferSize: 5000,
		},
		{
			name:               "auto: low EPS → clamped to min 1000",
			pipelineBufferSize: 0,
			eventsPerSecond:    100, expectedBufferSize: 1000,
		},
		{
			name:               "auto: medium EPS → exact value",
			pipelineBufferSize: 0,
			eventsPerSecond:    5000, expectedBufferSize: 15000,
		},
		{
			name:               "auto: high EPS → capped at 200000",
			pipelineBufferSize: 0,
			eventsPerSecond:    100000, expectedBufferSize: 200000,
		},
		{
			name:               "auto: zero EPS → min 1000",
			pipelineBufferSize: 0,
			eventsPerSecond:    0,
			expectedBufferSize: 1000,
		},
		{
			name:               "auto: negative EPS (edge case) → still min 1000",
			pipelineBufferSize: 0,
			eventsPerSecond:    -10,
			expectedBufferSize: 1000,
		},
		{
			name:               "auto: exactly at cap",
			pipelineBufferSize: 0,
			eventsPerSecond:    66666, expectedBufferSize: 199998,
		},
		{
			name:               "auto: just above cap",
			pipelineBufferSize: 0,
			eventsPerSecond:    66667, expectedBufferSize: 200000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				Pipeline: config.PipelineConfig{
					BufferSize: tt.pipelineBufferSize,
				},
				Generator: config.GeneratorConfig{
					EventsPerSecond: tt.eventsPerSecond,
				},
			}
			factory := NewPipelineFactory(cfg)

			actual := factory.calculateBufferSize()

			assert.Equal(t, tt.expectedBufferSize, actual,
				"buffer size mismatch: expected %d, got %d", tt.expectedBufferSize, actual)
		})
	}
}
