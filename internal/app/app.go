// Package app wires transports, storage, scheduler runtime and task runners into a running service.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"cron-weather/internal/config"
	"cron-weather/internal/scheduler"
	"cron-weather/internal/storage"
	"cron-weather/internal/task"
	"cron-weather/internal/task/weather"
	"cron-weather/internal/transport"
)

// App is the main application service that handles Telegram commands and manages schedules.
type App struct {
	logger *slog.Logger

	subs storage.Repo

	consumer transport.Consumer
	producer transport.Producer

	sched *scheduler.Engine
}

// New constructs the application with storage, transports and runtime scheduler.
func New(logger *slog.Logger, subs storage.Repo, consumer transport.Consumer, producer transport.Producer, cfg *config.Config) *App {
	client := weather.NewOpenWeatherClient(cfg.OpenWeather.APIKey)
	wt := weather.NewTask(logger, subs, client, cfg.OpenWeather.DailyLimit)
	runners := map[string]task.Runner{
		"weather": wt,
		"cron":    wt,
	}
	sched := scheduler.New(logger, subs, producer, runners)
	return &App{
		logger:   logger,
		subs:     subs,
		consumer: consumer,
		producer: producer,
		sched:    sched,
	}
}

// Start runs the application main loop and blocks until context is cancelled.
func (a *App) Start(ctx context.Context) error {
	// Bootstrap and start in-memory scheduler.
	if a.sched != nil {
		if err := a.sched.Start(ctx); err != nil {
			return fmt.Errorf("scheduler start: %w", err)
		}
		defer a.sched.Stop(context.Background())
	}

	jobs, err := a.consumer.Get(ctx)
	if err != nil {
		return fmt.Errorf("consumer get: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case job, ok := <-jobs:
			if !ok {
				return fmt.Errorf("consumer channel closed")
			}
			a.handle(ctx, job)
		}
	}
}

func (a *App) handle(ctx context.Context, job transport.CronJob) {
	switch job.Command {
	case "start_scheduler":
		a.cmdStartScheduler(ctx, job.ChatID)
	case "stop_scheduler":
		a.cmdStopScheduler(ctx, job.ChatID)
	case "list_scheduler":
		a.cmdListScheduler(ctx, job.ChatID)
	case "set_location":
		a.cmdSetLocation(ctx, job.ChatID, job.Args)
	case "start":
		a.cmdStartCron(ctx, job.ChatID, job.Args)
	case "stop":
		a.cmdStopCron(ctx, job.ChatID, job.Args)
	default:
	}
}

func (a *App) cmdStartScheduler(ctx context.Context, chatID int64) {
	if a.subs != nil {
		if _, err := a.subs.ActiveSubscription(ctx, chatID); err != nil {
			a.logger.Error("failed to persist subscription", slog.Any("err", err), slog.Int64("chat_id", chatID))
		}
	}

	_ = a.producer.Send(ctx, transport.Message{
		ChatID: chatID,
		Text:   "scheduler successfully added. /help_scheduler return available commands",
	})
}

func (a *App) cmdStopScheduler(ctx context.Context, chatID int64) {
	// Stop runtime schedules first.
	if a.subs != nil && a.sched != nil {
		items, err := a.subs.ListActiveSchedulers(ctx, chatID)
		if err == nil {
			for _, it := range items {
				a.sched.Remove(ctx, it.ID)
			}
		}
	}

	if a.subs != nil {
		if err := a.subs.DeactivateSubscription(ctx, chatID); err != nil {
			a.logger.Error("failed to deactivate subscription", slog.Any("err", err), slog.Int64("chat_id", chatID))
			_ = a.producer.Send(ctx, transport.Message{
				ChatID: chatID,
				Text:   "failed to stop scheduler",
			})
			return
		}
	}
	_ = a.producer.Send(ctx, transport.Message{
		ChatID: chatID,
		Text:   "scheduler stopped",
	})
}

