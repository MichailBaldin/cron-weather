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

	// Seed default subscription only if DB is empty.
	if len(dbSubs) == 0 && cfg.DefaultSub.ChatID != 0 {
		lat := cfg.DefaultSub.Lat
		lon := cfg.DefaultSub.Lon
		if lat == 0 && lon == 0 {
			lat = cfg.WeatherAPI.Lat
			lon = cfg.WeatherAPI.Lon
		}

		interval := cfg.DefaultSub.Interval
		if interval == 0 {
			interval = cfg.Interval
		}

		defaultSub := subscription.Subscription{
			ChatID:   cfg.DefaultSub.ChatID,
			Interval: interval,
			StartAt:  cfg.DefaultSub.StartAt,
			Lat:      lat,
			Lon:      lon,
		}

		if err := repo.Add(ctx, defaultSub); err == nil {
			cache.Add(defaultSub)
		} else {
			log.Error("failed to add default subscription", "error", err)
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
