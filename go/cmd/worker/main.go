package main

import (
	"context"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"

	"github.com/stonedt-yuqing/go-jin10/internal/app"
	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/logging"
	"github.com/stonedt-yuqing/go-jin10/internal/provider"
	"github.com/stonedt-yuqing/go-jin10/internal/provider/jin10flash"
	"github.com/stonedt-yuqing/go-jin10/internal/provider/jin10xnews"
	"github.com/stonedt-yuqing/go-jin10/internal/service"
	sqlitestore "github.com/stonedt-yuqing/go-jin10/internal/store/sqlite"
)

func main() {
	cfg := config.Load()
	logging.Setup(cfg.LogLevel)
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

	crawler := service.NewCrawler(store, provider.Registry{
		Flash:    jin10flash.NewProvider(httpClient, cfg.FlashURL),
		Headline: jin10xnews.NewProvider(httpClient, cfg.HeadlineURL),
	}, nil)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go runLoop(ctx, crawler, provider.SourceTypeFlash, cfg.FlashInterval)
	go runLoop(ctx, crawler, provider.SourceTypeHeadline, cfg.HeadlineInterval)

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
