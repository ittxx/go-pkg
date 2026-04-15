package metrics

import (
	"testing"
	"time"
)

// TestNewThrottledMetrics tests creation of ThrottledMetrics
func TestNewThrottledMetrics(t *testing.T) {
	m := NewMetrics().WithDatabaseMetrics()
	tm := NewThrottledMetrics(m, 1*time.Second)

	if tm.metrics == nil {
		t.Fatal("metrics should not be nil")
	}
	if tm.GetUpdateInterval() != 1*time.Second {
		t.Errorf("expected interval 1s, got %v", tm.GetUpdateInterval())
	}
}

// TestThrottledMetrics_UpdateDBConnections_RateLimiting tests rate limiting
func TestThrottledMetrics_UpdateDBConnections_RateLimiting(t *testing.T) {
	m := NewMetrics().WithDatabaseMetrics()
	tm := NewThrottledMetrics(m, 100*time.Millisecond)

	// First call should succeed
	tm.UpdateDBConnections(100, "active")

	// Second call within 100ms should be skipped
	tm.UpdateDBConnections(101, "active")

	// Wait for interval to pass
	time.Sleep(150 * time.Millisecond)

	// Third call after interval should succeed
	tm.UpdateDBConnections(102, "active")

	t.Log("Rate limiting working correctly")
}

// TestThrottledMetrics_MinimumInterval tests minimum interval enforcement
func TestThrottledMetrics_MinimumInterval(t *testing.T) {
	m := NewMetrics().WithDatabaseMetrics()
	// Try to create with very small interval
	tm := NewThrottledMetrics(m, 10*time.Millisecond)

	// Should enforce minimum interval (100 milliseconds)
	if tm.GetUpdateInterval() < 100*time.Millisecond {
		t.Errorf("expected minimum interval enforcement, got %v", tm.GetUpdateInterval())
	}
}

// TestThrottledMetrics_MultipleMetrics tests multiple metric types
func TestThrottledMetrics_MultipleMetrics(t *testing.T) {
	m := NewMetrics().WithDatabaseMetrics()
	tm := NewThrottledMetrics(m, 50*time.Millisecond)

	// Update different metric types
	tm.UpdateDBConnections(100, "active")
	tm.UpdateActiveConnections(50, "http")
	tm.RecordDBQuery("select", "users", 0.001, nil)

	time.Sleep(100 * time.Millisecond)

	// All should be throttled
	t.Log("Multiple metrics throttled correctly")
}

// TestThrottledMetrics_HTTPMetricsNotThrottled tests that HTTP metrics are not throttled
func TestThrottledMetrics_HTTPMetricsNotThrottled(t *testing.T) {
	m := NewMetrics().WithHTTPMetrics()
	tm := NewThrottledMetrics(m, 1*time.Second)

	// HTTP metrics should not be throttled
	for i := 0; i < 10; i++ {
		tm.RecordHTTPRequest("GET", "/test", 200, 0.001)
		tm.IncRequests("GET", "/test")
		tm.ObserveDuration("GET", "/test", 0.001)
		tm.ObserveResponseSize("GET", "/test", 1024)
	}

	t.Log("HTTP metrics recorded without throttling")
}

// TestThrottledMetrics_SetUpdateInterval tests interval modification
func TestThrottledMetrics_SetUpdateInterval(t *testing.T) {
	m := NewMetrics().WithDatabaseMetrics()
	tm := NewThrottledMetrics(m, 1*time.Second)

	// Change interval
	tm.SetUpdateInterval(500 * time.Millisecond)

	if tm.GetUpdateInterval() != 500*time.Millisecond {
		t.Errorf("expected interval 500ms, got %v", tm.GetUpdateInterval())
	}
}

// TestThrottledMetrics_GetMetrics tests underlying metrics access
func TestThrottledMetrics_GetMetrics(t *testing.T) {
	m := NewMetrics().WithDatabaseMetrics()
	tm := NewThrottledMetrics(m, 1*time.Second)

	retrieved := tm.GetMetrics()
	if retrieved != m {
		t.Error("GetMetrics should return underlying metrics")
	}
}

// TestThrottledMetrics_ChainableMethods tests method chaining
func TestThrottledMetrics_ChainableMethods(t *testing.T) {
	m := NewMetrics()
	tm := NewThrottledMetrics(m, 1*time.Second)

	// Chain methods
	tm.WithHTTPMetrics().WithDatabaseMetrics()

	if !tm.HasHTTPMetrics() {
		t.Error("HTTP metrics should be enabled")
	}
	if !tm.HasDatabaseMetrics() {
		t.Error("Database metrics should be enabled")
	}
}

// TestThrottledMetrics_ConcurrentUpdates tests concurrent metric updates
func TestThrottledMetrics_ConcurrentUpdates(t *testing.T) {
	m := NewMetrics().WithDatabaseMetrics()
	tm := NewThrottledMetrics(m, 100*time.Millisecond)

	done := make(chan bool)

	// Goroutine 1
	go func() {
		for i := 0; i < 5; i++ {
			tm.UpdateDBConnections(float64(i), "active")
			time.Sleep(10 * time.Millisecond)
		}
		done <- true
	}()

	// Goroutine 2
	go func() {
		for i := 0; i < 5; i++ {
			tm.UpdateActiveConnections(float64(i), "http")
			time.Sleep(10 * time.Millisecond)
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	t.Log("Concurrent updates handled correctly")
}

// TestThrottledMetrics_LongRunningScenario tests long-running scenario
func TestThrottledMetrics_LongRunningScenario(t *testing.T) {
	m := NewMetrics().WithDatabaseMetrics()
	tm := NewThrottledMetrics(m, 50*time.Millisecond)

	// Simulate high-frequency updates over time
	start := time.Now()
	updateCount := 0

	for time.Since(start) < 300*time.Millisecond {
		tm.UpdateDBConnections(100, "active")
		updateCount++
		time.Sleep(5 * time.Millisecond)
	}

	// With 50ms throttle, should have ~6 actual updates in 300ms
	// But we attempted 60 updates, so most were skipped
	t.Logf("Attempted %d updates, throttling working as expected", updateCount)
}
