package network

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockConn implements net.Conn for testing
type MockConn struct {
	readBuffer  *bytes.Buffer
	writeBuffer *bytes.Buffer
	closed      bool
	writeError  error
	readError   error
	mutex       sync.Mutex
}

func NewMockConn() *MockConn {
	return &MockConn{
		readBuffer:  bytes.NewBuffer(nil),
		writeBuffer: bytes.NewBuffer(nil),
	}
}

func (m *MockConn) Read(b []byte) (int, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closed {
		return 0, fmt.Errorf("connection closed")
	}
	if m.readError != nil {
		return 0, m.readError
	}
	return m.readBuffer.Read(b)
}

func (m *MockConn) Write(b []byte) (int, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.closed {
		return 0, fmt.Errorf("connection closed")
	}
	if m.writeError != nil {
		return 0, m.writeError
	}
	return m.writeBuffer.Write(b)
}

func (m *MockConn) Close() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.closed = true
	return nil
}

func (m *MockConn) LocalAddr() net.Addr                { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080} }
func (m *MockConn) RemoteAddr() net.Addr               { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8081} }
func (m *MockConn) SetDeadline(t time.Time) error      { return nil }
func (m *MockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *MockConn) SetWriteDeadline(t time.Time) error { return nil }

func (m *MockConn) SetWriteError(err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.writeError = err
}

func (m *MockConn) SetReadError(err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.readError = err
}

// Mock metrics for testing
type MockMetrics struct {
	connections int64
	reconnects  int64
}

func (m *MockMetrics) IncrementConnections() {
	atomic.AddInt64(&m.connections, 1)
}

func (m *MockMetrics) DecrementConnections() {
	atomic.AddInt64(&m.connections, -1)
}

func (m *MockMetrics) IncrementReconnects() {
	atomic.AddInt64(&m.reconnects, 1)
}

func (m *MockMetrics) GetConnections() int64 {
	return atomic.LoadInt64(&m.connections)
}

func (m *MockMetrics) GetReconnects() int64 {
	return atomic.LoadInt64(&m.reconnects)
}

// Helper function to create a test TCP server
func createTestServer(t *testing.T) (string, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create test server: %v", err)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buffer := make([]byte, 1024)
				for {
					n, err := c.Read(buffer)
					if err != nil {
						return
					}
					c.Write(buffer[:n])
				}
			}(conn)
		}
	}()

	return ln.Addr().String(), func() { ln.Close() }
}

func TestNewTCPConnectionPool(t *testing.T) {
	tests := []struct {
		name        string
		destination string
		poolSize    int
		expectError bool
	}{
		{
			name:        "valid pool creation",
			destination: "127.0.0.1:0",
			poolSize:    3,
			expectError: true, // No server listening
		},
		{
			name:        "zero pool size",
			destination: "127.0.0.1:8080",
			poolSize:    0,
			expectError: true,
		},
		{
			name:        "invalid destination",
			destination: "invalid:destination",
			poolSize:    2,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := NewTCPConnectionPool(tt.destination, tt.poolSize)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				if pool != nil {
					pool.Close()
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if pool == nil {
					t.Error("Expected pool but got nil")
				}
				if pool != nil {
					pool.Close()
				}
			}
		})
	}
}

func TestNewTCPConnectionPoolWithServer(t *testing.T) {
	serverAddr, cleanup := createTestServer(t)
	defer cleanup()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	pool, err := NewTCPConnectionPool(serverAddr, 2)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	total, healthy := pool.GetStats()
	if total != 2 {
		t.Errorf("Expected 2 total connections, got %d", total)
	}
	if healthy != 2 {
		t.Errorf("Expected 2 healthy connections, got %d", healthy)
	}
}

func TestTCPConnection_IsHealthy(t *testing.T) {
	conn := &TCPConnection{
		conn:      NewMockConn(),
		isHealthy: true,
	}

	if !conn.IsHealthy() {
		t.Error("Connection should be healthy")
	}

	conn.MarkUnhealthy()

	if conn.IsHealthy() {
		t.Error("Connection should be unhealthy after marking")
	}
}

