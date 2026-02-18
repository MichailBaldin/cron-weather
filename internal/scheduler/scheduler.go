// Package scheduler provides a simple cron-like service
package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// JobFunc represents a scheduled job function.
type JobFunc func(ctx context.Context, logger *slog.Logger)

// CronService runs a job on a fixed interval.
// It ensures graceful shutdown and prevents overlapping executions.
type CronService struct {
	interval time.Duration
	firstRun *time.Time
	job      JobFunc
	logger   *slog.Logger

	cancel  context.CancelFunc
	wg      sync.WaitGroup
	mu      sync.Mutex
	running bool
	jobRunning bool
}

func NewCronService(
	interval time.Duration,
	startAt string,
	job JobFunc,
	logger *slog.Logger,
) (*CronService, error) {
	var firstRun *time.Time
	if startAt != "" {
		now := time.Now()
		t, err := time.ParseInLocation("15:04", startAt, time.Local)
		if err != nil {
			return nil, errors.New("invalid START_AT format, expected HH:MM")
		}

		candidate := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)

		if candidate.Before(now) {
			candidate = candidate.Add(24 * time.Hour)
		}
		firstRun = &candidate
	}
	return &CronService{
		interval: interval,
		firstRun: firstRun,
		job:      job,
		logger:   logger,
	}, nil
}

// Start launches the cron loop and blocks until shutdown.
func (s *CronService) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("cron service already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.running = true
	s.mu.Unlock()

	s.logger.Info("cron service started", "interval", s.interval)

	if s.firstRun != nil {
		waitDuration := time.Until(*s.firstRun)
		if waitDuration > 0 {
			s.logger.Info("waiting for first run", "duration", waitDuration)
			select {
			case <-time.After(waitDuration):
				s.runJob(ctx)
			case <-ctx.Done():

				s.wg.Wait()
				s.logger.Info("cron service stopped during initial wait")
				return nil
			}
		} else {

			s.logger.Warn("first run time is in the past, running immediately")
			s.runJob(ctx)
		}
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runJob(ctx)
		case <-ctx.Done():
			s.logger.Info("shutdown signal received, waiting for active jobs...")
			s.wg.Wait()
			s.logger.Info("all jobs finished. cron service stopped")
			return nil
		}
	}
}

func (s *CronService) runJob(ctx context.Context) {
	s.mu.Lock()
	if s.jobRunning {
		s.logger.Warn("job skipped: previous run still in progress")
		s.mu.Unlock()
		return
	}
	s.jobRunning = true
	s.mu.Unlock()

	s.wg.Add(1)

	go func() {
		defer s.wg.Done()
		defer s.handlePanic()

		taskID := uuid.NewString()
		taskLogger := s.logger.With("task_id", taskID)

		s.job(ctx, taskLogger)

		// mark job as finished
		s.mu.Lock()
		s.jobRunning = false
		s.mu.Unlock()
	}()
}

// Shutdown gracefully stops the cron service
// and waits for active jobs up to the given timeout.
func (s *CronService) Shutdown(timeout time.Duration) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return errors.New("cron service not running")
	}
	s.cancel()
	s.running = false
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("all jobs finished")
		return nil
	case <-time.After(timeout):
		return errors.New("shutdown timeout: some jobs did not finish")
	}
}

func (s *CronService) FirstRun() *time.Time {
	return s.firstRun
}

// handlePanic recovers from job panics to prevent service crash.
func (s *CronService) handlePanic() {
	if r := recover(); r != nil {
		s.logger.Error("panic recovered in job", "panic", r)
	}
}
