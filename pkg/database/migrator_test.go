package database

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)


func TestRunDirectoryMigrations_Success(t *testing.T) {
	tmp := t.TempDir()
	upName := "001_create_table.up.sql"
	upPath := filepath.Join(tmp, upName)
	if err := os.WriteFile(upPath, []byte("CREATE TABLE test_table (id INT);"), 0644); err != nil {
		t.Fatalf("failed to write up migration: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	// expect transaction begin, exec of the up SQL, and commit
	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE test_table").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	l, _ := makeLoggerBuffer()
	sqlDB := &SQLDatabase{db: db, logger: l}

	if err := RunDirectoryMigrations(context.Background(), sqlDB, tmp, l); err != nil {
		t.Fatalf("RunDirectoryMigrations failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestRunDirectoryMigrations_EmptyDir_NoError(t *testing.T) {
	// pass a non-existent directory
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	l, _ := makeLoggerBuffer()
	sqlDB := &SQLDatabase{db: db, logger: l}

	if err := RunDirectoryMigrations(context.Background(), sqlDB, "./path/does/not/exist", l); err != nil {
		t.Fatalf("expected no error for missing migrations dir, got: %v", err)
	}
}

func TestRunDirectoryMigrations_ExecError_Rollback(t *testing.T) {
	tmp := t.TempDir()
	upName := "001_bad.up.sql"
	upPath := filepath.Join(tmp, upName)
	if err := os.WriteFile(upPath, []byte("INVALID SQL STATEMENT"), 0644); err != nil {
		t.Fatalf("failed to write up migration: %v", err)
	}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec("INVALID SQL STATEMENT").WillReturnError(errors.New("syntax error"))
	mock.ExpectRollback()

	l, _ := makeLoggerBuffer()
	sqlDB := &SQLDatabase{db: db, logger: l}

	if err := RunDirectoryMigrations(context.Background(), sqlDB, tmp, l); err == nil {
		t.Fatalf("expected error when exec fails")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMigrator_Run_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	l, _ := makeLoggerBuffer()
	migrator := NewMigrator(db, l)

	// create migrations table
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	// getAppliedMigrations -> return no rows
	mock.ExpectQuery("SELECT version FROM").WillReturnRows(sqlmock.NewRows([]string{"version"}))

	// For migration up:
	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE users").WillReturnResult(sqlmock.NewResult(0, 0))
	// record migration insert
	mock.ExpectExec("INSERT INTO .*schema_migrations").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	mig := NewSQLMigration("001", "create_users", "CREATE TABLE users (id INT);", "")

	if err := migrator.Run([]Migration{mig}); err != nil {
		t.Fatalf("migrator.Run failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMigrator_Run_FailureUp_Rollback(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	l, _ := makeLoggerBuffer()
	migrator := NewMigrator(db, l)

	// create migrations table
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS").WillReturnResult(sqlmock.NewResult(0, 0))
	// getAppliedMigrations -> return no rows
	mock.ExpectQuery("SELECT version FROM").WillReturnRows(sqlmock.NewRows([]string{"version"}))

	// Begin and fail migration Exec
	mock.ExpectBegin()
	mock.ExpectExec("ALTER TABLE").WillReturnError(errors.New("exec error"))
	// rollback
	mock.ExpectRollback()

	mig := NewSQLMigration("002", "alter_table", "ALTER TABLE nope;", "")

	if err := migrator.Run([]Migration{mig}); err == nil {
		t.Fatalf("expected error from migrator.Run when Up fails")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