func TestTCPConnection_Write(t *testing.T) {
	tests := []struct {
		name        string
		setupConn   func() *TCPConnection
		data        []byte
		expectError bool
	}{
		{
			name: "successful write",
			setupConn: func() *TCPConnection {
				return &TCPConnection{
					conn:      NewMockConn(),
					isHealthy: true,
				}
			},
			data:        []byte("test data"),
			expectError: false,
		},
		{
			name: "write to unhealthy connection",
			setupConn: func() *TCPConnection {
				return &TCPConnection{
					conn:      NewMockConn(),
					isHealthy: false,
				}
			},
			data:        []byte("test data"),
			expectError: true,
		},
		{
			name: "write to nil connection",
			setupConn: func() *TCPConnection {
				return &TCPConnection{
					conn:      nil,
					isHealthy: true,
				}
			},
			data:        []byte("test data"),
			expectError: true,
		},
		{
			name: "write error",
			setupConn: func() *TCPConnection {
				mockConn := NewMockConn()
				mockConn.SetWriteError(fmt.Errorf("write error"))
				return &TCPConnection{
					conn:      mockConn,
					isHealthy: true,
				}
			},
			data:        []byte("test data"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := tt.setupConn()

			n, err := conn.Write(tt.data)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if n != len(tt.data) {
					t.Errorf("Expected to write %d bytes, wrote %d", len(tt.data), n)
				}
			}
		})
	}
}

func TestTCPConnectionPool_GetConnection(t *testing.T) {
	serverAddr, cleanup := createTestServer(t)
	defer cleanup()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	pool, err := NewTCPConnectionPool(serverAddr, 3)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	// Test round-robin distribution
	connections := make([]*TCPConnection, 10)
	for i := 0; i < 10; i++ {
		connections[i] = pool.GetConnection()
		if connections[i] == nil {
			t.Errorf("GetConnection returned nil at iteration %d", i)
		}
	}

	// Verify round-robin behavior by checking send counts
	var totalSends uint64
	for i := 0; i < 3; i++ {
		if pool.connections[i] != nil {
			totalSends += atomic.LoadUint64(&pool.connections[i].sendCount)
		}
	}

	if totalSends != 10 {
		t.Errorf("Expected total sends to be 10, got %d", totalSends)
	}
}

func TestTCPConnectionPool_GetConnectionAllUnhealthy(t *testing.T) {
	serverAddr, cleanup := createTestServer(t)
	defer cleanup()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	pool, err := NewTCPConnectionPool(serverAddr, 2)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	// Mark all connections as unhealthy
	for _, conn := range pool.connections {
		if conn != nil {
			conn.MarkUnhealthy()
		}
	}

	// Should try to recreate a connection
	conn := pool.GetConnection()
	if conn == nil {
		t.Error("Expected recreated connection but got nil")
	} else if !conn.IsHealthy() {
		t.Error("Recreated connection should be healthy")
	}
}

func TestTCPConnectionPool_Close(t *testing.T) {
	serverAddr, cleanup := createTestServer(t)
	defer cleanup()

	time.Sleep(100 * time.Millisecond)

	pool, err := NewTCPConnectionPool(serverAddr, 2)
	require.NoError(t, err)

	pool.Close()

	// Проверяем, что все соединения помечены как unhealthy
	for i, conn := range pool.connections {
		if conn != nil {
			assert.False(t, conn.IsHealthy(), "Connection %d should be marked unhealthy after Close()", i)
		}
	}
}

func TestTCPConnectionPool_GetStats(t *testing.T) {
	serverAddr, cleanup := createTestServer(t)
	defer cleanup()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	pool, err := NewTCPConnectionPool(serverAddr, 3)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	total, healthy := pool.GetStats()
	expectedTotal := 3
	expectedHealthy := 3

	if total != expectedTotal {
		t.Errorf("Expected %d total connections, got %d", expectedTotal, total)
	}
	if healthy != expectedHealthy {
		t.Errorf("Expected %d healthy connections, got %d", expectedHealthy, healthy)
	}

	// Mark one connection as unhealthy
	if pool.connections[0] != nil {
		pool.connections[0].MarkUnhealthy()
	}

	_, healthy = pool.GetStats()
	if healthy != 2 {
		t.Errorf("Expected 2 healthy connections after marking one unhealthy, got %d", healthy)
	}
}

