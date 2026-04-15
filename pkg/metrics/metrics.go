package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds core Prometheus metrics (always available)
type Metrics struct {
	// Application metrics (core metrics)
	appStartTime      prometheus.Gauge
	activeConnections *prometheus.GaugeVec

	// Optional modules
	httpMetrics  *HTTPMetrics     // Optional HTTP metrics
	dbMetrics    *DatabaseMetrics // Optional database metrics
	customMetrics map[string]interface{} // Custom metrics
	registry    *prometheus.Registry // Custom registry
}

// HTTPMetrics holds HTTP-specific metrics (optional)
type HTTPMetrics struct {
	httpRequestsTotal    *prometheus.CounterVec
	httpRequestDuration  *prometheus.HistogramVec
	httpResponseSize     *prometheus.HistogramVec
	httpRequestsInFlight *prometheus.GaugeVec
}

// DatabaseMetrics holds database-specific metrics (optional)
type DatabaseMetrics struct {
	// Database metrics with labels
	dbConnections      *prometheus.GaugeVec
	dbQueriesTotal     *prometheus.CounterVec
	dbQueryDuration    *prometheus.HistogramVec
	dbConnectionErrors *prometheus.CounterVec
}

// NewMetrics creates a new core Metrics instance (basic metrics only)
func NewMetrics() *Metrics {
	// Create a new registry for this instance to avoid conflicts
	registry := prometheus.NewRegistry()
	
	// Create metrics
	appStartTime := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "app_start_time_unix",
		Help: "Application start time (Unix timestamp)",
	})
	
	activeConnections := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "active_connections_total",
			Help: "Number of active connections",
		},
		[]string{"type"}, // websocket, http, etc.
	)
	
	// Register metrics
	registry.MustRegister(appStartTime)
	registry.MustRegister(activeConnections)
	
	return &Metrics{
		appStartTime:      appStartTime,
		activeConnections: activeConnections,
		customMetrics:     make(map[string]interface{}),
		registry:          registry,
	}
}

// WithHTTPMetrics enables HTTP metrics module
func (m *Metrics) WithHTTPMetrics() *Metrics {
	if m.httpMetrics == nil {
		httpRequestsTotal := prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "endpoint", "status_code"},
		)
		httpRequestDuration := prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "HTTP request duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"method", "endpoint"},
		)
		httpResponseSize := prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_response_size_bytes",
				Help:    "HTTP response size in bytes",
				Buckets: []float64{100, 500, 1000, 5000, 10000, 50000, 100000, 500000, 1000000},
			},
			[]string{"method", "endpoint"},
		)
		httpRequestsInFlight := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "http_requests_in_flight",
				Help: "Number of currently active HTTP requests",
			},
			[]string{"method"},
		)

		// Register metrics in the instance registry (fallback to default registry if nil)
		if m.registry != nil {
			m.registry.MustRegister(httpRequestsTotal, httpRequestDuration, httpResponseSize, httpRequestsInFlight)
		} else {
			prometheus.MustRegister(httpRequestsTotal, httpRequestDuration, httpResponseSize, httpRequestsInFlight)
		}

		m.httpMetrics = &HTTPMetrics{
			httpRequestsTotal:    httpRequestsTotal,
			httpRequestDuration:  httpRequestDuration,
			httpResponseSize:     httpResponseSize,
			httpRequestsInFlight: httpRequestsInFlight,
		}
	}
	return m
}

// GetHTTPMetrics returns HTTP metrics module (nil if not enabled)
func (m *Metrics) GetHTTPMetrics() *HTTPMetrics {
	return m.httpMetrics
}

// HasHTTPMetrics checks if HTTP metrics are enabled
func (m *Metrics) HasHTTPMetrics() bool {
	return m.httpMetrics != nil
}

// WithDatabaseMetrics enables database metrics module
func (m *Metrics) WithDatabaseMetrics() *Metrics {
	if m.dbMetrics == nil {
		dbConnections := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "db_connections",
				Help: "Database connection pool metrics",
			},
			[]string{"state"}, // max, active, idle, in_use, open
		)
		dbQueriesTotal := prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "db_queries_total",
				Help: "Total number of database queries",
			},
			[]string{"operation", "table", "status"}, // select, insert, update, delete, success, error
		)
		dbQueryDuration := prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "db_query_duration_seconds",
				Help:    "Database query duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"operation", "table"},
		)
		dbConnectionErrors := prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "db_connection_errors_total",
				Help: "Total number of database connection errors",
			},
			[]string{"error_type"}, // timeout, connection_refused, etc.
		)

		// Register metrics in the instance registry (fallback to default registry if nil)
		if m.registry != nil {
			m.registry.MustRegister(dbConnections, dbQueriesTotal, dbQueryDuration, dbConnectionErrors)
		} else {
			prometheus.MustRegister(dbConnections, dbQueriesTotal, dbQueryDuration, dbConnectionErrors)
		}

		m.dbMetrics = &DatabaseMetrics{
			dbConnections:      dbConnections,
			dbQueriesTotal:     dbQueriesTotal,
			dbQueryDuration:    dbQueryDuration,
			dbConnectionErrors: dbConnectionErrors,
		}
	}
	return m
}

