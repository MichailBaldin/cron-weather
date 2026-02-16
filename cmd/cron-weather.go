package main

import (
	"log/slog"

	"cron-weather/internal/config"
	"cron-weather/pkg/logger"
)

func main() {
	cfg := config.MustLoad()

	log := logger.SetupLogger(cfg.Env)

	log.Info("start weather cron-job",
		slog.String("version", "0.1.0"))

	log.Debug("debug message are available")
}
