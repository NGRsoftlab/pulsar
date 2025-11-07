package event

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/netip"
	"time"
)

// EventType определяет тип события для генерации
type EventType int

const (
	EventTypeUnknown EventType = iota
	EventTypeNetflow
	EventTypeSyslog
)

func (et EventType) String() string {
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
	Type() EventType
	Timestamp() time.Time
	Size() int
	Validate() error
	GetID() string
	GetSourceIP() netip.Addr      // Источник трафика
	GetDestinationIP() netip.Addr // Получатель трафика
}

// generateEventID генерирует уникальный идентификатор события
func generateEventID(eventType EventType) string {
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

// EventFactory создает события на основе типа
type EventFactory struct{}

// NewEventFactory создает новую фабрику событий
func NewEventFactory() *EventFactory {
	return &EventFactory{}
}

// CreateEvent создает событие указанного типа
func (f *EventFactory) CreateEvent(eventType EventType) (Event, error) {
	switch eventType {
	case EventTypeNetflow:
		return NewNetflowEvent(), nil
	case EventTypeSyslog:
		return NewSyslogPaloAltoEvent(), nil
	default:
		return nil, fmt.Errorf("unknown event type: %v", eventType)
	}
}

// ParseEventType парсит строку в EventType
func (f *EventFactory) ParseEventType(s string) (EventType, error) {
	switch s {
	case "netflow":
		return EventTypeNetflow, nil
	case "syslog":
		return EventTypeSyslog, nil
	default:
		return EventTypeUnknown, fmt.Errorf("unknown event type: %v", s)
	}
}
