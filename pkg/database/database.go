package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go-skeleton/pkg/config"
	"go-skeleton/pkg/logger"
	"go-skeleton/pkg/metrics"

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

// NewPostgreSQL creates a new PostgreSQL connection using pgxpool
func NewPostgreSQL(ctx context.Context, cfg *config.DatabaseConfig, log *logger.Logger, m *metrics.Metrics) (*PostgreSQL, error) {
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
		log.WithComponent("database").Error("ошибка парсинга конфигурации БД", err, "connStr", connStr)
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
		log.WithComponent("database").Error("ошибка создания пула соединений БД", err)
		return nil, err
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		log.WithComponent("database").Error("ошибка пинга БД", err)
		pool.Close()
		return nil, err
	}

	db := &PostgreSQL{
		pool:    pool,
		config:  cfg,
		logger:  log,
		metrics: m,
	}

	log.WithComponent("database").Info("пул соединений БД создан",
		"host", cfg.Host,
		"port", cfg.Port,
		"database", cfg.Database,
		"maxConns", cfg.MaxConns,
		"minConns", cfg.MinConns,
	)

	return db, nil
}

// NewSQLDatabase creates a new database connection using sql.DB
func NewSQLDatabase(cfg Config, m *metrics.Metrics, log *logger.Logger) (*SQLDatabase, error) {
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
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Start metrics monitoring if metrics is available
	if m != nil {
		go monitorConnectionPool(db, m, log)
	}

	log.WithComponent("database").Info("Database connected successfully",
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
		logger:  log,
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
	db.logger.WithComponent("database").Info("пул соединений БД закрыт")
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
					db.logger.WithComponent("database").Warn("пул соединений БД близко к лимиту",
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
func (db *SQLDatabase) GetStats() sql.DBStats {
	if db.db == nil {
		return sql.DBStats{}
	}
	return db.db.Stats()
}

// Close safely closes the database connection
func (db *SQLDatabase) Close() error {
	if db.db != nil {
		if err := db.db.Close(); err != nil {
			db.logger.WithComponent("database").WithError(err).Error("Failed to close database")
			return fmt.Errorf("failed to close database: %w", err)
		}
		db.logger.WithComponent("database").Info("Database connection closed")
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
			logger.WithComponent("database").WithError(err).Error("Failed to close database")
		}
		return fmt.Errorf("failed to close database: %w", err)
	}
	if logger != nil {
		logger.WithComponent("database").Info("Database connection closed")
	}
	return nil
}
