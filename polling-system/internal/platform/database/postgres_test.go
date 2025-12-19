package database

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestNewPostgres_PingsAndReturnsDB(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectPing()

	origOpen := sqlOpen
	t.Cleanup(func() { sqlOpen = origOpen })
	sqlOpen = func(driverName, dsn string) (*sql.DB, error) {
		if driverName != "pgx" {
			t.Fatalf("expected driver pgx, got %q", driverName)
		}
		if dsn != "dsn" {
			t.Fatalf("expected dsn dsn, got %q", dsn)
		}
		return db, nil
	}

	got, err := NewPostgres("dsn")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != db {
		t.Fatalf("expected same *sql.DB instance")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestNewPostgres_ClosesAndReturnsErrorOnPingTimeout(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}

	pingErr := errors.New("ping fail")
	mock.ExpectPing().WillReturnError(pingErr)
	mock.ExpectClose()

	origOpen := sqlOpen
	origNow := timeNow
	origSleep := timeSleep
	t.Cleanup(func() {
		sqlOpen = origOpen
		timeNow = origNow
		timeSleep = origSleep
	})

	sqlOpen = func(driverName, dsn string) (*sql.DB, error) {
		return db, nil
	}

	base := time.Unix(0, 0)
	nowCalls := 0
	timeNow = func() time.Time {
		nowCalls++
		if nowCalls == 1 {
			return base
		}
		return base.Add(16 * time.Second)
	}

	timeSleep = func(time.Duration) {}

	got, err := NewPostgres("dsn")
	if got != nil {
		t.Fatalf("expected nil db on error")
	}
	if err == nil {
		t.Fatalf("expected error")
	}
	if err.Error() != pingErr.Error() {
		t.Fatalf("expected %q, got %q", pingErr.Error(), err.Error())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

