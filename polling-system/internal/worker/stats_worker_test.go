package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type recordingAgg struct {
	mu    sync.Mutex
	calls int
	failN int
	last  struct {
		pollID   int64
		optionID int64
	}
}

func (a *recordingAgg) IncrementAggregated(ctx context.Context, pollID, optionID int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	a.last.pollID = pollID
	a.last.optionID = optionID
	if a.failN > 0 {
		a.failN--
		return errors.New("fail")
	}
	return nil
}

func (a *recordingAgg) snapshot() (calls int, pollID int64, optionID int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls, a.last.pollID, a.last.optionID
}

func TestStatsWorker_RunProcessesEvents(t *testing.T) {
	ch := make(chan VoteEvent, 1)
	ch <- VoteEvent{PollID: 10, OptionID: 20, UserID: 30}
	close(ch)

	agg := &recordingAgg{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewStatsWorker(ch, agg, logger)
	w.workers = 1
	w.attempts = 1
	w.delay = time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	w.Run(ctx)

	calls, pollID, optionID := agg.snapshot()
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
	if pollID != 10 || optionID != 20 {
		t.Fatalf("unexpected args: pollID=%d optionID=%d", pollID, optionID)
	}
}

func TestStatsWorker_RetriesOnFailure(t *testing.T) {
	ch := make(chan VoteEvent, 1)
	ch <- VoteEvent{PollID: 1, OptionID: 2, UserID: 3}
	close(ch)

	agg := &recordingAgg{failN: 2}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewStatsWorker(ch, agg, logger)
	w.workers = 1
	w.attempts = 3
	w.delay = time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	w.Run(ctx)

	calls, _, _ := agg.snapshot()
	if calls != 3 {
		t.Fatalf("expected 3 calls (2 failures + success), got %d", calls)
	}
}

func TestStatsWorker_RunStopsOnContextCancel(t *testing.T) {
	ch := make(chan VoteEvent)
	agg := &recordingAgg{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	w := NewStatsWorker(ch, agg, logger)
	w.workers = 1

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for worker to stop")
	}
}

