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
	TenantID    string
	GitHub      GitHubConfig
}

type GitHubConfig struct {
	ClientID      string
	ClientSecret  string
	WebhookSecret string
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
		TenantID:    valueOrDefault(os.Getenv("ARGUS_TENANT_ID"), "default"),
		GitHub: GitHubConfig{
			ClientID:      os.Getenv("GITHUB_CLIENT_ID"),
			ClientSecret:  os.Getenv("GITHUB_CLIENT_SECRET"),
			WebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"),
		},
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
