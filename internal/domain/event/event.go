package event

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/netip"
	"time"
)

// Type определяет тип события для генерации
type Type int

const (
	EventTypeUnknown Type = iota
	EventTypeNetflow
	EventTypeSyslog
)

func (et Type) String() string {
	switch et {
	case EventTypeNetflow:
		return "netflow"
	case EventTypeSyslog:
		return "syslog"
	default:
		return "unknown"
	}
}

// Event общий интерфейс для всех видов событий
type Event interface {
	Type() Type
	Timestamp() time.Time
	Size() int
	Validate() error
	GetID() string
	GetSourceIP() netip.Addr      // Источник трафика
	GetDestinationIP() netip.Addr // Получатель трафика
}

// generateEventID генерирует уникальный идентификатор события
func generateEventID(eventType Type) string {
	// Генерируем 128-битный случайный идентификатор (16 байт)
	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		// Fallback: безопасный, но менее уникальный ID
		return fmt.Sprintf("evt_%s_%d", eventType.String(), time.Now().UnixNano())
	}

	// Префикс по типу события для удобства отладки
	prefix := "evt"
	switch eventType {
	case EventTypeSyslog:
		prefix = "pal" // или оставить "evt_syslog_..." — по вкусу
	case EventTypeNetflow:
		prefix = "nf"
	}

	return prefix + "_" + hex.EncodeToString(randBytes[:8]) // 8 байт = 16 hex-символов → достаточно для локальной уникальности
}

// Factory создает события на основе типа
type Factory struct{}

// NewFactory создает новую фабрику событий
func NewFactory() *Factory {
	return &Factory{}
}

// CreateEvent создает событие указанного типа
func (f *Factory) CreateEvent(eventType Type) (Event, error) {
	switch eventType {
	case EventTypeNetflow:
		return NewNetflowEvent(), nil
	case EventTypeSyslog:
		return NewSyslogPaloAltoEvent(), nil
	default:
		return nil, fmt.Errorf("unknown event type: %v", eventType)
	}
}

// ParseType парсит строку в Type
func (f *Factory) ParseEventType(s string) (Type, error) {
	switch s {
	case "netflow":
		return EventTypeNetflow, nil
	case "syslog":
		return EventTypeSyslog, nil
	default:
		return EventTypeUnknown, fmt.Errorf("unknown event type: %v", s)
	}
}
