package weather

import (
	"context"
	"log/slog"
	"time"

	"cron-weather/internal/scheduler"
	"cron-weather/internal/sender"
	"cron-weather/internal/subscription"
)

type Fetcher interface {
	FetchAlerts(ctx context.Context, lat, lon float64) ([]string, error)
}

func NewFetchJobForSubscription(
	fetcher Fetcher,
	sub subscription.Subscription,
	senderFactory func(chatID int64) (sender.Sender, error),
	logger *slog.Logger,
) (scheduler.JobFunc, error) {
	return NewFetchJobForSubscriptionWithMetrics(fetcher, sub, senderFactory, logger, time.Time{})
}


func NewFetchJobForSubscriptionWithMetrics(
	fetcher Fetcher,
	sub subscription.Subscription,
	senderFactory func(chatID int64) (sender.Sender, error),
	logger *slog.Logger,
	startedAt time.Time,
) (scheduler.JobFunc, error) {
	sdr, err := senderFactory(sub.ChatID)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, taskLogger *slog.Logger) {
		jobStart := time.Now()
		taskLogger.Debug("weather job started", "chat_id", sub.ChatID)

		fetchStart := time.Now()
		messages, err := fetcher.FetchAlerts(ctx, sub.Lat, sub.Lon)
		fetchDur := time.Since(fetchStart)

		if err != nil {
			if ctx.Err() != nil {
				taskLogger.Warn("job cancelled due to shutdown", "reason", ctx.Err(), "fetch_dur", fetchDur)
				return
			}
			taskLogger.Error("failed to fetch alerts", "error", err, "fetch_dur", fetchDur)
			return
		}

		if len(messages) == 0 {
			taskLogger.Debug("no alerts", "fetch_dur", fetchDur, "total_dur", time.Since(jobStart))
			return
		}

		sendStart := time.Now()
		err = sdr.Send(ctx, messages)
		sendDur := time.Since(sendStart)

		if err != nil {
			taskLogger.Error("failed to send alerts", "error", err, "fetch_dur", fetchDur, "send_dur", sendDur)
			return
		}

		taskLogger.Info("alerts sent successfully", "count", len(messages))

		debugFields := []any{
			"count", len(messages),
			"fetch_dur", fetchDur,
			"send_dur", sendDur,
			"total_dur", time.Since(jobStart),
		}
		if !startedAt.IsZero() {
			debugFields = append(debugFields, "startup_to_publish", time.Since(startedAt))
		}
		taskLogger.Debug("alerts sent successfully (metrics)", debugFields...)
	}, nil
}