// GetDatabaseMetrics returns database metrics module (nil if not enabled)
func (m *Metrics) GetDatabaseMetrics() *DatabaseMetrics {
	return m.dbMetrics
}

// HasDatabaseMetrics checks if database metrics are enabled
func (m *Metrics) HasDatabaseMetrics() bool {
	return m.dbMetrics != nil
}

// Init initializes metrics with current timestamp
func (m *Metrics) Init() {
	m.appStartTime.SetToCurrentTime()
}

// Handler returns the Prometheus metrics HTTP handler
func (m *Metrics) Handler() http.Handler {
	if m.registry != nil {
		return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
	}
	return promhttp.Handler()
}

// =====================================
// Core HTTP metrics methods
// =====================================

// RecordHTTPRequest records HTTP request metrics
func (m *Metrics) RecordHTTPRequest(method, endpoint string, statusCode int, duration float64) {
	if m.httpMetrics != nil {
		m.httpMetrics.httpRequestsTotal.WithLabelValues(method, endpoint, formatStatusCode(statusCode)).Inc()
		m.httpMetrics.httpRequestDuration.WithLabelValues(method, endpoint).Observe(duration)
		m.httpMetrics.httpRequestsInFlight.WithLabelValues(method).Dec() // Decrement when request completes
	}
}

// IncRequests increments HTTP request counter (for start of request)
func (m *Metrics) IncRequests(method, endpoint string) {
	if m.httpMetrics != nil {
		m.httpMetrics.httpRequestsInFlight.WithLabelValues(method).Inc()
	}
}

// ObserveDuration observes HTTP request duration
func (m *Metrics) ObserveDuration(method, endpoint string, duration float64) {
	if m.httpMetrics != nil {
		m.httpMetrics.httpRequestDuration.WithLabelValues(method, endpoint).Observe(duration)
	}
}

// ObserveResponseSize observes HTTP response size
func (m *Metrics) ObserveResponseSize(method, endpoint string, size float64) {
	if m.httpMetrics != nil {
		m.httpMetrics.httpResponseSize.WithLabelValues(method, endpoint).Observe(size)
	}
}

// =====================================
// Application metrics methods
// =====================================

// UpdateActiveConnections updates active connections count
func (m *Metrics) UpdateActiveConnections(count float64, connType string) {
	m.activeConnections.WithLabelValues(connType).Set(count)
}

// =====================================
// Database metrics methods (optional)
// =====================================

// UpdateDBConnections updates database connection metrics (requires database metrics)
func (m *Metrics) UpdateDBConnections(count float64, state string) {
	if m.dbMetrics != nil {
		m.dbMetrics.dbConnections.WithLabelValues(state).Set(count)
	}
}

// RecordDBQuery records database query metrics (requires database metrics)
func (m *Metrics) RecordDBQuery(operation, table string, duration float64, err error) {
	if m.dbMetrics != nil {
		status := "success"
		if err != nil {
			status = "error"
		}
		m.dbMetrics.dbQueriesTotal.WithLabelValues(operation, table, status).Inc()
		m.dbMetrics.dbQueryDuration.WithLabelValues(operation, table).Observe(duration)
	}
}

// RecordDBConnectionError records database connection error (requires database metrics)
func (m *Metrics) RecordDBConnectionError(errorType string) {
	if m.dbMetrics != nil {
		m.dbMetrics.dbConnectionErrors.WithLabelValues(errorType).Inc()
	}
}

// =====================================
// DatabaseMetrics methods (direct access)
// =====================================

// UpdateDBConnections updates database connection metrics (direct method)
func (dm *DatabaseMetrics) UpdateConnections(count float64, state string) {
	if dm != nil {
		dm.dbConnections.WithLabelValues(state).Set(count)
	}
}

