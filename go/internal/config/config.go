package config

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"

type Config struct {
	ListenAddr       string
	DatabasePath     string
	FlashURL         string
	HeadlineURL      string
	HTTPTimeout      time.Duration
	FlashInterval    time.Duration
	HeadlineInterval time.Duration
	UserAgent        string
	LogLevel         string
}

func Load() Config {
	dbPath := envOrDefault("JIN10_DB_PATH", filepath.Join("data", "jin10.db"))

	return Config{
		ListenAddr:       envOrDefault("JIN10_LISTEN_ADDR", ":8090"),
		DatabasePath:     dbPath,
		FlashURL:         envOrDefault("JIN10_FLASH_URL", "https://www.jin10.com/"),
		HeadlineURL:      envOrDefault("JIN10_HEADLINE_URL", "https://xnews.jin10.com/"),
		HTTPTimeout:      envDurationSeconds("JIN10_HTTP_TIMEOUT_SEC", 20),
		FlashInterval:    envDurationSeconds("JIN10_FLASH_INTERVAL_SEC", 15),
		HeadlineInterval: envDurationSeconds("JIN10_HEADLINE_INTERVAL_SEC", 60),
		UserAgent:        envOrDefault("JIN10_USER_AGENT", defaultUserAgent),
		LogLevel:         envOrDefault("JIN10_LOG_LEVEL", "info"),
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envDurationSeconds(key string, fallback int) time.Duration {
	raw := envOrDefault(key, strconv.Itoa(fallback))
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		seconds = fallback
	}
	return time.Duration(seconds) * time.Second
}
