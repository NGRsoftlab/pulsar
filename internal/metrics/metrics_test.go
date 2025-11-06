package metrics

import (
	"testing"
	"time"
)

func TestNewPerformanceMetrics(t *testing.T) {
	m := NewPerformanceMetrics()
	if m == nil {
		t.Fatal("Expected non-nil metrics")
	}

	if m.minProcessingTime != ^uint64(0) {
		t.Errorf("Expected minProcessingTime to be max uint64, got %d", m.minProcessingTime)
	}
}

func TestEventCounters(t *testing.T) {
	m := NewPerformanceMetrics()

	m.IncrementGenerated()
	m.IncrementSent()
	m.IncrementFailed()
	m.IncrementDropped()

	g, s, f, _ := m.GetStats()
	if g != 1 || s != 1 || f != 1 {
		t.Errorf("Expected (1,1,1), got (%d,%d,%d)", g, s, f)
	}

	d := atomicLoad(&m.droppedEvents)
	if d != 1 {
		t.Errorf("Expected dropped=1, got %d", d)
	}
}

func TestNetworkCounters(t *testing.T) {
	m := NewPerformanceMetrics()

	m.IncrementConnections()
	m.IncrementReconnects()
	m.IncrementTimeouts()
	m.DecrementConnections() // should go back to 0

	conns := atomicLoad(&m.networkConnections)
	if conns != 0 {
		t.Errorf("Expected connections=0 after inc+dec, got %d", conns)
	}

	if atomicLoad(&m.networkReconnects) != 1 ||
		atomicLoad(&m.networkTimeouts) != 1 {
		t.Error("Reconnects or timeouts not incremented correctly")
	}
}

func TestWorkerPoolCounters(t *testing.T) {
	m := NewPerformanceMetrics()

	m.IncrementActiveWorkers()
	m.IncrementActiveWorkers()
	m.DecrementActiveWorkers()
	m.SetQueuedJobs(5)
	m.IncrementCompletedJobs()
	m.IncrementRejectedJobs()

	active := atomicLoad(&m.activeWorkers)
	queued := atomicLoad(&m.queuedJobs)
	completed := atomicLoad(&m.completedJobs)
	rejected := atomicLoad(&m.rejectedJobs)

	if active != 1 {
		t.Errorf("Expected active workers = 1, got %d", active)
	}
	if queued != 5 {
		t.Errorf("Expected queued jobs = 5, got %d", queued)
	}
	if completed != 1 || rejected != 1 {
		t.Errorf("Completed/rejected mismatch: %d / %d", completed, rejected)
	}
}

func TestRecordProcessingTime(t *testing.T) {
	m := NewPerformanceMetrics()

	// Record valid durations
	m.RecordProcessingTime(100 * time.Millisecond)
	m.RecordProcessingTime(200 * time.Millisecond)
	m.RecordProcessingTime(50 * time.Millisecond)

	total := atomicLoad(&m.totalProcessingTime)
	max := atomicLoad(&m.maxProcessingTime)
	min := atomicLoad(&m.minProcessingTime)

	expectedTotal := uint64((100 + 200 + 50) * 1e6) // ns
	if total != expectedTotal {
		t.Errorf("Expected total=%d, got %d", expectedTotal, total)
	}
	if max != uint64(200*1e6) {
		t.Errorf("Expected max=200ms in ns, got %d", max)
	}
	if min != uint64(50*1e6) {
		t.Errorf("Expected min=50ms in ns, got %d", min)
	}

	// Ignore invalid durations
	m.RecordProcessingTime(-10 * time.Millisecond)
	m.RecordProcessingTime(0)

	// Values should not change
	if atomicLoad(&m.totalProcessingTime) != expectedTotal {
		t.Error("Invalid duration changed total")
	}
}

func TestEPSAndCurrentEPS(t *testing.T) {
	m := NewPerformanceMetrics()

	// Simulate some events over time
	m.IncrementGenerated()
	time.Sleep(100 * time.Millisecond)
	m.IncrementGenerated()
	m.IncrementGenerated()

	// Get average EPS (over full runtime)
	_, _, _, avgEPS := m.GetStats()
	if avgEPS <= 0 {
		t.Errorf("Expected positive avg EPS, got %.2f", avgEPS)
	}

	// First call to current EPS → returns 0 and initializes baseline
	currentEPS := m.calculateCurrentEPS()
	if currentEPS != 0 {
		t.Errorf("First current EPS should be 0, got %.2f", currentEPS)
	}

	// Wait and generate more
	time.Sleep(1100 * time.Millisecond)
	m.IncrementGenerated()
	m.IncrementGenerated()

	// Now current EPS should reflect recent rate
	currentEPS = m.calculateCurrentEPS()
	if currentEPS <= 0 {
		t.Errorf("Expected positive current EPS after second batch, got %.2f", currentEPS)
	}
}

func TestGetDetailedStats(t *testing.T) {
	m := NewPerformanceMetrics()

	m.IncrementGenerated()
	m.IncrementSent()
	m.RecordProcessingTime(100 * time.Millisecond)
	m.IncrementConnections()
	m.IncrementActiveWorkers()
	m.SetQueuedJobs(3)

	stats := m.GetDetailedStats()

	if stats.Generated != 1 ||
		stats.Sent != 1 ||
		stats.ActiveWorkers != 1 ||
		stats.QueuedJobs != 3 {
		t.Error("Detailed stats mismatch")
	}

	if stats.AvgProcessingTime != 100*time.Millisecond {
		t.Errorf("Expected avg processing = 100ms, got %v", stats.AvgProcessingTime)
	}

	if stats.Goroutines <= 0 {
		t.Error("Goroutines count should be > 0")
	}
}

func TestGlobalMetricsSingleton(t *testing.T) {
	m1 := GetGlobalMetrics()
	m2 := GetGlobalMetrics()

	if m1 != m2 {
		t.Error("Global metrics should be singleton")
	}

	m1.IncrementGenerated()
	g2, _, _, _ := m2.GetStats()
	if g2 != 1 {
		t.Error("Global instance not shared")
	}
}

// Helper for cleaner atomic reads in tests
func atomicLoad(ptr *uint64) uint64 {
	return *ptr // safe in single-threaded test context
}
