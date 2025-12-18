package vote

import (
	"context"
	"time"
)

type Vote struct {
	ID        int64
	PollID    int64
	OptionID  int64
	UserID    int64
	CreatedAt time.Time
}

type Repository interface {
	Create(ctx context.Context, v *Vote) error
	GetPollStatus(ctx context.Context, pollID int64) (string, error)

	GetPollWindow(ctx context.Context, pollID int64) (*time.Time, *time.Time, error)

	AggregatedByPoll(ctx context.Context, pollID int64) (map[int64]int64, int64, error)
	CountByPoll(ctx context.Context, pollID int64) (map[int64]int64, int64, error)
	GetVotesByUser(ctx context.Context, userID int64) ([]UserVote, error)
}
type UserVote struct {
	PollID   int64     `json:"poll_id"`
	OptionID int64     `json:"option_id"`
	VotedAt  time.Time `json:"voted_at"`
}
