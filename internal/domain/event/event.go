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

type BinarySerializable interface {
	Event
	ToBinaryNetFlow() ([]byte, error) // возвращает бинарное NetFlow представление
	BinarySize() int                  // размер бинарных данных
}

// generateEventID генерирует уникальный идентификатор события
func generateEventID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("evt_%d", time.Now().UnixNano())
	}
	return "evt_" + hex.EncodeToString(bytes)
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
		return NewSyslogEvent(), nil
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
