package poll

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"
)

type memoryPollRepo struct {
	mu     sync.Mutex
	polls  map[int64]*Poll
	opts   map[int64][]Option
	nextID int64
}

func newMemoryPollRepo() *memoryPollRepo {
	return &memoryPollRepo{
		polls:  make(map[int64]*Poll),
		opts:   make(map[int64][]Option),
		nextID: 1,
	}
}

func (r *memoryPollRepo) Create(ctx context.Context, p *Poll, options []Option) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p.ID = r.nextID
	r.nextID++
	p.CreatedAt = time.Now()
	p.UpdatedAt = p.CreatedAt

	copyPoll := *p
	r.polls[p.ID] = &copyPoll

	cloned := make([]Option, len(options))
	for i, opt := range options {
		opt.ID = int64(i + 1)
		opt.PollID = p.ID
		opt.CreatedAt = time.Now()
		cloned[i] = opt
	}
	r.opts[p.ID] = cloned
	return p.ID, nil
}

func (r *memoryPollRepo) GetByID(ctx context.Context, id int64) (*Poll, []Option, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.polls[id]
	if !ok {
		return nil, nil, sql.ErrNoRows
	}
	opts := r.opts[id]
	copyPoll := *p
	copiedOpts := make([]Option, len(opts))
	copy(copiedOpts, opts)
	return &copyPoll, copiedOpts, nil
}

func (r *memoryPollRepo) List(ctx context.Context, status *string) ([]Poll, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	res := []Poll{}
	for _, p := range r.polls {
		if status != nil && p.Status != *status {
			continue
		}
		res = append(res, *p)
	}
	return res, nil
}

func (r *memoryPollRepo) UpdateStatus(ctx context.Context, id int64, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.polls[id]
	if !ok {
		return sql.ErrNoRows
	}
	p.Status = status
	p.UpdatedAt = time.Now()
	return nil
}

func (r *memoryPollRepo) Update(ctx context.Context, id int64, input UpdateInput) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.polls[id]
	if !ok {
		return sql.ErrNoRows
	}
	if input.Title != nil {
		p.Title = *input.Title
	}
	if input.Description.Set {
		p.Description = input.Description.Value
	}
	if input.StartsAt.Set {
		p.StartsAt = input.StartsAt.Value
	}
	if input.EndsAt.Set {
		p.EndsAt = input.EndsAt.Value
	}
	p.UpdatedAt = time.Now()
	return nil
}

func (r *memoryPollRepo) Delete(ctx context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.polls[id]; !ok {
		return sql.ErrNoRows
	}
	delete(r.polls, id)
	delete(r.opts, id)
	return nil
}

func TestPollValidationAndStatus(t *testing.T) {
	repo := newMemoryPollRepo()
	svc := NewService(repo)
	ctx := context.Background()

	if _, err := svc.Create(ctx, &Poll{}, nil); err == nil {
		t.Fatalf("expected error for missing title")
	}
	if _, err := svc.Create(ctx, &Poll{Title: "Test"}, []Option{{Text: "A"}}); err == nil {
		t.Fatalf("expected error for too few options")
	}

	id, err := svc.Create(ctx, &Poll{Title: "Ready"}, []Option{{Text: "A"}, {Text: "B"}})
	if err != nil {
		t.Fatalf("unexpected create error: %v", err)
	}

	if err := svc.UpdateStatus(ctx, id, "unknown"); !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected invalid status error")
	}
	if err := svc.UpdateStatus(ctx, id, "active"); err != nil {
		t.Fatalf("expected status update success: %v", err)
	}
}

func TestPollUpdateValidatesDatesAgainstExisting(t *testing.T) {
	repo := newMemoryPollRepo()
	svc := NewService(repo)
	ctx := context.Background()

	start := time.Now().Add(1 * time.Hour)
	end := start.Add(2 * time.Hour)
	id, err := svc.Create(ctx, &Poll{Title: "Timed", StartsAt: &start, EndsAt: &end}, []Option{{Text: "A"}, {Text: "B"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	badEnd := start.Add(-1 * time.Minute)
	if err := svc.Update(ctx, id, UpdateInput{EndsAt: OptionalTime{Set: true, Value: &badEnd}}); !errors.Is(err, ErrInvalidDates) {
		t.Fatalf("expected invalid dates, got %v", err)
	}

	if err := svc.Update(ctx, id, UpdateInput{EndsAt: OptionalTime{Set: true, Value: nil}}); err != nil {
		t.Fatalf("clear ends_at: %v", err)
	}
	p, _, err := repo.GetByID(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if p.EndsAt != nil {
		t.Fatalf("expected ends_at to be cleared")
	}
}
