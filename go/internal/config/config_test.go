package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("YUQING_DB_PATH", "")
	t.Setenv("JIN10_DB_PATH", "")
	t.Setenv("YUQING_LISTEN_ADDR", "")
	t.Setenv("YUQING_FLASH_URL", "")
	t.Setenv("JIN10_FLASH_URL", "")
	t.Setenv("YUQING_HEADLINE_URL", "")
	t.Setenv("JIN10_HEADLINE_URL", "")
	t.Setenv("YUQING_CRYPTO_X_URL", "")
	t.Setenv("YUQING_CRYPTO_X_TOKEN", "")
	t.Setenv("YUQING_CRYPTO_TELEGRAM_URL", "")
	t.Setenv("YUQING_CRYPTO_TELEGRAM_TOKEN", "")
	t.Setenv("YUQING_JIN10_FULL_BACKFILL_DAYS", "")
	t.Setenv("YUQING_JIN10_FULL_MAX_PAGES_PER_RUN", "")
	t.Setenv("YUQING_JIN10_FULL_RATE_LIMIT_MS", "")
	t.Setenv("YUQING_JIN10_FULL_INCLUDE_SITEMAP", "")
	t.Setenv("YUQING_JIN10_FULL_ENABLED", "")
	t.Setenv("YUQING_JIN10_FULL_INTERVAL_SEC", "")
	t.Setenv("YUQING_HTTP_TIMEOUT_SEC", "")
	t.Setenv("YUQING_SCHEDULER_CRAWL_TIMEOUT_SEC", "")
	t.Setenv("JIN10_HTTP_TIMEOUT_SEC", "")
	t.Setenv("YUQING_EXTERNAL_RETRY_COUNT", "")
	t.Setenv("YUQING_EXTERNAL_RETRY_WAIT_MS", "")
	t.Setenv("YUQING_CRYPTO_SOCIAL_RATE_LIMIT_MS", "")
	t.Setenv("YUQING_CRYPTO_X_INTERVAL_SEC", "")
	t.Setenv("YUQING_CRYPTO_TELEGRAM_INTERVAL_SEC", "")
	t.Setenv("YUQING_LOG_LEVEL", "")
	t.Setenv("JIN10_LOG_LEVEL", "")

	cfg := Load()

	if cfg.DatabasePath != filepath.Join("data", "yuqing.db") {
		t.Fatalf("expected default database path, got %q", cfg.DatabasePath)
	}
	if cfg.ListenAddr != ":8090" {
		t.Fatalf("expected default listen addr, got %q", cfg.ListenAddr)
	}
	if cfg.GatewayWebAddr != ":80" {
		t.Fatalf("expected default gateway addr, got %q", cfg.GatewayWebAddr)
	}
	if cfg.FlashURL != "https://www.jin10.com/" {
		t.Fatalf("expected default flash url, got %q", cfg.FlashURL)
	}
	if cfg.HeadlineURL != "https://xnews.jin10.com/" {
		t.Fatalf("expected default headline url, got %q", cfg.HeadlineURL)
	}
	if cfg.CoinLoreURL != "https://api.coinlore.net" || cfg.CoinGeckoURL != "https://api.coingecko.com/api/v3" {
		t.Fatalf("expected default price source urls, got coinlore=%q coingecko=%q", cfg.CoinLoreURL, cfg.CoinGeckoURL)
	}
	if cfg.CryptoXURL != "" || cfg.CryptoTelegramURL != "" {
		t.Fatalf("expected empty crypto social urls by default, got x=%q tg=%q", cfg.CryptoXURL, cfg.CryptoTelegramURL)
	}
	if cfg.Jin10FullBackfillDays != 30 || cfg.Jin10FullMaxPages != 20 {
		t.Fatalf("expected default jin10 full backfill/pages, got days=%d pages=%d", cfg.Jin10FullBackfillDays, cfg.Jin10FullMaxPages)
	}
	if cfg.Jin10FullRateLimit != 800*time.Millisecond || !cfg.Jin10FullIncludeSitemap || cfg.Jin10FullEnabled {
		t.Fatalf("unexpected jin10 full defaults: rate=%s sitemap=%v enabled=%v", cfg.Jin10FullRateLimit, cfg.Jin10FullIncludeSitemap, cfg.Jin10FullEnabled)
	}
	if cfg.HTTPTimeout != 20*time.Second {
		t.Fatalf("expected default http timeout, got %s", cfg.HTTPTimeout)
	}
	if cfg.SchedulerCrawlTimeout != 120*time.Second {
		t.Fatalf("expected default scheduler crawl timeout, got %s", cfg.SchedulerCrawlTimeout)
	}
	if cfg.ExternalRetryCount != 2 || cfg.ExternalRetryWait != 500*time.Millisecond || cfg.CryptoSocialRateLimit != 0 {
		t.Fatalf("unexpected external defaults: retries=%d wait=%s social_rate=%s", cfg.ExternalRetryCount, cfg.ExternalRetryWait, cfg.CryptoSocialRateLimit)
	}
	if cfg.CryptoXInterval != 90*time.Second || cfg.CryptoTelegramInterval != 90*time.Second {
		t.Fatalf("expected default social intervals, got x=%s tg=%s", cfg.CryptoXInterval, cfg.CryptoTelegramInterval)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("expected default log level, got %q", cfg.LogLevel)
	}
	if cfg.GatewayWebURL != "http://127.0.0.1" {
		t.Fatalf("expected default gateway url, got %q", cfg.GatewayWebURL)
	}
}

