// Package network включает в себя интерфейс и реализации отправки данных через разные протоколы
package network

import "time"

type Sender interface {
	// Send отправляет данные по указанному destination
	// формат destination "host:port"
	Send(destination string, data []byte) error

	// Close закрывает все соединения
	Close() error

	// IsHealthy возвращает true, если Sender способен работать
	// Второе значение для диагностики
	IsHealthy() (bool, string)

	// GetStats возвращает метрики, в зависимости от реализации
	GetStats() map[string]any

	// SetTimeout устанавливает таймаут на операции записи
	SetTimeout(timeout time.Duration)
}
