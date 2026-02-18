// Package app wires application components together and manages lifecycle.
package app

import (
	"context"
	"log/slog"
	"time"

	"cron-weather/internal/config"
	"cron-weather/internal/scheduler"
	"cron-weather/internal/sender"
	"cron-weather/internal/storage"
	"cron-weather/internal/subscription"
	"cron-weather/internal/weather"
)

// App holds initialized dependencies and running services.
type App struct {
	log      *slog.Logger
	repo     *storage.SQLiteRepository
	cache    *storage.Cache
	services []*scheduler.CronService
}

// New builds the application with all dependencies.
// It loads subscriptions, optionally seeds default subscription, and prepares cron services.
func New(cfg *config.Config, log *slog.Logger, startedAt time.Time) (*App, error) {
	ctx := context.Background()

	repo, err := storage.NewSQLiteRepository(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	dbSubs, err := repo.GetAll(ctx)
	if err != nil {
		_ = repo.Close()
		return nil, err
	}

	cache := storage.NewCache()
	cache.LoadAll(dbSubs)

	if len(dbSubs) == 0 {
		// Build a single seed subscription from config.
		// Primary config has priority, DefaultSub is only a fallback.
		seedChatID := cfg.SenderAPI.ChatID
		if seedChatID == 0 {
			seedChatID = cfg.DefaultSub.ChatID
		}

		if seedChatID != 0 {
			interval := cfg.Interval
			if interval == 0 {
				interval = cfg.DefaultSub.Interval
			}

			startAt := cfg.StartAt
			if startAt == "" {
				startAt = cfg.DefaultSub.StartAt
			}

			lat, lon := cfg.WeatherAPI.Lat, cfg.WeatherAPI.Lon
			if lat == 0 && lon == 0 {
				lat, lon = cfg.DefaultSub.Lat, cfg.DefaultSub.Lon
			}

			seedSub := subscription.Subscription{
				ChatID:   seedChatID,
				Interval: interval,
				StartAt:  startAt,
				Lat:      lat,
				Lon:      lon,
			}

			log.Info("no subscriptions found, seeding subscription from config", "chat_id", seedSub.ChatID)

			if err := repo.Add(ctx, seedSub); err != nil {
				log.Error("failed to add seed subscription", "error", err)
			} else {
				cache.Add(seedSub)
				log.Info("seed subscription added", "chat_id", seedSub.ChatID)
			}
		} else {
			log.Warn("no subscriptions found and no chat_id configured; service will idle")
		}
	}

	// Timezone is already applied in main via time.Local; limiter uses same location.
	loc := time.Local
	dailyLimiter := weather.NewDailyLimiter(cfg.DailyLimit, loc)

	fetcher := weather.NewOpenWeatherFetcher(cfg.WeatherAPI.APIKey, cfg.WeatherAPI.HTTPTimeout).
		SetDailyLimiter(dailyLimiter)

	senderFactory := func(chatID int64) (sender.Sender, error) {
		return sender.NewTelegramSender(config.Telegram{
			Token:  cfg.SenderAPI.Token,
			ChatID: chatID,
		})
	}

	var services []*scheduler.CronService
	for _, sub := range cache.GetAll() {
		job, err := weather.NewFetchJobForSubscriptionWithMetrics(fetcher, sub, senderFactory, repo, log, startedAt)
		if err != nil {
			log.Error("failed to create job for subscription", "chat_id", sub.ChatID, "error", err)
			continue
		}

		svc, err := scheduler.NewCronService(sub.Interval, sub.StartAt, job, log)
		if err != nil {
			log.Error("failed to create cron service for subscription", "chat_id", sub.ChatID, "error", err)
			continue
		}

		services = append(services, svc)
	}

	return &App{
		log:      log,
		repo:     repo,
		cache:    cache,
		services: services,
	}, nil
}

// Run starts all cron services. It returns immediately (services run in goroutines).
func (a *App) Run() {
	for _, svc := range a.services {
		s := svc
		go func() {
			if err := s.Start(); err != nil && err.Error() != "cron service already running" {
				a.log.Error("cron service stopped with error", "error", err)
			}
		}()
	}
}

// Shutdown stops all services and closes repository.
func (a *App) Shutdown(timeout time.Duration) {
	for _, svc := range a.services {
		if err := svc.Shutdown(timeout); err != nil {
			a.log.Error("shutdown error", "error", err)
		}
	}
	if err := a.repo.Close(); err != nil {
		a.log.Error("failed to close repository", "error", err)
	}
}
