package poll

import (
	"context"
	"database/sql"
	"errors"
)

var (
	ErrInvalidStatus = errors.New("invalid poll status")
	ErrInvalidDates  = errors.New("ends_at must be after starts_at")
	ErrPollNotFound  = errors.New("poll not found")
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, p *Poll, options []Option) (int64, error) {
	if p.Title == "" {
		return 0, errors.New("title required")
	}
	if len(options) < 2 {
		return 0, errors.New("poll must have at least 2 options")
	}
	p.Status = "draft"
	return s.repo.Create(ctx, p, options)
}

func (s *Service) Get(ctx context.Context, id int64) (*Poll, []Option, error) {
	p, opts, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrPollNotFound
	}
	return p, opts, err
}

func (s *Service) List(ctx context.Context, status *string) ([]Poll, error) {
	return s.repo.List(ctx, status)
}

func (s *Service) UpdateStatus(ctx context.Context, id int64, status string) error {
	if status != "draft" && status != "active" && status != "closed" {
		return ErrInvalidStatus
	}
	err := s.repo.UpdateStatus(ctx, id, status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPollNotFound
	}
	return err
}

func (s *Service) Update(ctx context.Context, id int64, input UpdateInput) error {
	if input.Title != nil && *input.Title == "" {
		return errors.New("title required")
	}

	if input.Title == nil && !input.Description.Set && !input.StartsAt.Set && !input.EndsAt.Set {
		return errors.New("no fields to update")
	}

	if input.StartsAt.Set || input.EndsAt.Set {
		p, _, err := s.repo.GetByID(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPollNotFound
		}
		if err != nil {
			return err
		}

		startsAt := p.StartsAt
		endsAt := p.EndsAt
		if input.StartsAt.Set {
			startsAt = input.StartsAt.Value
		}
		if input.EndsAt.Set {
			endsAt = input.EndsAt.Value
		}
		if startsAt != nil && endsAt != nil && endsAt.Before(*startsAt) {
			return ErrInvalidDates
		}
	}

	err := s.repo.Update(ctx, id, input)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPollNotFound
	}
	return err
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	err := s.repo.Delete(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPollNotFound
	}
	return err
}
