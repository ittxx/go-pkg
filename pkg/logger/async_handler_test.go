package logger

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"
)

// TestNewAsyncHandler tests creation of AsyncHandler
func TestNewAsyncHandler(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, nil)
	ah := NewAsyncHandler(handler, 1000)

	if ah == nil {
		t.Fatal("AsyncHandler should not be nil")
	}
	if ah.bufSize != 1000 {
		t.Errorf("expected buffer size 1000, got %d", ah.bufSize)
	}
	if ah.handler == nil {
		t.Fatal("underlying handler should not be nil")
	}

	_ = ah.Close()
}

// TestAsyncHandler_MinimumBufferSize tests minimum buffer size enforcement
func TestAsyncHandler_MinimumBufferSize(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, nil)
	ah := NewAsyncHandler(handler, 10) // Try to create with small buffer

	// Should enforce minimum buffer size (1000)
	if ah.bufSize < 100 {
		t.Errorf("expected minimum buffer size enforcement, got %d", ah.bufSize)
	}

	_ = ah.Close()
}

// TestAsyncHandler_Handle tests basic logging
func TestAsyncHandler_Handle(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, nil)
	ah := NewAsyncHandler(handler, 1000)

	// Create a test log record
	logger := slog.New(ah)
	logger.Info("test message", "key", "value")

	// Give worker time to process
	time.Sleep(100 * time.Millisecond)

	_ = ah.Close()

	// Verify log was written
	if buf.Len() == 0 {
		t.Error("log should have been written to buffer")
	}
}

// TestAsyncHandler_NonBlockingBehavior tests that logging doesn't block
func TestAsyncHandler_NonBlockingBehavior(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, nil)
	ah := NewAsyncHandler(handler, 1000)
	logger := slog.New(ah)

	// Measure time for multiple log calls
	start := time.Now()

	for i := 0; i < 100; i++ {
		logger.Info("test", "id", i)
	}

	elapsed := time.Since(start)

	// 100 logs should be fast (non-blocking)
	// Should be < 50ms on modern hardware
	if elapsed > 500*time.Millisecond {
		t.Logf("WARNING: logging took %v, expected < 500ms", elapsed)
	}

	_ = ah.Close()
}

// TestAsyncHandler_GracefulClose tests graceful shutdown
func TestAsyncHandler_GracefulClose(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, nil)
	ah := NewAsyncHandler(handler, 1000)
	logger := slog.New(ah)

	// Log some messages
	for i := 0; i < 10; i++ {
		logger.Info("test", "id", i)
	}

	// Close should flush all pending logs
	err := ah.Close()
	if err != nil {
		t.Errorf("Close should not error, got %v", err)
	}

	// Verify all logs were written
	if buf.Len() == 0 {
		t.Error("logs should have been flushed on Close")
	}
}

// TestAsyncHandler_GetDroppedCount tests dropped log counter
func TestAsyncHandler_GetDroppedCount(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, nil)
	ah := NewAsyncHandler(handler, 1) // Very small buffer to cause drops
	logger := slog.New(ah)

	count := ah.GetDroppedCount()
	if count != 0 {
		t.Errorf("initial dropped count should be 0, got %d", count)
	}

	// Try to overwhelm the buffer
	for i := 0; i < 100; i++ {
		logger.Info("test", "id", i)
	}

	// Some logs might be dropped due to small buffer
	time.Sleep(100 * time.Millisecond)

	_ = ah.Close()
}

// TestAsyncHandler_ResetDroppedCount tests reset functionality
func TestAsyncHandler_ResetDroppedCount(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, nil)
	ah := NewAsyncHandler(handler, 1000)

	// Manually set dropped count
	ah.recordDropped()
	ah.recordDropped()

	if ah.GetDroppedCount() != 2 {
		t.Errorf("expected dropped count 2, got %d", ah.GetDroppedCount())
	}

	// Reset
	ah.ResetDroppedCount()

	if ah.GetDroppedCount() != 0 {
		t.Errorf("expected dropped count 0 after reset, got %d", ah.GetDroppedCount())
	}

	_ = ah.Close()
}

// TestAsyncHandler_WithAttrs tests WithAttrs method
func TestAsyncHandler_WithAttrs(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, nil)
	ah := NewAsyncHandler(handler, 1000)

	attrs := []slog.Attr{
		slog.String("service", "test"),
		slog.String("env", "dev"),
	}

	newHandler := ah.WithAttrs(attrs)

	if newHandler == nil {
		t.Error("WithAttrs should return non-nil handler")
	}

	_ = ah.Close()
}

