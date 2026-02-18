package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cron-weather/internal/config"
	"cron-weather/internal/scheduler"
	"cron-weather/internal/sender"
	"cron-weather/internal/storage"
	"cron-weather/internal/subscription"
	"cron-weather/internal/weather"
	"cron-weather/pkg/logger"

	"github.com/joho/godotenv"
)

func main() {
	appStart := time.Now()
	_ = godotenv.Load()

	cfg := config.MustLoad()
	log := logger.SetupLogger(cfg.Env)

	log.Info("start weather cron-job",
		slog.String("version", "0.1.0"),
		slog.Duration("since_start", time.Since(appStart)),
	)

	repo, err := storage.NewSQLiteRepository(cfg.DBPath)
	if err != nil {
		log.Error("failed to connect to SQLite", "error", err)
		os.Exit(1)
	}
	defer repo.Close()

	log.Debug("sqlite connected", slog.Duration("since_start", time.Since(appStart)))

	ctx := context.Background()
	dbSubs, err := repo.GetAll(ctx)
	if err != nil {
		log.Error("failed to load subscriptions from DB", "error", err)
		os.Exit(1)
	}

	cache := storage.NewCache()
	cache.LoadAll(dbSubs)

	log.Debug("loaded subscriptions from DB",
		"count", len(dbSubs),
		slog.Duration("since_start", time.Since(appStart)),
	)

	if len(dbSubs) == 0 && cfg.DefaultSub.ChatID != 0 {
		log.Info("no subscriptions found, creating default subscription from env", "chat_id", cfg.DefaultSub.ChatID)

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

		if err := repo.Add(ctx, defaultSub); err != nil {
			log.Error("failed to add default subscription", "error", err)
		} else {
			cache.Add(defaultSub)
			log.Info("default subscription added", "chat_id", defaultSub.ChatID)
		}
	}

	dailyLimiter := weather.NewDailyLimiter(1000, time.UTC)

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
		job, err := weather.NewFetchJobForSubscriptionWithMetrics(fetcher, sub, senderFactory, repo, log, appStart)
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

		go func(s *scheduler.CronService) {
			if err := s.Start(); err != nil && err.Error() != "cron service already running" {
				log.Error("cron service stopped with error", "error", err)
			}
		}(svc)
	}

	log.Debug("cron services started",
		"count", len(services),
		slog.Duration("since_start", time.Since(appStart)),
	)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Info("shutting down...")

	for _, svc := range services {
		if err := svc.Shutdown(10 * time.Second); err != nil {
			log.Error("shutdown error", "error", err)
		}
	}

	log.Info("all services stopped")
}
