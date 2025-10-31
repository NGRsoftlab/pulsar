package config

import (
	"fmt"
	"net"
	"strings"
)

// Validate проверяет корректность конфигурации
func (c *Config) Validate() error {
	if err := c.Generator.Validate(); err != nil {
		return fmt.Errorf("generator config: %w", err)
	}

	if err := c.Sender.Validate(); err != nil {
		return fmt.Errorf("sender config: %w", err)
	}

	if err := c.Pipeline.Validate(); err != nil {
		return fmt.Errorf("pipeline config: %w", err)
	}

	return nil
}

func (g *GeneratorConfig) Validate() error {
	if g.EventsPerSecond <= 0 {
		return fmt.Errorf("events_per_second must be positive, got: %d", g.EventsPerSecond)
	}

	if g.EventsPerSecond > 200000 {
		return fmt.Errorf("events_per_second too high (>100k), got: %d", g.EventsPerSecond)
	}

	if len(g.EventTypes) == 0 {
		return fmt.Errorf("at least one event type must be specified")
	}

	// Проверяем поддерживаемые типы событий
	validTypes := map[string]bool{"netflow": true, "syslog": true}
	for _, eventType := range g.EventTypes {
		if !validTypes[strings.ToLower(eventType)] {
			return fmt.Errorf("unsupported event type: %s", eventType)
		}
	}

	return nil
}

func (s *SenderConfig) Validate() error {
	if len(s.Destinations) == 0 {
		return fmt.Errorf("at least one destination must be specified")
	}

	// Валидируем каждый адрес
	for _, dest := range s.Destinations {
		if _, _, err := net.SplitHostPort(dest); err != nil {
			return fmt.Errorf("invalid destination address %s: %w", dest, err)
		}
	}

	// Проверяем протокол
	if s.Protocol != "tcp" && s.Protocol != "udp" {
		return fmt.Errorf("protocol must be 'tcp' or 'udp', got: %s", s.Protocol)
	}

	if s.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	return nil
}

func (p *PipelineConfig) Validate() error {
	if p.BufferSize <= 0 {
		return fmt.Errorf("buffer_size must be positive, got: %d", p.BufferSize)
	}

	return nil
}