func TestTCPConnectionPool_ConcurrentAccess(t *testing.T) {
	serverAddr, cleanup := createTestServer(t)
	defer cleanup()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	pool, err := NewTCPConnectionPool(serverAddr, 5)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	var wg sync.WaitGroup
	numGoroutines := 20
	connectionsPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < connectionsPerGoroutine; j++ {
				conn := pool.GetConnection()
				if conn == nil {
					t.Errorf("GetConnection returned nil in goroutine")
					return
				}
				// Simulate some work
				time.Sleep(time.Millisecond)
			}
		}()
	}

	wg.Wait()

	// Verify pool integrity
	total, _ := pool.GetStats()
	if total != 5 {
		t.Errorf("Expected pool size to remain 5, got %d", total)
	}
}

func TestTCPConnection_MarkUnhealthy(t *testing.T) {
	mockConn := NewMockConn()
	conn := &TCPConnection{
		conn:      mockConn,
		id:        1,
		isHealthy: true,
	}

	// First call should mark as unhealthy and close connection
	conn.MarkUnhealthy()

	if conn.IsHealthy() {
		t.Error("Connection should be marked as unhealthy")
	}

	// Second call should not panic or cause issues
	conn.MarkUnhealthy()

	if conn.IsHealthy() {
		t.Error("Connection should remain unhealthy")
	}
}

func TestTCPConnectionPool_tryRecreateConnection(t *testing.T) {
	serverAddr, cleanup := createTestServer(t)
	defer cleanup()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	pool, err := NewTCPConnectionPool(serverAddr, 2)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	// Mark first connection as unhealthy
	if pool.connections[0] != nil {
		pool.connections[0].MarkUnhealthy()
	}

	// Try to recreate connection
	newConn := pool.tryRecreateConnection()

	if newConn == nil {
		t.Error("Failed to recreate connection")
	} else if !newConn.IsHealthy() {
		t.Error("Recreated connection should be healthy")
	}
}

func TestTCPConnectionPool_RoundRobinDistribution(t *testing.T) {
	serverAddr, cleanup := createTestServer(t)
	defer cleanup()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	poolSize := 3
	pool, err := NewTCPConnectionPool(serverAddr, poolSize)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	// Reset send counts
	for _, conn := range pool.connections {
		if conn != nil {
			atomic.StoreUint64(&conn.sendCount, 0)
		}
	}

	// Get connections multiple times
	requestCount := poolSize * 3
	for i := 0; i < requestCount; i++ {
		conn := pool.GetConnection()
		if conn == nil {
			t.Errorf("GetConnection returned nil at iteration %d", i)
		}
	}

	// Verify round-robin distribution
	for i, conn := range pool.connections {
		if conn == nil {
			continue
		}
		sendCount := atomic.LoadUint64(&conn.sendCount)
		expectedCount := uint64(requestCount / poolSize)
		if sendCount != expectedCount {
			t.Errorf("Connection %d: expected %d sends, got %d", i, expectedCount, sendCount)
		}
	}
}

// Benchmark tests
func BenchmarkTCPConnectionPool_GetConnection(b *testing.B) {
	serverAddr, cleanup := createTestServer(&testing.T{})
	defer cleanup()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	pool, err := NewTCPConnectionPool(serverAddr, 10)
	if err != nil {
		b.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn := pool.GetConnection()
			if conn == nil {
				b.Error("GetConnection returned nil")
			}
		}
	})
}

func BenchmarkTCPConnection_Write(b *testing.B) {
	data := []byte("benchmark test data")
	conn := &TCPConnection{
		conn:      NewMockConn(),
		isHealthy: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := conn.Write(data)
		if err != nil {
			b.Errorf("Write failed: %v", err)
		}
	}
}
