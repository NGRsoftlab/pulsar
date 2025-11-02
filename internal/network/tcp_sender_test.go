package network

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func startTestTCPServer(t *testing.T) (string, func()) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	assert.NoError(t, err)

	done := make(chan bool)
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				for {
					_, err := c.Read(buf)
					if err != nil {
						break
					}
				}
			}(conn)
		}
	}()

	addr := listener.Addr().String()

	cleanup := func() {
		listener.Close()
		<-done
	}

	return addr, cleanup
}

func TestTCPSender_Send_Success(t *testing.T) {
	addr, cleanup := startTestTCPServer(t)
	defer cleanup()

	sender, err := NewTCPSender(addr, 2, 5*time.Second)
	assert.NoError(t, err)
	defer sender.Close()

	err = sender.Send(addr, []byte("hello"))
	assert.NoError(t, err)
}

func TestTCPSender_Send_WrongDestination(t *testing.T) {
	addr, cleanup := startTestTCPServer(t)
	defer cleanup()

	sender, err := NewTCPSender(addr, 1, 1*time.Second)
	assert.NoError(t, err)

	err = sender.Send("127.0.0.1:9999", []byte("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TCP sender supports only single destination")
}

func TestTCPSender_Send_NoHealthyConnections(t *testing.T) {
	// Неверный адрес — пул создастся частично или полностью провалится
	sender, err := NewTCPSender("127.0.0.1:65535", 1, 100*time.Millisecond) // порт точно недоступен
	if err != nil {
		// Если пул не создан — ок, но тогда Send не вызвать
		t.Skip("Pool creation failed, skipping send test")
	}
	defer sender.Close()

	err = sender.Send("127.0.0.1:65535", []byte("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no healthy TCP connection")
}

func TestTCPSender_IsHealthy(t *testing.T) {
	addr, cleanup := startTestTCPServer(t)
	defer cleanup()

	sender, err := NewTCPSender(addr, 2, 1*time.Second)
	assert.NoError(t, err)
	defer sender.Close()

	healthy, msg := sender.IsHealthy()
	assert.True(t, healthy)
	assert.Equal(t, "TCP sender healthy", msg)
}

func TestTCPSender_GetStats(t *testing.T) {
	addr, cleanup := startTestTCPServer(t)
	defer cleanup()

	sender, err := NewTCPSender(addr, 3, 1*time.Second)
	assert.NoError(t, err)
	defer sender.Close()

	stats := sender.GetStats()
	assert.Equal(t, "tcp", stats["type"])
	assert.Equal(t, addr, stats["addr"])
	assert.Equal(t, 3, stats["connections_total"])
	assert.Equal(t, 3, stats["connections_healthy"])
	assert.InDelta(t, 100.0, stats["efficiency_pct"], 0.01)
}

func TestTCPSender_Close(t *testing.T) {
	addr, cleanup := startTestTCPServer(t)
	defer cleanup()

	sender, err := NewTCPSender(addr, 1, 1*time.Second)
	assert.NoError(t, err)

	err = sender.Close()
	assert.NoError(t, err)

	// Повторный Close безопасен
	err = sender.Close()
	assert.NoError(t, err)
}
