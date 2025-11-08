package netflow

import (
	"encoding/binary"
	"math/rand"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testNow = time.Unix(1700000000, 123456789) // фиксированное "сейчас"

func mockGlobals() {
	bootTime = testNow.Add(-10 * time.Minute) // uptime = 10 минут = 600000 мс
	atomic.StoreUint64(&sequenceCounter, 999)
	rand.Seed(42)
	nowFunc = func() time.Time { return testNow }
}

func restoreGlobals(boot time.Time, seq uint64, origNow func() time.Time) {
	bootTime = boot
	atomic.StoreUint64(&sequenceCounter, seq)
	nowFunc = origNow
}

func saveGlobals() (boot time.Time, seq uint64, origNow func() time.Time) {
	return bootTime, atomic.LoadUint64(&sequenceCounter), nowFunc
}

func TestProtocolFromString(t *testing.T) {
	tests := []struct {
		proto string
		want  uint8
	}{
		{"tcp", 6},
		{"TCP", 6},
		{"udp", 17},
		{"UDP", 17},
		{"icmp", 1},
		{"ICMP", 1},
		{"sctp", 0},
		{"", 0},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, ProtocolFromString(tt.proto))
	}
}

func TestNewV5Record(t *testing.T) {
	srcIP := netip.MustParseAddr("192.168.1.10")
	dstIP := netip.MustParseAddr("8.8.8.8")

	rec := NewV5Record(srcIP, dstIP, 50000, 443, 6, 1500, 3, 0x18)

	// Проверим IP
	assert.Equal(t, []byte{192, 168, 1, 10}, rec.SrcAddr[:])
	assert.Equal(t, []byte{8, 8, 8, 8}, rec.DstAddr[:])

	// Проверим порты и данные
	assert.Equal(t, uint16(50000), rec.SrcPort)
	assert.Equal(t, uint16(443), rec.DstPort)
	assert.Equal(t, uint32(1500), rec.Bytes)
	assert.Equal(t, uint32(3), rec.Packets)
	assert.Equal(t, uint8(6), rec.Protocol)
	assert.Equal(t, uint8(0x18), rec.TCPFlags)
	assert.Equal(t, uint8(24), rec.SrcMask)
	assert.Equal(t, uint8(24), rec.DstMask)

	// Проверим, что NextHop = 0
	assert.Equal(t, [4]byte{0, 0, 0, 0}, rec.NextHop)
}

func TestV5Record_ToBytes(t *testing.T) {
	srcIP := netip.MustParseAddr("10.0.0.1")
	dstIP := netip.MustParseAddr("1.1.1.1")

	rec := NewV5Record(srcIP, dstIP, 12345, 53, 17, 200, 2, 0)
	buf := rec.ToBytes()

	assert.Len(t, buf, 48)

	// Проверим начало: IP-адреса
	assert.Equal(t, []byte{10, 0, 0, 1}, buf[0:4])
	assert.Equal(t, []byte{1, 1, 1, 1}, buf[4:8])

	// Порты
	assert.Equal(t, uint16(12345), binary.BigEndian.Uint16(buf[32:34]))
	assert.Equal(t, uint16(53), binary.BigEndian.Uint16(buf[34:36]))

	// Протокол
	assert.Equal(t, uint8(17), buf[38])

	// Байты и пакеты
	assert.Equal(t, uint32(200), binary.BigEndian.Uint32(buf[20:24]))
	assert.Equal(t, uint32(2), binary.BigEndian.Uint32(buf[16:20]))
}

func TestNewV5Header(t *testing.T) {
	mockGlobals()
	defer func() {
		// Восстановим, если нужно
	}()

	header := NewV5Header(1, 1000)

	assert.Equal(t, uint16(5), header.Version)
	assert.Equal(t, uint16(1), header.Count)
	assert.Equal(t, uint32(1000), header.FlowSequence)
	assert.Equal(t, uint32(1700000000), header.UnixSecs) // зависит от bootTime
	assert.Equal(t, uint8(0), header.EngineType)
	assert.Equal(t, uint8(0), header.EngineID)
	assert.Equal(t, uint16(0), header.SamplingMode)

	// SysUptime должно быть >= 0 (но точное значение зависит от времени теста)
	assert.GreaterOrEqual(t, header.SysUptime, uint32(0))
}

