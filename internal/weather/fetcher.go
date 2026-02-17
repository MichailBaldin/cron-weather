package weather

import (
	"context"
	"fmt"
	"log/slog"

	"cron-weather/internal/scheduler"
)

type Fetcher interface {
	FetchAlerts(ctx context.Context, lat, lon float64) ([]string, error)
}

func NewFetchJob(fetcher Fetcher, lat, lon float64) scheduler.JobFunc {
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
		for i, msg := range messages {
			logger.Info(fmt.Sprintf("alert #%d", i+1), "message", msg)
		}
	}
}