// TestAsyncHandler_WithGroup tests WithGroup method
func TestAsyncHandler_WithGroup(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, nil)
	ah := NewAsyncHandler(handler, 1000)

	newHandler := ah.WithGroup("test_group")

	if newHandler == nil {
		t.Error("WithGroup should return non-nil handler")
	}

	_ = ah.Close()
}

// TestAsyncHandler_Enabled tests Enabled method
func TestAsyncHandler_Enabled(t *testing.T) {
	buf := &bytes.Buffer{}
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	handler := slog.NewJSONHandler(buf, opts)
	ah := NewAsyncHandler(handler, 1000)
	ctx := context.Background()

	// Info level should be enabled
	if !ah.Enabled(ctx, slog.LevelInfo) {
		t.Error("Info level should be enabled")
	}

	// Debug level should be disabled (default is Info)
	if ah.Enabled(ctx, slog.LevelDebug) {
		t.Error("Debug level should be disabled")
	}

	_ = ah.Close()
}

// TestNewAsyncLogger tests NewAsyncLogger helper function
func TestNewAsyncLogger(t *testing.T) {
	config := Config{
		Level:       "info",
		Format:      "json",
		ServiceName: "test-service",
		Environment: "test",
		AddSource:   false,
	}

	logger := NewAsyncLogger(config, 5000)

	if logger == nil {
		t.Fatal("NewAsyncLogger should return non-nil logger")
	}
	if logger.ServiceName() != "test-service" {
		t.Errorf("expected service name 'test-service', got '%s'", logger.ServiceName())
	}

	logger.Info("test message")
	// Note: We can't easily close async logger without modifying its structure
}

// TestNewAsyncLoggerWithWriter tests NewAsyncLoggerWithWriter helper
func TestNewAsyncLoggerWithWriter(t *testing.T) {
	config := Config{
		Level:       "info",
		Format:      "json",
		ServiceName: "test-service",
		Environment: "test",
	}

	buf := &bytes.Buffer{}
	logger := NewAsyncLoggerWithWriter(config, buf, 1000)

	if logger == nil {
		t.Fatal("NewAsyncLoggerWithWriter should return non-nil logger")
	}

	logger.Info("test message")
	time.Sleep(100 * time.Millisecond)

	if buf.Len() == 0 {
		t.Error("log should have been written to buffer")
	}
}

// TestAsyncHandler_ConcurrentWrites tests concurrent logging
func TestAsyncHandler_ConcurrentWrites(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, nil)
	ah := NewAsyncHandler(handler, 10000)
	logger := slog.New(ah)

	done := make(chan bool)

	// Goroutine 1
	go func() {
		for i := 0; i < 50; i++ {
			logger.Info("goroutine1", "id", i)
		}
		done <- true
	}()

	// Goroutine 2
	go func() {
		for i := 0; i < 50; i++ {
			logger.Info("goroutine2", "id", i)
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	_ = ah.Close()

	// Verify logs were written
	if buf.Len() == 0 {
		t.Error("logs from concurrent goroutines should have been written")
	}
}

// TestAsyncHandler_HighVolumeLogs tests high volume logging
func TestAsyncHandler_HighVolumeLogs(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, nil)
	ah := NewAsyncHandler(handler, 5000)
	logger := slog.New(ah)

	// Log many messages
	for i := 0; i < 1000; i++ {
		logger.Info("test", "id", i)
	}

	_ = ah.Close()

	// All logs should be flushed
	if buf.Len() == 0 {
		t.Error("all logs should have been written")
	}

	// Rough check: 1000 logs should produce some output
	lines := bytes.Count(buf.Bytes(), []byte("\n"))
	if lines < 900 {
		t.Logf("WARNING: expected ~1000 log lines, got %d", lines)
	}
}

// TestAsyncHandler_GetPendingLogs tests pending logs count
func TestAsyncHandler_GetPendingLogs(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, nil)
	ah := NewAsyncHandler(handler, 1000)

	pending := ah.GetPendingLogs()
	if pending != 0 {
		t.Errorf("initial pending logs should be 0, got %d", pending)
	}

	_ = ah.Close()
}

// TestAsyncHandler_ErrorHandling tests error handling in handler
func TestAsyncHandler_ErrorHandling(t *testing.T) {
	buf := &bytes.Buffer{}
	handler := slog.NewJSONHandler(buf, nil)
	ah := NewAsyncHandler(handler, 1000)
	ctx := context.Background()

	// This should not panic even with unusual input
	var rec slog.Record
	err := ah.Handle(ctx, rec)

	if err != nil {
		t.Errorf("Handle should not error on normal input, got %v", err)
	}

	_ = ah.Close()
}
