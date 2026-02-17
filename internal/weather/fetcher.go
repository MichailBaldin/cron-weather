package weather

import (
	"context"
	"log/slog"

	"cron-weather/internal/scheduler"
	"cron-weather/internal/sender"
)

type Fetcher interface {
	FetchAlerts(ctx context.Context, lat, lon float64) ([]string, error)
}

func NewFetchJob(fetcher Fetcher, sender sender.Sender, lat, lon float64) scheduler.JobFunc {
	return func(ctx context.Context, logger *slog.Logger) {
		logger.Info("weather job started")

		messages, err := fetcher.FetchAlerts(ctx, lat, lon)
		if err != nil {
			if ctx.Err() != nil {
				logger.Warn("weather job cancelled due to shutdown", "reason", ctx.Err())
				return
			}
			logger.Error("failed to fetch alerts", "error", err)
			return
		}

		if len(messages) == 0 {
			logger.Info("no alerts at this moment")
			return
		}

		logger.Info("received alerts", "count", len(messages))

		if err := sender.Send(ctx, messages); err != nil {
			logger.Error("failed to send alerts", "error", err)
			return
		}

		logger.Info("alerts sent successfully")
	}
}
