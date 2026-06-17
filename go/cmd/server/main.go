package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"

	"github.com/pcdogyu/yuqing/go/internal/app"
	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/httpapi"
	"github.com/pcdogyu/yuqing/go/internal/logging"
	"github.com/pcdogyu/yuqing/go/internal/provider"
	"github.com/pcdogyu/yuqing/go/internal/provider/cryptonews"
	"github.com/pcdogyu/yuqing/go/internal/provider/cryptosocial"
	"github.com/pcdogyu/yuqing/go/internal/provider/eastmoneykuaixun"
	"github.com/pcdogyu/yuqing/go/internal/provider/jin10flash"
	"github.com/pcdogyu/yuqing/go/internal/provider/jin10full"
	"github.com/pcdogyu/yuqing/go/internal/provider/jin10xnews"
	"github.com/pcdogyu/yuqing/go/internal/provider/publicfinance"
	"github.com/pcdogyu/yuqing/go/internal/service"
)

func main() {
	cfg := config.Load()
	logging.Setup(cfg.LogLevel, "server")
	app.LogStartup("server", cfg.ListenAddr, cfg)

	store, err := app.NewStore(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("open store")
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
	crawler := service.NewCrawler(store, registry, nil)
	api := httpapi.NewServer(cfg, crawler, store)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	app.LogServiceReady("server", cfg.ListenAddr)

	go func() {
		log.Info().Str("addr", cfg.ListenAddr).Msg("http server listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("http server stopped")
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("graceful shutdown failed")
	}
}
