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

	"github.com/stonedt-yuqing/go-jin10/internal/app"
	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/httpapi"
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
	app.LogStartup("server", cfg.ListenAddr, cfg)

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
	})
	api := httpapi.NewServer(cfg, crawler, store)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

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
