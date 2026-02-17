package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cron-weather/internal/config"
	"cron-weather/internal/scheduler"
	"cron-weather/pkg/logger"

	"github.com/joho/godotenv"
)

func exampleJob(ctx context.Context, logger *slog.Logger) {
	logger.Info("job started")

	if err := doWork(ctx); err != nil {
		wrappedErr := fmt.Errorf("proccessing data: %w", err)

		logger.Error("job failed", "error", wrappedErr)

		return
	}
}

var ErrInvalidState = errors.New("invalid state")

func doWork(ctx context.Context) error {
	select {
	case <-time.After(2 * time.Second):
		return fmt.Errorf("validation failed: %w", ErrInvalidState)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func main() {
	_ = godotenv.Load()

	cfg := config.MustLoad()
	log := logger.SetupLogger(cfg.Env)

	log.Info("start weather cron-job", slog.String("version", "0.1.0"))

	service, err := scheduler.NewCronService(cfg.Interval, cfg.StartAt, exampleJob, log)
	if err != nil {
		log.Error("failed to create cron service", "error", err)
		os.Exit(1)
	}

	if first := service.FirstRun(); first != nil {
		log.Info("first run scheduled", "at", first.Format(time.RFC3339))
	}

	go func() {
		if err := service.Start(); err != nil {
			log.Error("failed to start cron service", "error", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigCh
	log.Info("received signal", "signal", sig)

	if err := service.Shutdown(30 * time.Second); err != nil {
		log.Error("shutdown error", "error", err)
	}

	log.Info("cron service stop")
}
