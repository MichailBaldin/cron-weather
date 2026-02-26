package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"cron-weather/internal/app"
	"cron-weather/internal/config"
	"cron-weather/internal/storage/postgres"
	"cron-weather/internal/transport/telegram"
	"cron-weather/pkg/logger"
)

func main() {
	cfg := config.MustLoad()
	log := logger.SetupLogger(cfg.Env)

	log.Info("start cron-weather service",
		slog.String("version", "0.2.0"))

	log.Debug("debug messages is available")

	tg, err := telegram.NewTelegramBot(cfg, log)
	if err != nil {
		log.Error("failed to init telegram bot", slog.Any("err", err))
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	repo, err := postgres.New(ctx, cfg.Postgres.DSN())
	if err != nil {
		log.Error("failed to init postgres", slog.Any("err", err))
		os.Exit(1)
	}
	defer repo.Close()

	a := app.New(log, repo, tg, tg, cfg)

	if err := a.Start(ctx); err != nil {
		log.Error("app stopped with error", slog.Any("err", err))
		os.Exit(1)
	}
}
