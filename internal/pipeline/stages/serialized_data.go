package stages

import (
	"fmt"
	"time"

	"github.com/NGRsoftlab/pulsar/internal/domain/event"
	"github.com/NGRsoftlab/pulsar/internal/types"
)

// SerializedData представляет готовые данные для отправки
type SerializedData struct {
	Data              []byte
	EventType         event.Type
	Size              int
	Timestamp         time.Time
	EventID           string
	SerializationMode SerializationMode

	Destination string // Куда отправлять (IP:port)
	Protocol    string // UDP/TCP
}

// NewSerializedData создает новый экземпляр SerializedData
func NewSerializedData(data []byte, eventType event.Type, eventID string, mode SerializationMode) *SerializedData {
	return &SerializedData{
		Data:              data,
		EventType:         eventType,
		Size:              len(data),
		Timestamp:         time.Now().UTC(),
		EventID:           eventID,
		SerializationMode: mode,
	}
}

func NewSerializedDataFromEvent(evt event.Event, mode SerializationMode) (*SerializedData, error) {
	var data []byte
	var err error

	switch mode {
	case SerializationModeBinary:
		if bEvt, ok := evt.(types.BinarySerializable); ok {
			data, err = bEvt.ToBinaryNetFlow()
			if err != nil {
				return nil, fmt.Errorf("binary serialization failed: %w", err)
			}
		} else {
			return nil, fmt.Errorf("event type %v does not support binary serialization", evt.Type())
		}

	case SerializationModeRaw:
		if rEvt, ok := evt.(types.RawSerializable); ok {
			rawStr := rEvt.ToRawSyslog()
			data = []byte(rawStr)
		} else {
			return nil, fmt.Errorf("event type %s does not support raw serialization", evt.Type())
		}

	default:
		return nil, fmt.Errorf("unsupported serialization mode: %s", mode)
	}

	return NewSerializedData(data, evt.Type(), evt.GetID(), mode), nil
}

// Validate проверяет корректность данных
func (sd *SerializedData) Validate() error {
	if sd.EventID == "" {
		return fmt.Errorf("event ID cannot be empty")
	}

	if sd.Size != len(sd.Data) {
		return fmt.Errorf("size mismatch: expected %d, got %d", len(sd.Data), sd.Size)
	}

	if len(sd.Data) == 0 {
		return fmt.Errorf("serialized data cannot be empty")
	}

	return nil
}

// String возвращает строковое представление SerializedData
func (sd *SerializedData) String() string {
	return fmt.Sprintf("SerializedData{Type: %s, Size: %d, ID: %s}",
		sd.EventType, sd.Size, sd.EventID)
}

type SerializationMode string

const (
	SerializationModeBinary SerializationMode = "binary" // Netflow
	SerializationModeRaw    SerializationMode = "raw"    // Syslog, CEF
)
