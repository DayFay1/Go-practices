package poll

import (
	"context"
	"time"
)

type Poll struct {
	ID          int64      `json:"id"`
	Title       string     `json:"title"`
	Description *string    `json:"description,omitempty"`
	Status      string     `json:"status"`
	StartsAt    *time.Time `json:"starts_at,omitempty"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
	CreatorID   int64      `json:"creator_id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Option struct {
	ID        int64     `json:"id"`
	PollID    int64     `json:"poll_id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

type UpdateInput struct {
	Title       *string
	Description OptionalString
	StartsAt    OptionalTime
	EndsAt      OptionalTime
}

type OptionalString struct {
	Set   bool
	Value *string
}

type OptionalTime struct {
	Set   bool
	Value *time.Time
}

type Repository interface {
	Create(ctx context.Context, p *Poll, options []Option) (int64, error)
	GetByID(ctx context.Context, id int64) (*Poll, []Option, error)
	List(ctx context.Context, status *string) ([]Poll, error)
	UpdateStatus(ctx context.Context, id int64, status string) error
	Update(ctx context.Context, id int64, input UpdateInput) error
	Delete(ctx context.Context, id int64) error
}
