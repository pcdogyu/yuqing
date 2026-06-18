package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"

	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/external"
	"github.com/pcdogyu/yuqing/go/internal/provider"
	"github.com/pcdogyu/yuqing/go/internal/provider/cryptonews"
	"github.com/pcdogyu/yuqing/go/internal/provider/cryptosocial"
	"github.com/pcdogyu/yuqing/go/internal/provider/eastmoneykuaixun"
	"github.com/pcdogyu/yuqing/go/internal/provider/jin10flash"
	"github.com/pcdogyu/yuqing/go/internal/provider/jin10full"
	"github.com/pcdogyu/yuqing/go/internal/provider/jin10xnews"
	"github.com/pcdogyu/yuqing/go/internal/provider/publicfinance"
	"github.com/pcdogyu/yuqing/go/internal/service"
	sqlitestore "github.com/pcdogyu/yuqing/go/internal/store/sqlite"
)

var (
	Version    = "dev"
	GitCommit  = "unknown"
	BuildTime  = "unknown"
	BranchName = "unknown"
)

func NewStore(cfg config.Config) (*sqlitestore.Store, error) {
	if cfg.DatabaseDriver == "postgres" {
		store, err := sqlitestore.NewPostgres(postgresDSN(cfg))
		if err != nil && isPostgresDatabaseMissing(err) {
			if createErr := ensureConfiguredPostgresDatabase(context.Background(), cfg); createErr != nil {
				return nil, fmt.Errorf("open postgres store: %w; create database: %v", err, createErr)
			}
			store, err = sqlitestore.NewPostgres(postgresDSN(cfg))
		}
		if err != nil {
			return nil, err
		}
		if err := store.EnsureDefaultAdmin(context.Background(), cfg.DefaultAdminUser, cfg.DefaultAdminPass); err != nil {
			_ = store.Close()
			return nil, err
		}
		if err := store.EnsureSeedData(context.Background()); err != nil {
			_ = store.Close()
			return nil, err
		}
		return store, nil
	}
	store, err := sqlitestore.New(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	if err := store.EnsureDefaultAdmin(context.Background(), cfg.DefaultAdminUser, cfg.DefaultAdminPass); err != nil {
		_ = store.Close()
		return nil, err
	}
	if err := store.EnsureSeedData(context.Background()); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

func ensureConfiguredPostgresDatabase(ctx context.Context, cfg config.Config) error {
	adminDSN, database, err := postgresAdminDSN(cfg)
	if err != nil {
		return err
	}
	if !validPostgresDatabaseName(database) {
		return fmt.Errorf("database name %q must contain only letters, digits, and underscore", database)
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	db, err := sql.Open("pgx", adminDSN)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `CREATE DATABASE `+quotePostgresIdent(database)); err != nil && !isPostgresDuplicateDatabase(err) {
		return err
	}
	return nil
}

func postgresAdminDSN(cfg config.Config) (string, string, error) {
	if strings.TrimSpace(cfg.DatabaseURL) != "" {
		u, err := url.Parse(strings.TrimSpace(cfg.DatabaseURL))
		if err != nil {
			return "", "", err
		}
		database := strings.Trim(strings.TrimSpace(u.Path), "/")
		if database == "" {
			database = "yuqing"
		}
		u.Path = "/postgres"
		return u.String(), database, nil
	}
	database := strings.TrimSpace(cfg.PostgresDatabase)
	if database == "" {
		database = "yuqing"
	}
	admin := cfg
	admin.PostgresDatabase = "postgres"
	return postgresDSN(admin), database, nil
}

func validPostgresDatabaseName(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func quotePostgresIdent(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func isPostgresDatabaseMissing(err error) bool {
	return postgresErrorCode(err, "3D000") || strings.Contains(err.Error(), "SQLSTATE 3D000")
}

func isPostgresDuplicateDatabase(err error) bool {
	return postgresErrorCode(err, "42P04") || strings.Contains(err.Error(), "SQLSTATE 42P04")
}

func postgresErrorCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

func postgresDSN(cfg config.Config) string {
	if strings.TrimSpace(cfg.DatabaseURL) != "" {
		return strings.TrimSpace(cfg.DatabaseURL)
	}
	host := strings.TrimSpace(cfg.PostgresHost)
	if host == "" {
		host = "127.0.0.1"
	}
	port := strings.TrimSpace(cfg.PostgresPort)
	if port == "" {
		port = "5432"
	}
	database := strings.TrimSpace(cfg.PostgresDatabase)
	if database == "" {
		database = "yuqing"
	}
	user := strings.TrimSpace(cfg.PostgresUser)
	if user == "" {
		user = "postgres"
	}
	sslMode := strings.TrimSpace(cfg.PostgresSSLMode)
	if sslMode == "" {
		sslMode = "disable"
	}
	u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(host, port), Path: "/" + database}
	if strings.TrimSpace(cfg.PostgresPassword) != "" {
		u.User = url.UserPassword(user, cfg.PostgresPassword)
	} else {
		u.User = url.User(user)
	}
	q := u.Query()
	q.Set("sslmode", sslMode)
	u.RawQuery = q.Encode()
	return u.String()
}

func NewCrawler(cfg config.Config, store *sqlitestore.Store) *service.Crawler {
	httpClient := resty.New().
		SetTimeout(cfg.HTTPTimeout).
		SetRetryCount(cfg.ExternalRetryCount).
		SetRetryWaitTime(cfg.ExternalRetryWait).
		SetRetryMaxWaitTime(maxDuration(cfg.ExternalRetryWait*6, cfg.ExternalRetryWait)).
		AddRetryCondition(external.ShouldRetryResponse).
		SetHeader("User-Agent", cfg.UserAgent)
	registry := provider.Registry{
		Flash:     jin10flash.NewProvider(httpClient, cfg.FlashURL),
		Headline:  jin10xnews.NewProvider(httpClient, cfg.HeadlineURL),
		Jin10Full: jin10full.NewProvider(httpClient, store, jin10FullOptions(cfg)),
	}
	if cfg.CryptoXURL != "" {
		registry.CryptoX = cryptosocial.NewXProviderWithOptions(httpClient, cfg.CryptoXURL, cfg.CryptoXToken, cryptosocial.Options{RateLimit: cfg.CryptoSocialRateLimit})
	}
	if cfg.CryptoTelegramURL != "" {
		registry.CryptoTelegram = cryptosocial.NewTelegramProviderWithOptions(httpClient, cfg.CryptoTelegramURL, cfg.CryptoTelegramToken, cryptosocial.Options{RateLimit: cfg.CryptoSocialRateLimit})
	}
	if cfg.ForesightNewsflashURL != "" {
		registry.ForesightNewsflash = cryptonews.NewForesightNewsflashProvider(httpClient, cfg.ForesightNewsflashURL)
	}
	if cfg.CoinDeskZHLatestURL != "" {
		registry.CoinDeskZHLatest = cryptonews.NewCoinDeskZHLatestProvider(httpClient, cfg.CoinDeskZHLatestURL)
	}
	if cfg.PANewsNewsflashURL != "" {
		registry.PANewsNewsflash = cryptonews.NewPANewsNewsflashProvider(httpClient, cfg.PANewsNewsflashURL)
	}
	if cfg.TheBlockLatestURL != "" {
		registry.TheBlockLatest = cryptonews.NewTheBlockLatestProvider(httpClient, cfg.TheBlockLatestURL)
	}
	if cfg.EastMoneyKuaixunURL != "" {
		registry.EastMoneyKuaixun = eastmoneykuaixun.NewProvider(httpClient, cfg.EastMoneyKuaixunURL)
	}
	if cfg.WallStreetCNAStockURL != "" {
		registry.WallStreetCNAStock = publicfinance.NewWallStreetCNAStockProvider(httpClient, cfg.WallStreetCNAStockURL)
	}
	if cfg.CLSTelegraphURL != "" {
		registry.CLSTelegraph = publicfinance.NewCLSTelegraphProvider(httpClient, cfg.CLSTelegraphURL)
	}
	if cfg.SinaFinance7x24URL != "" {
		registry.SinaFinance7x24 = publicfinance.NewSinaFinance7x24Provider(httpClient, cfg.SinaFinance7x24URL)
	}
	return service.NewCrawler(store, registry, nil)
}

func maxDuration(value, fallback time.Duration) time.Duration {
	if value > fallback {
		return value
	}
	return fallback
}

func LogStartup(serviceName, listenAddr string, cfg config.Config) {
	dbPath, dbPathErr := filepath.Abs(cfg.DatabasePath)

	event := log.Debug().
		Str("service", serviceName).
		Str("version", Version).
		Str("git_commit", GitCommit).
		Str("build_time", BuildTime).
		Str("branch_name", BranchName).
		Str("go_version", runtime.Version()).
		Str("log_level", cfg.LogLevel).
		Str("listen_addr", listenAddr).
		Dur("http_timeout", cfg.HTTPTimeout).
		Int("external_retry_count", cfg.ExternalRetryCount).
		Dur("external_retry_wait", cfg.ExternalRetryWait).
		Dur("flash_interval", cfg.FlashInterval).
		Dur("headline_interval", cfg.HeadlineInterval).
		Dur("jin10_full_interval", cfg.Jin10FullInterval).
		Bool("jin10_full_enabled", cfg.Jin10FullEnabled).
		Int("jin10_full_backfill_days", cfg.Jin10FullBackfillDays).
		Int("jin10_full_max_pages", cfg.Jin10FullMaxPages).
		Dur("jin10_full_rate_limit", cfg.Jin10FullRateLimit).
		Bool("jin10_full_include_sitemap", cfg.Jin10FullIncludeSitemap).
		Dur("crypto_social_rate_limit", cfg.CryptoSocialRateLimit).
		Dur("crypto_x_interval", cfg.CryptoXInterval).
		Dur("crypto_telegram_interval", cfg.CryptoTelegramInterval).
		Dur("foresight_newsflash_interval", cfg.ForesightNewsflashInterval).
		Dur("coindesk_zh_latest_interval", cfg.CoinDeskZHLatestInterval).
		Dur("panews_newsflash_interval", cfg.PANewsNewsflashInterval).
		Dur("theblock_latest_interval", cfg.TheBlockLatestInterval).
		Dur("analysis_interval", cfg.AnalysisInterval).
		Dur("session_ttl", cfg.SessionTTL).
		Str("database_driver", cfg.DatabaseDriver).
		Str("database_path", cfg.DatabasePath).
		Str("database_path_abs", dbPath).
		Bool("database_path_resolved", dbPathErr == nil).
		Bool("database_file_exists", fileExists(cfg.DatabasePath)).
		Bool("database_url_configured", strings.TrimSpace(cfg.DatabaseURL) != "").
		Str("postgres_host", cfg.PostgresHost).
		Str("postgres_port", cfg.PostgresPort).
		Str("postgres_database", cfg.PostgresDatabase).
		Str("postgres_user", cfg.PostgresUser).
		Str("postgres_sslmode", cfg.PostgresSSLMode).
		Str("flash_url", cfg.FlashURL).
		Str("headline_url", cfg.HeadlineURL).
		Str("binance_base_url", cfg.BinanceBaseURL).
		Str("coinlore_url", cfg.CoinLoreURL).
		Str("coingecko_url", cfg.CoinGeckoURL).
		Str("crypto_x_url", cfg.CryptoXURL).
		Str("crypto_telegram_url", cfg.CryptoTelegramURL).
		Str("foresight_newsflash_url", cfg.ForesightNewsflashURL).
		Str("coindesk_zh_latest_url", cfg.CoinDeskZHLatestURL).
		Str("panews_newsflash_url", cfg.PANewsNewsflashURL).
		Str("theblock_latest_url", cfg.TheBlockLatestURL).
		Str("eastmoney_kuaixun_url", cfg.EastMoneyKuaixunURL).
		Str("wallstreetcn_a_stock_url", cfg.WallStreetCNAStockURL).
		Str("cls_telegraph_url", cfg.CLSTelegraphURL).
		Str("sina_finance_7x24_url", cfg.SinaFinance7x24URL).
		Str("gateway_web_url", cfg.GatewayWebURL).
		Str("auth_url", cfg.AuthURL).
		Str("wechat_url", cfg.WechatURL).
		Str("content_url", cfg.ContentURL).
		Str("crawler_url", cfg.CrawlerURL).
		Str("analysis_url", cfg.AnalysisURL).
		Str("nlp_url", cfg.NLPURL).
		Str("llm_base_url", cfg.LLMBaseURL).
		Str("llm_model", cfg.LLMModel).
		Str("generated_at", time.Now().UTC().Format(time.RFC3339))
	event.Msg("startup debug info")
}

func LogServiceReady(serviceName, listenAddr string) {
	event := log.Info().Str("service", serviceName)
	if strings.TrimSpace(listenAddr) != "" {
		event = event.Str("addr", listenAddr)
	}
	event.Msg("service started")
}

func jin10FullOptions(cfg config.Config) jin10full.Options {
	return jin10full.Options{
		FlashURL:       cfg.FlashURL,
		HeadlineURL:    cfg.HeadlineURL,
		BackfillDays:   cfg.Jin10FullBackfillDays,
		MaxPagesPerRun: cfg.Jin10FullMaxPages,
		RateLimit:      cfg.Jin10FullRateLimit,
		IncludeSitemap: cfg.Jin10FullIncludeSitemap,
	}
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
