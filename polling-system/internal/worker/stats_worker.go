package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"polling-system/internal/retry"
)

type VoteEvent struct {
	PollID   int64
	OptionID int64
	UserID   int64
}

type Aggregator interface {
	IncrementAggregated(ctx context.Context, pollID, optionID int64) error
}

type StatsWorker struct {
	Ch      <-chan VoteEvent
	agg     Aggregator
	workers int
	logger  *slog.Logger
}

func NewStatsWorker(ch <-chan VoteEvent, agg Aggregator, logger *slog.Logger) *StatsWorker {
	return &StatsWorker{
		Ch:      ch,
		agg:     agg,
		workers: 4,
		logger:  logger,
	}
}

func (w *StatsWorker) Run(ctx context.Context) {
	if w.logger == nil {
		w.logger = slog.Default()
	}
	w.logger.Info("stats worker pool started", "workers", w.workers)
	var wg sync.WaitGroup
	for i := 0; i < w.workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			w.loop(ctx, id)
		}(i)
	}

	workersDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(workersDone)
	}()

	select {
	case <-ctx.Done():
		<-workersDone
	case <-workersDone:
	}
	w.logger.Info("stats worker pool stopped")
}

func (w *StatsWorker) loop(ctx context.Context, workerID int) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.Ch:
			if !ok {
				return
			}
			w.process(ctx, workerID, ev)
		}
	}
}

func (w *StatsWorker) process(ctx context.Context, workerID int, ev VoteEvent) {
	err := retry.DoWithRetry(ctx, 4, 150*time.Millisecond, func() error {
		return w.agg.IncrementAggregated(ctx, ev.PollID, ev.OptionID)
	})
	if err != nil {
		w.logger.Error("failed to aggregate vote", "worker", workerID, "poll_id", ev.PollID, "option_id", ev.OptionID, "error", err)
		return
	}
	w.logger.Debug("aggregated vote", "worker", workerID, "poll_id", ev.PollID, "option_id", ev.OptionID, "user_id", ev.UserID)
}
