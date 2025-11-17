// Package types общие интерфейсы для корректной типизации
package types

import "github.com/NGRsoftlab/pulsar/internal/domain/event"

// JobBatch — интерфейс для пакетной обработки задач
type JobBatch interface {
	ExecuteBatch() error
}

type BinarySerializable interface {
	event.Event
	ToBinaryNetFlow() ([]byte, error) // возвращает бинарное NetFlow представление
	BinarySize() int                  // размер бинарных данных
}

type RawSerializable interface {
	event.Event
	ToRawSyslog() string   // Возвращает готовое сырое сообщение для отправки
	IsUDPCompatible() bool // Проверка, умещается ли в UDP пакет (MTU 1500)
}
