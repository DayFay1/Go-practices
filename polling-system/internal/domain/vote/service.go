package vote

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"
)

var (
	ErrAlreadyVoted    = errors.New("user already voted in this poll")
	ErrPollNotActive   = errors.New("poll is not active")
	ErrOptionNotInPoll = errors.New("option not in poll")
	ErrPollNotFound    = errors.New("poll not found")
)

type Service struct {
	repo     Repository
	cacheTTL time.Duration
	cache    map[int64]cachedResult
	mu       sync.RWMutex
}

type cachedResult struct {
	results   []Result
	total     int64
	expiresAt time.Time
}

func NewService(repo Repository) *Service {
	return &Service{
		repo:     repo,
		cacheTTL: 10 * time.Second,
		cache:    make(map[int64]cachedResult),
	}
}

func (s *Service) Vote(ctx context.Context, pollID, optionID, userID int64) error {
	status, err := s.repo.GetPollStatus(ctx, pollID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPollNotFound
		}
		return err
	}
	if status == "draft" {
		return errors.New("poll_not_started")
	}

	if status == "closed" {
		return errors.New("poll_closed")
	}
	if status != "active" {
		return ErrPollNotActive
	}
	startsAt, endsAt, err := s.repo.GetPollWindow(ctx, pollID)
	if err != nil {
		return err
	}

	now := time.Now()

	if startsAt != nil && now.Before(*startsAt) {
		return errors.New("poll_not_started")
	}

	if endsAt != nil && now.After(*endsAt) {
		return errors.New("poll_closed")
	}

	v := &Vote{
		PollID:   pollID,
		OptionID: optionID,
		UserID:   userID,
	}

	err = s.repo.Create(ctx, v)
	if err != nil {
		if errors.Is(err, ErrAlreadyVoted) {
			return ErrAlreadyVoted
		}
		if errors.Is(err, ErrOptionNotInPoll) {
			return ErrOptionNotInPoll
		}
		if errors.Is(err, ErrPollNotFound) {
			return ErrPollNotFound
		}
		return err
	}

	s.invalidateCache(pollID)
	return nil
}

type Result struct {
	OptionID   int64   `json:"option_id"`
	Votes      int64   `json:"votes"`
	Percentage float64 `json:"percentage"`
}

func (s *Service) Results(ctx context.Context, pollID int64) ([]Result, int64, error) {
	if cached, ok := s.getCached(pollID); ok {
		return cached.results, cached.total, nil
	}

	counts, total, err := s.repo.AggregatedByPoll(ctx, pollID)
	if err != nil {
		return nil, 0, err
	}

	if len(counts) == 0 && total == 0 {
		counts, total, err = s.repo.CountByPoll(ctx, pollID)
		if err != nil {
			return nil, 0, err
		}
	}

	results := make([]Result, 0, len(counts))
	for optionID, c := range counts {
		var p float64
		if total > 0 {
			p = float64(c) * 100.0 / float64(total)
		}
		results = append(results, Result{
			OptionID:   optionID,
			Votes:      c,
			Percentage: p,
		})
	}

	s.setCached(pollID, results, total)
	return results, total, nil
}

func (s *Service) getCached(pollID int64) (cachedResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res, ok := s.cache[pollID]
	if !ok || time.Now().After(res.expiresAt) {
		return cachedResult{}, false
	}
	return res, true
}

func (s *Service) setCached(pollID int64, results []Result, total int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[pollID] = cachedResult{
		results:   results,
		total:     total,
		expiresAt: time.Now().Add(s.cacheTTL),
	}
}

func (s *Service) invalidateCache(pollID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.cache, pollID)
}
func (s *Service) UserVotes(ctx context.Context, userID int64) ([]UserVote, error) {
	return s.repo.GetVotesByUser(ctx, userID)
}
