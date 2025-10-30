package config

import (
	"time"
)

func DefaultConfig() *Config {
	return &Config{
		Generator: GeneratorConfig{
			Name:            "default-generator",
			EventsPerSecond: 10,
			EventTypes:      []string{"netflow"},
			Duration:        0, // бесконечно
			EventConfig: EventConfig{
				Netflow: DefaultNetflowConfig(),
				Syslog:  DefaultSyslogConfig(),
			},
		},
		Sender: SenderConfig{
			Name:         "default-sender",
			Protocol:     "udp",
			Destinations: []string{"127.0.0.1:514"},
			Retries:      3,
			Timeout:      5 * time.Second,
		},
		Pipeline: PipelineConfig{
			BufferSize:  10000,
			WorkerCount: 1,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			File:   "", //stdout
		},
	}
}

func DefaultNetflowConfig() NetflowConfig {
	return NetflowConfig{
		Version:   9,
		SourceIPs: []string{"192.168.1.0/24", "10.0.0.0/8"},
		DestIPs:   []string{"8.8.8.8", "1.1.1.1", "208.67.222.222"},
		Ports:     []int{80, 443, 53, 22, 25, 110, 995, 993, 143},
		Protocols: []string{"TCP", "UDP", "ICMP"},
	}
}

func DefaultSyslogConfig() SyslogConfig {
	return SyslogConfig{
		Facility:  16, // local0
		Severity:  6,  // info
		Hostnames: []string{"server01", "server02", "workstation01"},
		Programs:  []string{"sshd", "kernel", "systemd", "nginx", "apache"},
	}
}
