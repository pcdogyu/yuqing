package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"

type Config struct {
	ListenAddr                 string
	DatabaseDriver             string
	DatabasePath               string
	DatabaseURL                string
	DatabaseConfigPath         string
	PostgresHost               string
	PostgresPort               string
	PostgresDatabase           string
	PostgresUser               string
	PostgresPassword           string
	PostgresSSLMode            string
	FlashURL                   string
	HeadlineURL                string
	BinanceBaseURL             string
	CoinLoreURL                string
	CoinGeckoURL               string
	Jin10FullBackfillDays      int
	Jin10FullMaxPages          int
	Jin10FullRateLimit         time.Duration
	Jin10FullIncludeSitemap    bool
	Jin10FullEnabled           bool
	Jin10FullInterval          time.Duration
	CryptoXURL                 string
	CryptoXToken               string
	CryptoTelegramURL          string
	CryptoTelegramToken        string
	ForesightNewsflashURL      string
	CoinDeskZHLatestURL        string
	PANewsNewsflashURL         string
	TheBlockLatestURL          string
	EastMoneyKuaixunURL        string
	WallStreetCNAStockURL      string
	CLSTelegraphURL            string
	SinaFinance7x24URL         string
	AStockAuctionURL           string
	AStockHoldingURL           string
	StockResearchURL           string
	StockResearchPDFDir        string
	InvestorRelationsURL       string
	TuShareToken               string
	StockResearchPublicEnabled bool
	SinaFinanceReportURL       string
	EastMoneyReportURL         string
	SohuFinanceReportURL       string
	HTTPTimeout                time.Duration
	SchedulerCrawlTimeout      time.Duration
	ExternalRetryCount         int
	ExternalRetryWait          time.Duration
	FlashInterval              time.Duration
	HeadlineInterval           time.Duration
	CryptoSocialRateLimit      time.Duration
	CryptoXInterval            time.Duration
	CryptoTelegramInterval     time.Duration
	ForesightNewsflashInterval time.Duration
	CoinDeskZHLatestInterval   time.Duration
	PANewsNewsflashInterval    time.Duration
	TheBlockLatestInterval     time.Duration
	AnalysisInterval           time.Duration
	WechatCleanupInterval      time.Duration
	WechatPushInterval         time.Duration
	TimelyWebSocketURL         string
	UserAgent                  string
	LogLevel                   string
	ServiceToken               string
	SessionTTL                 time.Duration
	WechatPrivateKey           string
	WechatAccountName          string
	WechatPushEnabled          bool
	WechatPushWebhookURL       string

	GatewayWebAddr string
	AuthAddr       string
	WechatAddr     string
	ContentAddr    string
	CrawlerAddr    string
	AnalysisAddr   string
	NLPAddr        string
	SchedulerAddr  string

	GatewayWebHTTPAddrs    []string
	GatewayWebRedirectAddr string
	GatewayWebTLSAddr      string
	GatewayWebTLSCertFile  string
	GatewayWebTLSKeyFile   string

	GatewayWebURL string
	AuthURL       string
	WechatURL     string
	ContentURL    string
	CrawlerURL    string
	AnalysisURL   string
	NLPURL        string
	SchedulerURL  string

	LLMBaseURL string
	LLMAPIKey  string
	LLMModel   string

	DefaultAdminUser string
	DefaultAdminPass string
}

