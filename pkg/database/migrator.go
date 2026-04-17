package database

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ittxx/go-pkg/pkg/logger"
	"github.com/ittxx/go-pkg/pkg/metrics"
)

// Migration represents a single migration
type Migration interface {
	// Up executes the migration using a transaction
	Up(ctx context.Context, tx *sql.Tx) error
	// Down rolls back the migration using a transaction
	Down(ctx context.Context, tx *sql.Tx) error
	// Version returns the migration version
	Version() string
	// Description returns the migration description
	Description() string
}

// Migrator handles database migrations and recording applied versions
type Migrator struct {
	db        *sql.DB
	logger    *logger.Logger
	tableName string
}

// NewMigrator creates a new migrator instance
func NewMigrator(db *sql.DB, logger *logger.Logger) *Migrator {
	return &Migrator{
		db:        db,
		logger:    logger,
		tableName: "schema_migrations",
	}
}

// Run executes all pending migrations (Migration interface). It creates schema_migrations table,
// filters already applied versions and executes pending migrations inside transactions.
func (m *Migrator) Run(migrations []Migration) error {
	ctx := context.Background()

	// Create migrations table if it doesn't exist
	if err := m.createMigrationsTable(ctx); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get applied migrations
	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Filter pending migrations
	pending := m.filterPendingMigrations(migrations, applied)

	if len(pending) == 0 {
		m.logger.WithComponent("migrator").Info("No pending migrations")
		return nil
	}

	m.logger.WithComponent("migrator").Info("Running migrations", "count", len(pending))

	// Execute pending migrations
	for _, migration := range pending {
		if err := m.executeMigration(ctx, migration); err != nil {
			return fmt.Errorf("failed to execute migration %s: %w", migration.Version(), err)
		}
	}

	m.logger.WithComponent("migrator").Info("All migrations completed successfully")
	return nil
}

