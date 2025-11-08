package logger

import "log"

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

type StdLogger struct{}

func NewStdLogger() *StdLogger {
	return &StdLogger{}
}

func (l *StdLogger) Debug(msg string, args ...any) {
	log.Printf("[DEBUG] "+msg, args...)
}

func (l *StdLogger) Info(msg string, args ...any) {
	log.Printf("[INFO] "+msg, args...)
}

func (l *StdLogger) Warn(msg string, args ...any) {
	log.Printf("[WARNING] "+msg, args...)
}

func (l *StdLogger) Error(msg string, args ...any) {
	log.Printf("[ERROR] "+msg, args...)
}
