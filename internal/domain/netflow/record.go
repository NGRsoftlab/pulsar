package netflow

import (
	"encoding/binary"
	"math/rand"
	"net/netip"
	"time"
)

// NetFlowV5Record представляет одну flow запись в NetFlow v5 пакете (48 байт)
type NetFlowV5Record struct {
	SrcAddr  [4]byte // IP адрес источника
	DstAddr  [4]byte // IP адрес назначения
	NextHop  [4]byte // IP адрес следующего hop (обычно 0.0.0.0)
	Input    uint16  // SNMP индекс входного интерфейса
	Output   uint16  // SNMP индекс выходного интерфейса
	Packets  uint32  // Количество пакетов в flow
	Bytes    uint32  // Количество байт в flow
	First    uint32  // SysUptime первого пакета
	Last     uint32  // SysUptime последнего пакета
	SrcPort  uint16  // TCP/UDP порт источника
	DstPort  uint16  // TCP/UDP порт назначения
	Pad1     uint8   // Unused (zero)
	TCPFlags uint8   // Совокупность TCP флагов
	Protocol uint8   // IP протокол (6=TCP, 17=UDP, 1=ICMP)
	ToS      uint8   // IP Type of Service
	SrcAS    uint16  // Autonomous system источника
	DstAS    uint16  // Autonomous system назначения
	SrcMask  uint8   // Маска подсети источника
	DstMask  uint8   // Маска подсети назначения
	Pad2     uint16  // Unused (zero)
}

// ToBytes сериализует запись в байты (network byte order)
func (r *NetFlowV5Record) ToBytes() []byte {
	buf := make([]byte, 48)

	copy(buf[0:4], r.SrcAddr[:])
	copy(buf[4:8], r.DstAddr[:])
	copy(buf[8:12], r.NextHop[:])

	binary.BigEndian.PutUint16(buf[12:14], r.Input)
	binary.BigEndian.PutUint16(buf[14:16], r.Output)

	binary.BigEndian.PutUint32(buf[16:20], r.Packets)
	binary.BigEndian.PutUint32(buf[20:24], r.Bytes)
	binary.BigEndian.PutUint32(buf[24:28], r.First)
	binary.BigEndian.PutUint32(buf[28:32], r.Last)

	binary.BigEndian.PutUint16(buf[32:34], r.SrcPort)
	binary.BigEndian.PutUint16(buf[34:36], r.DstPort)
	buf[36] = r.Pad1
	buf[37] = r.TCPFlags
	buf[38] = r.Protocol
	buf[39] = r.ToS

	binary.BigEndian.PutUint16(buf[40:42], r.SrcAS)
	binary.BigEndian.PutUint16(buf[42:44], r.DstAS)
	buf[44] = r.SrcMask
	buf[45] = r.DstMask
	binary.BigEndian.PutUint16(buf[46:48], r.Pad2)

	return buf
}

// NewNetFlowV5Record создает новую NetFlow запись из параметров
func NewNetFlowV5Record(srcIP, dstIP netip.Addr, srcPort, dstPort uint16,
	protocol uint8, bytes, packets uint32, tcpFlags uint8) *NetFlowV5Record {

	uptime := uint32(time.Since(bootTime).Milliseconds())

	first := uptime - uint32(rand.Intn(1000)) // случайное смещение до 1 сек
	last := uptime

	record := &NetFlowV5Record{
		Input:    0,
		Output:   0,
		Packets:  packets,
		Bytes:    bytes,
		First:    first,
		Last:     last,
		SrcPort:  srcPort,
		DstPort:  dstPort,
		Pad1:     0,
		TCPFlags: tcpFlags,
		Protocol: protocol,
		ToS:      0,
		SrcAS:    0,
		DstAS:    0,
		SrcMask:  24, // /24 подсеть по умолчанию
		DstMask:  24,
		Pad2:     0,
	}

	copy(record.SrcAddr[:], srcIP.AsSlice())
	copy(record.DstAddr[:], dstIP.AsSlice())
	// NextHop оставляем как 0.0.0.0

	return record
}

// ProtocolFromString конвертирует строковое название протокола в число
func ProtocolFromString(protocol string) uint8 {
	switch protocol {
	case "tcp", "TCP":
		return 6
	case "udp", "UDP":
		return 17
	case "icmp", "ICMP":
		return 1
	default:
		return 0
	}
}
