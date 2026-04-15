package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"
)

// Level represents logging level
type Level string

const (
	DebugLevel Level = "debug"
	InfoLevel  Level = "info"
	WarnLevel  Level = "warn"
	ErrorLevel Level = "error"
)

// Config represents logger configuration
type Config struct {
	Level       string `json:"level" yaml:"level"`
	Format      string `json:"format" yaml:"format"` // "json" or "text"
	ServiceName string `json:"service_name" yaml:"service_name"`
	Environment string `json:"environment" yaml:"environment"` // "dev", "staging", "prod"
	AddSource   bool   `json:"add_source" yaml:"add_source"`   // add source:file:line
	}

// DefaultConfig returns default logger configuration
func DefaultConfig() Config {
	return Config{
		Level:       "info",
		Format:      "json",
		ServiceName: "go-service",
		Environment: "development",
		AddSource:   false,
	}
}

// Logger wrapper around slog.Logger
type Logger struct {
	*slog.Logger
	serviceName string
	environment string
}

// New creates a new logger instance
func New(config Config) *Logger {
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

	// Create handler based on format
	var handler slog.Handler
	switch config.Format {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	// Create logger with global attributes
	logger := slog.New(handler).
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

// NewWithWriter creates a new logger with custom writer
func NewWithWriter(config Config, writer io.Writer) *Logger {
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

	// Create handler based on format
	var handler slog.Handler
	switch config.Format {
	case "json":
		handler = slog.NewJSONHandler(writer, opts)
	default:
		handler = slog.NewTextHandler(writer, opts)
	}

	// Create logger with global attributes
	logger := slog.New(handler).
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

// With returns a new logger with additional attributes
func (l *Logger) With(args ...interface{}) *Logger {
	return &Logger{
		Logger:      l.Logger.With(args...),
		serviceName: l.serviceName,
		environment: l.environment,
	}
}

// WithGroup returns a new logger with a group name
func (l *Logger) WithGroup(name string) *Logger {
	return &Logger{
		Logger:      l.Logger.WithGroup(name),
		serviceName: l.serviceName,
		environment: l.environment,
	}
}

// WithComponent adds component field to logger
func (l *Logger) WithComponent(component string) *Logger {
	return l.With("component", component)
}

// WithUser adds user field to logger
func (l *Logger) WithUser(userID string) *Logger {
	return l.With("user_id", userID)
}

// WithRequestID adds request_id field to logger
func (l *Logger) WithRequestID(requestID string) *Logger {
	return l.With("request_id", requestID)
}

// WithTraceID adds trace_id field to logger
func (l *Logger) WithTraceID(traceID string) *Logger {
	return l.With("trace_id", traceID)
}

// WithError adds error to logger
func (l *Logger) WithError(err error) *Logger {
	if err != nil {
		return l.With("error", err.Error())
	}
	return l
}

// WithContext returns a logger with context
func (l *Logger) WithContext(ctx context.Context) *Logger {
	logger := l.Logger

	// Try to extract common context values
	if requestID := ctx.Value("request_id"); requestID != nil {
		if id, ok := requestID.(string); ok {
			logger = logger.With("request_id", id)
		}
	}

	if traceID := ctx.Value("trace_id"); traceID != nil {
		if id, ok := traceID.(string); ok {
			logger = logger.With("trace_id", id)
		}
	}

	if userID := ctx.Value("user_id"); userID != nil {
		if id, ok := userID.(string); ok {
			logger = logger.With("user_id", id)
		}
	}

	return &Logger{
		Logger:      logger,
		serviceName: l.serviceName,
		environment: l.environment,
	}
}

// Request logs HTTP request information
func (l *Logger) Request(ctx context.Context, msg string, args ...interface{}) {
	l.InfoContext(ctx, msg, args...)
}

// Business logs business operation information
func (l *Logger) Business(ctx context.Context, operation string, args ...interface{}) {
	allArgs := append([]interface{}{"operation", operation}, args...)
	l.InfoContext(ctx, operation, allArgs...)
}

// Performance logs performance metrics
func (l *Logger) Performance(ctx context.Context, operation string, duration time.Duration, args ...interface{}) {
	allArgs := append([]interface{}{
		"operation", operation,
		"duration_ms", duration.Milliseconds(),
	}, args...)
	l.InfoContext(ctx, "Performance metric", allArgs...)
}

// HTTPRequest logs HTTP request with common fields
func (l *Logger) HTTPRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration, args ...interface{}) {
	allArgs := append([]interface{}{
		"method", method,
		"path", path,
		"status_code", statusCode,
		"duration_ms", duration.Milliseconds(),
	}, args...)
	l.InfoContext(ctx, "HTTP request completed", allArgs...)
}

// DatabaseOperation logs database operation
func (l *Logger) DatabaseOperation(ctx context.Context, operation, table string, duration time.Duration, err error, args ...interface{}) {
	allArgs := append([]interface{}{
		"operation", operation,
		"table", table,
		"duration_ms", duration.Milliseconds(),
	}, args...)

	if err != nil {
		errorArgs := append([]interface{}{"error", err.Error()}, allArgs...)
		l.ErrorContext(ctx, "Database operation failed", errorArgs...)
	} else {
		l.InfoContext(ctx, "Database operation completed", allArgs...)
	}
}

// Fatal logs a fatal error and exits the application
func (l *Logger) Fatal(msg string, args ...interface{}) {
	l.Error(msg, args...)
	os.Exit(1)
}

// FatalWithContext logs a fatal message with context and exits
func (l *Logger) FatalWithContext(ctx context.Context, msg string, args ...interface{}) {
	l.WithContext(ctx).Error(msg, args...)
	os.Exit(1)
}

// ServiceName returns the service name
func (l *Logger) ServiceName() string {
	return l.serviceName
}

// Environment returns the environment
func (l *Logger) Environment() string {
	return l.environment
}

// GetLevel returns current log level
func (l *Logger) GetLevel() slog.Level {
	// slog doesn't expose level getter directly
	return slog.LevelInfo // Default fallback
}

// SetLevel sets new log level (returns new logger instance)
func (l *Logger) SetLevel(level string) *Logger {
	cfg := Config{
		Level:       level,
		Format:      "json", // Default format
		ServiceName: l.serviceName,
		Environment: l.environment,
		AddSource:   false,
	}
	return New(cfg)
}

// Global logger instance
var globalLogger *Logger

// InitGlobal initializes the global logger
func InitGlobal(config Config) {
	globalLogger = New(config)
	slog.SetDefault(globalLogger.Logger)
}

// GetGlobal returns the global logger instance
func GetGlobal() *Logger {
	if globalLogger == nil {
		globalLogger = New(DefaultConfig())
		slog.SetDefault(globalLogger.Logger)
	}
	return globalLogger
}

// Global convenience functions
func Debug(msg string, args ...interface{}) {
	GetGlobal().Debug(msg, args...)
}

func Info(msg string, args ...interface{}) {
	GetGlobal().Info(msg, args...)
}

func Warn(msg string, args ...interface{}) {
	GetGlobal().Warn(msg, args...)
}

func Error(msg string, args ...interface{}) {
	GetGlobal().Error(msg, args...)
}

func Fatal(msg string, args ...interface{}) {
	GetGlobal().Fatal(msg, args...)
}

// Context-aware global functions
func DebugContext(ctx context.Context, msg string, args ...interface{}) {
	GetGlobal().DebugContext(ctx, msg, args...)
}

func InfoContext(ctx context.Context, msg string, args ...interface{}) {
	GetGlobal().InfoContext(ctx, msg, args...)
}

func WarnContext(ctx context.Context, msg string, args ...interface{}) {
	GetGlobal().WarnContext(ctx, msg, args...)
}

func ErrorContext(ctx context.Context, msg string, args ...interface{}) {
	GetGlobal().ErrorContext(ctx, msg, args...)
}

// Convenience functions for common patterns
func Request(ctx context.Context, msg string, args ...interface{}) {
	GetGlobal().Request(ctx, msg, args...)
}

func Business(ctx context.Context, operation string, args ...interface{}) {
	GetGlobal().Business(ctx, operation, args...)
}

func Performance(ctx context.Context, operation string, duration time.Duration, args ...interface{}) {
	GetGlobal().Performance(ctx, operation, duration, args...)
}

func HTTPRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration, args ...interface{}) {
	GetGlobal().HTTPRequest(ctx, method, path, statusCode, duration, args...)
}

func DatabaseOperation(ctx context.Context, operation, table string, duration time.Duration, err error, args ...interface{}) {
	GetGlobal().DatabaseOperation(ctx, operation, table, duration, err, args...)
}

// WithContext adds context to the global logger
func WithContext(ctx context.Context) *Logger {
	if requestID := ctx.Value("request_id"); requestID != nil {
		if userID := ctx.Value("user_id"); userID != nil {
			return GetGlobal().With("request_id", requestID, "user_id", userID)
		}
		return GetGlobal().With("request_id", requestID)
	}
	return GetGlobal()
}

// WithRequest creates a logger with common request fields
func WithRequest(ctx context.Context, method, path string) *Logger {
	logger := GetGlobal().With("method", method, "path", path)

	if requestID := ctx.Value("request_id"); requestID != nil {
		logger = logger.With("request_id", requestID)
	}
	if userID := ctx.Value("user_id"); userID != nil {
		logger = logger.With("user_id", userID)
	}

	return logger
}

// WithComponent creates a logger with component field
func WithComponent(component string) *Logger {
	return GetGlobal().With("component", component)
}

// WithError adds error to logger
func WithError(err error) *Logger {
	if err != nil {
		return GetGlobal().With("error", err.Error())
	}
	return GetGlobal()
}