func Load() Config {
	databaseConfigPath := envOrDefault("YUQING_DB_CONFIG_PATH", filepath.Join("data", "database-config.json"))
	databaseConfig := loadRuntimeDatabaseConfig(databaseConfigPath)
	dbPath := firstNonEmpty(firstEnv("YUQING_DB_PATH", "JIN10_DB_PATH"), databaseConfig.SQLitePath)
	if dbPath == "" {
		dbPath = filepath.Join("data", "yuqing.db")
	}
	databaseURL := firstNonEmpty(firstEnv("YUQING_DATABASE_URL", "YUQING_POSTGRES_DSN", "JIN10_DATABASE_URL"), databaseConfig.PostgresDSN)
	databaseDriver := firstNonEmpty(firstEnv("YUQING_DB_DRIVER", "JIN10_DB_DRIVER"), databaseConfig.Driver, "sqlite")

	gatewayWebAddr := envOrDefaultWithAliases("YUQING_GATEWAY_ADDR", ":8079", "JIN10_PORTAL_WEB_ADDR")
	gatewayWebHTTPAddrs := envListOrDefault("YUQING_GATEWAY_HTTP_ADDRS", []string{gatewayWebAddr})

	return Config{
		ListenAddr:                 envOrDefault("YUQING_LISTEN_ADDR", ":8090"),
		DatabaseDriver:             normalizeDatabaseDriver(databaseDriver),
		DatabasePath:               dbPath,
		DatabaseURL:                databaseURL,
		DatabaseConfigPath:         databaseConfigPath,
		PostgresHost:               firstNonEmpty(os.Getenv("YUQING_POSTGRES_HOST"), databaseConfig.PostgresHost, "127.0.0.1"),
		PostgresPort:               firstNonEmpty(os.Getenv("YUQING_POSTGRES_PORT"), databaseConfig.PostgresPort, "5432"),
		PostgresDatabase:           firstNonEmpty(os.Getenv("YUQING_POSTGRES_DB"), databaseConfig.PostgresDatabase, "yuqing"),
		PostgresUser:               firstNonEmpty(os.Getenv("YUQING_POSTGRES_USER"), databaseConfig.PostgresUser, "postgres"),
		PostgresPassword:           firstNonEmpty(os.Getenv("YUQING_POSTGRES_PASSWORD"), databaseConfig.PostgresPassword),
		PostgresSSLMode:            firstNonEmpty(os.Getenv("YUQING_POSTGRES_SSLMODE"), databaseConfig.PostgresSSLMode, "disable"),
		FlashURL:                   envOrDefaultWithAliases("YUQING_FLASH_URL", "https://www.jin10.com/", "JIN10_FLASH_URL"),
		HeadlineURL:                envOrDefaultWithAliases("YUQING_HEADLINE_URL", "https://xnews.jin10.com/", "JIN10_HEADLINE_URL"),
		BinanceBaseURL:             envOrDefault("YUQING_BINANCE_BASE_URL", "https://api.binance.com"),
		CoinLoreURL:                envOrDefault("YUQING_COINLORE_URL", "https://api.coinlore.net"),
		CoinGeckoURL:               envOrDefault("YUQING_COINGECKO_URL", "https://api.coingecko.com/api/v3"),
		Jin10FullBackfillDays:      envInt(30, "YUQING_JIN10_FULL_BACKFILL_DAYS"),
		Jin10FullMaxPages:          envInt(20, "YUQING_JIN10_FULL_MAX_PAGES_PER_RUN"),
		Jin10FullRateLimit:         envDurationMillis(800, "YUQING_JIN10_FULL_RATE_LIMIT_MS"),
		Jin10FullIncludeSitemap:    envBool(true, "YUQING_JIN10_FULL_INCLUDE_SITEMAP"),
		Jin10FullEnabled:           envBool(false, "YUQING_JIN10_FULL_ENABLED"),
		Jin10FullInterval:          envDurationSeconds(300, "YUQING_JIN10_FULL_INTERVAL_SEC"),
		CryptoXURL:                 envOrDefault("YUQING_CRYPTO_X_URL", ""),
		CryptoXToken:               envOrDefault("YUQING_CRYPTO_X_TOKEN", ""),
		CryptoTelegramURL:          envOrDefault("YUQING_CRYPTO_TELEGRAM_URL", ""),
		CryptoTelegramToken:        envOrDefault("YUQING_CRYPTO_TELEGRAM_TOKEN", ""),
		ForesightNewsflashURL:      envOrDefaultAllowEmpty("YUQING_FORESIGHT_NEWSFLASH_URL", "https://foresightnews.pro/news"),
		CoinDeskZHLatestURL:        envOrDefaultAllowEmpty("YUQING_COINDESK_ZH_LATEST_URL", "https://www.coindesk.com/zh/latest-crypto-news"),
		PANewsNewsflashURL:         envOrDefaultAllowEmpty("YUQING_PANEWS_NEWSFLASH_URL", "https://www.panewslab.com/rss.xml?lang=zh&type=NEWS"),
		TheBlockLatestURL:          envOrDefaultAllowEmpty("YUQING_THEBLOCK_LATEST_URL", "https://www.theblock.co/latest-crypto-news"),
		EastMoneyKuaixunURL:        envOrDefaultAllowEmpty("YUQING_EASTMONEY_KUAIXUN_URL", "https://kuaixun.eastmoney.com/"),
		WallStreetCNAStockURL:      envOrDefaultAllowEmpty("YUQING_WALLSTREETCN_A_STOCK_URL", "https://wallstreetcn.com/live/a-stock"),
		CLSTelegraphURL:            envOrDefaultAllowEmpty("YUQING_CLS_TELEGRAPH_URL", "https://www.cls.cn/telegraph"),
		SinaFinance7x24URL:         envOrDefaultAllowEmpty("YUQING_SINA_FINANCE_7X24_URL", "https://finance.sina.com.cn/7x24/?tag=10"),
		AStockAuctionURL:           envOrDefault("YUQING_ASTOCK_AUCTION_URL", ""),
		AStockHoldingURL:           envOrDefault("YUQING_ASTOCK_HOLDING_URL", ""),
		StockResearchURL:           envOrDefault("YUQING_STOCK_RESEARCH_URL", ""),
		StockResearchPDFDir:        envOrDefault("YUQING_STOCK_RESEARCH_PDF_DIR", filepath.Join("data", "stock-research-pdfs")),
		InvestorRelationsURL:       envOrDefaultAllowEmpty("YUQING_INVESTOR_RELATIONS_URL", "https://irm.cninfo.com.cn/newircs/index/search"),
		TuShareToken:               envOrDefault("YUQING_TUSHARE_TOKEN", ""),
		StockResearchPublicEnabled: envBool(true, "YUQING_STOCK_RESEARCH_PUBLIC_ENABLED"),
		SinaFinanceReportURL:       envOrDefaultAllowEmpty("YUQING_SINA_FINANCE_REPORT_URL", "https://stock.finance.sina.com.cn/stock/go.php/vReport_List/kind/company/index.phtml"),
		EastMoneyReportURL:         envOrDefaultAllowEmpty("YUQING_EASTMONEY_REPORT_URL", "https://data.eastmoney.com/report/stock.jshtml"),
		SohuFinanceReportURL:       envOrDefaultAllowEmpty("YUQING_SOHU_FINANCE_REPORT_URL", "https://q.stock.sohu.com/jlp/"),
		HTTPTimeout:                envDurationSeconds(20, "YUQING_HTTP_TIMEOUT_SEC", "JIN10_HTTP_TIMEOUT_SEC"),
		SchedulerCrawlTimeout:      envDurationSeconds(120, "YUQING_SCHEDULER_CRAWL_TIMEOUT_SEC"),
		ExternalRetryCount:         envIntAllowZero(2, "YUQING_EXTERNAL_RETRY_COUNT"),
		ExternalRetryWait:          envDurationMillis(500, "YUQING_EXTERNAL_RETRY_WAIT_MS"),
		FlashInterval:              envDurationSeconds(15, "YUQING_FLASH_INTERVAL_SEC", "JIN10_FLASH_INTERVAL_SEC"),
		HeadlineInterval:           envDurationSeconds(60, "YUQING_HEADLINE_INTERVAL_SEC", "JIN10_HEADLINE_INTERVAL_SEC"),
		CryptoSocialRateLimit:      envDurationMillisAllowZero(0, "YUQING_CRYPTO_SOCIAL_RATE_LIMIT_MS"),
		CryptoXInterval:            envDurationSeconds(90, "YUQING_CRYPTO_X_INTERVAL_SEC"),
		CryptoTelegramInterval:     envDurationSeconds(90, "YUQING_CRYPTO_TELEGRAM_INTERVAL_SEC"),
		ForesightNewsflashInterval: envDurationSeconds(120, "YUQING_FORESIGHT_NEWSFLASH_INTERVAL_SEC"),
		CoinDeskZHLatestInterval:   envDurationSeconds(300, "YUQING_COINDESK_ZH_LATEST_INTERVAL_SEC"),
		PANewsNewsflashInterval:    envDurationSeconds(120, "YUQING_PANEWS_NEWSFLASH_INTERVAL_SEC"),
		TheBlockLatestInterval:     envDurationSeconds(300, "YUQING_THEBLOCK_LATEST_INTERVAL_SEC"),
		AnalysisInterval:           envDurationSeconds(120, "YUQING_ANALYSIS_INTERVAL_SEC"),
		WechatCleanupInterval:      envDurationSeconds(3600, "YUQING_WECHAT_CLEANUP_INTERVAL_SEC"),
		WechatPushInterval:         envDurationSeconds(86400, "YUQING_WECHAT_PUSH_INTERVAL_SEC"),
		TimelyWebSocketURL:         envOrDefault("YUQING_TIMELY_WEBSOCKET_URL", "ws://s1.stonedt.com:6388/ws"),
		UserAgent:                  envOrDefaultWithAliases("YUQING_USER_AGENT", defaultUserAgent, "JIN10_USER_AGENT"),
		LogLevel:                   envOrDefaultWithAliases("YUQING_LOG_LEVEL", "info", "JIN10_LOG_LEVEL"),
		ServiceToken:               envOrDefaultWithAliases("YUQING_SERVICE_TOKEN", "stonedt-internal-token", "JIN10_SERVICE_TOKEN"),
		SessionTTL:                 envDurationSeconds(86400, "YUQING_SESSION_TTL_SEC", "JIN10_SESSION_TTL_SEC"),
		WechatPrivateKey:           envOrDefaultWithAliases("YUQING_WECHAT_PRIVATE_KEY", "yuqing-wechat-private-key", "JIN10_TOKEN_PRIVATE_KEY"),
		WechatAccountName:          envOrDefaultWithAliases("YUQING_WECHAT_NAME", "简苏舆情", "JIN10_WECHAT_NAME"),
		WechatPushEnabled:          envBool(false, "YUQING_WECHAT_PUSH_ENABLED"),
		WechatPushWebhookURL:       envOrDefault("YUQING_WECHAT_PUSH_WEBHOOK_URL", ""),

		GatewayWebAddr: gatewayWebAddr,
		AuthAddr:       envOrDefault("YUQING_AUTH_ADDR", ":8081"),
		WechatAddr:     envOrDefault("YUQING_WECHAT_ADDR", ":8088"),
		ContentAddr:    envOrDefaultWithAliases("YUQING_CONTENT_ADDR", ":8082", "JIN10_CONTENT_ADDR"),
		CrawlerAddr:    envOrDefaultWithAliases("YUQING_CRAWLER_ADDR", ":8083", "JIN10_CRAWLER_ADDR"),
		AnalysisAddr:   envOrDefault("YUQING_ANALYSIS_ADDR", ":8084"),
		NLPAddr:        envOrDefaultWithAliases("YUQING_NLP_ADDR", ":8085", "JIN10_NLP_ADDR"),
		SchedulerAddr:  envOrDefaultWithAliases("YUQING_SCHEDULER_ADDR", ":8086", "JIN10_SCHEDULER_ADDR"),

		GatewayWebHTTPAddrs:    gatewayWebHTTPAddrs,
		GatewayWebRedirectAddr: envOrDefaultAllowEmpty("YUQING_GATEWAY_REDIRECT_ADDR", ":80"),
		GatewayWebTLSAddr:      envOrDefaultAllowEmpty("YUQING_GATEWAY_TLS_ADDR", ":443"),
		GatewayWebTLSCertFile:  envOrDefault("YUQING_GATEWAY_TLS_CERT_FILE", ""),
		GatewayWebTLSKeyFile:   envOrDefault("YUQING_GATEWAY_TLS_KEY_FILE", ""),

		GatewayWebURL: envOrDefaultWithAliases("YUQING_GATEWAY_URL", "http://127.0.0.1:8079", "JIN10_PORTAL_WEB_URL"),
		AuthURL:       envOrDefault("YUQING_AUTH_URL", "http://127.0.0.1:8081"),
		WechatURL:     envOrDefault("YUQING_WECHAT_URL", "http://127.0.0.1:8088"),
		ContentURL:    envOrDefaultWithAliases("YUQING_CONTENT_URL", "http://127.0.0.1:8082", "JIN10_CONTENT_URL"),
		CrawlerURL:    envOrDefaultWithAliases("YUQING_CRAWLER_URL", "http://127.0.0.1:8083", "JIN10_CRAWLER_URL"),
		AnalysisURL:   envOrDefault("YUQING_ANALYSIS_URL", "http://127.0.0.1:8084"),
		NLPURL:        envOrDefaultWithAliases("YUQING_NLP_URL", "http://127.0.0.1:8085", "JIN10_NLP_URL"),
		SchedulerURL:  envOrDefaultWithAliases("YUQING_SCHEDULER_URL", "http://127.0.0.1:8086", "JIN10_SCHEDULER_URL"),
		LLMBaseURL:    envOrDefault("YUQING_LLM_BASE_URL", ""),
		LLMAPIKey:     envOrDefault("YUQING_LLM_API_KEY", ""),
		LLMModel:      envOrDefault("YUQING_LLM_MODEL", ""),

		DefaultAdminUser: envOrDefaultWithAliases("YUQING_DEFAULT_ADMIN_USER", "admin", "JIN10_DEFAULT_ADMIN_USER"),
		DefaultAdminPass: envOrDefaultWithAliases("YUQING_DEFAULT_ADMIN_PASS", "admin123", "JIN10_DEFAULT_ADMIN_PASS"),
	}
}