func (a *App) cmdListScheduler(ctx context.Context, chatID int64) {
	if a.subs == nil {
		_ = a.producer.Send(ctx, transport.Message{
			ChatID: chatID,
			Text:   "no storage configured",
		})
		return
	}

	items, err := a.subs.ListActiveSchedulers(ctx, chatID)
	if err != nil {
		a.logger.Error("failed to list schedulers", slog.Any("err", err), slog.Int64("chat_id", chatID))
		_ = a.producer.Send(ctx, transport.Message{
			ChatID: chatID,
			Text:   "failed to list schedulers",
		})
		return
	}
	if len(items) == 0 {
		_ = a.producer.Send(ctx, transport.Message{
			ChatID: chatID,
			Text:   "no active schedulers",
		})
		return
	}

	var b strings.Builder
	b.WriteString("active schedulers:\n")
	for _, it := range items {
		b.WriteString("- id: ")
		b.WriteString(it.ID)
		b.WriteString(" | expr: ")
		b.WriteString(it.Expr)
		b.WriteString(" | start_at: ")
		b.WriteString(formatTime(it.StartAt))
		b.WriteString(" | end_at: ")
		b.WriteString(formatTime(it.EndAt))
		b.WriteString("\n")
	}
	_ = a.producer.Send(ctx, transport.Message{
		ChatID: chatID,
		Text:   b.String(),
	})
}

func (a *App) cmdStartCron(ctx context.Context, chatID int64, argsRaw string) {
	if a.subs == nil {
		_ = a.producer.Send(ctx, transport.Message{
			ChatID: chatID,
			Text:   "no storage configured",
		})
		return
	}

	cronExpr, startAt, endAt, err := parseStartArgs(argsRaw)
	if err != nil {
		_ = a.producer.Send(ctx, transport.Message{
			ChatID: chatID,
			Text:   "usage: /start <cron expr> <start_at|-> <end_at|-> (times RFC3339)",
		})
		return
	}

	id, err := a.subs.CreateScheduler(ctx, chatID, cronExpr, startAt, endAt)
	if err != nil {
		a.logger.Error("failed to create scheduler", slog.Any("err", err), slog.Int64("chat_id", chatID))
		_ = a.producer.Send(ctx, transport.Message{
			ChatID: chatID,
			Text:   "failed to create scheduler",
		})
		return
	}

	a.logger.Info("scheduler created",
		slog.String("scheduler_id", id),
		slog.Int64("chat_id", chatID),
		slog.String("cron_expr", cronExpr),
		slog.String("start_at", formatTime(startAt)),
		slog.String("end_at", formatTime(endAt)),
	)

	_ = a.producer.Send(ctx, transport.Message{
		ChatID: chatID,
		Text:   fmt.Sprintf("scheduler created: %s", id),
	})

	// Register in runtime cron.
	if a.sched != nil {
		if err := a.sched.AddByID(ctx, id); err != nil {
			a.logger.Error("failed to register scheduler in runtime", slog.Any("err", err), slog.String("scheduler_id", id))
		}
	}
}

func (a *App) cmdStopCron(ctx context.Context, chatID int64, argsRaw string) {
	if a.subs == nil {
		_ = a.producer.Send(ctx, transport.Message{
			ChatID: chatID,
			Text:   "no storage configured",
		})
		return
	}

	id := strings.TrimSpace(argsRaw)
	if id == "" {
		_ = a.producer.Send(ctx, transport.Message{
			ChatID: chatID,
			Text:   "usage: /stop <scheduler_id>",
		})
		return
	}

	if err := a.subs.StopScheduler(ctx, chatID, id); err != nil {
		a.logger.Error("failed to stop scheduler",
			slog.Any("err", err),
			slog.Int64("chat_id", chatID),
			slog.String("scheduler_id", id),
		)
		_ = a.producer.Send(ctx, transport.Message{
			ChatID: chatID,
			Text:   "failed to stop scheduler",
		})
		return
	}
	if a.sched != nil {
		a.sched.Remove(ctx, id)
	}

	a.logger.Info("scheduler stopped",
		slog.String("scheduler_id", id),
		slog.Int64("chat_id", chatID),
	)

	_ = a.producer.Send(ctx, transport.Message{
		ChatID: chatID,
		Text:   "scheduler stopped",
	})
}

