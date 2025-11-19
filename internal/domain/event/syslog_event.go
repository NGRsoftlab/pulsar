package event

import (
	"errors"
	"fmt"
	"math/rand"
	"net/netip"
	"time"
)

// SyslogPaloAltoEvent представляет событие syslog от Palo Alto Networks
type SyslogPaloAltoEvent struct {
	rawMessage string

	id            string
	timestamp     time.Time
	sourceIP      netip.Addr
	destinationIP netip.Addr
	validationErr error // Кэширование ошибки валидации
}

// NewSyslogPaloAltoEvent создаёт пустое, но инициализированное событие
func NewSyslogPaloAltoEvent() *SyslogPaloAltoEvent {
	now := time.Now()
	return &SyslogPaloAltoEvent{
		id:        generateEventID(EventTypeSyslog), // ← важно: передаём тип!
		timestamp: now,
	}
}

// Type реализует Event
func (e *SyslogPaloAltoEvent) Type() Type {
	return EventTypeSyslog
}

// Timestamp реализует Event
func (e *SyslogPaloAltoEvent) Timestamp() time.Time {
	return e.timestamp
}

// Size реализует Event: размер сырого сообщения в байтах
func (e *SyslogPaloAltoEvent) Size() int {
	return len(e.rawMessage)
}

// Validate реализует Event
func (e *SyslogPaloAltoEvent) Validate() error {
	if e.validationErr != nil {
		return e.validationErr
	}
	if e.rawMessage == "" {
		return e.cacheError("raw syslog message is empty")
	}
	if len(e.rawMessage) > 1500 {
		return e.cacheError("syslog message exceeds MTU (1500 bytes)")
	}
	if !e.sourceIP.IsValid() {
		return e.cacheError("invalid source IP")
	}
	if !e.destinationIP.IsValid() {
		return e.cacheError("invalid destination IP")
	}
	return nil
}

// GetID реализует Event
func (e *SyslogPaloAltoEvent) GetID() string {
	return e.id
}

// GetSourceIP реализует Event
func (e *SyslogPaloAltoEvent) GetSourceIP() netip.Addr {
	return e.sourceIP
}

// GetDestinationIP реализует Event
func (e *SyslogPaloAltoEvent) GetDestinationIP() netip.Addr {
	return e.destinationIP
}

// ToRawSyslog возвращает сырое syslog-сообщение
func (e *SyslogPaloAltoEvent) ToRawSyslog() string {
	return e.rawMessage
}

// IsUDPCompatible проверяет, помещается ли сообщение в UDP (MTU 1500 - 28)
func (e *SyslogPaloAltoEvent) IsUDPCompatible() bool {
	return len(e.rawMessage) <= 1472
}

