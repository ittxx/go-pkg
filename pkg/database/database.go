package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ittxx/go-pkg/pkg/config"
	"github.com/ittxx/go-pkg/pkg/logger"
	"github.com/ittxx/go-pkg/pkg/metrics"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
	"github.com/jackc/pgx/v5/pgxpool"
)

// Database interface that combines both sql.DB and pgxpool.Pool
type Database interface {
	Health(ctx context.Context) error
	Close() error
	GetStats() interface{}
}

// PostgreSQL implementation using pgxpool
type PostgreSQL struct {
	pool    *pgxpool.Pool
	config  *config.DatabaseConfig
	logger  *logger.Logger
	metrics *metrics.Metrics
}

// SQL implementation for compatibility
type SQLDatabase struct {
	db      *sql.DB
	config  *config.DatabaseConfig
	logger  *logger.Logger
	metrics *metrics.Metrics
}

// Config holds database configuration
type Config struct {
	Host            string
	Port            int
	User            string
	Password        string
	Name            string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime int // in minutes
	UsePool         bool // use pgxpool vs sql.DB
}

// NewSQLDatabaseFromExisting wraps an existing *sql.DB with the SQLDatabase adapter
// so caller code can use the instrumented Exec/Query wrappers without reopening the connection.
// It will start the connection pool monitor if metrics are provided.
func NewSQLDatabaseFromExisting(db *sql.DB, cfg Config, m *metrics.Metrics, logger *logger.Logger) (*SQLDatabase, error) {
	if db == nil {
		return nil, fmt.Errorf("provided *sql.DB is nil")
	}

	wrapped := &SQLDatabase{
		db:     db,
		config: &config.DatabaseConfig{
			Host:        cfg.Host,
			Port:        cfg.Port,
			User:        cfg.User,
			Password:    cfg.Password,
			Database:    cfg.Name,
			SSLMode:     cfg.SSLMode,
			MaxConns:    int32(cfg.MaxOpenConns),
			MinConns:    int32(cfg.MaxIdleConns),
			MaxIdleTime: time.Duration(cfg.ConnMaxLifetime) * time.Minute,
		},
		logger:  logger,
		metrics: m,
	}

	// Start connection pool monitoring if metrics available
	if m != nil {
		go monitorConnectionPool(db, m, logger)
	}

	return wrapped, nil
}

// NewPostgreSQL creates a new PostgreSQL connection using pgxpool
func NewPostgreSQL(ctx context.Context, cfg *config.DatabaseConfig, logger *logger.Logger, m *metrics.Metrics) (*PostgreSQL, error) {
	// Build connection string
	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database,
		cfg.SSLMode,
	)

	// Create pool configuration
	poolConfig, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		// Avoid logging full connection string (may contain password/secret).
		// Log only safe, non-sensitive fields.
		logger.WithComponent("database").WithError(err).Error("failed to parse db pool config",
			"host", cfg.Host,
			"port", cfg.Port,
			"user", cfg.User,
			"database", cfg.Database,
		)
		return nil, err
	}

	// Configure pool parameters
	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConnIdleTime = cfg.MaxIdleTime
	poolConfig.ConnConfig.ConnectTimeout = 10 * time.Second

	// Create pool
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		logger.WithComponent("database").WithError(err).Error("failed to create db pool")
		return nil, err
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		logger.WithComponent("database").WithError(err).Error("failed to ping db")
		if m != nil {
			// Record connection error on startup ping
			m.RecordDBConnectionError("ping_error")
		}
		pool.Close()
		return nil, err
	}

	db := &PostgreSQL{
		pool:    pool,
		config:  cfg,
		logger:  logger,
		metrics: m,
	}

	logger.WithComponent("database").Info("database pool created",
		"host", cfg.Host,
		"port", cfg.Port,
		"database", cfg.Database,
		"max_conns", cfg.MaxConns,
		"min_conns", cfg.MinConns,
	)

	return db, nil
}