func normalizeDatabaseDriver(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "sqlite", "sqlite3":
		return "sqlite"
	case "postgres", "postgresql", "pg":
		return "postgres"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

type runtimeDatabaseConfig struct {
	Driver           string `json:"driver"`
	SQLitePath       string `json:"sqlite_path"`
	PostgresDSN      string `json:"postgres_dsn"`
	PostgresHost     string `json:"postgres_host"`
	PostgresPort     string `json:"postgres_port"`
	PostgresDatabase string `json:"postgres_database"`
	PostgresUser     string `json:"postgres_user"`
	PostgresPassword string `json:"postgres_password"`
	PostgresSSLMode  string `json:"postgres_sslmode"`
}

func loadRuntimeDatabaseConfig(path string) runtimeDatabaseConfig {
	path = strings.TrimSpace(path)
	if path == "" {
		return runtimeDatabaseConfig{}
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return runtimeDatabaseConfig{}
	}
	var cfg runtimeDatabaseConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return runtimeDatabaseConfig{}
	}
	cfg.Driver = normalizeDatabaseDriver(cfg.Driver)
	return cfg
}

func envInt(fallback int, keys ...string) int {
	raw := strconv.Itoa(fallback)
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			raw = value
			break
		}
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envIntAllowZero(fallback int, keys ...string) int {
	raw := strconv.Itoa(fallback)
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			raw = value
			break
		}
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func envBool(fallback bool, keys ...string) bool {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		switch strings.ToLower(value) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
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

func envOrDefaultAllowEmpty(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
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

func envListOrDefault(key string, fallback []string) []string {
	if value, ok := os.LookupEnv(key); ok {
		return splitEnvList(value)
	}
	return fallback
}

func splitEnvList(value string) []string {
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
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

func envDurationMillis(fallback int, keys ...string) time.Duration {
	ms := envInt(fallback, keys...)
	return time.Duration(ms) * time.Millisecond
}

func envDurationMillisAllowZero(fallback int, keys ...string) time.Duration {
	ms := envIntAllowZero(fallback, keys...)
	return time.Duration(ms) * time.Millisecond
}
