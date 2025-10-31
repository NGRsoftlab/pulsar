package config

import (
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid default config",
			config:      DefaultConfig(),
			expectError: false,
		},
		{
			name: "Invalid events per second - zero",
			config: &Config{
				Generator: GeneratorConfig{
					EventsPerSecond: 0,
					EventTypes:      []string{"netflow"},
				},
				Sender: SenderConfig{
					Destinations: []string{"127.0.0.1:514"},
					Protocol:     "udp",
					Timeout:      5 * time.Second,
				},
				Pipeline: PipelineConfig{
					BufferSize: 100,
				},
			},
			expectError: true,
			errorMsg:    "events_per_second must be positive",
		},
		{
			name: "Empty event types",
			config: &Config{
				Generator: GeneratorConfig{
					EventsPerSecond: 10,
					EventTypes:      []string{},
				},
				Sender: SenderConfig{
					Destinations: []string{"127.0.0.1:514"},
					Protocol:     "udp",
					Timeout:      5 * time.Second,
				},
				Pipeline: PipelineConfig{
					BufferSize: 100,
				},
			},
			expectError: true,
			errorMsg:    "at least one event type must be specified",
		},
		{
			name: "Invalid event type",
			config: &Config{
				Generator: GeneratorConfig{
					EventsPerSecond: 10,
					EventTypes:      []string{"invalid-type"},
				},
				Sender: SenderConfig{
					Destinations: []string{"127.0.0.1:514"},
					Protocol:     "udp",
					Timeout:      5 * time.Second,
				},
				Pipeline: PipelineConfig{
					BufferSize: 100,
				},
			},
			expectError: true,
			errorMsg:    "unsupported event type",
		},
		{
			name: "Empty destinations",
			config: &Config{
				Generator: GeneratorConfig{
					EventsPerSecond: 10,
					EventTypes:      []string{"netflow"},
				},
				Sender: SenderConfig{
					Destinations: []string{},
					Protocol:     "udp",
					Timeout:      5 * time.Second,
				},
				Pipeline: PipelineConfig{
					BufferSize: 100,
				},
			},
			expectError: true,
			errorMsg:    "at least one destination must be specified",
		},
		{
			name: "Invalid destination format",
			config: &Config{
				Generator: GeneratorConfig{
					EventsPerSecond: 10,
					EventTypes:      []string{"netflow"},
				},
				Sender: SenderConfig{
					Destinations: []string{"invalid-address"},
					Protocol:     "udp",
					Timeout:      5 * time.Second,
				},
				Pipeline: PipelineConfig{
					BufferSize: 100,
				},
			},
			expectError: true,
			errorMsg:    "invalid destination address",
		},
		{
			name: "Invalid protocol",
			config: &Config{
				Generator: GeneratorConfig{
					EventsPerSecond: 10,
					EventTypes:      []string{"netflow"},
				},
				Sender: SenderConfig{
					Destinations: []string{"127.0.0.1:514"},
					Protocol:     "http",
					Timeout:      5 * time.Second,
				},
				Pipeline: PipelineConfig{
					BufferSize: 100,
				},
			},
			expectError: true,
			errorMsg:    "protocol must be 'tcp' or 'udp'",
		},
		{
			name: "Invalid buffer size",
			config: &Config{
				Generator: GeneratorConfig{
					EventsPerSecond: 10,
					EventTypes:      []string{"netflow"},
				},
				Sender: SenderConfig{
					Destinations: []string{"127.0.0.1:514"},
					Protocol:     "udp",
					Timeout:      5 * time.Second,
				},
				Pipeline: PipelineConfig{
					BufferSize: 0,
				},
			},
			expectError: true,
			errorMsg:    "buffer_size must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.errorMsg)
				} else if !contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}
		})
	}
}

func contains(str, substr string) bool {
	return len(str) >= len(substr) && (str == substr ||
		(len(str) > len(substr) && (str[:len(substr)] == substr ||
			str[len(str)-len(substr):] == substr ||
			containsSubstring(str, substr))))
}

func containsSubstring(str, substr string) bool {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
