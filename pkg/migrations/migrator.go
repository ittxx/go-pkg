package migrations

import (
	"context"
	"database/sql"
	"io/fs"

	"github.com/ittxx/go-pkg/pkg/database"
	"github.com/ittxx/go-pkg/pkg/logger"
)

// This package now delegates migration responsibilities to pkg/database.
// The original implementation was moved into pkg/database to consolidate DB-related code.
// The types and helpers below are thin aliases/wrappers so existing callers keep working.

// Type aliases to reuse implementations from pkg/database
type Migration = database.Migration
type Migrator = database.Migrator
type MigrationStatus = database.MigrationStatus
type SQLMigration = database.SQLMigration

// NewMigrator creates a new migrator backed by the database package implementation.
func NewMigrator(db *sql.DB, logger *logger.Logger) *Migrator {
	return database.NewMigrator(db, logger)
}

// LoadMigrationsFromFS delegates to database.LoadMigrationsFromFS.
func LoadMigrationsFromFS(fsys fs.FS, dir string) ([]Migration, error) {
	return database.LoadMigrationsFromFS(fsys, dir)
}

// RunDirectoryMigrations delegates to database.RunDirectoryMigrations.
// Provided for backwards compatibility with examples that called pkg/migrations helper.
func RunDirectoryMigrations(ctx context.Context, db *database.SQLDatabase, migrationsDir string, logger *logger.Logger) error {
	return database.RunDirectoryMigrations(ctx, db, migrationsDir, logger)
}
