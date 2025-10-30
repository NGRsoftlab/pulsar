package netflow

import (
	"encoding/binary"
	"fmt"
	"sync/atomic"
)

// NetFlowV5Packet представляет полный NetFlow v5 пакет
type NetFlowV5Packet struct {
	Header  *NetFlowV5Header
	Records []*NetFlowV5Record
}

// sequenceCounter глобальный счетчик для sequence numbers
var sequenceCounter uint64

// MaxRecordsPerPacket максимальное количество записей в одном пакете
const MaxRecordsPerPacket = 30

// NewNetFlowV5Packet создает новый NetFlow пакет с записями
func NewNetFlowV5Packet(records []*NetFlowV5Record) (*NetFlowV5Packet, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("packet must contain at least one record")
	}

	if len(records) > MaxRecordsPerPacket {
		return nil, fmt.Errorf("too many records: %d (max %d)", len(records), MaxRecordsPerPacket)
	}

	// Получаем следующий sequence number
	sequence := atomic.AddUint64(&sequenceCounter, uint64(len(records)))

	header := NewNetFlowV5Header(uint16(len(records)), uint32(sequence))

	return &NetFlowV5Packet{
		Header:  header,
		Records: records,
	}, nil
}

// ToBytes сериализует NetFlow v5 пакет в бинарный вид
// ToBytes сериализует NetFlow v5 пакет в бинарный вид
func (p *NetFlowV5Packet) ToBytes() ([]byte, error) {
	// Валидация
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Размер пакета: 24 (header) + 48 * records
	totalSize := 24 + len(p.Records)*48
	buf := make([]byte, totalSize)

	// --- Сериализуем заголовок вручную ---
	binary.BigEndian.PutUint16(buf[0:2], p.Header.Version)
	binary.BigEndian.PutUint16(buf[2:4], p.Header.Count)
	binary.BigEndian.PutUint32(buf[4:8], p.Header.SysUptime)
	binary.BigEndian.PutUint32(buf[8:12], p.Header.UnixSecs)
	binary.BigEndian.PutUint32(buf[12:16], p.Header.UnixNsecs)
	binary.BigEndian.PutUint32(buf[16:20], p.Header.FlowSequence)
	buf[20] = p.Header.EngineType
	buf[21] = p.Header.EngineID
	binary.BigEndian.PutUint16(buf[22:24], p.Header.SamplingMode)

	// --- Сериализуем записи ---
	offset := 24
	for _, rec := range p.Records {
		recordBytes := rec.ToBytes()
		copy(buf[offset:offset+48], recordBytes)
		offset += 48
	}

	return buf, nil
}

// Size возвращает размер пакета в байтах
func (p *NetFlowV5Packet) Size() int {
	return 24 + len(p.Records)*48
}

// Validate проверяет корректность пакета
func (p *NetFlowV5Packet) Validate() error {
	if p.Header == nil {
		return fmt.Errorf("header is required")
	}

	if len(p.Records) == 0 {
		return fmt.Errorf("packet must contain at least one record")
	}

	if len(p.Records) > MaxRecordsPerPacket {
		return fmt.Errorf("too many records: %d (max %d)", len(p.Records), MaxRecordsPerPacket)
	}

	if p.Header.Count != uint16(len(p.Records)) {
		return fmt.Errorf("header count (%d) doesn't match records count (%d)",
			p.Header.Count, len(p.Records))
	}

	if p.Header.Version != 5 {
		return fmt.Errorf("invalid version: %d (expected 5)", p.Header.Version)
	}

	return nil
}
