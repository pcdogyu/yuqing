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
	releasesvc "github.com/pcdogyu/yuqing/go/internal/release"
)

func main() {
	cfg := config.Load()
	logging.Setup(cfg.LogLevel, "release-service")
	app.LogStartup("release-service", cfg.ReleaseAddr, cfg)

	server := &http.Server{
		Addr:              cfg.ReleaseAddr,
		Handler:           releasesvc.NewService(cfg).Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	app.LogServiceReady("release-service", cfg.ReleaseAddr)
	run(server, "release-service", cfg.ReleaseAddr)
}

func run(server *http.Server, name, addr string) {
	go func() {
		log.Info().Str("service", name).Str("addr", addr).Msg("listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Str("service", name).Msg("server stopped")
		}
	}()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
