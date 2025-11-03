// Package network tcp_sender.go - релизация интерфейса Sender для отправки по tcp
package network

import (
	"fmt"
	"time"
)

type ConnectionPool interface {
	GetConnection() *TCPConnection
	Close()
	GetStats() (total, healthy int)
}

type TCPSender struct {
	pool    ConnectionPool
	timeout time.Duration
	addr    string
}

func NewTCPSender(destination string, poolSize int, timeout time.Duration) (*TCPSender, error) {
	pool, err := NewTCPConnectionPool(destination, poolSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create TCP pool: %w", err)
	}
	return &TCPSender{
		pool:    pool,
		timeout: timeout,
		addr:    destination,
	}, nil
}

func (t *TCPSender) Send(destination string, data []byte) error {
	if destination != t.addr {
		return fmt.Errorf("TCP sender supports only single destination: %s", t.addr)
	}

	conn := t.pool.GetConnection()
	if conn == nil {
		return fmt.Errorf("no healthy TCP connection to %s", destination)
	}

	dataWithNewline := append(data, '\n')
	written := 0
	for written < len(dataWithNewline) {
		conn.SetWriteDeadline(time.Now().Add(t.timeout))
		n, err := conn.Write(dataWithNewline[written:])
		if err != nil {
			conn.MarkUnhealthy()
			return fmt.Errorf("TCP write failed to %s: %w", destination, err)
		}
		written += n
	}

	return nil
}

func (t *TCPSender) Close() error {
	if t.pool != nil {
		t.pool.Close()
		t.pool = nil
	}
	return nil
}

func (t *TCPSender) IsHealthy() (bool, string) {
	if t.pool == nil {
		return false, "TCP pool not initialized"
	}
	total, healthy := t.pool.GetStats()
	if healthy == 0 {
		return false, fmt.Sprintf("no healthy TCP connections (0/%d)", total)
	}
	if float64(healthy)/float64(total) < 0.5 {
		return false, fmt.Sprintf("TCP pool degraded: %d/%d healthy", healthy, total)
	}
	return true, "TCP sender healthy"
}

func (t *TCPSender) GetStats() map[string]any {
	if t.pool == nil {
		return map[string]any{"type": "tcp", "status": "closed"}
	}
	total, healthy := t.pool.GetStats()
	return map[string]any{
		"type":                "tcp",
		"addr":                t.addr,
		"connections_total":   total,
		"connections_healthy": healthy,
		"efficiency_pct":      float64(healthy) / float64(total) * 100,
	}
}

func (t *TCPSender) SetTimeout(timeout time.Duration) {
	t.timeout = timeout
}
