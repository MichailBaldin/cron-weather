// Package weather contains logic for fetching weather alerts and running scheduled jobs.
package weather

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"time"

	"cron-weather/internal/scheduler"
	"cron-weather/internal/sender"
	"cron-weather/internal/subscription"
)

// Fetcher provides weather alerts for given coordinates.
type Fetcher interface {
	FetchAlerts(ctx context.Context, lat, lon float64) ([]string, error)
}

// NewFetchJobForSubscription builds a scheduled job that fetches alerts and sends them to the user.
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

	// Dedup state is per-subscription job (lives in the closure).
	seen := make(map[string]struct{})

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

			if errors.Is(err, ErrDailyWeatherLimitExceeded) {
				taskLogger.Warn("openweather daily limit exceeded, skipping run", "fetch_dur", fetchDur)
				return
			}

			taskLogger.Error("failed to fetch alerts", "error", err, "fetch_dur", fetchDur)
			return
		}

		if len(messages) == 0 {
			taskLogger.Debug("no alerts", "fetch_dur", fetchDur, "total_dur", time.Since(jobStart))
			return
		}

		// Filter already sent alerts for this subscription (in-memory dedup).
		unique := make([]string, 0, len(messages))
		newFPs := make([]string, 0, len(messages))

		for _, m := range messages {
			fp := fingerprintMessage(m)
			if _, ok := seen[fp]; ok {
				continue
			}
			unique = append(unique, m)
			newFPs = append(newFPs, fp)
		}

		if len(unique) == 0 {
			taskLogger.Debug("all alerts already sent ранее (dedup)", "fetch_dur", fetchDur, "total_dur", time.Since(jobStart))
			return
		}

		sendStart := time.Now()
		err = sdr.Send(ctx, unique)
		sendDur := time.Since(sendStart)

		if err != nil {
			taskLogger.Error("failed to send alerts", "error", err, "fetch_dur", fetchDur, "send_dur", sendDur)
			return
		}

		// Mark alerts as sent only after successful delivery.
		for _, fp := range newFPs {
			seen[fp] = struct{}{}
		}

		taskLogger.Info("alerts sent successfully", "count", len(unique))

		debugFields := []any{
			"count", len(unique),
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

func fingerprintMessage(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}