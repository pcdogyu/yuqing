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
	ServiceToken     string
	SessionTTL       time.Duration
	PortalWebAddr    string
	ContentAddr      string
	CrawlerAddr      string
	NLPAddr          string
	SchedulerAddr    string
	PortalWebURL     string
	ContentURL       string
	CrawlerURL       string
	NLPURL           string
	DefaultAdminUser string
	DefaultAdminPass string
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
		ServiceToken:     envOrDefault("JIN10_SERVICE_TOKEN", "stonedt-internal-token"),
		SessionTTL:       envDurationSeconds("JIN10_SESSION_TTL_SEC", 86400),
		PortalWebAddr:    envOrDefault("JIN10_PORTAL_WEB_ADDR", ":8080"),
		ContentAddr:      envOrDefault("JIN10_CONTENT_ADDR", ":8081"),
		CrawlerAddr:      envOrDefault("JIN10_CRAWLER_ADDR", ":8082"),
		NLPAddr:          envOrDefault("JIN10_NLP_ADDR", ":8083"),
		SchedulerAddr:    envOrDefault("JIN10_SCHEDULER_ADDR", ":8085"),
		PortalWebURL:     envOrDefault("JIN10_PORTAL_WEB_URL", "http://127.0.0.1:8080"),
		ContentURL:       envOrDefault("JIN10_CONTENT_URL", "http://127.0.0.1:8081"),
		CrawlerURL:       envOrDefault("JIN10_CRAWLER_URL", "http://127.0.0.1:8082"),
		NLPURL:           envOrDefault("JIN10_NLP_URL", "http://127.0.0.1:8083"),
		DefaultAdminUser: envOrDefault("JIN10_DEFAULT_ADMIN_USER", "admin"),
		DefaultAdminPass: envOrDefault("JIN10_DEFAULT_ADMIN_PASS", "admin123"),
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
