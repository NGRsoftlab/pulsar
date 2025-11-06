package stages

import (
	"fmt"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nashabanov/ueba-event-generator/internal/domain/event"
)

// fakeBinaryEvent — событие, поддерживающее бинарную сериализацию
type fakeBinaryEvent struct {
	id string
}

func (f *fakeBinaryEvent) Type() event.EventType { return event.EventTypeNetflow }
func (f *fakeBinaryEvent) Timestamp() time.Time  { return time.Unix(12345, 0) }
func (f *fakeBinaryEvent) Size() int             { return 100 }
func (f *fakeBinaryEvent) Validate() error       { return nil }
func (f *fakeBinaryEvent) GetID() string         { return f.id }
func (f *fakeBinaryEvent) GetSourceIP() netip.Addr {
	addr, _ := netip.ParseAddr("192.0.2.1")
	return addr
}

func (f *fakeBinaryEvent) GetDestinationIP() netip.Addr {
	addr, _ := netip.ParseAddr("192.0.2.2")
	return addr
}

func (f *fakeBinaryEvent) BinarySize() int {
	// Возвращаем фиксированный размер для тестов
	return 100
}

func (f *fakeBinaryEvent) ToBinaryNetFlow() ([]byte, error) {
	return []byte("binary-data"), nil
}

var (
	_ event.Event              = (*fakeBinaryEvent)(nil)
	_ event.BinarySerializable = (*fakeBinaryEvent)(nil)
)

// fakeNonBinaryEvent — событие, НЕ поддерживающее бинарную сериализацию
type fakeNonBinaryEvent struct {
	id string
}

func (f *fakeNonBinaryEvent) Type() event.EventType { return event.EventTypeSyslog }
func (f *fakeNonBinaryEvent) Timestamp() time.Time  { return time.Unix(12345, 0) }
func (f *fakeNonBinaryEvent) Size() int             { return 50 }
func (f *fakeNonBinaryEvent) Validate() error       { return nil }
func (f *fakeNonBinaryEvent) GetID() string         { return f.id }
func (f *fakeNonBinaryEvent) GetSourceIP() netip.Addr {
	addr, _ := netip.ParseAddr("192.0.2.3")
	return addr
}

func (f *fakeNonBinaryEvent) GetDestinationIP() netip.Addr {
	addr, _ := netip.ParseAddr("192.0.2.4")
	return addr
}

var _ event.Event = (*fakeNonBinaryEvent)(nil)

// ================
// Тесты
// ================

func TestNewSerializedData(t *testing.T) {
	data := []byte("test payload")
	eventType := event.EventTypeNetflow
	eventID := "test-123"
	mode := SerializationModeBinary

	sd := NewSerializedData(data, eventType, eventID, mode)

	assert.NotNil(t, sd)
	assert.Equal(t, data, sd.Data)
	assert.Equal(t, eventType, sd.EventType)
	assert.Equal(t, len(data), sd.Size)
	assert.Equal(t, eventID, sd.EventID)
	assert.Equal(t, mode, sd.SerializationMode)
	assert.NotZero(t, sd.Timestamp)
}

func TestNewSerializedDataFromEvent_BinarySupported(t *testing.T) {
	evt := &fakeBinaryEvent{id: "binary-event-1"}

	sd, err := NewSerializedDataFromEvent(evt, SerializationModeBinary)
	require.NoError(t, err)
	assert.NotNil(t, sd)
	assert.Equal(t, []byte("binary-data"), sd.Data)
	assert.Equal(t, event.EventTypeNetflow, sd.EventType)
	assert.Equal(t, "binary-event-1", sd.EventID)
	assert.Equal(t, SerializationModeBinary, sd.SerializationMode)
	assert.Equal(t, len("binary-data"), sd.Size)
}

func TestNewSerializedDataFromEvent_UnsupportedMode(t *testing.T) {
	evt := &fakeBinaryEvent{id: "event"}

	// Попытка использовать несуществующий режим (например, "json")
	sd, err := NewSerializedDataFromEvent(evt, "json")
	require.Error(t, err)
	assert.Nil(t, sd)
	assert.Contains(t, err.Error(), "unsupported serialization mode")
}

func TestNewSerializedDataFromEvent_NonBinaryEvent(t *testing.T) {
	evt := &fakeNonBinaryEvent{id: "non-binary-event"}

	sd, err := NewSerializedDataFromEvent(evt, SerializationModeBinary)
	require.Error(t, err)
	assert.Nil(t, sd)
	assert.Contains(t, err.Error(), "does not support binary serialization")
	assert.Contains(t, err.Error(), "syslog")
}

func TestNewSerializedDataFromEvent_BinarySerializationError(t *testing.T) {
	// Событие, которое возвращает ошибку при сериализации
	errEvent := &errorBinaryEvent{id: "error-event"}

	sd, err := NewSerializedDataFromEvent(errEvent, SerializationModeBinary)
	require.Error(t, err)
	assert.Nil(t, sd)
	assert.Contains(t, err.Error(), "binary serialization failed")
	assert.Contains(t, err.Error(), "simulated serialization error")
}

// errorBinaryEvent — для теста ошибки сериализа��ии
type errorBinaryEvent struct {
	id string
}

func (f *errorBinaryEvent) Type() event.EventType { return event.EventTypeNetflow }
func (f *errorBinaryEvent) Timestamp() time.Time  { return time.Unix(12345, 0) }
func (f *errorBinaryEvent) Size() int             { return 100 }
func (f *errorBinaryEvent) Validate() error       { return nil }
func (f *errorBinaryEvent) GetID() string         { return f.id }
func (f *errorBinaryEvent) GetSourceIP() netip.Addr {
	addr, _ := netip.ParseAddr("192.0.2.1")
	return addr
}

func (f *errorBinaryEvent) GetDestinationIP() netip.Addr {
	addr, _ := netip.ParseAddr("192.0.2.2")
	return addr
}

func (f *errorBinaryEvent) ToBinaryNetFlow() ([]byte, error) {
	return nil, fmt.Errorf("simulated serialization error")
}

func (f *errorBinaryEvent) BinarySize() int {
	// Возвращаем фиксированный размер для тестов
	return 100
}

var (
	_ event.Event              = (*errorBinaryEvent)(nil)
	_ event.BinarySerializable = (*errorBinaryEvent)(nil)
)

func TestSerializedData_Validate_Success(t *testing.T) {
	sd := &SerializedData{
		Data:    []byte("valid data"),
		EventID: "valid-id",
		Size:    10,
	}
	err := sd.Validate()
	require.NoError(t, err)
}

func TestSerializedData_Validate_EmptyEventID(t *testing.T) {
	sd := &SerializedData{
		Data:    []byte("data"),
		EventID: "", // пустой ID
		Size:    4,
	}
	err := sd.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "event ID cannot be empty")
}

func TestSerializedData_Validate_EmptyData(t *testing.T) {
	sd := &SerializedData{
		Data:    []byte{},
		EventID: "id",
		Size:    0,
	}
	err := sd.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "serialized data cannot be empty")
}

func TestSerializedData_Validate_SizeMismatch(t *testing.T) {
	sd := &SerializedData{
		Data:    []byte("hello"),
		EventID: "id",
		Size:    10, // не совпадает с len("hello") = 5
	}
	err := sd.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "size mismatch")
}

func TestSerializedData_String(t *testing.T) {
	sd := &SerializedData{
		EventType: event.EventTypeNetflow,
		Size:      42,
		EventID:   "test-id",
	}
	expected := `SerializedData{Type: netflow, Size: 42, ID: test-id}`
	assert.Equal(t, expected, sd.String())
}
