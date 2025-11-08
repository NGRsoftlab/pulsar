package netflow

import (
	"time"
)

var nowFunc = time.Now

// V5Header представляет заголовок NetFlow v5 пакета (24 байта)
type V5Header struct {
	Version      uint16 // Версия NetFlow (5)
	Count        uint16 // Количество flow records в пакете (максимум 30)
	SysUptime    uint32 // Время работы системы (миллисекунды)
	UnixSecs     uint32 // Секунды с 1 января 1970 UTC
	UnixNsecs    uint32 // Наносекунды (остаток)
	FlowSequence uint32 // Номер последовательности flow записей
	EngineType   uint8  // Тип engine (0)
	EngineID     uint8  // ID engine (0)
	SamplingMode uint16 // Режим сэмплирования (0 = не используется)
}

// NewV5Header создает новый заголовок с текущим временем
func NewV5Header(count uint16, sequence uint32) *V5Header {
	now := nowFunc()

	return &V5Header{
		Version:      5,
		Count:        count,
		SysUptime:    uint32(time.Since(bootTime).Milliseconds()),
		UnixSecs:     uint32(now.Unix()),
		UnixNsecs:    uint32(now.Nanosecond()),
		FlowSequence: sequence,
		EngineType:   0,
		EngineID:     0,
		SamplingMode: 0,
	}
}

// bootTime используется для расчета SysUptime (имитируем время запуска)
var bootTime = time.Now()