// createMigrationsTable creates the schema migrations table
func (m *Migrator) createMigrationsTable(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version VARCHAR(255) PRIMARY KEY,
			description TEXT,
			applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)
	`, m.tableName)

	start := time.Now()
	_, err := m.db.ExecContext(ctx, query)
	duration := time.Since(start).Seconds()
	// Record query metric for migrations table creation
	if metrics.DefaultMetrics != nil {
		metrics.DefaultMetrics.RecordDBQuery("create_table", "schema_migrations", duration, err)
	}
	return err
}

// getAppliedMigrations returns a map of applied migration versions
func (m *Migrator) getAppliedMigrations(ctx context.Context) (map[string]bool, error) {
	query := fmt.Sprintf("SELECT version FROM %s", m.tableName)
	start := time.Now()
	rows, err := m.db.QueryContext(ctx, query)
	duration := time.Since(start).Seconds()
	// Record query metric for reading applied migrations
	if metrics.DefaultMetrics != nil {
		metrics.DefaultMetrics.RecordDBQuery("select", "schema_migrations", duration, err)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}

	return applied, rows.Err()
}

// filterPendingMigrations filters migrations that haven't been applied yet
func (m *Migrator) filterPendingMigrations(migrations []Migration, applied map[string]bool) []Migration {
	var pending []Migration

	for _, migration := range migrations {
		if !applied[migration.Version()] {
			pending = append(pending, migration)
		}
	}

	// Sort migrations by version to ensure consistent order
	sort.Slice(pending, func(i, j int) bool {
		return pending[i].Version() < pending[j].Version()
	})

	return pending
}

// executeMigration executes a single migration
func (m *Migrator) executeMigration(ctx context.Context, migration Migration) error {
	start := time.Now()
	m.logger.WithComponent("migrator").Info("Running migration",
		"version", migration.Version(),
		"description", migration.Description())

	// Start transaction
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute migration
	if err := migration.Up(ctx, tx); err != nil {
		return fmt.Errorf("failed to execute migration up: %w", err)
	}

	// Record migration
	query := fmt.Sprintf(`
		INSERT INTO %s (version, description, applied_at) 
		VALUES ($1, $2, $3)
	`, m.tableName)

	startRec := time.Now()
	_, err = tx.ExecContext(ctx, query, migration.Version(), migration.Description(), time.Now())
	durationRec := time.Since(startRec).Seconds()
	if metrics.DefaultMetrics != nil {
		metrics.DefaultMetrics.RecordDBQuery("insert", "schema_migrations", durationRec, err)
	}
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration: %w", err)
	}

	duration := time.Since(start)
	m.logger.WithComponent("migrator").Info("Migration completed",
		"version", migration.Version(),
		"duration", duration.String())

	return nil
}

// Rollback rolls back the last migration
func (m *Migrator) Rollback(migrations []Migration) error {
	ctx := context.Background()

	// Get applied migrations
	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Find the last applied migration
	var lastMigration Migration
	for i := len(migrations) - 1; i >= 0; i-- {
		migration := migrations[i]
		if applied[migration.Version()] {
			lastMigration = migration
			break
		}
	}

	if lastMigration == nil {
		m.logger.WithComponent("migrator").Info("No migrations to rollback")
		return nil
	}

	m.logger.WithComponent("migrator").Info("Rolling back migration",
		"version", lastMigration.Version(),
		"description", lastMigration.Description())

	// Start transaction
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute rollback
	if err := lastMigration.Down(ctx, tx); err != nil {
		return fmt.Errorf("failed to execute migration down: %w", err)
	}

	// Remove migration record
	query := fmt.Sprintf("DELETE FROM %s WHERE version = $1", m.tableName)
	_, err = tx.ExecContext(ctx, query, lastMigration.Version())
	if err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit rollback: %w", err)
	}

	m.logger.WithComponent("migrator").Info("Migration rolled back successfully",
		"version", lastMigration.Version())

	return nil
}

// GetStatus returns the current migration status
func (m *Migrator) GetStatus(migrations []Migration) ([]MigrationStatus, error) {
	ctx := context.Background()

	// Ensure migrations table exists
	if err := m.createMigrationsTable(ctx); err != nil {
		return nil, fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get applied migrations
	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Build status list
	var status []MigrationStatus
	for _, migration := range migrations {
		status = append(status, MigrationStatus{
			Version:     migration.Version(),
			Description: migration.Description(),
			Applied:     applied[migration.Version()],
		})
	}

	return status, nil
}

// MigrationStatus represents the status of a migration
type MigrationStatus struct {
	Version     string `json:"version"`
	Description string `json:"description"`
	Applied     bool   `json:"applied"`
}

// SQLMigration represents a SQL file-based migration
type SQLMigration struct {
	version     string
	description string
	upSQL       string
	downSQL     string
}

// NewSQLMigration creates a new SQL migration
func NewSQLMigration(version, description, upSQL, downSQL string) *SQLMigration {
	return &SQLMigration{
		version:     version,
		description: description,
		upSQL:       upSQL,
		downSQL:     downSQL,
	}
}

// Version returns the migration version
func (m *SQLMigration) Version() string {
	return m.version
}

// Description returns the migration description
func (m *SQLMigration) Description() string {
	return m.description
}

// Up executes the migration
func (m *SQLMigration) Up(ctx context.Context, tx *sql.Tx) error {
	if m.upSQL == "" {
		return nil
	}
	start := time.Now()
	_, err := tx.ExecContext(ctx, m.upSQL)
	duration := time.Since(start).Seconds()
	// Record migration query (up)
	if metrics.DefaultMetrics != nil {
		metrics.DefaultMetrics.RecordDBQuery("migration_up", "sql_migration", duration, err)
	}
	return err
}

// Down rolls back the migration
func (m *SQLMigration) Down(ctx context.Context, tx *sql.Tx) error {
	if m.downSQL == "" {
		return nil
	}
	start := time.Now()
	_, err := tx.ExecContext(ctx, m.downSQL)
	duration := time.Since(start).Seconds()
	// Record migration query (down)
	if metrics.DefaultMetrics != nil {
		metrics.DefaultMetrics.RecordDBQuery("migration_down", "sql_migration", duration, err)
	}
	return err
}

// LoadMigrationsFromFS loads migrations from an embedded filesystem
func LoadMigrationsFromFS(fsys fs.FS, dir string) ([]Migration, error) {
	var migrations []Migration

	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}

		// Parse version from filename (e.g., "001_create_users.up.sql")
		parts := strings.Split(name, "_")
		if len(parts) < 2 {
			continue
		}

		version := parts[0]
		description := strings.TrimSuffix(strings.Join(parts[1:], "_"), ".up.sql")

		// Read up SQL
		upPath := pathJoin(dir, name)
		upSQL, err := fs.ReadFile(fsys, upPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read up migration %s: %w", upPath, err)
		}

		// Read down SQL
		downName := strings.Replace(name, ".up.sql", ".down.sql", 1)
		downPath := pathJoin(dir, downName)
		var downSQL []byte
		if _, err := fs.Stat(fsys, downPath); err == nil {
			downSQL, err = fs.ReadFile(fsys, downPath)
			if err != nil {
				return nil, fmt.Errorf("failed to read down migration %s: %w", downPath, err)
			}
		}

		migration := NewSQLMigration(
			version,
			description,
			string(upSQL),
			string(downSQL),
		)

		migrations = append(migrations, migration)
	}

	// Sort migrations by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version() < migrations[j].Version()
	})

	return migrations, nil
}

// RunDirectoryMigrations loads *.up.sql files from the provided directory (relative to CWD or absolute)
// and executes them against the provided *SQLDatabase inside a transaction.
//
// Files must be named like: 001_description.up.sql (optional corresponding .down.sql ignored here).
// Migrations are executed in lexical order by filename.
func RunDirectoryMigrations(ctx context.Context, db *SQLDatabase, migrationsDir string, logger *logger.Logger) error {
	if db == nil || db.GetDB() == nil {
		return fmt.Errorf("database is nil")
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		// If directory does not exist, treat as no migrations
		if os.IsNotExist(err) {
			logger.WithComponent("migrator").Info("migrations directory not found, skipping", "dir", migrationsDir)
			return nil
		}
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	var ups []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".up.sql") {
			ups = append(ups, name)
		}
	}

	if len(ups) == 0 {
		logger.WithComponent("migrator").Info("no migration files found", "dir", migrationsDir)
		return nil
	}

	// Ensure deterministic order
	sort.Strings(ups)

	for _, name := range ups {
		upPath := filepath.Join(migrationsDir, name)
		content, err := os.ReadFile(upPath)
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", upPath, err)
		}
		upSQL := strings.TrimSpace(string(content))
		if upSQL == "" {
			// nothing to run
			continue
		}

		// Start transaction
		tx, err := db.GetDB().BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return fmt.Errorf("failed to begin transaction for %s: %w", name, err)
		}

		start := time.Now()
		logger.WithComponent("migrator").Info("running migration", "file", name)

		_, err = tx.ExecContext(ctx, upSQL)
		// Use a single time measurement variable and derive both seconds for metrics
		// and a time.Duration for logging to avoid shadowing and type errors.
		dur := time.Since(start)
		seconds := dur.Seconds()
		// Record metric for running migration file
		if metrics.DefaultMetrics != nil {
			metrics.DefaultMetrics.RecordDBQuery("migration_file", name, seconds, err)
		}
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %s failed: %w", name, err)
		}

		// Commit
		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to commit migration %s: %w", name, err)
		}

		logger.WithComponent("migrator").Info("migration applied", "file", name, "duration", dur.String())
	}

	return nil
}

// helper path join that treats "./" prefix consistently for fs.ReadFile with fs.FS style dirs
func pathJoin(dir, name string) string {
	if dir == "" || dir == "." || strings.HasPrefix(dir, "./") {
		// fs.ReadFile expects paths without leading "./" for many fs.FS implementations
		return strings.TrimPrefix(filepath.Join(dir, name), "./")
	}
	return filepath.Join(dir, name)
}