// NewSQLDatabase creates a new database connection using sql.DB
func NewSQLDatabase(cfg Config, m *metrics.Metrics, logger *logger.Logger) (*SQLDatabase, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host,
		cfg.Port,
		cfg.User,
		cfg.Password,
		cfg.Name,
		cfg.SSLMode,
	)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Minute)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		if m != nil {
			// Record connection error on startup ping
			m.RecordDBConnectionError("ping_error")
		}
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Start metrics monitoring if metrics is available
	if m != nil {
		go monitorConnectionPool(db, m, logger)
	}

	logger.WithComponent("database").Info("Database connected successfully",
		"host", cfg.Host,
		"port", cfg.Port,
		"database", cfg.Name,
		"max_open_conns", cfg.MaxOpenConns,
		"max_idle_conns", cfg.MaxIdleConns,
	)

	return &SQLDatabase{
		db:      db,
		config:  &config.DatabaseConfig{
			Host:        cfg.Host,
			Port:        cfg.Port,
			User:        cfg.User,
			Password:    cfg.Password,
			Database:    cfg.Name,
			SSLMode:     cfg.SSLMode,
			MaxConns:    int32(cfg.MaxOpenConns),
			MinConns:    int32(cfg.MaxIdleConns),
			MaxIdleTime: time.Duration(cfg.ConnMaxLifetime) * time.Minute,
		},
		logger:  logger,
		metrics: m,
	}, nil
}

// PostgreSQL methods implementation

// GetPool returns the underlying pgxpool.Pool
func (db *PostgreSQL) GetPool() *pgxpool.Pool {
	return db.pool
}

// Health checks the health of the database connection
func (db *PostgreSQL) Health(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// Stats returns pool statistics
func (db *PostgreSQL) GetStats() *pgxpool.Stat {
	return db.pool.Stat()
}

// UpdateMetrics updates metrics based on pool statistics
func (db *PostgreSQL) UpdateMetrics() {
	stats := db.GetStats()
	if db.metrics != nil {
		db.metrics.UpdateDBConnections(float64(stats.AcquiredConns()), "acquired")
		db.metrics.UpdateDBConnections(float64(stats.IdleConns()), "idle")
		db.metrics.UpdateDBConnections(float64(stats.MaxConns()), "max")
	}
}

// Close closes the database pool
func (db *PostgreSQL) Close() error {
	db.pool.Close()
	db.logger.WithComponent("database").Info("database pool closed")
	return nil
}

// StartConnectionMonitor starts monitoring connection pool statistics
func (db *PostgreSQL) StartConnectionMonitor(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				db.UpdateMetrics()
				stats := db.GetStats()
					if stats.AcquiredConns() > db.config.MaxConns {
					db.logger.WithComponent("database").Warn("database pool nearing connection limit",
						"acquired", stats.AcquiredConns(),
						"max", db.config.MaxConns,
					)
				}
			}
		}
	}()
}

// SQLDatabase methods implementation

// GetDB returns the underlying sql.DB
func (db *SQLDatabase) GetDB() *sql.DB {
	return db.db
}

