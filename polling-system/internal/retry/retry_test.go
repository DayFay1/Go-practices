package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDoWithRetry_StopsOnSuccess(t *testing.T) {
	calls := 0
	err := DoWithRetry(context.Background(), 3, 0, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestDoWithRetry_RetriesUntilSuccess(t *testing.T) {
	calls := 0
	err := DoWithRetry(context.Background(), 5, 0, func() error {
		calls++
		if calls < 3 {
			return errors.New("fail")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestDoWithRetry_ReturnsLastError(t *testing.T) {
	calls := 0
	want := errors.New("fail")
	err := DoWithRetry(context.Background(), 3, 0, func() error {
		calls++
		return want
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if err != want {
		t.Fatalf("expected %v, got %v", want, err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestDoWithRetry_ContextCanceledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	err := DoWithRetry(ctx, 3, 0, func() error {
		calls++
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected 0 calls, got %d", calls)
	}
}

func TestDoWithRetry_ContextCanceledDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	calls := 0
	err := DoWithRetry(ctx, 3, time.Hour, func() error {
		calls++
		cancel()
		return errors.New("fail")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

