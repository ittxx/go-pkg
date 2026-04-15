package metrics

import (
	"testing"
	"time"
)

func TestNewMetricsAndModules(t *testing.T) {
	m := NewMetrics()
	if m == nil {
		t.Fatal("NewMetrics returned nil")
	}

	if m.HasHTTPMetrics() {
		t.Fatal("expected HTTP metrics to be disabled by default")
	}

	m.WithHTTPMetrics()
	if !m.HasHTTPMetrics() {
		t.Fatal("expected HTTP metrics to be enabled after WithHTTPMetrics()")
	}

	if m.HasDatabaseMetrics() {
		t.Fatal("expected DB metrics to be disabled by default")
	}

	m.WithDatabaseMetrics()
	if !m.HasDatabaseMetrics() {
		t.Fatal("expected DB metrics to be enabled after WithDatabaseMetrics()")
	}

	// Basic exercise of API to ensure no panics and metrics update paths work.
	m.Init()
	m.IncRequests("GET", "/test")
	m.RecordHTTPRequest("GET", "/test", 200, 0.001)
	m.ObserveDuration("GET", "/test", 0.001)
	m.ObserveResponseSize("GET", "/test", 123.0)

	m.UpdateDBConnections(2, "active")
	m.RecordDBQuery("SELECT", "users", 0.002, nil)

	// small wait to ensure nothing races in background (no background goroutines in metrics)
	time.Sleep(10 * time.Millisecond)
}
