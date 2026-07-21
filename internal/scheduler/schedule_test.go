package scheduler

import (
	"testing"
	"time"
)

func TestNextRunDue_OneShot(t *testing.T) {
	now := time.Now()
	createdAt := now.Add(-time.Hour)

	t.Run("never run is due", func(t *testing.T) {
		due, err := NextRunDue(nil, nil, createdAt, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !due {
			t.Fatal("expected one-shot job with no prior run to be due")
		}
	})

	t.Run("already scheduled is not due", func(t *testing.T) {
		last := now.Add(-time.Minute)
		due, err := NextRunDue(nil, &last, createdAt, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if due {
			t.Fatal("expected one-shot job that already ran to not be due again")
		}
	})
}

func TestNextRunDue_Cron(t *testing.T) {
	now := time.Now()
	createdAt := now.Add(-24 * time.Hour)

	t.Run("next fire in the past is due", func(t *testing.T) {
		expr := "* * * * *" // every minute
		last := now.Add(-5 * time.Minute)
		due, err := NextRunDue(&expr, &last, createdAt, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !due {
			t.Fatal("expected cron job whose next fire is in the past to be due")
		}
	})

	t.Run("next fire in the future is not due", func(t *testing.T) {
		expr := "0 0 1 1 *" // once a year, Jan 1st midnight
		last := now
		due, err := NextRunDue(&expr, &last, createdAt, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if due {
			t.Fatal("expected cron job whose next fire is far in the future to not be due")
		}
	})

	t.Run("invalid expression errors", func(t *testing.T) {
		expr := "not a cron expression"
		_, err := NextRunDue(&expr, nil, createdAt, now)
		if err == nil {
			t.Fatal("expected error for invalid cron expression, got nil")
		}
	})
}
