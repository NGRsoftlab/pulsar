package event

import (
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestNewSyslogPaloAltoEvent(t *testing.T) {
	event := NewSyslogPaloAltoEvent()
	if event == nil {
		t.Fatal("Expected non-nil event")
	}
	if event.id == "" {
		t.Error("Expected non-empty ID")
	}
	if event.timestamp.IsZero() {
		t.Error("Expected non-zero timestamp")
	}
	if event.Type() != EventTypeSyslog {
		t.Errorf("Expected type %v, got %v", EventTypeSyslog, event.Type())
	}
}

func TestSyslogPaloAltoEvent_Validate_Before_SetTrafficData(t *testing.T) {
	event := NewSyslogPaloAltoEvent()
	if err := event.Validate(); err == nil {
		t.Error("Expected validation error before SetTrafficData")
	} else if !strings.Contains(err.Error(), "raw syslog message is empty") {
		t.Errorf("Unexpected validation error: %v", err)
	}
}

func TestSyslogPaloAltoEvent_SetTrafficData_Valid(t *testing.T) {
	event := NewSyslogPaloAltoEvent()

	err := event.SetTrafficData(
		"192.168.1.10", "8.8.8.8",
		443, "tcp",
		100, 1024, 2048,
		"allow", "end",
	)
	if err != nil {
		t.Fatalf("SetTrafficData failed: %v", err)
	}

	// Проверка полей
	if !event.sourceIP.IsValid() || event.sourceIP.String() != "192.168.1.10" {
		t.Errorf("Unexpected source IP: %v", event.sourceIP)
	}
	if !event.destinationIP.IsValid() || event.destinationIP.String() != "8.8.8.8" {
		t.Errorf("Unexpected destination IP: %v", event.destinationIP)
	}

	if event.rawMessage == "" {
		t.Error("Raw message should not be empty")
	}

	if err := event.Validate(); err != nil {
		t.Errorf("Validation failed after SetTrafficData: %v", err)
	}

	// Проверка, что сообщение содержит ключевые части CEF
	expectedParts := []string{
		"CEF:0|Palo Alto Networks|PAN-OS|10.0.0|end|TRAFFIC|3|",
		"src=192.168.1.10",
		"dst=8.8.8.8",
		"dpt=443",
		"proto=tcp",
		"deviceAction=allow",
		"deviceEventClassId=end",
	}
	for _, part := range expectedParts {
		if !strings.Contains(event.rawMessage, part) {
			t.Errorf("Raw message missing expected part: %q", part)
		}
	}
}

func TestSyslogPaloAltoEvent_SetTrafficData_InvalidIP(t *testing.T) {
	event := NewSyslogPaloAltoEvent()

	err := event.SetTrafficData(
		"not.an.ip", "8.8.8.8",
		443, "tcp",
		100, 1024, 2048,
		"allow", "end",
	)
	if err == nil {
		t.Fatal("Expected error for invalid source IP")
	}
	if !strings.Contains(err.Error(), "invalid source IP") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestSyslogPaloAltoEvent_IsUDPCompatible(t *testing.T) {
	event := NewSyslogPaloAltoEvent()
	err := event.SetTrafficData(
		"192.168.1.1", "8.8.8.8",
		443, "tcp",
		1, 64, 64,
		"allow", "end",
	)
	if err != nil {
		t.Fatalf("SetTrafficData failed: %v", err)
	}

	if !event.IsUDPCompatible() {
		t.Errorf("Small message should be UDP-compatible; size=%d", len(event.rawMessage))
	}

	longMessage := strings.Repeat("X", 1500) // точно >1472
	event = &SyslogPaloAltoEvent{
		id:            "test",
		timestamp:     time.Now(),
		rawMessage:    longMessage,
		sourceIP:      netip.MustParseAddr("192.168.1.1"),
		destinationIP: netip.MustParseAddr("8.8.8.8"),
	}

	if event.IsUDPCompatible() {
		t.Errorf("Message of size %d should NOT be UDP-compatible", len(event.rawMessage))
	}
}

func TestSyslogPaloAltoEvent_GenerateRandomTrafficData(t *testing.T) {
	event := NewSyslogPaloAltoEvent()
	err := event.GenerateRandomTrafficData()
	if err != nil {
		t.Fatalf("GenerateRandomTrafficData failed: %v", err)
	}

	if event.rawMessage == "" {
		t.Error("Raw message should not be empty after generation")
	}
	if !event.sourceIP.IsValid() {
		t.Error("Source IP should be valid")
	}
	if !event.destinationIP.IsValid() {
		t.Error("Destination IP should be valid")
	}
	if event.validationErr != nil {
		t.Errorf("Validation error after generation: %v", event.validationErr)
	}
	if err := event.Validate(); err != nil {
		t.Errorf("Validation failed after random generation: %v", err)
	}
}

func TestSyslogPaloAltoEvent_Getters(t *testing.T) {
	event := NewSyslogPaloAltoEvent()
	err := event.SetTrafficData(
		"10.0.1.100", "1.1.1.1",
		53, "udp",
		10, 512, 256,
		"deny", "deny",
	)
	if err != nil {
		t.Fatalf("SetTrafficData failed: %v", err)
	}

	if event.GetID() != event.id {
		t.Error("GetID mismatch")
	}
	if event.GetSourceIP() != event.sourceIP {
		t.Error("GetSourceIP mismatch")
	}
	if event.GetDestinationIP() != event.destinationIP {
		t.Error("GetDestinationIP mismatch")
	}
	if event.Size() != len(event.rawMessage) {
		t.Error("Size mismatch")
	}
	if event.Timestamp() != event.timestamp {
		t.Error("Timestamp mismatch")
	}
}

func TestSyslogPaloAltoEvent_Validate_MTU_Exact(t *testing.T) {
	// Создадим событие с сообщением ровно 1500 байт
	event := &SyslogPaloAltoEvent{
		id:            "test",
		timestamp:     time.Now(),
		rawMessage:    strings.Repeat("A", 1500),
		sourceIP:      netip.MustParseAddr("192.168.1.1"),
		destinationIP: netip.MustParseAddr("8.8.8.8"),
	}

	if err := event.Validate(); err != nil {
		t.Errorf("Validation should pass for 1500-byte message: %v", err)
	}

	// Превышение
	event.rawMessage += "X"
	if err := event.Validate(); err == nil {
		t.Error("Expected validation error for >1500-byte message")
	} else if !strings.Contains(err.Error(), "exceeds MTU") {
		t.Errorf("Unexpected error: %v", err)
	}
}
