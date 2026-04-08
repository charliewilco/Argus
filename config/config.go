package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

const (
	defaultDatabaseURL = "sqlite:./argus.db"
	defaultPort        = 8080
)

type Config struct {
	BaseURL     string
	DatabaseURL string
	Port        int
	RedisURL    string
	SecretKey   string
}

func Load() (Config, error) {
	_ = godotenv.Load()

	port := defaultPort
	if raw := os.Getenv("ARGUS_PORT"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("config.Load: parse ARGUS_PORT: %w", err)
		}
		port = parsed
	}

	cfg := Config{
		BaseURL:     os.Getenv("ARGUS_BASE_URL"),
		DatabaseURL: valueOrDefault(os.Getenv("ARGUS_DATABASE_URL"), defaultDatabaseURL),
		Port:        port,
		RedisURL:    os.Getenv("ARGUS_REDIS_URL"),
		SecretKey:   os.Getenv("ARGUS_SECRET_KEY"),
	}

	if cfg.SecretKey == "" {
		return Config{}, errors.New("config.Load: ARGUS_SECRET_KEY is required")
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = fmt.Sprintf("http://localhost:%d", cfg.Port)
	}

	return cfg, nil
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}