// RecordQuery records database query metrics (direct method)
func (dm *DatabaseMetrics) RecordQuery(operation, table string, duration float64, err error) {
	if dm != nil {
		status := "success"
		if err != nil {
			status = "error"
		}
		dm.dbQueriesTotal.WithLabelValues(operation, table, status).Inc()
		dm.dbQueryDuration.WithLabelValues(operation, table).Observe(duration)
	}
}

// RecordConnectionError records database connection error (direct method)
func (dm *DatabaseMetrics) RecordConnectionError(errorType string) {
	if dm != nil {
		dm.dbConnectionErrors.WithLabelValues(errorType).Inc()
	}
}

// =====================================
// Legacy compatibility methods (for test-ai compatibility)
// =====================================

// RecordDBQueryDuration records database query duration (legacy method)
func (m *Metrics) RecordDBQueryDuration(duration float64, err error) {
	if m.dbMetrics != nil {
		status := "success"
		if err != nil {
			status = "error"
			m.dbMetrics.dbConnectionErrors.WithLabelValues("query_error").Inc()
		}
		m.dbMetrics.dbQueriesTotal.WithLabelValues("query", "unknown", status).Inc()
		m.dbMetrics.dbQueryDuration.WithLabelValues("query", "unknown").Observe(duration)
	}
}

// UpdateDBConnectionStats updates database connection stats (legacy method)
func (m *Metrics) UpdateDBConnectionStats(active, idle int32) {
	if m.dbMetrics != nil {
		m.dbMetrics.dbConnections.WithLabelValues("active").Set(float64(active))
		m.dbMetrics.dbConnections.WithLabelValues("idle").Set(float64(idle))
	}
}

// =====================================
// Utility functions
// =====================================

// formatStatusCode formats status code for Prometheus labels
func formatStatusCode(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return "2xx"
	case statusCode >= 300 && statusCode < 400:
		return "3xx"
	case statusCode >= 400 && statusCode < 500:
		return "4xx"
	case statusCode >= 500:
		return "5xx"
	default:
		return "unknown"
	}
}

// =====================================
// Middleware helper functions
// =====================================

// MetricsMiddleware creates HTTP middleware for metrics collection
func MetricsMiddleware(m *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Increment request counter
			m.IncRequests(r.Method, r.URL.Path)

			// Wrap response writer to capture status code and size
			rw := &responseWriter{ResponseWriter: w, statusCode: 200}

			// Serve the request
			next.ServeHTTP(rw, r)

			// Record metrics
			duration := time.Since(start).Seconds()
			m.RecordHTTPRequest(r.Method, r.URL.Path, rw.statusCode, duration)
			m.ObserveResponseSize(r.Method, r.URL.Path, float64(rw.size))
		})
	}
}

// responseWriter is a wrapper around http.ResponseWriter to capture status code and response size
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.size += n
	return n, err
}

// HealthCheckMiddleware creates middleware for health check endpoints
func HealthCheckMiddleware(healthCheck func() error) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				if err := healthCheck(); err != nil {
					w.WriteHeader(http.StatusServiceUnavailable)
					w.Write([]byte(`{"status":"unhealthy","error":"` + err.Error() + `"}`))
					return
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status":"healthy"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// =====================================
// Convenience functions for different use cases
// =====================================

// NewHTTPMetrics creates metrics instance with HTTP metrics enabled
func NewHTTPMetrics() *Metrics {
	return NewMetrics().WithHTTPMetrics()
}

// NewFullMetrics creates metrics instance with both HTTP and database metrics
func NewFullMetrics() *Metrics {
	return NewMetrics().WithHTTPMetrics().WithDatabaseMetrics()
}

// =====================================
// Global metrics instance for backward compatibility
// =====================================

var DefaultMetrics *Metrics

func init() {
	DefaultMetrics = NewMetrics() // Default to basic metrics only
}

// InitDefaultMetrics initializes default metrics
func InitDefaultMetrics() {
	DefaultMetrics.Init()
}

// EnableDefaultDatabaseMetrics enables database metrics on default instance
func EnableDefaultDatabaseMetrics() {
	DefaultMetrics.WithDatabaseMetrics()
}

// Global convenience functions
func RecordDefaultHTTPRequest(method, endpoint string, statusCode int, duration float64) {
	DefaultMetrics.RecordHTTPRequest(method, endpoint, statusCode, duration)
}

func UpdateDefaultDBConnections(count float64, state string) {
	DefaultMetrics.UpdateDBConnections(count, state)
}

func RecordDefaultDBQuery(operation, table string, duration float64, err error) {
	DefaultMetrics.RecordDBQuery(operation, table, duration, err)
}