func TestNewV5Packet(t *testing.T) {
	bootOrig, seqOrig, nowOrig := saveGlobals()
	mockGlobals()
	defer restoreGlobals(bootOrig, seqOrig, nowOrig)

	srcIP := netip.MustParseAddr("192.168.1.10")
	dstIP := netip.MustParseAddr("8.8.8.8")
	rec := NewV5Record(srcIP, dstIP, 12345, 443, 6, 1500, 3, 0x18)

	packet, err := NewV5Packet([]*V5Record{rec})
	require.NoError(t, err)
	assert.Equal(t, uint16(1), packet.Header.Count)
	assert.Equal(t, uint32(1000), packet.Header.FlowSequence) // sequenceCounter был 999 → +1 = 1000

	// Проверим, что запись сохранена
	assert.Len(t, packet.Records, 1)
	assert.Equal(t, rec.SrcPort, packet.Records[0].SrcPort)
}

func TestNewV5Packet_TooManyRecords(t *testing.T) {
	var records []*V5Record
	for i := 0; i <= MaxRecordsPerPacket; i++ {
		rec := NewV5Record(
			netip.MustParseAddr("1.1.1.1"),
			netip.MustParseAddr("2.2.2.2"),
			10000+uint16(i), 80, 6, 100, 1, 0,
		)
		records = append(records, rec)
	}

	_, err := NewV5Packet(records)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too many records")
}

func TestNewV5Packet_EmptyRecords(t *testing.T) {
	_, err := NewV5Packet(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one record")

	_, err = NewV5Packet([]*V5Record{})
	assert.Error(t, err)
}

func TestV5Packet_ToBytes(t *testing.T) {
	bootOrig, seqOrig, nowOrig := saveGlobals()
	mockGlobals()
	defer restoreGlobals(bootOrig, seqOrig, nowOrig)

	rec := NewV5Record(
		netip.MustParseAddr("10.0.1.100"),
		netip.MustParseAddr("93.184.216.34"),
		54321, 80, 6, 2048, 10, 0x18,
	)

	packet, err := NewV5Packet([]*V5Record{rec})
	require.NoError(t, err)

	buf, err := packet.ToBytes()
	require.NoError(t, err)
	assert.Len(t, buf, 72) // 24 + 48

	// Проверим сигнатуру: версия = 5
	assert.Equal(t, uint16(5), binary.BigEndian.Uint16(buf[0:2]))
	// Количество записей = 1
	assert.Equal(t, uint16(1), binary.BigEndian.Uint16(buf[2:4]))

	// Проверим IP в записи (начинается с offset 24)
	assert.Equal(t, []byte{10, 0, 1, 100}, buf[24:28])
	assert.Equal(t, []byte{93, 184, 216, 34}, buf[28:32])

	// Порт 54321 = 0xD431
	assert.Equal(t, uint16(54321), binary.BigEndian.Uint16(buf[56:58]))
	assert.Equal(t, uint16(80), binary.BigEndian.Uint16(buf[58:60]))
	assert.Equal(t, uint8(6), buf[62])
	assert.Equal(t, uint8(0x18), buf[61])
}

func TestV5Packet_Validate(t *testing.T) {
	rec := NewV5Record(netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("2.2.2.2"), 1000, 80, 6, 100, 1, 0)

	packet, err := NewV5Packet([]*V5Record{rec})
	require.NoError(t, err)

	assert.NoError(t, packet.Validate())

	// Испорченный header
	packet.Header = nil
	assert.Error(t, packet.Validate())

	packet.Header = &V5Header{Count: 2} // должно быть 1
	packet.Records = []*V5Record{rec}
	assert.Error(t, packet.Validate())

	packet.Header.Count = 1
	packet.Header.Version = 4
	assert.Error(t, packet.Validate())
}
