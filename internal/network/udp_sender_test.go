package network

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// startTestUDPServer запускает UDP-сервер на случайном порту и возвращает адрес и функцию остановки.
// Также возвращает канал, в который приходят полученные сообщения.
func startTestUDPServer(t *testing.T) (string, <-chan []byte, func()) {
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	assert.NoError(t, err)

	conn, err := net.ListenUDP("udp", addr)
	assert.NoError(t, err)

	messages := make(chan []byte, 10)
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer close(messages)
		buf := make([]byte, 65536)
		for {
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				return // соединение закрыто
			}
			msg := make([]byte, n)
			copy(msg, buf[:n])
			select {
			case messages <- msg:
			default:
				// игнорируем переполнение
			}
		}
	}()

	cleanup := func() {
		conn.Close()
		<-done
	}

	return conn.LocalAddr().String(), messages, cleanup
}

func TestUDPSender_Send_Success(t *testing.T) {
	addr, messages, cleanup := startTestUDPServer(t)
	defer cleanup()

	sender, err := NewUDPSender(addr, 2*time.Second)
	assert.NoError(t, err)
	defer sender.Close()

	message := []byte("hello from UDP")
	err = sender.Send(addr, message)
	assert.NoError(t, err)

	select {
	case received := <-messages:
		assert.Equal(t, message, received)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("UDP message not received")
	}
}

func TestUDPSender_Send_WrongDestination(t *testing.T) {
	addr, _, cleanup := startTestUDPServer(t)
	defer cleanup()

	sender, err := NewUDPSender(addr, 1*time.Second)
	assert.NoError(t, err)
	defer sender.Close()

	err = sender.Send("127.0.0.1:9999", []byte("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "UDP sender supports only single destination")
}

func TestUDPSender_Send_AfterClose(t *testing.T) {
	addr, _, cleanup := startTestUDPServer(t)
	defer cleanup()

	sender, err := NewUDPSender(addr, 1*time.Second)
	assert.NoError(t, err)

	err = sender.Close()
	assert.NoError(t, err)

	err = sender.Send(addr, []byte("should fail"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "UDP sender not initialized")
}

func TestUDPSender_IsHealthy(t *testing.T) {
	addr, _, cleanup := startTestUDPServer(t)
	defer cleanup()

	sender, err := NewUDPSender(addr, 1*time.Second)
	assert.NoError(t, err)
	defer sender.Close()

	healthy, msg := sender.IsHealthy()
	assert.True(t, healthy)
	assert.Equal(t, "UDP sender healthy", msg)

	sender.Close()
	healthy, msg = sender.IsHealthy()
	assert.False(t, healthy)
	assert.Equal(t, "UDP socket closed", msg)
}

func TestUDPSender_GetStats(t *testing.T) {
	addr, _, cleanup := startTestUDPServer(t)
	defer cleanup()

	sender, err := NewUDPSender(addr, 1*time.Second)
	assert.NoError(t, err)
	defer sender.Close()

	stats := sender.GetStats()
	assert.Equal(t, "udp", stats["type"])
	assert.Equal(t, addr, stats["addr"])
}

func TestUDPSender_SetTimeout(t *testing.T) {
	addr, _, cleanup := startTestUDPServer(t)
	defer cleanup()

	sender, err := NewUDPSender(addr, 5*time.Second)
	assert.NoError(t, err)
	defer sender.Close()

	sender.SetTimeout(10 * time.Second)
}

func TestUDPSender_Send_WriteError(t *testing.T) {
	sender := &UDPSender{
		conn:       nil,
		remoteAddr: "127.0.0.1:12345",
		timeout:    1 * time.Second,
	}

	err := sender.Send("127.0.0.1:12345", []byte("test"))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "UDP sender not initialized")
}