// Health performs database health check
func (db *SQLDatabase) Health(ctx context.Context) error {
	if db.db == nil {
		return fmt.Errorf("database connection is nil")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.db.PingContext(ctx); err != nil {
		return fmt.Errorf("database health check failed: %w", err)
	}

	return nil
}

 // GetStats returns database statistics
func (db *SQLDatabase) GetStats() interface{} {
	if db.db == nil {
		return sql.DBStats{}
	}
	return db.db.Stats()
}

// Close safely closes the database connection
func (db *SQLDatabase) Close() error {
	if db.db != nil {
		if err := db.db.Close(); err != nil {
			db.logger.WithComponent("database").WithError(err).Error("failed to close database")
			return fmt.Errorf("failed to close database: %w", err)
		}
		db.logger.WithComponent("database").Info("database connection closed")
	}
	return nil
}

// monitorConnectionPool monitors database connection pool metrics
func monitorConnectionPool(db *sql.DB, metrics *metrics.Metrics, logger *logger.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if db == nil || metrics == nil {
			continue
		}

		stats := db.Stats()

		// Update metrics
		metrics.UpdateDBConnections(float64(stats.MaxOpenConnections), "max")
		metrics.UpdateDBConnections(float64(stats.OpenConnections), "open")
		metrics.UpdateDBConnections(float64(stats.Idle), "idle")
		metrics.UpdateDBConnections(float64(stats.InUse), "in_use")

		// Debug logging
		logger.WithComponent("database").Debug("Connection pool stats",
			"max", stats.MaxOpenConnections,
			"open", stats.OpenConnections,
			"in_use", stats.InUse,
			"idle", stats.Idle,
			"wait_count", stats.WaitCount,
			"wait_duration", stats.WaitDuration.String(),
		)
	}
}

/*
   Exec/Query wrappers for SQLDatabase to add automatic metrics recording.
   These provide a convenient instrumentation layer without changing call sites
   that use the higher-level helpers in this package. They use simple heuristics
   to extract SQL operation and table name for labeling; avoid high-cardinality
   labels in production by keeping this heuristic coarse.
*/

func (db *SQLDatabase) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	start := time.Now()
	res, err := db.db.ExecContext(ctx, query, args...)
	duration := time.Since(start).Seconds()

	op, table := parseSQLOpAndTable(query)
	if db.metrics != nil {
		db.metrics.RecordDBQuery(op, table, duration, err)
		if err != nil {
			db.metrics.RecordDBConnectionError("query_error")
		}
	}
	return res, err
}

func (db *SQLDatabase) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	start := time.Now()
	rows, err := db.db.QueryContext(ctx, query, args...)
	duration := time.Since(start).Seconds()

	op, table := parseSQLOpAndTable(query)
	if db.metrics != nil {
		db.metrics.RecordDBQuery(op, table, duration, err)
		if err != nil {
			db.metrics.RecordDBConnectionError("query_error")
		}
	}
	return rows, err
}

func (db *SQLDatabase) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	start := time.Now()
	row := db.db.QueryRowContext(ctx, query, args...)
	duration := time.Since(start).Seconds()

	op, table := parseSQLOpAndTable(query)
	if db.metrics != nil {
		// QueryRow defers error until Scan; record that a query occurred.
		db.metrics.RecordDBQuery(op, table, duration, nil)
	}
	return row
}

// parseSQLOpAndTable is a lightweight heuristic to extract SQL operation and a coarse table name.
// It intentionally avoids deep parsing to prevent high-cardinality labels.
func parseSQLOpAndTable(query string) (string, string) {
	q := strings.TrimSpace(strings.ToLower(query))
	if q == "" {
		return "unknown", "unknown"
	}
	fields := strings.Fields(q)
	op := "unknown"
	if len(fields) > 0 {
		op = fields[0]
	}

	table := "unknown"
	for i, f := range fields {
		switch f {
		case "from", "into", "update", "delete", "join":
			if i+1 < len(fields) {
				// strip common delimiters
				table = strings.Trim(fields[i+1], "\"`;")
				// remove any trailing punctuation like comma
				table = strings.Trim(table, ",")
				return op, table
			}
		}
	}
	return op, table
}

// HealthCheck performs database health check (utility function)
func HealthCheck(db Database) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return db.Health(ctx)
}

// Close database connection safely (utility function)
func Close(db Database, logger *logger.Logger) error {
	if err := db.Close(); err != nil {
		if logger != nil {
			logger.WithComponent("database").WithError(err).Error("failed to close database")
		}
		return fmt.Errorf("failed to close database: %w", err)
	}
	if logger != nil {
		logger.WithComponent("database").Info("database connection closed")
	}
	return nil
}
