package postgres

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"polling-system/internal/domain/poll"
)

func TestPollRepo_Update_NoFields(t *testing.T) {
	repo := &PollRepo{}
	err := repo.Update(context.Background(), 1, poll.UpdateInput{})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestPollRepo_Update_TitleAndNullDescription(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewPollRepo(db)

	title := "New title"
	input := poll.UpdateInput{
		Title: &title,
		Description: poll.OptionalString{
			Set:   true,
			Value: nil,
		},
	}

	wantQuery := "UPDATE polls SET title = $1, description = NULL, updated_at = now() WHERE id = $2"
	mock.ExpectExec(regexp.QuoteMeta(wantQuery)).
		WithArgs(title, int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Update(context.Background(), 99, input); err != nil {
		t.Fatalf("update: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestPollRepo_Update_NoRows(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewPollRepo(db)

	title := "New title"
	input := poll.UpdateInput{Title: &title}

	wantQuery := "UPDATE polls SET title = $1, updated_at = now() WHERE id = $2"
	mock.ExpectExec(regexp.QuoteMeta(wantQuery)).
		WithArgs(title, int64(99)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.Update(context.Background(), 99, input)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

