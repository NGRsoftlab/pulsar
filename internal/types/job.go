// Package types общие интерфейсы для корректной типизации
package types

// JobBatch — интерфейс для пакетной обработки задач
type JobBatch interface {
	ExecuteBatch() error
}
