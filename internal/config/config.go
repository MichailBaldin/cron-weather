// Package config loads and validates application configuration from environment variables.
package config

import (
	"log"
	"net/url"
	"os"
	"strconv"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// Config contains application configuration loaded from environment variables.
type Config struct {
	Env string `env:"ENV" envDefault:"prod"`

	TgBot       TgBotConfig       `envPrefix:"TG_"`
	Postgres    PostgressConfig   `envPrefix:"PG_"`
	OpenWeather OpenWeatherConfig `envPrefix:"OWM_"`
}

// TgBotConfig contains Telegram Bot API configuration.
type TgBotConfig struct {
	BotToken string `env:"BOT_TOKEN,required"`
	Debug    bool   `env:"DEBUG" envDefault:"false"`
}

// PostgressConfig contains PostgreSQL connection settings.
type PostgressConfig struct {
	Host     string `env:"HOST" envDefault:"postgres"`
	Port     int    `env:"PORT" envDefault:"5432"`
	User     string `env:"USER" envDefault:"app"`
	Password string `env:"PASSWORD" envDefault:"app"`
	DBName   string `env:"DB" envDefault:"app"`
	SSLMode  string `env:"SSLMODE" envDefault:"disable"`
}

// DSN returns a PostgreSQL DSN built from the config fields.
func (p PostgressConfig) DSN() string {
	u := &url.URL{Scheme: "postgres"}
	if p.User != "" {
		u.User = url.UserPassword(p.User, p.Password)
	}
	u.Host = p.Host + ":" + strconv.Itoa(p.Port)
	u.Path = "/" + p.DBName
	q := u.Query()
	q.Set("sslmode", p.SSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}

// OpenWeatherConfig contains OpenWeather API configuration.
type OpenWeatherConfig struct {
	APIKey     string `env:"API_KEY,required"`
	DailyLimit int    `env:"DAILY_LIMIT" envDefault:"1000"`
}

// MustLoad loads configuration from .env (outside Docker) and the process environment.
// It terminates the process on error.
func MustLoad() *Config {
	if os.Getenv("IN_DOCKER") == "" {
		if err := godotenv.Load(); err != nil {
			log.Fatalf("failed to load env file: %v", err)
		}
	}

	var cfg Config

	if err := env.Parse(&cfg); err != nil {
		log.Fatalf("failed to read env file: %v", err)
	}

	return &cfg
}


