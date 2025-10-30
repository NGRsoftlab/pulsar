package event

import (
	"fmt"
	"math/rand"
	"net/netip"
	"time"

	"github.com/nashabanov/ueba-event-generator/internal/domain/netflow"
)

// NetflowEvent представляет Netflow v5 событие с поддержкой бинарной сериализации
type NetflowEvent struct {
	ID             string
	EventTimestamp time.Time
	LogTimestamp   time.Time

	InBytes         uint64
	InPackets       uint32
	SourceAddr      netip.Addr
	DestinationAddr netip.Addr
	SourcePort      uint16
	DestinationPort uint16
	Protocol        string
	TCPFlags        string

	HostIP        netip.Addr
	HostIPListStr string
	ObserverType  string
}

func NewNetflowEvent() *NetflowEvent {
	now := time.Now().UTC()
	return &NetflowEvent{
		ID:             generateEventID(),
		EventTimestamp: now,
		LogTimestamp:   now,
		ObserverType:   "netflow",
	}
}

func (e NetflowEvent) Type() EventType {
	return EventTypeNetflow
}

func (e NetflowEvent) Timestamp() time.Time {
	return e.EventTimestamp
}

func (e NetflowEvent) GetID() string {
	return e.ID
}

func (e NetflowEvent) GetSourceIP() netip.Addr {
	return e.SourceAddr
}

func (e NetflowEvent) GetDestinationIP() netip.Addr {
	return e.DestinationAddr
}

// ToBinaryNetFlow создает бинарный NetFlow v5 пакет из события
func (e NetflowEvent) ToBinaryNetFlow() ([]byte, error) {
	protocolNum := netflow.ProtocolFromString(e.Protocol)

	var tcpFlags uint8
	if e.Protocol == "tcp" && e.TCPFlags != "" {
		tcpFlags = 0x18 // Пример TCP флагов PSH+ACK
	}

	record := netflow.NewNetFlowV5Record(
		e.SourceAddr,
		e.DestinationAddr,
		e.SourcePort,
		e.DestinationPort,
		protocolNum,
		uint32(e.InBytes),
		e.InPackets,
		tcpFlags,
	)

	packet, err := netflow.NewNetFlowV5Packet([]*netflow.NetFlowV5Record{record})
	if err != nil {
		return nil, fmt.Errorf("failed to create NetFlow packet: %w", err)
	}

	if err := packet.Validate(); err != nil {
		return nil, fmt.Errorf("invalid NetFlow packet: %w", err)
	}

	return packet.ToBytes()
}

// Size возвращает размер события — теперь привязан к бинарному представлению
func (e NetflowEvent) Size() int {
	return e.BinarySize()
}

func (e NetflowEvent) BinarySize() int {
	// Стандартный размер NetFlow v5 пакета с одной записью - 72 байта
	return 72
}

func (e NetflowEvent) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("id is required")
	}
	if e.EventTimestamp.IsZero() {
		return fmt.Errorf("event_time is required")
	}
	if !e.SourceAddr.IsValid() {
		return fmt.Errorf("source address is invalid: %v", e.SourceAddr)
	}
	if !e.DestinationAddr.IsValid() {
		return fmt.Errorf("destination address is invalid: %v", e.DestinationAddr)
	}
	if e.Protocol == "" {
		return fmt.Errorf("protocol is required")
	}
	if e.ObserverType != "netflow" {
		return fmt.Errorf("observer type must be 'netflow', got: %v", e.ObserverType)
	}
	return nil
}

func (e *NetflowEvent) SetTrafficData(srcIP, destIP string, srcPort, dstPort uint16, protocol string) error {
	var err error
	e.SourceAddr, err = netip.ParseAddr(srcIP)
	if err != nil {
		return fmt.Errorf("invalid source IP address: %v", err)
	}

	e.DestinationAddr, err = netip.ParseAddr(destIP)
	if err != nil {
		return fmt.Errorf("invalid destination IP address: %v", err)
	}

	e.SourcePort = srcPort
	e.DestinationPort = dstPort
	e.Protocol = protocol
	e.HostIP = e.SourceAddr
	e.HostIPListStr = e.SourceAddr.String()

	return nil
}

func (e *NetflowEvent) SetBytesAndPackets(bytes uint64, packets uint32) {
	e.InBytes = bytes
	e.InPackets = packets
}

// GenerateRandomTrafficData заполняет событие случайными реалистичными данными
func (e *NetflowEvent) GenerateRandomTrafficData() error {
	srcIPs := []string{
		"192.168.1.10", "192.168.1.15", "192.168.1.20",
		"10.0.1.100", "10.0.1.101", "10.0.2.50",
		"172.16.0.10", "172.16.0.20",
	}

	dstIPs := []string{
		"8.8.8.8", "1.1.1.1", "208.67.222.222",
		"192.168.1.1", "10.0.1.1",
		"93.184.216.34", "151.101.193.140",
	}

	protocols := []string{"tcp", "udp"}

	tcpPorts := []uint16{80, 443, 22, 21, 25, 53, 993, 995, 8080, 8443}
	udpPorts := []uint16{53, 123, 161, 514, 1194, 4500}

	protocol := protocols[rand.Intn(len(protocols))]
	srcIP := srcIPs[rand.Intn(len(srcIPs))]
	dstIP := dstIPs[rand.Intn(len(dstIPs))]

	var srcPort, dstPort uint16
	if protocol == "tcp" {
		srcPort = 1024 + uint16(rand.Intn(64511))
		dstPort = tcpPorts[rand.Intn(len(tcpPorts))]
	} else {
		srcPort = 1024 + uint16(rand.Intn(64511))
		dstPort = udpPorts[rand.Intn(len(udpPorts))]
	}

	if err := e.SetTrafficData(srcIP, dstIP, srcPort, dstPort, protocol); err != nil {
		return err
	}

	var bytes uint64
	var packets uint32

	switch dstPort {
	case 80, 443:
		packets = uint32(10 + rand.Intn(100))
		bytes = uint64(packets) * (500 + uint64(rand.Intn(2000)))
	case 53:
		packets = uint32(1 + rand.Intn(5))
		bytes = uint64(packets) * (50 + uint64(rand.Intn(200)))
	case 22:
		packets = uint32(5 + rand.Intn(50))
		bytes = uint64(packets) * (100 + uint64(rand.Intn(500)))
	default:
		packets = uint32(1 + rand.Intn(20))
		bytes = uint64(packets) * (64 + uint64(rand.Intn(1000)))
	}

	e.SetBytesAndPackets(bytes, packets)

	if protocol == "tcp" {
		tcpFlagsOptions := []string{"", ".A", ".AP", ".S", ".SA", ".F", ".R"}
		e.TCPFlags = tcpFlagsOptions[rand.Intn(len(tcpFlagsOptions))]
	}

	return nil
}
