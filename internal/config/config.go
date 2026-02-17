package config

import (
	"log"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env      string        `env:"ENV"`
	Interval time.Duration `env:"INTERVAL"`
	StartAt  string        `env:"START_AT"`
}

func MustLoad() *Config {
	var cfg Config

	if err := cleanenv.ReadEnv(&cfg); err != nil {
		log.Fatalf("read env config: %v", err)
	}

	return &cfg
}
