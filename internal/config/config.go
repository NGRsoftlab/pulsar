// Package config provides app configuration managment & validation
package config

import (
	"time"
)

type Config struct {
	Generator GeneratorConfig `yaml:"generator" json:"generator"`
	Sender    SenderConfig    `yaml:"sender" json:"sender"`
	Pipeline  PipelineConfig  `yaml:"pipeline" json:"pipeline"`
	Logging   LoggingConfig   `yaml:"logging" json:"logging"`
}

type GeneratorConfig struct {
	EventsPerSecond   int           `yaml:"events_per_second" json:"events_per_second" env:"UEBA_EVENTS_PER_SEC"`
	EventTypes        []string      `yaml:"event_types" json:"event_types" env:"UEBA_EVENT_TYPES"`
	Duration          time.Duration `yaml:"duration" json:"duration" env:"UEBA_DURATION"`
	SerializationMode string        `yaml:"serialization_mode" json:"serialization_mode" env:"UEBA_SERIALIZATION_MODE"`
	PacketMode        bool          `yaml:"packet_mode" json:"packet_mode" env:"UEBA_PACKET_MODE"`
}

type SenderConfig struct {
	Protocol     string        `yaml:"protocol" json:"protocol" env:"UEBA_PROTOCOL"`
	Destinations []string      `yaml:"destinations" json:"destinations" env:"UEBA_DESTINATIONS"`
	Retries      int           `yaml:"retries" json:"retries" env:"UEBA_RETRIES"`
	Timeout      time.Duration `yaml:"timeout" json:"timeout" env:"UEBA_TIMEOUT"`
}

type PipelineConfig struct {
	BufferSize int `yaml:"buffer_size" json:"buffer_size" env:"UEBA_BUFFER_SIZE"`
}

type LoggingConfig struct {
	Level  string `yaml:"level" json:"level" env:"UEBA_LOG_LEVEL"`
	Format string `yaml:"format" json:"format" env:"UEBA_LOG_FORMAT"`
	File   string `yaml:"file" json:"file" env:"UEBA_LOG_FILE"`
}
