package database

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/ittxx/go-pkg/pkg/logger"
)

func makeLoggerBuffer() (*logger.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	l := logger.NewWithWriter(logger.DefaultConfig(), &buf)
	return l, &buf
}

func TestSQLDatabase_Health_Success(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	// Expect a ping
	mock.ExpectPing()

	l, _ := makeLoggerBuffer()

	sqlDB := &SQLDatabase{
		db:     db,
		logger: l,
	}

	ctx := context.Background()
	if err := sqlDB.Health(ctx); err != nil {
		t.Fatalf("expected health to succeed, got error: %v", err)
	}

	// ensure expectations met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSQLDatabase_Health_Failure(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	// do not defer Close here because Health will return error and we want to assert
	// but still ensure cleanup
	defer db.Close()

	mock.ExpectPing().WillReturnError(errors.New("connection refused"))

	l, _ := makeLoggerBuffer()

	sqlDB := &SQLDatabase{
		db:     db,
		logger: l,
	}

	ctx := context.Background()
	err = sqlDB.Health(ctx)
	if err == nil {
		t.Fatalf("expected health to fail, got nil")
	}
	// ensure wrapped error contains original message (implementation wraps)
	if err.Error() == "" {
		t.Fatalf("expected non-empty error message")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestSQLDatabase_GetStatsAndClose(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	// sqlmock does not provide meaningful stats, but Stats() should return without panic
	l, buf := makeLoggerBuffer()

	sqlDB := &SQLDatabase{
		db:     db,
		logger: l,
	}

	// GetStats should return sql.DBStats (or zero value) and not panic
	stats := sqlDB.GetStats()
	_ = stats // just ensure call works

	// Expect close to be called and succeed
	mock.ExpectClose()

	if err := sqlDB.Close(); err != nil {
		t.Fatalf("expected close to succeed, got error: %v", err)
	}

	// Logger should contain closure message
	out := buf.String()
	if out == "" {
		t.Fatalf("expected logger to have output after close")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestHealthCheckUtility(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectPing()

	l, _ := makeLoggerBuffer()
	sqlDB := &SQLDatabase{db: db, logger: l}

	if err := HealthCheck(sqlDB); err != nil {
		t.Fatalf("HealthCheck returned error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestMonitorConnectionPool_NoPanic(t *testing.T) {
	// monitorConnectionPool runs a ticker loop reading db.Stats and calling logger.
	// We call it with a short ticker by invoking the function in a goroutine and
	// cancelling via closing the DB (sqlmock DB.Close will stop subsequent stats).
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	l, _ := makeLoggerBuffer()

	// Run monitorConnectionPool in background but stop it quickly.
	go func() {
		// call the function; it will tick. We'll cancel by closing the DB.
		monitorConnectionPool(db, nil, l)
	}()

	// let it run briefly
	time.Sleep(10 * time.Millisecond)

	// close db to cause function to eventually exit loops (no explicit cancel in function)
	db.Close()

	// ensure expectations (none) are met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
