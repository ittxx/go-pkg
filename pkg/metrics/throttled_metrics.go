package metrics

import (
	"sync"
	"time"
)

// ThrottledMetrics wraps Metrics with rate limiting to prevent performance degradation
// and memory leaks on high-frequency updates
type ThrottledMetrics struct {
	metrics *Metrics

	// Tracking last update times for each metric type
	lastDBConnUpdate   time.Time
	lastDBQueryUpdate  time.Time
	lastHTTPUpdate     time.Time
	lastActiveConnUpd  time.Time

	// Configuration
	updateInterval time.Duration
	mu             sync.Mutex
}

// NewThrottledMetrics creates a new throttled metrics wrapper
// interval: minimum time between metric updates (recommended: 1-5 seconds)
// Example: NewThrottledMetrics(metrics, 1*time.Second)
func NewThrottledMetrics(m *Metrics, interval time.Duration) *ThrottledMetrics {
	if interval < 100*time.Millisecond {
		interval = 1 * time.Second // Enforce minimum interval
	}
	return &ThrottledMetrics{
		metrics:        m,
		updateInterval: interval,
	}
}

// UpdateDBConnections updates database connections metric with rate limiting
func (tm *ThrottledMetrics) UpdateDBConnections(count float64, state string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if time.Since(tm.lastDBConnUpdate) < tm.updateInterval {
		return
	}

	tm.metrics.UpdateDBConnections(count, state)
	tm.lastDBConnUpdate = time.Now()
}

// RecordDBQuery records database query metric with rate limiting
func (tm *ThrottledMetrics) RecordDBQuery(operation, table string, duration float64, err error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if time.Since(tm.lastDBQueryUpdate) < tm.updateInterval {
		return
	}

	tm.metrics.RecordDBQuery(operation, table, duration, err)
	tm.lastDBQueryUpdate = time.Now()
}

// UpdateActiveConnections updates active connections metric with rate limiting
func (tm *ThrottledMetrics) UpdateActiveConnections(count float64, connType string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if time.Since(tm.lastActiveConnUpd) < tm.updateInterval {
		return
	}

	tm.metrics.UpdateActiveConnections(count, connType)
	tm.lastActiveConnUpd = time.Now()
}

// RecordHTTPRequest records HTTP request (NOT throttled - counters are cumulative)
func (tm *ThrottledMetrics) RecordHTTPRequest(method, endpoint string, statusCode int, duration float64) {
	tm.metrics.RecordHTTPRequest(method, endpoint, statusCode, duration)
}

// IncRequests increments HTTP request counter (NOT throttled)
func (tm *ThrottledMetrics) IncRequests(method, endpoint string) {
	tm.metrics.IncRequests(method, endpoint)
}

// ObserveDuration observes HTTP request duration (NOT throttled - histograms aggregate)
func (tm *ThrottledMetrics) ObserveDuration(method, endpoint string, duration float64) {
	tm.metrics.ObserveDuration(method, endpoint, duration)
}

// ObserveResponseSize observes HTTP response size (NOT throttled)
func (tm *ThrottledMetrics) ObserveResponseSize(method, endpoint string, size float64) {
	tm.metrics.ObserveResponseSize(method, endpoint, size)
}

// GetMetrics returns the underlying Metrics instance
func (tm *ThrottledMetrics) GetMetrics() *Metrics {
	return tm.metrics
}

// GetUpdateInterval returns the current update interval
func (tm *ThrottledMetrics) GetUpdateInterval() time.Duration {
	return tm.updateInterval
}

// SetUpdateInterval changes the update interval
func (tm *ThrottledMetrics) SetUpdateInterval(interval time.Duration) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}
	tm.updateInterval = interval
}

// WithHTTPMetrics enables HTTP metrics on the underlying Metrics instance
func (tm *ThrottledMetrics) WithHTTPMetrics() *ThrottledMetrics {
	tm.metrics.WithHTTPMetrics()
	return tm
}

// WithDatabaseMetrics enables database metrics on the underlying Metrics instance
func (tm *ThrottledMetrics) WithDatabaseMetrics() *ThrottledMetrics {
	tm.metrics.WithDatabaseMetrics()
	return tm
}

// HasHTTPMetrics checks if HTTP metrics are enabled
func (tm *ThrottledMetrics) HasHTTPMetrics() bool {
	return tm.metrics.HasHTTPMetrics()
}

// HasDatabaseMetrics checks if database metrics are enabled
func (tm *ThrottledMetrics) HasDatabaseMetrics() bool {
	return tm.metrics.HasDatabaseMetrics()
}
