package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cron-weather/internal/app"
	"cron-weather/internal/config"
	"cron-weather/pkg/logger"

	"github.com/joho/godotenv"
)

func main() {
	appStart := time.Now()
	_ = godotenv.Load()

	cfg := config.MustLoad()
	log := logger.SetupLogger(cfg.Env)

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Warn("invalid TIMEZONE, fallback to UTC", "timezone", cfg.Timezone, "error", err)
		loc = time.UTC
	}
	time.Local = loc

	log.Info("start weather cron-job",
		slog.String("version", "0.1.0"),
		slog.String("timezone", loc.String()),
		slog.Duration("since_start", time.Since(appStart)),
	)

	a, err := app.New(cfg, log, appStart)
	if err != nil {
		log.Error("failed to init app", "error", err)
		os.Exit(1)
	}

	a.Run()
	log.Debug("cron services started", slog.Duration("since_start", time.Since(appStart)))

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Info("shutting down...")
	a.Shutdown(10 * time.Second)
	log.Info("all services stopped")
}