// SetTrafficData устанавливает параметры трафика и генерирует cef-сообщение
func (e *SyslogPaloAltoEvent) SetTrafficData(
	srcIP, dstIP string,
	dstPort, srcPort uint16, // ← добавлен srcPort
	proto string,
	packets uint32,
	bytesIn, bytesOut uint64,
	action, subtype string,
) error {
	var err error
	e.sourceIP, err = netip.ParseAddr(srcIP)
	if err != nil {
		return fmt.Errorf("invalid source IP: %w", err)
	}
	e.destinationIP, err = netip.ParseAddr(dstIP)
	if err != nil {
		return fmt.Errorf("invalid destination IP: %w", err)
	}

	now := time.Now()
	cefTime := now.Format("Jan 02 2006 15:04:05")
	syslogTime := formatSyslogTimestamp(now)

	totalBytes := bytesIn + bytesOut

	// Генерируем дополнительные временные метки (можно сделать одинаковыми или разными)
	startTime := now.Add(-time.Duration(packets) * time.Millisecond)
	endTime := now
	deviceReceiptTime := now

	// Генерируем уникальный eventID (можно использовать timestamp + rand)
	eventID := fmt.Sprintf("%d", now.UnixNano())

	extension := fmt.Sprintf(
		"rt=%s "+
			"deviceExternalId=007057000057896 "+
			"externalId=%s "+ // ← внешний ID события
			"eventId=%s "+ // ← ID события
			"deviceEventClassId=%s "+
			"deviceEventCategory=%s "+ // ← явно указываем категорию
			"src=%s dst=%s "+
			"spt=%d dpt=%d "+ // ← srcPort теперь не 0
			"transportProtocol=%s "+ // ← вместо proto, но можно и оставить proto, если парсер знает
			"deviceAction=%s "+
			"app=web-browsing "+
			"cs1Label=Rule cs1=allow-all "+
			"cs4Label=FromZone cs4=trust "+
			"cs5Label=ToZone cs5=untrust "+
			"deviceInboundInterface=ethernet1/1 "+
			"deviceOutboundInterface=ethernet1/2 "+
			"cn1Label=SessionID cn1=%d "+
			"cnt=%d "+
			"in=%d out=%d "+
			"cn3Label=Packets cn3=%d "+
			"cs6Label=LogProfile cs6=default "+
			"startTime=%d "+
			"endTime=%d "+
			"deviceReceiptTime=%d",
		cefTime,
		eventID, // externalId
		eventID, // eventId
		subtype, // deviceEventClassId
		subtype, // deviceEventCategory (может быть уточнён)
		srcIP, dstIP,
		srcPort, dstPort, // ← теперь оба порта
		proto, // или можно использовать proto напрямую
		action,
		now.Unix(),
		totalBytes,
		bytesIn, bytesOut,
		packets,
		startTime.Unix(),         // startTime
		endTime.Unix(),           // endTime
		deviceReceiptTime.Unix(), // deviceReceiptTime
	)

	e.rawMessage = fmt.Sprintf(
		"<134>%s palo-device CEF:0|Palo Alto Networks|PAN-OS|10.0.0|%s|TRAFFIC|3|%s",
		syslogTime,
		subtype,
		extension,
	)

	e.timestamp = now
	e.validationErr = nil
	return nil
}

// GenerateRandomTrafficData заполняет событие случайными реалистичными данными
func (e *SyslogPaloAltoEvent) GenerateRandomTrafficData() error {
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
	srcPort := uint16(1024 + rand.Intn(64511))

	proto := protocols[rand.Intn(len(protocols))]
	srcIP := srcIPs[rand.Intn(len(srcIPs))]
	dstIP := dstIPs[rand.Intn(len(dstIPs))]

	var dstPort uint16
	if proto == "tcp" {
		dstPort = tcpPorts[rand.Intn(len(tcpPorts))]
	} else {
		dstPort = udpPorts[rand.Intn(len(udpPorts))]
	}

	// Генерация объёмов трафика
	packets := uint32(1 + rand.Intn(100))
	bytesIn := uint64(packets) * (64 + uint64(rand.Intn(1400)))
	bytesOut := uint64(packets) * (64 + uint64(rand.Intn(1400)))

	// Выбор action и subtype
	actions := []struct {
		action  string
		subtype string
	}{
		{"allow", "end"},
		{"allow", "start"},
		{"deny", "deny"},
		{"drop", "drop"},
	}
	selected := actions[rand.Intn(len(actions))]

	// Модифицируйте SetTrafficData, чтобы принимать action и subtype
	return e.SetTrafficData(
		srcIP, dstIP, dstPort, srcPort, proto,
		packets, bytesIn, bytesOut,
		selected.action, selected.subtype,
	)
}

// cacheError кэширует ошибку валидации
func (e *SyslogPaloAltoEvent) cacheError(msg string) error {
	e.validationErr = errors.New(msg)
	return e.validationErr
}

// formatSyslogTimestamp форматирует время по стандарту syslog (RFC 3164)
func formatSyslogTimestamp(t time.Time) string {
	month := t.Month().String()[:3]
	day := fmt.Sprintf("%d", t.Day())
	return fmt.Sprintf("%s %s %02d:%02d:%02d", month, day, t.Hour(), t.Minute(), t.Second())
}
