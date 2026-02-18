package scheduler

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestNewCronService(t *testing.T) {
	log := slog.Default()
	job := func(ctx context.Context, logger *slog.Logger) {}

	t.Run("valid interval and no startAt", func(t *testing.T) {
		s, err := NewCronService(5*time.Second, "", job, log)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.interval != 5*time.Second {
			t.Errorf("expected interval 5s, got %v", s.interval)
		}
		if s.firstRun != nil {
			t.Error("expected firstRun nil")
		}
	})

	t.Run("valid startAt", func(t *testing.T) {
		s, err := NewCronService(5*time.Second, "15:04", job, log)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if s.firstRun == nil {
			t.Error("expected firstRun not nil")
		}
	})

	t.Run("invalid startAt", func(t *testing.T) {
		_, err := NewCronService(5*time.Second, "invalid", job, log)
		if err == nil {
			t.Error("expected error for invalid startAt")
		}
	})
}

func TestCronService_NonOverlap(t *testing.T) {
	log := slog.Default()

	executions := 0

	job := func(ctx context.Context, logger *slog.Logger) {
		executions++
		time.Sleep(100 * time.Millisecond) // simulate long work
	}

	s, err := NewCronService(10*time.Millisecond, "", job, log)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	go s.Start()
	time.Sleep(150 * time.Millisecond)

	_ = s.Shutdown(1 * time.Second)

	if executions < 1 {
		t.Errorf("expected at least 1 execution")
	}

	if executions > 3 {
		t.Errorf("job overlapped, executions=%d", executions)
	}
}