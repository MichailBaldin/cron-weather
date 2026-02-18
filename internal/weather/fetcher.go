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
	"cron-weather/internal/storage"
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
	// No persistent dedup repository by default.
	return NewFetchJobForSubscriptionWithMetrics(fetcher, sub, senderFactory, nil, logger, time.Time{})
}

// NewFetchJobForSubscriptionWithMetrics is the same job builder but allows passing a startedAt timestamp
// for metrics logging and an optional repository for persistent deduplication.
func NewFetchJobForSubscriptionWithMetrics(
	fetcher Fetcher,
	sub subscription.Subscription,
	senderFactory func(chatID int64) (sender.Sender, error),
	sentRepo storage.SentAlertsRepository,
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

		// Filter already sent alerts for this subscription (persistent dedup if repo is provided).
		unique := make([]string, 0, len(messages))
		fps := make([]string, 0, len(messages))

		for _, m := range messages {
			fp := fingerprintMessage(m)

			if sentRepo != nil {
				ok, err := sentRepo.WasSent(ctx, sub.ChatID, fp)
				if err != nil {
					taskLogger.Error("failed to check dedup state", "error", err)
					return
				}
				if ok {
					continue
				}
			}

			unique = append(unique, m)
			fps = append(fps, fp)
		}

		if len(unique) == 0 {
			taskLogger.Debug("all alerts already sent (dedup)", "fetch_dur", fetchDur, "total_dur", time.Since(jobStart))
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
		if sentRepo != nil {
			now := time.Now()
			for _, fp := range fps {
				if err := sentRepo.MarkSent(ctx, sub.ChatID, fp, now); err != nil {
					taskLogger.Error("failed to mark alert as sent", "error", err)
					// Do not return: delivery succeeded, marking can be retried next time if needed.
				}
			}
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