func parseStartArgs(argsRaw string) (cronExpr string, startAt *time.Time, endAt *time.Time, err error) {
	fields := strings.Fields(strings.TrimSpace(argsRaw))
	if len(fields) == 0 {
		return "", nil, nil, fmt.Errorf("empty args")
	}

	// Try to strip optional time arguments from the right.
	// 1) If the last two tokens look like time or "-", treat them as start/end.
	// 2) If the last token looks like time or "-", treat it as start (end is nil).
	// 3) Otherwise: only cron expression (start/end are nil).
	n := len(fields)

	// case: maybe have both start and end
	if n >= 3 {
		t2, ok2, e2 := parseOptionalRFC3339(fields[n-1])
		if e2 != nil {
			return "", nil, nil, e2
		}
		t1, ok1, e1 := parseOptionalRFC3339(fields[n-2])
		if e1 != nil {
			return "", nil, nil, e1
		}

		if ok1 && ok2 {
			endAt = t2
			startAt = t1
			cronExpr = strings.Join(fields[:n-2], " ")
			cronExpr = strings.TrimSpace(cronExpr)
			if cronExpr == "" {
				return "", nil, nil, fmt.Errorf("empty cron")
			}
			return cronExpr, startAt, endAt, nil
		}
	}

	// case: maybe have only start
	if n >= 2 {
		t1, ok1, e1 := parseOptionalRFC3339(fields[n-1])
		if e1 != nil {
			return "", nil, nil, e1
		}
		if ok1 {
			startAt = t1
			endAt = nil
			cronExpr = strings.Join(fields[:n-1], " ")
			cronExpr = strings.TrimSpace(cronExpr)
			if cronExpr == "" {
				return "", nil, nil, fmt.Errorf("empty cron")
			}
			return cronExpr, startAt, endAt, nil
		}
	}

	// case: only cron expr
	cronExpr = strings.TrimSpace(argsRaw)
	if cronExpr == "" {
		return "", nil, nil, fmt.Errorf("empty cron")
	}
	return cronExpr, nil, nil, nil
}

func parseOptionalRFC3339(tok string) (t *time.Time, ok bool, err error) {
	if tok == "-" {
		return nil, true, nil
	}
	parsed, e := time.Parse(time.RFC3339, tok)
	if e != nil {
		// Not RFC3339: treat it as part of the cron expression.
		return nil, false, nil
	}
	return &parsed, true, nil
}

func formatTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format(time.RFC3339)
}

func (a *App) cmdSetLocation(ctx context.Context, chatID int64, argsRaw string) {
	if a.subs == nil {
		_ = a.producer.Send(ctx, transport.Message{ChatID: chatID, Text: "no storage configured"})
		return
	}

	parts := strings.Fields(strings.TrimSpace(argsRaw))
	if len(parts) != 2 {
		_ = a.producer.Send(ctx, transport.Message{ChatID: chatID, Text: "usage: /set_location <lat> <lon>"})
		return
	}

	lat, err1 := strconv.ParseFloat(parts[0], 64)
	lon, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil {
		_ = a.producer.Send(ctx, transport.Message{ChatID: chatID, Text: "invalid coordinates; usage: /set_location <lat> <lon>"})
		return
	}

	// Ensure subscription exists.
	if _, err := a.subs.ActiveSubscription(ctx, chatID); err != nil {
		a.logger.Error("failed to ensure subscription", slog.Any("err", err))
	}

	if err := a.subs.SetSubscriptionLocation(ctx, chatID, lat, lon); err != nil {
		a.logger.Error("failed to set location", slog.Any("err", err), slog.Int64("chat_id", chatID))
		_ = a.producer.Send(ctx, transport.Message{ChatID: chatID, Text: "failed to set location"})
		return
	}

	_ = a.producer.Send(ctx, transport.Message{ChatID: chatID, Text: fmt.Sprintf("location set: lat=%v lon=%v", lat, lon)})
}
