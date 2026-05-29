package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/logging"
	"github.com/stonedt-yuqing/go-jin10/internal/portal"
)

func main() {
	cfg := config.Load()
	logging.Setup(cfg.LogLevel)
	server := &http.Server{
		Addr:              cfg.PortalWebAddr,
		Handler:           portal.NewServer(cfg).Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Info().Str("service", "portal-web").Str("addr", cfg.PortalWebAddr).Msg("listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("server stopped")
		}
	}()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
