package network

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/nashabanov/ueba-event-generator/internal/metrics"
)

type UDPSender struct {
	mu         sync.RWMutex
	conn       net.Conn
	timeout    time.Duration
	remoteAddr string
}

func NewUDPSender(destination string, timeout time.Duration) (*UDPSender, error) {
	conn, err := net.Dial("udp", destination)
	if err != nil {
		return nil, fmt.Errorf("failed to dial UDP %s: %w", destination, err)
	}
	metrics.GetGlobalMetrics().IncrementConnections()
	return &UDPSender{
		conn:       conn,
		timeout:    timeout,
		remoteAddr: destination,
	}, nil
}

func (u *UDPSender) Send(destination string, data []byte) error {
	u.mu.RLock()
	defer u.mu.RUnlock()

	if u.conn == nil {
		return fmt.Errorf("UDP sender not initialized")
	}

	if destination != u.remoteAddr {
		return fmt.Errorf("UDP sender supports only single destination: %s", u.remoteAddr)
	}

	u.conn.SetWriteDeadline(time.Now().Add(u.timeout))
	_, err := u.conn.Write(data)
	if err != nil {
		metrics.GetGlobalMetrics().IncrementTimeouts()
		return fmt.Errorf("UDP write failde to %s: %w", destination, err)
	}

	metrics.GetGlobalMetrics().IncrementSent()
	return nil
}

func (u *UDPSender) Close() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.conn != nil {
		err := u.conn.Close()
		u.conn = nil
		metrics.GetGlobalMetrics().DecrementConnections()
		return err
	}

	return nil
}

func (u *UDPSender) IsHealthy() (bool, string) {
	u.mu.RLock()
	defer u.mu.RUnlock()
	if u.conn == nil {
		return false, "UDP socket closed"
	}
	return true, "UDP sender healthy"
}

func (u *UDPSender) GetStats() map[string]any {
	return map[string]any{
		"type": "udp",
		"addr": u.remoteAddr,
	}
}

func (u *UDPSender) SetTimeout(timeout time.Duration) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.timeout = timeout
}
