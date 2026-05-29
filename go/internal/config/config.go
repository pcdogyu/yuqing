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
	AnalysisInterval time.Duration
	UserAgent        string
	LogLevel         string
	ServiceToken     string
	SessionTTL       time.Duration

	GatewayWebAddr string
	AuthAddr       string
	ContentAddr    string
	CrawlerAddr    string
	AnalysisAddr   string
	NLPAddr        string
	SchedulerAddr  string

	GatewayWebURL string
	AuthURL       string
	ContentURL    string
	CrawlerURL    string
	AnalysisURL   string
	NLPURL        string

	DefaultAdminUser string
	DefaultAdminPass string
}

func Load() Config {
	dbPath := firstEnv("YUQING_DB_PATH", "JIN10_DB_PATH")
	if dbPath == "" {
		dbPath = filepath.Join("data", "yuqing.db")
	}

	return Config{
		ListenAddr:       envOrDefault("YUQING_LISTEN_ADDR", ":8090"),
		DatabasePath:     dbPath,
		FlashURL:         envOrDefaultWithAliases("YUQING_FLASH_URL", "https://www.jin10.com/", "JIN10_FLASH_URL"),
		HeadlineURL:      envOrDefaultWithAliases("YUQING_HEADLINE_URL", "https://xnews.jin10.com/", "JIN10_HEADLINE_URL"),
		HTTPTimeout:      envDurationSeconds(20, "YUQING_HTTP_TIMEOUT_SEC", "JIN10_HTTP_TIMEOUT_SEC"),
		FlashInterval:    envDurationSeconds(15, "YUQING_FLASH_INTERVAL_SEC", "JIN10_FLASH_INTERVAL_SEC"),
		HeadlineInterval: envDurationSeconds(60, "YUQING_HEADLINE_INTERVAL_SEC", "JIN10_HEADLINE_INTERVAL_SEC"),
		AnalysisInterval: envDurationSeconds(120, "YUQING_ANALYSIS_INTERVAL_SEC"),
		UserAgent:        envOrDefaultWithAliases("YUQING_USER_AGENT", defaultUserAgent, "JIN10_USER_AGENT"),
		LogLevel:         envOrDefaultWithAliases("YUQING_LOG_LEVEL", "info", "JIN10_LOG_LEVEL"),
		ServiceToken:     envOrDefaultWithAliases("YUQING_SERVICE_TOKEN", "stonedt-internal-token", "JIN10_SERVICE_TOKEN"),
		SessionTTL:       envDurationSeconds(86400, "YUQING_SESSION_TTL_SEC", "JIN10_SESSION_TTL_SEC"),

		GatewayWebAddr: envOrDefaultWithAliases("YUQING_GATEWAY_ADDR", ":80", "JIN10_PORTAL_WEB_ADDR"),
		AuthAddr:       envOrDefault("YUQING_AUTH_ADDR", ":8081"),
		ContentAddr:    envOrDefaultWithAliases("YUQING_CONTENT_ADDR", ":8082", "JIN10_CONTENT_ADDR"),
		CrawlerAddr:    envOrDefaultWithAliases("YUQING_CRAWLER_ADDR", ":8083", "JIN10_CRAWLER_ADDR"),
		AnalysisAddr:   envOrDefault("YUQING_ANALYSIS_ADDR", ":8084"),
		NLPAddr:        envOrDefaultWithAliases("YUQING_NLP_ADDR", ":8085", "JIN10_NLP_ADDR"),
		SchedulerAddr:  envOrDefaultWithAliases("YUQING_SCHEDULER_ADDR", ":8086", "JIN10_SCHEDULER_ADDR"),

		GatewayWebURL: envOrDefaultWithAliases("YUQING_GATEWAY_URL", "http://127.0.0.1", "JIN10_PORTAL_WEB_URL"),
		AuthURL:       envOrDefault("YUQING_AUTH_URL", "http://127.0.0.1:8081"),
		ContentURL:    envOrDefaultWithAliases("YUQING_CONTENT_URL", "http://127.0.0.1:8082", "JIN10_CONTENT_URL"),
		CrawlerURL:    envOrDefaultWithAliases("YUQING_CRAWLER_URL", "http://127.0.0.1:8083", "JIN10_CRAWLER_URL"),
		AnalysisURL:   envOrDefault("YUQING_ANALYSIS_URL", "http://127.0.0.1:8084"),
		NLPURL:        envOrDefaultWithAliases("YUQING_NLP_URL", "http://127.0.0.1:8085", "JIN10_NLP_URL"),

		DefaultAdminUser: envOrDefaultWithAliases("YUQING_DEFAULT_ADMIN_USER", "admin", "JIN10_DEFAULT_ADMIN_USER"),
		DefaultAdminPass: envOrDefaultWithAliases("YUQING_DEFAULT_ADMIN_PASS", "admin123", "JIN10_DEFAULT_ADMIN_PASS"),
	}
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envOrDefaultWithAliases(key, fallback string, aliases ...string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	for _, alias := range aliases {
		if value := os.Getenv(alias); value != "" {
			return value
		}
	}
	return fallback
}

func envDurationSeconds(fallback int, keys ...string) time.Duration {
	raw := strconv.Itoa(fallback)
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			raw = value
			break
		}
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		seconds = fallback
	}
	return time.Duration(seconds) * time.Second
}