func TestLoadPrefersPrimaryAndAliasEnv(t *testing.T) {
	t.Setenv("YUQING_DB_PATH", "")
	t.Setenv("JIN10_DB_PATH", "alias.db")
	t.Setenv("YUQING_FLASH_URL", "")
	t.Setenv("JIN10_FLASH_URL", "https://flash.example.com")
	t.Setenv("YUQING_HEADLINE_URL", "https://headline.example.com")
	t.Setenv("JIN10_HEADLINE_URL", "https://ignored.example.com")
	t.Setenv("YUQING_COINLORE_URL", "https://coinlore.example.com")
	t.Setenv("YUQING_COINGECKO_URL", "https://coingecko.example.com/api/v3")
	t.Setenv("YUQING_CRYPTO_X_URL", "https://social.example.com/x")
	t.Setenv("YUQING_CRYPTO_X_TOKEN", "x-token")
	t.Setenv("YUQING_CRYPTO_TELEGRAM_URL", "https://social.example.com/tg")
	t.Setenv("YUQING_CRYPTO_TELEGRAM_TOKEN", "tg-token")
	t.Setenv("YUQING_HTTP_TIMEOUT_SEC", "45")
	t.Setenv("YUQING_SCHEDULER_CRAWL_TIMEOUT_SEC", "180")
	t.Setenv("YUQING_EXTERNAL_RETRY_COUNT", "3")
	t.Setenv("YUQING_EXTERNAL_RETRY_WAIT_MS", "750")
	t.Setenv("YUQING_CRYPTO_SOCIAL_RATE_LIMIT_MS", "1200")
	t.Setenv("JIN10_HTTP_TIMEOUT_SEC", "99")
	t.Setenv("YUQING_FLASH_INTERVAL_SEC", "")
	t.Setenv("JIN10_FLASH_INTERVAL_SEC", "33")
	t.Setenv("YUQING_JIN10_FULL_BACKFILL_DAYS", "90")
	t.Setenv("YUQING_JIN10_FULL_MAX_PAGES_PER_RUN", "12")
	t.Setenv("YUQING_JIN10_FULL_RATE_LIMIT_MS", "250")
	t.Setenv("YUQING_JIN10_FULL_INCLUDE_SITEMAP", "false")
	t.Setenv("YUQING_JIN10_FULL_ENABLED", "true")
	t.Setenv("YUQING_JIN10_FULL_INTERVAL_SEC", "600")
	t.Setenv("YUQING_CRYPTO_X_INTERVAL_SEC", "77")
	t.Setenv("YUQING_CRYPTO_TELEGRAM_INTERVAL_SEC", "88")

	cfg := Load()

	if cfg.DatabasePath != "alias.db" {
		t.Fatalf("expected alias db path, got %q", cfg.DatabasePath)
	}
	if cfg.FlashURL != "https://flash.example.com" {
		t.Fatalf("expected alias flash url, got %q", cfg.FlashURL)
	}
	if cfg.HeadlineURL != "https://headline.example.com" {
		t.Fatalf("expected primary headline url, got %q", cfg.HeadlineURL)
	}
	if cfg.CoinLoreURL != "https://coinlore.example.com" || cfg.CoinGeckoURL != "https://coingecko.example.com/api/v3" {
		t.Fatalf("expected price source urls loaded, got coinlore=%q coingecko=%q", cfg.CoinLoreURL, cfg.CoinGeckoURL)
	}
	if cfg.HTTPTimeout != 45*time.Second {
		t.Fatalf("expected primary http timeout, got %s", cfg.HTTPTimeout)
	}
	if cfg.SchedulerCrawlTimeout != 180*time.Second {
		t.Fatalf("expected scheduler crawl timeout loaded, got %s", cfg.SchedulerCrawlTimeout)
	}
	if cfg.ExternalRetryCount != 3 || cfg.ExternalRetryWait != 750*time.Millisecond || cfg.CryptoSocialRateLimit != 1200*time.Millisecond {
		t.Fatalf("expected external config loaded, got retries=%d wait=%s social_rate=%s", cfg.ExternalRetryCount, cfg.ExternalRetryWait, cfg.CryptoSocialRateLimit)
	}
	if cfg.FlashInterval != 33*time.Second {
		t.Fatalf("expected alias flash interval, got %s", cfg.FlashInterval)
	}
	if cfg.Jin10FullBackfillDays != 90 || cfg.Jin10FullMaxPages != 12 || cfg.Jin10FullRateLimit != 250*time.Millisecond {
		t.Fatalf("expected jin10 full config loaded, got days=%d pages=%d rate=%s", cfg.Jin10FullBackfillDays, cfg.Jin10FullMaxPages, cfg.Jin10FullRateLimit)
	}
	if cfg.Jin10FullIncludeSitemap || !cfg.Jin10FullEnabled || cfg.Jin10FullInterval != 600*time.Second {
		t.Fatalf("expected jin10 full flags loaded, got sitemap=%v enabled=%v interval=%s", cfg.Jin10FullIncludeSitemap, cfg.Jin10FullEnabled, cfg.Jin10FullInterval)
	}
	if cfg.CryptoXURL != "https://social.example.com/x" || cfg.CryptoXToken != "x-token" {
		t.Fatalf("expected x config loaded, got url=%q token=%q", cfg.CryptoXURL, cfg.CryptoXToken)
	}
	if cfg.CryptoTelegramURL != "https://social.example.com/tg" || cfg.CryptoTelegramToken != "tg-token" {
		t.Fatalf("expected telegram config loaded, got url=%q token=%q", cfg.CryptoTelegramURL, cfg.CryptoTelegramToken)
	}
	if cfg.CryptoXInterval != 77*time.Second || cfg.CryptoTelegramInterval != 88*time.Second {
		t.Fatalf("expected social intervals loaded, got x=%s tg=%s", cfg.CryptoXInterval, cfg.CryptoTelegramInterval)
	}
}

func TestEnvDurationSecondsFallsBackOnInvalidValues(t *testing.T) {
	t.Setenv("TEST_DURATION_A", "bad")
	t.Setenv("TEST_DURATION_B", "-10")

	if got := envDurationSeconds(12, "TEST_DURATION_A"); got != 12*time.Second {
		t.Fatalf("expected fallback on parse error, got %s", got)
	}
	if got := envDurationSeconds(12, "TEST_DURATION_B"); got != 12*time.Second {
		t.Fatalf("expected fallback on non-positive value, got %s", got)
	}
}
