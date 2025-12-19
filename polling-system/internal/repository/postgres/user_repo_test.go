package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"polling-system/internal/domain/user"
)

func TestUserRepo_Create(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewUserRepo(db)

	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	mock.ExpectQuery(`INSERT INTO users`).
		WithArgs("john@example.com", "hash", "user").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at", "is_active"}).
			AddRow(int64(123), now, true))

	u := &user.User{Email: "john@example.com", PasswordHash: "hash", Role: "user"}
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.ID != 123 {
		t.Fatalf("expected id 123, got %d", u.ID)
	}
	if !u.CreatedAt.Equal(now) {
		t.Fatalf("unexpected created_at: %v", u.CreatedAt)
	}
	if !u.IsActive {
		t.Fatalf("expected active user")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestUserRepo_UpdateRole_NoRows(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewUserRepo(db)

	mock.ExpectExec(`UPDATE users SET role`).
		WithArgs("admin", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.UpdateRole(context.Background(), 1, "admin")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestUserRepo_Deactivate_NoRows(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewUserRepo(db)

	mock.ExpectExec(`UPDATE users SET is_active = FALSE`).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.Deactivate(context.Background(), 1)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

