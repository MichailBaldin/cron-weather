package weather

import (
	"context"
	"log/slog"

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
	sdr, err := senderFactory(sub.ChatID)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, taskLogger *slog.Logger) {
		taskLogger.Info("weather job started", "chat_id", sub.ChatID)

		messages, err := fetcher.FetchAlerts(ctx, sub.Lat, sub.Lon) // используем координаты из подписки
		if err != nil {
			if ctx.Err() != nil {
				taskLogger.Warn("job cancelled due to shutdown", "reason", ctx.Err())
				return
			}
			taskLogger.Error("failed to fetch alerts", "error", err)
			return
		}
		if len(messages) == 0 {
			taskLogger.Info("no alerts")
			return
		}
		if err := sdr.Send(ctx, messages); err != nil {
			taskLogger.Error("failed to send alerts", "error", err)
			return
		}
		taskLogger.Info("alerts sent successfully", "count", len(messages))
	}, nil
}
