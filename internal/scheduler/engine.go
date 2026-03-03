// Package scheduler provides a runtime cron engine backed by persistent storage.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"cron-weather/internal/domain"
	"cron-weather/internal/storage"
	"cron-weather/internal/task"
	"cron-weather/internal/transport"

	"github.com/robfig/cron/v3"
)

// Engine is an in-memory cron runner backed by Postgres.
//
// Responsibilities:
//   - bootstrap active schedules from DB on startup
//   - (re)register schedules in robfig/cron
//   - execute schedule tasks (pluggable via task.Runner)
//   - record runs + next_run_at in DB
//   - stop schedules when they expire (ends_at)
type Engine struct {
	log      *slog.Logger
	repo     storage.Repo
	producer transport.Producer

	cron *cron.Cron

	// kind -> runner
	runners map[string]task.Runner

	mu     sync.RWMutex
	entry  map[string]cron.EntryID // scheduleID -> cron entry id
	closed bool
}

// New creates a scheduler Engine with the given repository, producer and task runners.
func New(log *slog.Logger, repo storage.Repo, producer transport.Producer, runners map[string]task.Runner, tz string) *Engine {
	parser := cron.NewParser(
		cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)

	tz = strings.TrimSpace(tz)
	if tz == "" {
		tz = "UTC"
	}

	loc, err := time.LoadLocation(strings.TrimSpace(tz))
	if err != nil || loc == nil {
		loc = time.UTC
	}

	c := cron.New(
		cron.WithSeconds(),
		cron.WithParser(parser),
		cron.WithLocation(loc),
		cron.WithChain(cron.Recover(cron.DefaultLogger)),
	)

	if runners == nil {
		runners = map[string]task.Runner{}
	}

	return &Engine{
		log:      log,
		repo:     repo,
		producer: producer,
		cron:     c,
		runners:  runners,
		entry:    make(map[string]cron.EntryID),
	}
}

// Start bootstraps active schedules from storage and starts the cron loop.
func (e *Engine) Start(ctx context.Context) error {
	if e.repo == nil {
		e.log.Warn("scheduler engine: no repo configured; nothing to start")
		return nil
	}

	items, err := e.repo.ListAllActiveSchedulers(ctx)
	if err != nil {
		return fmt.Errorf("list active schedulers: %w", err)
	}

	for _, it := range items {
		if err := e.Add(ctx, it); err != nil {
			e.log.Error("failed to register schedule", slog.Any("err", err), slog.String("schedule_id", it.Scheduler.ID))
		}
	}

	e.cron.Start()
	return nil
}

// Stop stops the cron engine and waits for running jobs to finish.
func (e *Engine) Stop(ctx context.Context) {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return
	}
	e.closed = true
	e.mu.Unlock()

	stopCtx := e.cron.Stop()
	select {
	case <-ctx.Done():
		return
	case <-stopCtx.Done():
		return
	}
}

// AddByID loads the scheduler from DB and registers it.
func (e *Engine) AddByID(ctx context.Context, schedulerID string) error {
	if e.repo == nil {
		return nil
	}
	it, err := e.repo.GetActiveScheduler(ctx, schedulerID)
	if err != nil {
		return err
	}
	return e.Add(ctx, it)
}

// Add registers a scheduler in cron. Safe to call multiple times; it will replace existing entry.
func (e *Engine) Add(ctx context.Context, it domain.SchedulerWithTarget) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return fmt.Errorf("engine stopped")
	}

	// Replace existing.
	if old, ok := e.entry[it.Scheduler.ID]; ok {
		e.cron.Remove(old)
		delete(e.entry, it.Scheduler.ID)
	}

	entryID, err := e.cron.AddFunc(it.Scheduler.Expr, func() {
		e.run(context.Background(), it)
	})
	if err != nil {
		return fmt.Errorf("cron add: %w", err)
	}

	e.entry[it.Scheduler.ID] = entryID

	// Log runtime registration (prod-relevant event).
	e.log.Info("scheduler registered",
		slog.String("scheduler_id", it.Scheduler.ID),
		slog.String("subscription_id", it.Scheduler.SubscriptionID),
		slog.String("endpoint_id", it.Target.Address),
		slog.String("kind", it.Scheduler.Kind),
		slog.String("cron_expr", it.Scheduler.Expr),
	)

	// Store computed next_run_at.
	if e.repo != nil {
		next := e.cron.Entry(entryID).Next
		_ = e.repo.UpdateSchedulerNextRunAt(ctx, it.Scheduler.ID, &next)
	}

	return nil
}

