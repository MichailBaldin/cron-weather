package config

import (
	"log"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env        string        `env:"ENV"`
	Interval   time.Duration `env:"INTERVAL" default:"30s"`
	StartAt    string        `env:"START_AT"`
	WeatherAPI Client
}

type Client struct {
	APIKey      string        `env:"WEATHER_API_KEY" required:"true"`
	Lat         float64       `env:"LAT"`
	Lon         float64       `env:"LON"`
	HTTPTimeout time.Duration `env:"HTTP_TIMEOUT" default:"10s"`
}

func MustLoad() *Config {
	var cfg Config

	if err := cleanenv.ReadEnv(&cfg); err != nil {
		log.Fatalf("read env config: %v", err)
	}

	return &cfg
}
