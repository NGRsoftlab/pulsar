package config

import (
	"time"
)

func DefaultConfig() *Config {
	return &Config{
		Generator: GeneratorConfig{
			EventsPerSecond: 10,
			EventTypes:      []string{"netflow"},
			Duration:        0, // бесконечно
		},
		Sender: SenderConfig{
			Protocol:     "udp",
			Destinations: []string{"127.0.0.1:514"},
			Retries:      3,
			Timeout:      5 * time.Second,
		},
		Pipeline: PipelineConfig{
			BufferSize: 100000,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			File:   "", // stdout
		},
	}
}