// Remove unregisters a schedule from runtime cron by its ID.
func (e *Engine) Remove(ctx context.Context, schedulerID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if id, ok := e.entry[schedulerID]; ok {
		e.cron.Remove(id)
		delete(e.entry, schedulerID)
		_ = e.repo.UpdateSchedulerNextRunAt(ctx, schedulerID, nil)
	}
}

func (e *Engine) run(ctx context.Context, it domain.SchedulerWithTarget) {
	now := time.Now()
	start := time.Now()

	// Log each run start (prod-relevant event).
	e.log.Info("schedule run started",
		slog.String("scheduler_id", it.Scheduler.ID),
		slog.String("subscription_id", it.Scheduler.SubscriptionID),
		slog.String("endpoint_id", it.Target.Address),
		slog.String("kind", it.Scheduler.Kind),
		slog.Time("scheduled_for", now),
	)

	// Respect starts_at / ends_at.
	if it.Scheduler.StartAt != nil && now.Before(*it.Scheduler.StartAt) {
		return
	}
	if it.Scheduler.EndAt != nil && now.After(*it.Scheduler.EndAt) {
		if e.repo != nil {
			_ = e.repo.DeactivateScheduler(ctx, it.Scheduler.ID)
			_ = e.repo.UpdateSchedulerNextRunAt(ctx, it.Scheduler.ID, nil)
		}
		e.Remove(ctx, it.Scheduler.ID)
		return
	}

	status := "success"
	errText := ""
	payload := ""

	// Pick runner by schedule kind (fallback to "cron" for backward-compat).
	kind := it.Scheduler.Kind
	if kind == "" {
		kind = "cron"
	}
	runner := e.runners[kind]
	if runner == nil {
		status = "error"
		errText = fmt.Sprintf("no runner for schedule kind=%q", kind)
	} else {
		res, err := runner.Run(ctx, task.Input{
			Scheduler:    it.Scheduler,
			Subscription: it.Subscription,
			Target:       it.Target,
			ScheduledFor: now,
		})
		if err != nil {
			status = "error"
			errText = err.Error()
		} else {
			payload = res.Payload
			// Deliver messages to endpoint.
			if it.Target.Kind == "telegram" {
				chatID, perr := strconv.ParseInt(it.Target.Address, 10, 64)
				if perr != nil {
					status = "error"
					errText = fmt.Sprintf("invalid telegram chat_id address: %v", perr)
				} else if e.producer != nil {
					for _, m := range res.Messages {
						if strings.TrimSpace(m) == "" {
							continue
						}
						if serr := e.producer.Send(ctx, transport.Message{ChatID: chatID, Text: m}); serr != nil {
							status = "error"
							errText = serr.Error()
							break
						}
					}
				}
			} else {
				// unsupported target kind for now
				status = "error"
				errText = "unsupported endpoint kind"
			}
		}
	}

	if e.repo != nil {
		_ = e.repo.InsertRun(ctx, it.Scheduler.SubscriptionID, it.Scheduler.ID, now, status, payload, errText)
	}

	e.mu.RLock()
	entryID, ok := e.entry[it.Scheduler.ID]
	e.mu.RUnlock()
	if ok && e.repo != nil {
		next := e.cron.Entry(entryID).Next
		_ = e.repo.UpdateSchedulerNextRunAt(ctx, it.Scheduler.ID, &next)
	}

	duration := time.Since(start)
	attrs := []slog.Attr{
		slog.String("scheduler_id", it.Scheduler.ID),
		slog.String("subscription_id", it.Scheduler.SubscriptionID),
		slog.String("endpoint_id", it.Target.Address),
		slog.String("kind", it.Scheduler.Kind),
		slog.String("status", status),
		slog.Int64("duration_ms", duration.Milliseconds()),
	}
	if errText != "" {
		attrs = append(attrs, slog.String("error", errText))
	}
	if status == "success" {
		e.log.Info("schedule run finished")
	} else {
		e.log.Warn("schedule run finished")
	}
}
