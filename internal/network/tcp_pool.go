// Package network tcp_pool.go - релизация пула tcp подключений
package network

import (
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nashabanov/ueba-event-generator/internal/metrics"
)

type TCPConnection struct {
	conn      net.Conn
	id        int
	isHealthy bool
	sendCount uint64
	mutex     sync.RWMutex
}

func (conn *TCPConnection) SetWriteDeadline(deadline time.Time) error {
	conn.mutex.RLock()
	defer conn.mutex.RUnlock()
	if conn.conn == nil {
		return fmt.Errorf("no underlying connection")
	}
	return conn.conn.SetWriteDeadline(deadline)
}

type TCPConnectionPool struct {
	connections []*TCPConnection
	destination string
	poolSize    int
	roundRobin  uint64
	mutex       sync.RWMutex
}

func NewTCPConnectionPool(destination string, poolSize int) (*TCPConnectionPool, error) {
	pool := &TCPConnectionPool{
		destination: destination,
		poolSize:    poolSize,
		connections: make([]*TCPConnection, poolSize),
	}

	log.Printf("Creating TCP connection pool: %d connections to %s", poolSize, destination)

	// Создаем все соединения сразу
	successCount := 0
	for i := range poolSize {
		conn, err := pool.createConnection(i)
		if err != nil {
			log.Printf("Failed to create connection %d: %v", i, err)
			continue
		}
		pool.connections[i] = conn
		successCount++
	}

	if successCount == 0 {
		return nil, fmt.Errorf("failed to create any connections")
	}

	log.Printf("TCP pool created: %d/%d connections successful", successCount, poolSize)
	return pool, nil
}

func (pool *TCPConnectionPool) createConnection(id int) (*TCPConnection, error) {
	conn, err := net.DialTimeout("tcp", pool.destination, 5*time.Second)
	if err != nil {
		return nil, err
	}

	// Оптимизируем TCP соединение
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		if err := tcpConn.SetNoDelay(true); err != nil {
			log.Printf("SetNoDelay failed for conn %p: %v", tcpConn, err)
		}
		if err := tcpConn.SetKeepAlive(true); err != nil {
			log.Printf("SetKeepAlive failed: %v", err)
		}
		if err := tcpConn.SetKeepAlivePeriod(30 * time.Second); err != nil {
			log.Printf("SetKeepAlivePeriod failed: %v", err)
		}
		if err := tcpConn.SetWriteBuffer(64 * 1024); err != nil {
			log.Printf("SetWriteBuffer failed: %v", err)
		}
		if err := tcpConn.SetReadBuffer(64 * 1024); err != nil {
			log.Printf("SetReadBuffer failed: %v", err)
		}
	}

	metrics.GetGlobalMetrics().IncrementConnections()

	return &TCPConnection{
		conn:      conn,
		id:        id,
		isHealthy: true,
	}, nil
}

func (pool *TCPConnectionPool) GetConnection() *TCPConnection {
	// Round-robin selection с проверкой здоровья
	attempts := 0
	maxAttempts := pool.poolSize * 2 // Даем два полных круга

	for attempts < maxAttempts {
		index := atomic.AddUint64(&pool.roundRobin, 1) % uint64(pool.poolSize)

		pool.mutex.RLock()
		conn := pool.connections[index]
		pool.mutex.RUnlock()

		if conn != nil && conn.IsHealthy() {
			atomic.AddUint64(&conn.sendCount, 1)
			return conn
		}

		attempts++
	}

	// Все соединения нездоровы - пытаемся восстановить одно
	return pool.tryRecreateConnection()
}

func (conn *TCPConnection) IsHealthy() bool {
	conn.mutex.RLock()
	defer conn.mutex.RUnlock()
	return conn.isHealthy
}

func (conn *TCPConnection) MarkUnhealthy() {
	conn.mutex.Lock()
	defer conn.mutex.Unlock()

	if conn.isHealthy {
		conn.isHealthy = false
		if conn.conn != nil {
			conn.conn.Close()
			metrics.GetGlobalMetrics().DecrementConnections()
			metrics.GetGlobalMetrics().IncrementReconnects()
		}
		log.Printf("TCP connection %d marked as unhealthy", conn.id)
	}
}

func (conn *TCPConnection) Write(data []byte) (int, error) {
	conn.mutex.RLock()
	defer conn.mutex.RUnlock()

	if !conn.isHealthy || conn.conn == nil {
		return 0, fmt.Errorf("connection %d is not healthy", conn.id)
	}

	// Устанавливаем deadline
	if err := conn.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		log.Printf("SetWriteDeadline failed on conn %d: %v", conn.id, err)
		conn.MarkUnhealthy()
		return 0, fmt.Errorf("deadline set failed: %w", err)
	}

	return conn.conn.Write(data)
}

func (pool *TCPConnectionPool) tryRecreateConnection() *TCPConnection {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	// Ищем первое нездоровое соединение для пересоздания
	for i, conn := range pool.connections {
		if conn == nil || !conn.IsHealthy() {
			newConn, err := pool.createConnection(i)
			if err != nil {
				log.Printf("Failed to recreate connection %d: %v", i, err)
				continue
			}

			pool.connections[i] = newConn
			log.Printf("Recreated TCP connection %d", i)
			return newConn
		}
	}

	return nil
}

func (pool *TCPConnectionPool) Close() {
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	for i, conn := range pool.connections {
		if conn != nil {
			conn.MarkUnhealthy()
			log.Printf("Closed TCP connection %d", i)
		}
	}

	log.Printf("TCP connection pool closed")
}

func (pool *TCPConnectionPool) GetStats() (total, healthy int) {
	pool.mutex.RLock()
	defer pool.mutex.RUnlock()

	total = len(pool.connections)
	for _, conn := range pool.connections {
		if conn != nil && conn.IsHealthy() {
			healthy++
		}
	}
	return
}
