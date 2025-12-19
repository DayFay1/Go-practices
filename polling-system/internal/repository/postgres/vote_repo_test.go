package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"

	"polling-system/internal/domain/vote"
)

func TestMapVoteError_DuplicateVote(t *testing.T) {
	err := mapVoteError(&pgconn.PgError{
		Code:           "23505",
		ConstraintName: "votes_poll_id_user_id_key",
	})
	if !errors.Is(err, vote.ErrAlreadyVoted) {
		t.Fatalf("expected ErrAlreadyVoted, got %v", err)
	}
}

func TestMapVoteError_OptionNotInPoll(t *testing.T) {
	err := mapVoteError(&pgconn.PgError{
		Code:           "23503",
		ConstraintName: "votes_option_poll_fkey",
	})
	if !errors.Is(err, vote.ErrOptionNotInPoll) {
		t.Fatalf("expected ErrOptionNotInPoll, got %v", err)
	}
}

func TestMapVoteError_PollNotFound(t *testing.T) {
	err := mapVoteError(&pgconn.PgError{
		Code:           "23503",
		ConstraintName: "votes_poll_id_fkey",
	})
	if !errors.Is(err, vote.ErrPollNotFound) {
		t.Fatalf("expected ErrPollNotFound, got %v", err)
	}
}

func TestMapVoteError_Passthrough(t *testing.T) {
	want := errors.New("boom")
	if got := mapVoteError(want); got != want {
		t.Fatalf("expected passthrough error")
	}
}

func TestVoteRepo_GetPollWindow_ConvertsNullsToPointers(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewVoteRepo(db)

	start := time.Date(2025, 2, 3, 4, 5, 6, 0, time.UTC)
	mock.ExpectQuery(`SELECT starts_at, ends_at FROM polls`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"starts_at", "ends_at"}).
			AddRow(start, nil))

	startsAt, endsAt, err := repo.GetPollWindow(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetPollWindow: %v", err)
	}
	if startsAt == nil || !startsAt.Equal(start) {
		t.Fatalf("unexpected starts_at: %v", startsAt)
	}
	if endsAt != nil {
		t.Fatalf("expected nil ends_at, got %v", endsAt)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestVoteRepo_CountByPoll_ReturnsMapAndTotal(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewVoteRepo(db)

	mock.ExpectQuery(`SELECT option_id, COUNT\(\*\)`).
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"option_id", "count"}).
			AddRow(int64(10), int64(2)).
			AddRow(int64(11), int64(3)))

	got, total, err := repo.CountByPoll(context.Background(), 1)
	if err != nil {
		t.Fatalf("CountByPoll: %v", err)
	}
	if total != 5 {
		t.Fatalf("expected total 5, got %d", total)
	}
	if got[10] != 2 || got[11] != 3 {
		t.Fatalf("unexpected map: %+v", got)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

