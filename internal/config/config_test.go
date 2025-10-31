package config

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Generator.EventsPerSecond != 10 {
		t.Errorf("Expected events per second 10, got %d", cfg.Generator.EventsPerSecond)
	}

	if len(cfg.Generator.EventTypes) != 1 || cfg.Generator.EventTypes[0] != "netflow" {
		t.Errorf("Expected event types [netflow], got %v", cfg.Generator.EventTypes)
	}

	if cfg.Sender.Protocol != "udp" {
		t.Errorf("Expected protocol 'udp', got '%s'", cfg.Sender.Protocol)
	}

	if len(cfg.Sender.Destinations) != 1 || cfg.Sender.Destinations[0] != "127.0.0.1:514" {
		t.Errorf("Expected destinations [127.0.0.1:514], got %v", cfg.Sender.Destinations)
	}

	if cfg.Pipeline.BufferSize != 100 {
		t.Errorf("Expected buffer size 100, got %d", cfg.Pipeline.BufferSize)
	}

	if cfg.Logging.Level != "info" {
		t.Errorf("Expected log level 'info', got '%s'", cfg.Logging.Level)
	}
}
