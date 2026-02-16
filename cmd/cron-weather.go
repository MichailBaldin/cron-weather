package main

import "cron-weather/internal/config"

func main() {
	cfg := config.MustLoad()

	_ = cfg
}
