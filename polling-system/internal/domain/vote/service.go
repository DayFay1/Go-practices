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
	repo                 Repository
	cacheTTL             time.Duration
	cacheCleanupInterval time.Duration
	cacheMaxEntries      int
	cache                map[int64]cachedResult
	lastCleanup          time.Time
	mu                   sync.RWMutex
}

type cachedResult struct {
	results   []Result
	total     int64
	expiresAt time.Time
}

func NewService(repo Repository) *Service {
	return &Service{
		repo:                 repo,
		cacheTTL:             10 * time.Second,
		cacheCleanupInterval: time.Minute,
		cacheMaxEntries:      1024,
		cache:                make(map[int64]cachedResult),
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
	if status != "active" {
		return ErrPollNotActive
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
	if _, err := s.repo.GetPollStatus(ctx, pollID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, ErrPollNotFound
		}
		return nil, 0, err
	}

	if cached, ok := s.getCached(pollID); ok {
		return cached.results, cached.total, nil
	}

	aggCounts, aggTotal, err := s.repo.AggregatedByPoll(ctx, pollID)
	if err != nil {
		return nil, 0, err
	}

	totalReal, err := s.repo.TotalVotes(ctx, pollID)
	if err != nil {
		return nil, 0, err
	}

	counts := aggCounts
	total := aggTotal

	if totalReal != aggTotal {
		counts, total, err = s.repo.CountByPoll(ctx, pollID)
		if err != nil {
			return nil, 0, err
		}
	} else {
		total = totalReal
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
	res, ok := s.cache[pollID]
	s.mu.RUnlock()
	if !ok {
		return cachedResult{}, false
	}
	if time.Now().After(res.expiresAt) {
		s.mu.Lock()
		if res2, ok2 := s.cache[pollID]; ok2 && time.Now().After(res2.expiresAt) {
			delete(s.cache, pollID)
		}
		s.mu.Unlock()
		return cachedResult{}, false
	}
	return res, true
}

func (s *Service) setCached(pollID int64, results []Result, total int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupCacheLocked()
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

func (s *Service) cleanupCacheLocked() {
	now := time.Now()
	runFullCleanup := s.lastCleanup.IsZero() || now.Sub(s.lastCleanup) >= s.cacheCleanupInterval
	if runFullCleanup {
		s.lastCleanup = now
	}

	if runFullCleanup || (s.cacheMaxEntries > 0 && len(s.cache) > s.cacheMaxEntries) {
		for k, v := range s.cache {
			if now.After(v.expiresAt) {
				delete(s.cache, k)
			}
		}
	}

	if s.cacheMaxEntries > 0 {
		for len(s.cache) > s.cacheMaxEntries {
			for k := range s.cache {
				delete(s.cache, k)
				break
			}
		}
	}
}
