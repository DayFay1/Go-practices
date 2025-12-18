package vote

import (
	"context"
	"time"
)

type Vote struct {
	ID        int64     `json:"id"`
	PollID    int64     `json:"poll_id"`
	OptionID  int64     `json:"option_id"`
	UserID    int64     `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Repository interface {
	Create(ctx context.Context, v *Vote) error
	CountByPoll(ctx context.Context, pollID int64) (map[int64]int64, int64, error)
	TotalVotes(ctx context.Context, pollID int64) (int64, error)
	AggregatedByPoll(ctx context.Context, pollID int64) (map[int64]int64, int64, error)
	IncrementAggregated(ctx context.Context, pollID, optionID int64) error
	GetPollStatus(ctx context.Context, pollID int64) (string, error)
}
