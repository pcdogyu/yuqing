package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/pcdogyu/yuqing/go/internal/app"
	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/logging"
	"github.com/pcdogyu/yuqing/go/internal/portal"
)

func main() {
	cfg := config.Load()
	logging.Setup(cfg.LogLevel, "gateway-web")
	app.LogStartup("gateway-web", cfg.GatewayWebAddr, cfg)
	server := &http.Server{
		Addr:              cfg.GatewayWebAddr,
		Handler:           portal.NewServer(cfg).Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	app.LogServiceReady("gateway-web", cfg.GatewayWebAddr)
	go func() {
		log.Info().Str("service", "gateway-web").Str("addr", cfg.GatewayWebAddr).Msg("listening")
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
