package main

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"

	"github.com/pcdogyu/yuqing/go/internal/app"
	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/logging"
	"github.com/pcdogyu/yuqing/go/internal/provider"
	"github.com/pcdogyu/yuqing/go/internal/provider/cryptonews"
	"github.com/pcdogyu/yuqing/go/internal/provider/cryptosocial"
	"github.com/pcdogyu/yuqing/go/internal/provider/jin10flash"
	"github.com/pcdogyu/yuqing/go/internal/provider/jin10full"
	"github.com/pcdogyu/yuqing/go/internal/provider/jin10xnews"
	"github.com/pcdogyu/yuqing/go/internal/service"
	sqlitestore "github.com/pcdogyu/yuqing/go/internal/store/sqlite"
)

func main() {
	cfg := config.Load()
	logging.Setup(cfg.LogLevel, "worker")
	app.LogStartup("worker", "", cfg)

	store, err := sqlitestore.New(cfg.DatabasePath)
	if err != nil {
		log.Fatal().Err(err).Msg("open sqlite store")
	}
	defer store.Close()

	httpClient := resty.New().
		SetTimeout(cfg.HTTPTimeout).
		SetRetryCount(2).
		SetHeader("User-Agent", cfg.UserAgent)

	registry := provider.Registry{
		Flash:    jin10flash.NewProvider(httpClient, cfg.FlashURL),
		Headline: jin10xnews.NewProvider(httpClient, cfg.HeadlineURL),
		Jin10Full: jin10full.NewProvider(httpClient, store, jin10full.Options{
			FlashURL:       cfg.FlashURL,
			HeadlineURL:    cfg.HeadlineURL,
			BackfillDays:   cfg.Jin10FullBackfillDays,
			MaxPagesPerRun: cfg.Jin10FullMaxPages,
			RateLimit:      cfg.Jin10FullRateLimit,
			IncludeSitemap: cfg.Jin10FullIncludeSitemap,
		}),
	}
	if cfg.CryptoXURL != "" {
		registry.CryptoX = cryptosocial.NewXProvider(httpClient, cfg.CryptoXURL, cfg.CryptoXToken)
	}
	if cfg.CryptoTelegramURL != "" {
		registry.CryptoTelegram = cryptosocial.NewTelegramProvider(httpClient, cfg.CryptoTelegramURL, cfg.CryptoTelegramToken)
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
	crawler := service.NewCrawler(store, registry, nil)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go runLoop(ctx, crawler, provider.SourceTypeFlash, cfg.FlashInterval)
	go runLoop(ctx, crawler, provider.SourceTypeHeadline, cfg.HeadlineInterval)
	if cfg.Jin10FullEnabled {
		go runLoop(ctx, crawler, provider.SourceTypeJin10Full, cfg.Jin10FullInterval)
	}
	if cfg.CryptoXURL != "" {
		go runLoop(ctx, crawler, provider.SourceTypeCryptoX, cfg.CryptoXInterval)
	}
	if cfg.CryptoTelegramURL != "" {
		go runLoop(ctx, crawler, provider.SourceTypeCryptoTelegram, cfg.CryptoTelegramInterval)
	}
	if cfg.ForesightNewsflashURL != "" {
		go runLoop(ctx, crawler, provider.SourceTypeForesightNewsflash, cfg.ForesightNewsflashInterval)
	}
	if cfg.CoinDeskZHLatestURL != "" {
		go runLoop(ctx, crawler, provider.SourceTypeCoinDeskZHLatest, cfg.CoinDeskZHLatestInterval)
	}
	if cfg.PANewsNewsflashURL != "" {
		go runLoop(ctx, crawler, provider.SourceTypePANewsNewsflash, cfg.PANewsNewsflashInterval)
	}

	app.LogServiceReady("worker", "")
	<-ctx.Done()
	log.Info().Msg("worker stopped")
}

func runLoop(ctx context.Context, crawler *service.Crawler, sourceType string, interval time.Duration) {
	runOnce(ctx, crawler, sourceType)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runOnce(ctx, crawler, sourceType)
		}
	}
}

func runOnce(ctx context.Context, crawler *service.Crawler, sourceType string) {
	summary, err := crawler.Run(ctx, sourceType)
	if err != nil {
		log.Error().Err(err).Str("source_type", sourceType).Msg("crawl failed")
		return
	}

	log.Info().
		Str("source_type", sourceType).
		Int("fetched", summary.FetchedCount).
		Int("inserted", summary.InsertedCount).
		Int("updated", summary.UpdatedCount).
		Msg("crawl completed")
}
