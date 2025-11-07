package event

import (
	"math/rand"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewNetflowEvent(t *testing.T) {
	event := NewNetflowEvent()
	assert.NotEmpty(t, event.ID)
	assert.Equal(t, "netflow", event.ObserverType)
	assert.False(t, event.EventTimestamp.IsZero())
	assert.False(t, event.LogTimestamp.IsZero())
}

func TestNetflowEvent_Type(t *testing.T) {
	event := NewNetflowEvent()
	assert.Equal(t, EventTypeNetflow, event.Type())
}

func TestNetflowEvent_Timestamp(t *testing.T) {
	now := time.Now().UTC()
	event := &NetflowEvent{EventTimestamp: now}
	assert.Equal(t, now, event.Timestamp())
}

func TestNetflowEvent_GetID(t *testing.T) {
	event := &NetflowEvent{ID: "test-id"}
	assert.Equal(t, "test-id", event.GetID())
}

func TestNetflowEvent_GetSourceIP(t *testing.T) {
	ip := netip.MustParseAddr("192.168.1.10")
	event := &NetflowEvent{SourceAddr: ip}
	assert.Equal(t, ip, event.GetSourceIP())
}

func TestNetflowEvent_GetDestinationIP(t *testing.T) {
	ip := netip.MustParseAddr("8.8.8.8")
	event := &NetflowEvent{DestinationAddr: ip}
	assert.Equal(t, ip, event.GetDestinationIP())
}

func TestNetflowEvent_Validate(t *testing.T) {
	tests := []struct {
		name    string
		event   NetflowEvent
		wantErr bool
	}{
		{
			name: "valid event",
			event: NetflowEvent{
				ID:              "123",
				EventTimestamp:  time.Now(),
				SourceAddr:      netip.MustParseAddr("192.168.1.10"),
				DestinationAddr: netip.MustParseAddr("8.8.8.8"),
				Protocol:        "tcp",
				ObserverType:    "netflow",
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			event: NetflowEvent{
				EventTimestamp:  time.Now(),
				SourceAddr:      netip.MustParseAddr("192.168.1.10"),
				DestinationAddr: netip.MustParseAddr("8.8.8.8"),
				Protocol:        "tcp",
				ObserverType:    "netflow",
			},
			wantErr: true,
		},
		{
			name: "zero timestamp",
			event: NetflowEvent{
				ID:              "123",
				SourceAddr:      netip.MustParseAddr("192.168.1.10"),
				DestinationAddr: netip.MustParseAddr("8.8.8.8"),
				Protocol:        "tcp",
				ObserverType:    "netflow",
			},
			wantErr: true,
		},
		{
			name: "invalid source IP",
			event: NetflowEvent{
				ID:              "123",
				EventTimestamp:  time.Now(),
				DestinationAddr: netip.MustParseAddr("8.8.8.8"),
				Protocol:        "tcp",
				ObserverType:    "netflow",
			},
			wantErr: true,
		},
		{
			name: "invalid destination IP",
			event: NetflowEvent{
				ID:             "123",
				EventTimestamp: time.Now(),
				SourceAddr:     netip.MustParseAddr("192.168.1.10"),
				Protocol:       "tcp",
				ObserverType:   "netflow",
			},
			wantErr: true,
		},
		{
			name: "missing protocol",
			event: NetflowEvent{
				ID:              "123",
				EventTimestamp:  time.Now(),
				SourceAddr:      netip.MustParseAddr("192.168.1.10"),
				DestinationAddr: netip.MustParseAddr("8.8.8.8"),
				ObserverType:    "netflow",
			},
			wantErr: true,
		},
		{
			name: "wrong observer type",
			event: NetflowEvent{
				ID:              "123",
				EventTimestamp:  time.Now(),
				SourceAddr:      netip.MustParseAddr("192.168.1.10"),
				DestinationAddr: netip.MustParseAddr("8.8.8.8"),
				Protocol:        "tcp",
				ObserverType:    "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNetflowEvent_SetTrafficData(t *testing.T) {
	event := NewNetflowEvent()
	err := event.SetTrafficData("192.168.1.10", "8.8.8.8", 50000, 443, "tcp")
	require.NoError(t, err)

	assert.Equal(t, netip.MustParseAddr("192.168.1.10"), event.SourceAddr)
	assert.Equal(t, netip.MustParseAddr("8.8.8.8"), event.DestinationAddr)
	assert.Equal(t, uint16(50000), event.SourcePort)
	assert.Equal(t, uint16(443), event.DestinationPort)
	assert.Equal(t, "tcp", event.Protocol)
	assert.Equal(t, event.SourceAddr, event.HostIP)
	assert.Equal(t, "192.168.1.10", event.HostIPListStr)
}

func TestNetflowEvent_SetTrafficData_InvalidIP(t *testing.T) {
	event := NewNetflowEvent()
	err := event.SetTrafficData("not-an-ip", "8.8.8.8", 50000, 443, "tcp")
	assert.Error(t, err)

	err = event.SetTrafficData("192.168.1.10", "also-not-an-ip", 50000, 443, "tcp")
	assert.Error(t, err)
}

func TestNetflowEvent_SetBytesAndPackets(t *testing.T) {
	event := NewNetflowEvent()
	event.SetBytesAndPackets(12345, 100)
	assert.Equal(t, uint64(12345), event.InBytes)
	assert.Equal(t, uint32(100), event.InPackets)
}

func TestNetflowEvent_BinarySize(t *testing.T) {
	event := NewNetflowEvent()
	assert.Equal(t, 72, event.BinarySize())
	assert.Equal(t, 72, event.Size())
}

func TestNetflowEvent_ToBinaryNetFlow(t *testing.T) {
	event := NewNetflowEvent()
	err := event.SetTrafficData("192.168.1.10", "8.8.8.8", 12345, 443, "tcp")
	require.NoError(t, err)
	event.SetBytesAndPackets(1500, 3)

	b, err := event.ToBinaryNetFlow()
	require.NoError(t, err)
	assert.Len(t, b, 72)
}

func TestNetflowEvent_GenerateRandomTrafficData(t *testing.T) {
	rand.Seed(42) // для воспроизводимости

	event := NewNetflowEvent()
	err := event.GenerateRandomTrafficData()
	require.NoError(t, err)

	// Проверяем, что данные установлены
	assert.NotEmpty(t, event.ID)
	assert.True(t, event.SourceAddr.IsValid())
	assert.True(t, event.DestinationAddr.IsValid())
	assert.NotEmpty(t, event.Protocol)
	assert.Greater(t, event.InPackets, uint32(0))
	assert.Greater(t, event.InBytes, uint64(0))
	assert.NotEmpty(t, event.HostIPListStr)

	// Проверяем валидность
	assert.NoError(t, event.Validate())
}
