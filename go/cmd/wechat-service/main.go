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
	"github.com/pcdogyu/yuqing/go/internal/auth"
	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/logging"
)

func main() {
	cfg := config.Load()
	logging.Setup(cfg.LogLevel, "wechat-service")
	app.LogStartup("wechat-service", cfg.WechatAddr, cfg)

	store, err := app.NewStore(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("open store")
	}
	defer store.Close()

	server := &http.Server{
		Addr:              cfg.WechatAddr,
		Handler:           auth.NewService(cfg, store).WechatRouter(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	app.LogServiceReady("wechat-service", cfg.WechatAddr)
	run(server, "wechat-service", cfg.WechatAddr)
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
