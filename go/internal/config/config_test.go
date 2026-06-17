package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("YUQING_DB_CONFIG_PATH", filepath.Join(t.TempDir(), "missing-database-config.json"))
	t.Setenv("YUQING_DB_PATH", "")
	t.Setenv("JIN10_DB_PATH", "")
	t.Setenv("YUQING_DB_DRIVER", "")
	t.Setenv("JIN10_DB_DRIVER", "")
	t.Setenv("YUQING_DATABASE_URL", "")
	t.Setenv("YUQING_POSTGRES_DSN", "")
	t.Setenv("JIN10_DATABASE_URL", "")
	t.Setenv("YUQING_POSTGRES_HOST", "")
	t.Setenv("YUQING_POSTGRES_PORT", "")
	t.Setenv("YUQING_POSTGRES_DB", "")
	t.Setenv("YUQING_POSTGRES_USER", "")
	t.Setenv("YUQING_POSTGRES_PASSWORD", "")
	t.Setenv("YUQING_POSTGRES_SSLMODE", "")
	t.Setenv("YUQING_LISTEN_ADDR", "")
	t.Setenv("YUQING_FLASH_URL", "")
	t.Setenv("JIN10_FLASH_URL", "")
	t.Setenv("YUQING_HEADLINE_URL", "")
	t.Setenv("JIN10_HEADLINE_URL", "")
	t.Setenv("YUQING_CRYPTO_X_URL", "")
	t.Setenv("YUQING_CRYPTO_X_TOKEN", "")
	t.Setenv("YUQING_CRYPTO_TELEGRAM_URL", "")
	t.Setenv("YUQING_CRYPTO_TELEGRAM_TOKEN", "")
	unsetEnv(t, "YUQING_FORESIGHT_NEWSFLASH_URL")
	unsetEnv(t, "YUQING_COINDESK_ZH_LATEST_URL")
	unsetEnv(t, "YUQING_PANEWS_NEWSFLASH_URL")
	unsetEnv(t, "YUQING_EASTMONEY_KUAIXUN_URL")
	unsetEnv(t, "YUQING_WALLSTREETCN_A_STOCK_URL")
	unsetEnv(t, "YUQING_CLS_TELEGRAPH_URL")
	unsetEnv(t, "YUQING_SINA_FINANCE_7X24_URL")
	t.Setenv("YUQING_ASTOCK_AUCTION_URL", "")
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
	if cfg.DatabaseDriver != "sqlite" || cfg.DatabaseURL != "" {
		t.Fatalf("expected default sqlite database config, got driver=%q url=%q", cfg.DatabaseDriver, cfg.DatabaseURL)
	}
	if cfg.PostgresHost != "127.0.0.1" || cfg.PostgresPort != "5432" || cfg.PostgresDatabase != "yuqing" || cfg.PostgresUser != "postgres" || cfg.PostgresSSLMode != "disable" {
		t.Fatalf("unexpected default postgres config: host=%q port=%q db=%q user=%q ssl=%q", cfg.PostgresHost, cfg.PostgresPort, cfg.PostgresDatabase, cfg.PostgresUser, cfg.PostgresSSLMode)
	}
	if cfg.ListenAddr != ":8090" {
		t.Fatalf("expected default listen addr, got %q", cfg.ListenAddr)
	}
	if cfg.GatewayWebAddr != ":80" {
		t.Fatalf("expected default gateway addr, got %q", cfg.GatewayWebAddr)
	}
	if cfg.WechatAddr != ":8088" {
		t.Fatalf("expected default wechat addr, got %q", cfg.WechatAddr)
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
	if cfg.ForesightNewsflashURL != "https://foresightnews.pro/news" || cfg.CoinDeskZHLatestURL != "https://www.coindesk.com/zh/latest-crypto-news" {
		t.Fatalf("expected default crypto news urls, got foresight=%q coindesk=%q", cfg.ForesightNewsflashURL, cfg.CoinDeskZHLatestURL)
	}
	if cfg.PANewsNewsflashURL != "https://www.panewslab.com/rss.xml?lang=zh&type=NEWS" {
		t.Fatalf("expected default panews url, got %q", cfg.PANewsNewsflashURL)
	}
	if cfg.TheBlockLatestURL != "https://www.theblock.co/latest-crypto-news" {
		t.Fatalf("expected default theblock url, got %q", cfg.TheBlockLatestURL)
	}
	if cfg.EastMoneyKuaixunURL != "https://kuaixun.eastmoney.com/" {
		t.Fatalf("expected default eastmoney kuaixun url, got %q", cfg.EastMoneyKuaixunURL)
	}
	if cfg.WallStreetCNAStockURL != "https://wallstreetcn.com/live/a-stock" || cfg.CLSTelegraphURL != "https://www.cls.cn/telegraph" || cfg.SinaFinance7x24URL != "https://finance.sina.com.cn/7x24/?tag=10" {
		t.Fatalf("expected default A股 public news urls, got wallstreet=%q cls=%q sina=%q", cfg.WallStreetCNAStockURL, cfg.CLSTelegraphURL, cfg.SinaFinance7x24URL)
	}
	if cfg.AStockAuctionURL != "" {
		t.Fatalf("expected empty A股 auction url by default, got %q", cfg.AStockAuctionURL)
	}
	if cfg.AStockHoldingURL != "" {
		t.Fatalf("expected empty A股 holding url by default, got %q", cfg.AStockHoldingURL)
	}
	if cfg.StockResearchPDFDir != filepath.Join("data", "stock-research-pdfs") {
		t.Fatalf("expected default stock research PDF dir, got %q", cfg.StockResearchPDFDir)
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
	if cfg.ForesightNewsflashInterval != 120*time.Second || cfg.CoinDeskZHLatestInterval != 300*time.Second {
		t.Fatalf("expected default crypto news intervals, got foresight=%s coindesk=%s", cfg.ForesightNewsflashInterval, cfg.CoinDeskZHLatestInterval)
	}
	if cfg.PANewsNewsflashInterval != 120*time.Second {
		t.Fatalf("expected default panews interval, got %s", cfg.PANewsNewsflashInterval)
	}
	if cfg.TheBlockLatestInterval != 300*time.Second {
		t.Fatalf("expected default theblock interval, got %s", cfg.TheBlockLatestInterval)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("expected default log level, got %q", cfg.LogLevel)
	}
	if cfg.GatewayWebURL != "http://127.0.0.1" {
		t.Fatalf("expected default gateway url, got %q", cfg.GatewayWebURL)
	}
	if cfg.WechatURL != "http://127.0.0.1:8088" {
		t.Fatalf("expected default wechat url, got %q", cfg.WechatURL)
	}
}

func TestLoadPrefersPrimaryAndAliasEnv(t *testing.T) {
	t.Setenv("YUQING_DB_CONFIG_PATH", filepath.Join(t.TempDir(), "missing-database-config.json"))
	t.Setenv("YUQING_DB_PATH", "")
	t.Setenv("JIN10_DB_PATH", "alias.db")
	t.Setenv("YUQING_DB_DRIVER", "postgresql")
	t.Setenv("YUQING_DATABASE_URL", "postgres://dbuser:secret@db.example.com:5432/yuqing?sslmode=require")
	t.Setenv("YUQING_POSTGRES_HOST", "db.example.com")
	t.Setenv("YUQING_POSTGRES_PORT", "6543")
	t.Setenv("YUQING_POSTGRES_DB", "yuqing_prod")
	t.Setenv("YUQING_POSTGRES_USER", "yuqing_user")
	t.Setenv("YUQING_POSTGRES_PASSWORD", "secret")
	t.Setenv("YUQING_POSTGRES_SSLMODE", "require")
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
	t.Setenv("YUQING_FORESIGHT_NEWSFLASH_URL", "https://foresight.example.com/news")
	t.Setenv("YUQING_COINDESK_ZH_LATEST_URL", "https://coindesk.example.com/zh/latest")
	t.Setenv("YUQING_PANEWS_NEWSFLASH_URL", "https://panews.example.com/rss.xml")
	t.Setenv("YUQING_THEBLOCK_LATEST_URL", "https://theblock.example.com/latest")
	t.Setenv("YUQING_EASTMONEY_KUAIXUN_URL", "https://eastmoney.example.com/kuaixun")
	t.Setenv("YUQING_WALLSTREETCN_A_STOCK_URL", "https://wallstreet.example.com/a-stock")
	t.Setenv("YUQING_CLS_TELEGRAPH_URL", "https://cls.example.com/telegraph")
	t.Setenv("YUQING_SINA_FINANCE_7X24_URL", "https://sina.example.com/7x24")
	t.Setenv("YUQING_ASTOCK_AUCTION_URL", "http://akshare.example.com")
	t.Setenv("YUQING_ASTOCK_HOLDING_URL", "http://holding.example.com")
	t.Setenv("YUQING_STOCK_RESEARCH_PDF_DIR", "D:\\research-pdfs")
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
	t.Setenv("YUQING_FORESIGHT_NEWSFLASH_INTERVAL_SEC", "121")
	t.Setenv("YUQING_COINDESK_ZH_LATEST_INTERVAL_SEC", "301")
	t.Setenv("YUQING_PANEWS_NEWSFLASH_INTERVAL_SEC", "122")
	t.Setenv("YUQING_THEBLOCK_LATEST_INTERVAL_SEC", "303")
	t.Setenv("YUQING_WECHAT_ADDR", ":18087")
	t.Setenv("YUQING_WECHAT_URL", "http://127.0.0.1:18087")

	cfg := Load()

	if cfg.DatabasePath != "alias.db" {
		t.Fatalf("expected alias db path, got %q", cfg.DatabasePath)
	}
	if cfg.DatabaseDriver != "postgres" || cfg.DatabaseURL == "" {
		t.Fatalf("expected postgres database config loaded, got driver=%q url=%q", cfg.DatabaseDriver, cfg.DatabaseURL)
	}
	if cfg.PostgresHost != "db.example.com" || cfg.PostgresPort != "6543" || cfg.PostgresDatabase != "yuqing_prod" || cfg.PostgresUser != "yuqing_user" || cfg.PostgresPassword != "secret" || cfg.PostgresSSLMode != "require" {
		t.Fatalf("unexpected postgres config loaded: host=%q port=%q db=%q user=%q password=%q ssl=%q", cfg.PostgresHost, cfg.PostgresPort, cfg.PostgresDatabase, cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresSSLMode)
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
	if cfg.AStockAuctionURL != "http://akshare.example.com" {
		t.Fatalf("expected A股 auction url loaded, got %q", cfg.AStockAuctionURL)
	}
	if cfg.AStockHoldingURL != "http://holding.example.com" {
		t.Fatalf("expected A股 holding url loaded, got %q", cfg.AStockHoldingURL)
	}
	if cfg.StockResearchPDFDir != "D:\\research-pdfs" {
		t.Fatalf("expected stock research PDF dir loaded, got %q", cfg.StockResearchPDFDir)
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
	if cfg.ForesightNewsflashURL != "https://foresight.example.com/news" || cfg.CoinDeskZHLatestURL != "https://coindesk.example.com/zh/latest" {
		t.Fatalf("expected crypto news urls loaded, got foresight=%q coindesk=%q", cfg.ForesightNewsflashURL, cfg.CoinDeskZHLatestURL)
	}
	if cfg.PANewsNewsflashURL != "https://panews.example.com/rss.xml" {
		t.Fatalf("expected panews url loaded, got %q", cfg.PANewsNewsflashURL)
	}
	if cfg.TheBlockLatestURL != "https://theblock.example.com/latest" {
		t.Fatalf("expected theblock url loaded, got %q", cfg.TheBlockLatestURL)
	}
	if cfg.EastMoneyKuaixunURL != "https://eastmoney.example.com/kuaixun" {
		t.Fatalf("expected eastmoney kuaixun url loaded, got %q", cfg.EastMoneyKuaixunURL)
	}
	if cfg.WallStreetCNAStockURL != "https://wallstreet.example.com/a-stock" || cfg.CLSTelegraphURL != "https://cls.example.com/telegraph" || cfg.SinaFinance7x24URL != "https://sina.example.com/7x24" {
		t.Fatalf("expected A股 public news urls loaded, got wallstreet=%q cls=%q sina=%q", cfg.WallStreetCNAStockURL, cfg.CLSTelegraphURL, cfg.SinaFinance7x24URL)
	}
	if cfg.CryptoXInterval != 77*time.Second || cfg.CryptoTelegramInterval != 88*time.Second {
		t.Fatalf("expected social intervals loaded, got x=%s tg=%s", cfg.CryptoXInterval, cfg.CryptoTelegramInterval)
	}
	if cfg.ForesightNewsflashInterval != 121*time.Second || cfg.CoinDeskZHLatestInterval != 301*time.Second {
		t.Fatalf("expected crypto news intervals loaded, got foresight=%s coindesk=%s", cfg.ForesightNewsflashInterval, cfg.CoinDeskZHLatestInterval)
	}
	if cfg.PANewsNewsflashInterval != 122*time.Second {
		t.Fatalf("expected panews interval loaded, got %s", cfg.PANewsNewsflashInterval)
	}
	if cfg.TheBlockLatestInterval != 303*time.Second {
		t.Fatalf("expected theblock interval loaded, got %s", cfg.TheBlockLatestInterval)
	}
	if cfg.WechatAddr != ":18087" || cfg.WechatURL != "http://127.0.0.1:18087" {
		t.Fatalf("expected wechat service config loaded, got addr=%q url=%q", cfg.WechatAddr, cfg.WechatURL)
	}
}

func TestCryptoNewsURLsCanBeDisabledWithExplicitEmptyEnv(t *testing.T) {
	t.Setenv("YUQING_DB_CONFIG_PATH", filepath.Join(t.TempDir(), "missing-database-config.json"))
	t.Setenv("YUQING_FORESIGHT_NEWSFLASH_URL", "")
	t.Setenv("YUQING_COINDESK_ZH_LATEST_URL", "")
	t.Setenv("YUQING_PANEWS_NEWSFLASH_URL", "")
	t.Setenv("YUQING_THEBLOCK_LATEST_URL", "")
	t.Setenv("YUQING_EASTMONEY_KUAIXUN_URL", "")
	t.Setenv("YUQING_WALLSTREETCN_A_STOCK_URL", "")
	t.Setenv("YUQING_CLS_TELEGRAPH_URL", "")
	t.Setenv("YUQING_SINA_FINANCE_7X24_URL", "")

	cfg := Load()

	if cfg.ForesightNewsflashURL != "" || cfg.CoinDeskZHLatestURL != "" || cfg.PANewsNewsflashURL != "" || cfg.TheBlockLatestURL != "" || cfg.EastMoneyKuaixunURL != "" || cfg.WallStreetCNAStockURL != "" || cfg.CLSTelegraphURL != "" || cfg.SinaFinance7x24URL != "" {
		t.Fatalf("expected explicit empty news URLs to disable providers, got foresight=%q coindesk=%q panews=%q theblock=%q eastmoney=%q wallstreet=%q cls=%q sina=%q", cfg.ForesightNewsflashURL, cfg.CoinDeskZHLatestURL, cfg.PANewsNewsflashURL, cfg.TheBlockLatestURL, cfg.EastMoneyKuaixunURL, cfg.WallStreetCNAStockURL, cfg.CLSTelegraphURL, cfg.SinaFinance7x24URL)
	}
}

func TestLoadUsesRuntimeDatabaseConfigFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "database-config.json")
	if err := os.WriteFile(configPath, []byte(`{
  "driver": "postgresql",
  "sqlite_path": "data/custom.db",
  "postgres_dsn": "postgres://fileuser:filepass@10.15.0.19:5432/yuqing?sslmode=disable",
  "postgres_host": "10.15.0.19",
  "postgres_port": "5432",
  "postgres_database": "yuqing",
  "postgres_user": "admin",
  "postgres_password": "AdminAdmin",
  "postgres_sslmode": "disable"
}`), 0o600); err != nil {
		t.Fatalf("write runtime database config: %v", err)
	}
	t.Setenv("YUQING_DB_CONFIG_PATH", configPath)
	t.Setenv("YUQING_DB_PATH", "")
	t.Setenv("JIN10_DB_PATH", "")
	t.Setenv("YUQING_DB_DRIVER", "")
	t.Setenv("JIN10_DB_DRIVER", "")
	t.Setenv("YUQING_DATABASE_URL", "")
	t.Setenv("YUQING_POSTGRES_DSN", "")
	t.Setenv("JIN10_DATABASE_URL", "")
	t.Setenv("YUQING_POSTGRES_HOST", "")
	t.Setenv("YUQING_POSTGRES_PORT", "")
	t.Setenv("YUQING_POSTGRES_DB", "")
	t.Setenv("YUQING_POSTGRES_USER", "")
	t.Setenv("YUQING_POSTGRES_PASSWORD", "")
	t.Setenv("YUQING_POSTGRES_SSLMODE", "")

	cfg := Load()

	if cfg.DatabaseConfigPath != configPath || cfg.DatabaseDriver != "postgres" || cfg.DatabasePath != "data/custom.db" {
		t.Fatalf("expected runtime database config file values, got path=%q driver=%q sqlite=%q", cfg.DatabaseConfigPath, cfg.DatabaseDriver, cfg.DatabasePath)
	}
	if cfg.DatabaseURL == "" || cfg.PostgresHost != "10.15.0.19" || cfg.PostgresUser != "admin" || cfg.PostgresPassword != "AdminAdmin" {
		t.Fatalf("expected postgres runtime config loaded, got url=%q host=%q user=%q password=%q", cfg.DatabaseURL, cfg.PostgresHost, cfg.PostgresUser, cfg.PostgresPassword)
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

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	old, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}
