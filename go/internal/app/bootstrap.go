package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/provider"
	"github.com/stonedt-yuqing/go-jin10/internal/provider/cryptosocial"
	"github.com/stonedt-yuqing/go-jin10/internal/provider/jin10flash"
	"github.com/stonedt-yuqing/go-jin10/internal/provider/jin10full"
	"github.com/stonedt-yuqing/go-jin10/internal/provider/jin10xnews"
	"github.com/stonedt-yuqing/go-jin10/internal/service"
	sqlitestore "github.com/stonedt-yuqing/go-jin10/internal/store/sqlite"
)

var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildTime = "unknown"
)

func NewStore(cfg config.Config) (*sqlitestore.Store, error) {
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

func NewCrawler(cfg config.Config, store *sqlitestore.Store) *service.Crawler {
	httpClient := resty.New().
		SetTimeout(cfg.HTTPTimeout).
		SetRetryCount(2).
		SetHeader("User-Agent", cfg.UserAgent)
	registry := provider.Registry{
		Flash:     jin10flash.NewProvider(httpClient, cfg.FlashURL),
		Headline:  jin10xnews.NewProvider(httpClient, cfg.HeadlineURL),
		Jin10Full: jin10full.NewProvider(httpClient, store, jin10FullOptions(cfg)),
	}
	if cfg.CryptoXURL != "" {
		registry.CryptoX = cryptosocial.NewXProvider(httpClient, cfg.CryptoXURL, cfg.CryptoXToken)
	}
	if cfg.CryptoTelegramURL != "" {
		registry.CryptoTelegram = cryptosocial.NewTelegramProvider(httpClient, cfg.CryptoTelegramURL, cfg.CryptoTelegramToken)
	}
	return service.NewCrawler(store, registry, nil)
}

func LogStartup(serviceName, listenAddr string, cfg config.Config) {
	dbPath, dbPathErr := filepath.Abs(cfg.DatabasePath)

	event := log.Debug().
		Str("service", serviceName).
		Str("version", Version).
		Str("git_commit", GitCommit).
		Str("build_time", BuildTime).
		Str("go_version", runtime.Version()).
		Str("log_level", cfg.LogLevel).
		Str("listen_addr", listenAddr).
		Dur("http_timeout", cfg.HTTPTimeout).
		Dur("flash_interval", cfg.FlashInterval).
		Dur("headline_interval", cfg.HeadlineInterval).
		Dur("jin10_full_interval", cfg.Jin10FullInterval).
		Bool("jin10_full_enabled", cfg.Jin10FullEnabled).
		Int("jin10_full_backfill_days", cfg.Jin10FullBackfillDays).
		Int("jin10_full_max_pages", cfg.Jin10FullMaxPages).
		Dur("jin10_full_rate_limit", cfg.Jin10FullRateLimit).
		Bool("jin10_full_include_sitemap", cfg.Jin10FullIncludeSitemap).
		Dur("crypto_x_interval", cfg.CryptoXInterval).
		Dur("crypto_telegram_interval", cfg.CryptoTelegramInterval).
		Dur("analysis_interval", cfg.AnalysisInterval).
		Dur("session_ttl", cfg.SessionTTL).
		Str("database_path", cfg.DatabasePath).
		Str("database_path_abs", dbPath).
		Bool("database_path_resolved", dbPathErr == nil).
		Bool("database_file_exists", fileExists(cfg.DatabasePath)).
		Str("flash_url", cfg.FlashURL).
		Str("headline_url", cfg.HeadlineURL).
		Str("binance_base_url", cfg.BinanceBaseURL).
		Str("coinlore_url", cfg.CoinLoreURL).
		Str("coingecko_url", cfg.CoinGeckoURL).
		Str("crypto_x_url", cfg.CryptoXURL).
		Str("crypto_telegram_url", cfg.CryptoTelegramURL).
		Str("gateway_web_url", cfg.GatewayWebURL).
		Str("auth_url", cfg.AuthURL).
		Str("content_url", cfg.ContentURL).
		Str("crawler_url", cfg.CrawlerURL).
		Str("analysis_url", cfg.AnalysisURL).
		Str("nlp_url", cfg.NLPURL).
		Str("llm_base_url", cfg.LLMBaseURL).
		Str("llm_model", cfg.LLMModel).
		Str("generated_at", time.Now().UTC().Format(time.RFC3339))
	event.Msg("startup debug info")
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
