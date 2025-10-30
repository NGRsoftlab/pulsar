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
	Name              string        `yaml:"name" json:"name" env:"UEBA_GENERATOR_NAME"`
	EventsPerSecond   int           `yaml:"events_per_second" json:"events_per_second" env:"UEBA_EVENTS_PER_SEC"`
	EventTypes        []string      `yaml:"event_types" json:"event_types" env:"UEBA_EVENT_TYPES"`
	Duration          time.Duration `yaml:"duration" json:"duration" env:"UEBA_DURATION"`
	EventConfig       EventConfig   `yaml:"event_config" json:"event_config"`
	SerializationMode string        `yaml:"serialization_mode" json:"serialization_mode" env:"UEBA_SERIALIZATION_MODE"`
	PacketMode        bool          `yaml:"packet_mode" json:"packet_mode" env:"UEBA_PACKET_MODE"`
}

type SenderConfig struct {
	Name         string        `yaml:"name" json:"name" env:"UEBA_SENDER_NAME"`
	Protocol     string        `yaml:"protocol" json:"protocol" env:"UEBA_PROTOCOL"`
	Destinations []string      `yaml:"destinations" json:"destinations" env:"UEBA_DESTINATIONS"`
	Retries      int           `yaml:"retries" json:"retries" env:"UEBA_RETRIES"`
	Timeout      time.Duration `yaml:"timeout" json:"timeout" env:"UEBA_TIMEOUT"`
}

type PipelineConfig struct {
	BufferSize  int `yaml:"buffer_size" json:"buffer_size" env:"UEBA_BUFFER_SIZE"`
	WorkerCount int `yaml:"worker_count" json:"worker_count" env:"UEBA_WORKER_COUNT"`
}

type EventConfig struct {
	Netflow NetflowConfig `yaml:"netflow" json:"netflow"`
	Syslog  SyslogConfig  `yaml:"syslog" json:"syslog"`
}

type NetflowConfig struct {
	Version   int      `yaml:"version" json:"version"`
	SourceIPs []string `yaml:"source_ips" json:"source_ips"`
	DestIPs   []string `yaml:"dest_ips" json:"dest_ips"`
	Ports     []int    `yaml:"ports" json:"ports"`
	Protocols []string `yaml:"protocols" json:"protocols"`
}

type SyslogConfig struct {
	Facility  int      `yaml:"facility" json:"facility"`
	Severity  int      `yaml:"severity" json:"severity"`
	Hostnames []string `yaml:"hostnames" json:"hostnames"`
	Programs  []string `yaml:"programs" json:"programs"`
}

type LoggingConfig struct {
	Level  string `yaml:"level" json:"level" env:"UEBA_LOG_LEVEL"`
	Format string `yaml:"format" json:"format" env:"UEBA_LOG_FORMAT"`
	File   string `yaml:"file" json:"file" env:"UEBA_LOG_FILE"`
}
