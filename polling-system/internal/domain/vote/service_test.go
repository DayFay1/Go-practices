package vote

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memoryVoteRepo struct {
	mu            sync.Mutex
	votes         map[int64]map[int64]int64
	userVotes     map[int64]map[int64]bool
	aggregated    map[int64]map[int64]int64
	pollStatus    map[int64]string
	countCalls    int
	aggregatedHit int
}

func newMemoryVoteRepo() *memoryVoteRepo {
	return &memoryVoteRepo{
		votes:      make(map[int64]map[int64]int64),
		userVotes:  make(map[int64]map[int64]bool),
		aggregated: make(map[int64]map[int64]int64),
		pollStatus: make(map[int64]string),
	}
}

func (r *memoryVoteRepo) Create(ctx context.Context, v *Vote) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.userVotes[v.PollID] == nil {
		r.userVotes[v.PollID] = make(map[int64]bool)
	}
	if r.userVotes[v.PollID][v.UserID] {
		return ErrAlreadyVoted
	}
	r.userVotes[v.PollID][v.UserID] = true
	if r.votes[v.PollID] == nil {
		r.votes[v.PollID] = make(map[int64]int64)
	}
	r.votes[v.PollID][v.OptionID]++
	return nil
}

func (r *memoryVoteRepo) CountByPoll(ctx context.Context, pollID int64) (map[int64]int64, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.countCalls++
	res := make(map[int64]int64)
	var total int64
	for opt, c := range r.votes[pollID] {
		res[opt] = c
		total += c
	}
	return res, total, nil
}

func (r *memoryVoteRepo) TotalByPoll(ctx context.Context, pollID int64) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var total int64
	for _, c := range r.votes[pollID] {
		total += c
	}
	return total, nil
}

func (r *memoryVoteRepo) AggregatedByPoll(ctx context.Context, pollID int64) (map[int64]int64, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.aggregatedHit++
	res := make(map[int64]int64)
	var total int64
	for opt, c := range r.aggregated[pollID] {
		res[opt] = c
		total += c
	}
	return res, total, nil
}

func (r *memoryVoteRepo) IncrementAggregated(ctx context.Context, pollID, optionID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.aggregated[pollID] == nil {
		r.aggregated[pollID] = make(map[int64]int64)
	}
	r.aggregated[pollID][optionID]++
	return nil
}

func (r *memoryVoteRepo) GetPollStatus(ctx context.Context, pollID int64) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if status, ok := r.pollStatus[pollID]; ok {
		return status, nil
	}
	// default to active for tests if not preset
	return "active", nil
}

func TestVoteIdempotencyAndCache(t *testing.T) {
	repo := newMemoryVoteRepo()
	svc := NewService(repo)
	svc.cacheTTL = time.Hour
	ctx := context.Background()

	if err := svc.Vote(ctx, 1, 10, 42); err != nil {
		t.Fatalf("expected first vote ok, got %v", err)
	}
	if !errors.Is(svc.Vote(ctx, 1, 10, 42), ErrAlreadyVoted) {
		t.Fatalf("expected duplicate vote error")
	}

	results, total, err := svc.Results(ctx, 1)
	if err != nil {
		t.Fatalf("results error: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if len(results) != 1 || results[0].Percentage != 100 {
		t.Fatalf("unexpected results %+v", results)
	}
	if repo.countCalls != 1 {
		t.Fatalf("expected one count call, got %d", repo.countCalls)
	}

	if _, _, err := svc.Results(ctx, 1); err != nil {
		t.Fatalf("cache lookup failed: %v", err)
	}
	if repo.countCalls != 1 {
		t.Fatalf("expected cached results to be used, count calls %d", repo.countCalls)
	}
}

func TestVoteRejectsWhenPollNotActive(t *testing.T) {
	repo := newMemoryVoteRepo()
	repo.pollStatus[1] = "closed"
	svc := NewService(repo)
	ctx := context.Background()

	if err := svc.Vote(ctx, 1, 10, 1); !errors.Is(err, ErrPollNotActive) {
		t.Fatalf("expected poll not active error, got %v", err)
	}
}

func TestResultsFallsBackWhenAggregatesStale(t *testing.T) {
	repo := newMemoryVoteRepo()
	svc := NewService(repo)
	svc.cacheTTL = time.Hour
	ctx := context.Background()

	if err := svc.Vote(ctx, 1, 10, 1); err != nil {
		t.Fatalf("vote 1: %v", err)
	}
	if err := svc.Vote(ctx, 1, 11, 2); err != nil {
		t.Fatalf("vote 2: %v", err)
	}
	if err := repo.IncrementAggregated(ctx, 1, 10); err != nil {
		t.Fatalf("aggregate: %v", err)
	}

	results, total, err := svc.Results(ctx, 1)
	if err != nil {
		t.Fatalf("results error: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total 2, got %d", total)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 options, got %d", len(results))
	}
	byOption := make(map[int64]int64, 2)
	for _, r := range results {
		byOption[r.OptionID] = r.Votes
	}
	if byOption[10] != 1 || byOption[11] != 1 {
		t.Fatalf("unexpected results %+v", results)
	}
}
