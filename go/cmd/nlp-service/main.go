package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/stonedt-yuqing/go-jin10/internal/app"
	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/logging"
	"github.com/stonedt-yuqing/go-jin10/internal/nlp"
)

func main() {
	cfg := config.Load()
	logging.Setup(cfg.LogLevel, "nlp-service")
	app.LogStartup("nlp-service", cfg.NLPAddr, cfg)
	server := &http.Server{
		Addr:              cfg.NLPAddr,
		Handler:           nlp.NewService().Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	app.LogServiceReady("nlp-service", cfg.NLPAddr)
	go func() {
		log.Info().Str("service", "nlp-service").Str("addr", cfg.NLPAddr).Msg("listening")
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
