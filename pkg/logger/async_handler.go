package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"
)

// AsyncHandler implements slog.Handler with non-blocking async writes
// Prevents blocking request processing due to disk I/O latency
type AsyncHandler struct {
	handler     slog.Handler
	ch          chan *slog.Record
	bufSize     int
	closeOnce   sync.Once
	wg          sync.WaitGroup
	dropMetrics struct {
		mu       sync.Mutex
		dropped  int64
		lastWarn time.Time
	}
}

// NewAsyncHandler creates a new async logging handler
// handler: the underlying slog.Handler to write logs
// bufSize: buffer size for the async channel (recommended: 1000-10000)
// Example: NewAsyncHandler(slog.NewJSONHandler(os.Stdout, opts), 5000)
func NewAsyncHandler(handler slog.Handler, bufSize int) *AsyncHandler {
	if bufSize < 100 {
		bufSize = 1000 // Enforce minimum buffer size
	}

	ah := &AsyncHandler{
		handler: handler,
		ch:      make(chan *slog.Record, bufSize),
		bufSize: bufSize,
	}

	// Start background worker goroutine
	ah.wg.Add(1)
	go ah.worker()

	return ah
}

// Handle implements slog.Handler.Handle with non-blocking send
func (ah *AsyncHandler) Handle(ctx context.Context, r slog.Record) error {
	// Create a copy of the record to avoid issues with mutable state
	recordCopy := r

	// Non-blocking send with timeout
	select {
	case ah.ch <- &recordCopy:
		return nil
	case <-time.After(100 * time.Millisecond):
		// Timeout - log is dropped to prevent blocking request processing
		// In production, consider alternative strategies (e.g., buffering, dropping older logs)
		ah.recordDropped()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// worker runs in background and writes logs to the underlying handler
func (ah *AsyncHandler) worker() {
	defer ah.wg.Done()

	for r := range ah.ch {
		if r == nil {
			break
		}

		// Write to underlying handler - this can be slow (disk I/O)
		// but doesn't block request processing
		if err := ah.handler.Handle(context.Background(), *r); err != nil {
			// Log error to stderr to avoid infinite recursion
			fmt.Fprintf(os.Stderr, "async logger: failed to handle record: %v\n", err)
		}
	}

	// Flush remaining records before closing
	ah.flush()
}

// flush writes all remaining records in the channel
func (ah *AsyncHandler) flush() {
	for {
		select {
		case r := <-ah.ch:
			if r == nil {
				return
			}
			if err := ah.handler.Handle(context.Background(), *r); err != nil {
				fmt.Fprintf(os.Stderr, "async logger flush: failed to handle record: %v\n", err)
			}
		default:
			return
		}
	}
}

// Close gracefully shuts down the async handler
// Waits for all pending logs to be written before returning
func (ah *AsyncHandler) Close() error {
	var err error

	ah.closeOnce.Do(func() {
		// Signal worker to stop reading
		close(ah.ch)

		// Wait for worker to finish
		ah.wg.Wait()

		// Close underlying handler if it supports Close()
		if closer, ok := ah.handler.(interface{ Close() error }); ok {
			err = closer.Close()
		}
	})

	return err
}

// WithAttrs implements slog.Handler.WithAttrs
func (ah *AsyncHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &AsyncHandler{
		handler: ah.handler.WithAttrs(attrs),
		ch:      ah.ch,
		bufSize: ah.bufSize,
	}
}

// WithGroup implements slog.Handler.WithGroup
func (ah *AsyncHandler) WithGroup(name string) slog.Handler {
	return &AsyncHandler{
		handler: ah.handler.WithGroup(name),
		ch:      ah.ch,
		bufSize: ah.bufSize,
	}
}

// Enabled implements slog.Handler.Enabled
func (ah *AsyncHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return ah.handler.Enabled(ctx, level)
}

// recordDropped increments dropped log counter and warns periodically
func (ah *AsyncHandler) recordDropped() {
	ah.dropMetrics.mu.Lock()
	defer ah.dropMetrics.mu.Unlock()

	ah.dropMetrics.dropped++

	// Warn every 100 dropped logs or once per minute
	now := time.Now()
	if ah.dropMetrics.dropped%100 == 0 ||
		(ah.dropMetrics.lastWarn.IsZero() || now.Sub(ah.dropMetrics.lastWarn) > time.Minute) {
		fmt.Fprintf(os.Stderr, "async logger: %d logs dropped due to buffer timeout\n", ah.dropMetrics.dropped)
		ah.dropMetrics.lastWarn = now
	}
}

// GetDroppedCount returns the number of logs dropped
func (ah *AsyncHandler) GetDroppedCount() int64 {
	ah.dropMetrics.mu.Lock()
	defer ah.dropMetrics.mu.Unlock()
	return ah.dropMetrics.dropped
}

// ResetDroppedCount resets the dropped log counter
func (ah *AsyncHandler) ResetDroppedCount() {
	ah.dropMetrics.mu.Lock()
	defer ah.dropMetrics.mu.Unlock()
	ah.dropMetrics.dropped = 0
}

// GetPendingLogs returns the number of logs pending in the queue
func (ah *AsyncHandler) GetPendingLogs() int {
	return len(ah.ch)
}

// ============================================================
// Helper function for creating async-enabled loggers
// ============================================================

// NewAsyncLogger creates a Logger with async logging enabled
// This prevents disk I/O from blocking request processing
func NewAsyncLogger(config Config, bufSize int) *Logger {
	// Set default values
	if config.Level == "" {
		config.Level = "info"
	}
	if config.Format == "" {
		config.Format = "json"
	}
	if config.Environment == "" {
		config.Environment = "dev"
	}

	// Parse log level
	var level slog.Level
	switch config.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// Create handler options
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: config.AddSource,
	}

	// Create base handler
	var baseHandler slog.Handler
	switch config.Format {
	case "json":
		baseHandler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		baseHandler = slog.NewTextHandler(os.Stdout, opts)
	}

	// Wrap with async handler
	asyncHandler := NewAsyncHandler(baseHandler, bufSize)

	// Create logger with global attributes
	logger := slog.New(asyncHandler).
		With(
			"service", config.ServiceName,
			"env", config.Environment,
		)

	return &Logger{
		Logger:      logger,
		serviceName: config.ServiceName,
		environment: config.Environment,
	}
}

// NewAsyncLoggerWithWriter creates a Logger with async logging and custom writer
func NewAsyncLoggerWithWriter(config Config, writer io.Writer, bufSize int) *Logger {
	// Set default values
	if config.Level == "" {
		config.Level = "info"
	}
	if config.Format == "" {
		config.Format = "json"
	}
	if config.Environment == "" {
		config.Environment = "dev"
	}

	// Parse log level
	var level slog.Level
	switch config.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// Create handler options
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: config.AddSource,
	}

	// Create base handler
	var baseHandler slog.Handler
	switch config.Format {
	case "json":
		baseHandler = slog.NewJSONHandler(writer, opts)
	default:
		baseHandler = slog.NewTextHandler(writer, opts)
	}

	// Wrap with async handler
	asyncHandler := NewAsyncHandler(baseHandler, bufSize)

	// Create logger with global attributes
	logger := slog.New(asyncHandler).
		With(
			"service", config.ServiceName,
			"env", config.Environment,
		)

	return &Logger{
		Logger:      logger,
		serviceName: config.ServiceName,
		environment: config.Environment,
	}
}
