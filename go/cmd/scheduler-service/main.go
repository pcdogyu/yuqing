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
	"github.com/pcdogyu/yuqing/go/internal/scheduler"
)

func main() {
	cfg := config.Load()
	logging.Setup(cfg.LogLevel, "scheduler-service")
	app.LogStartup("scheduler-service", cfg.SchedulerAddr, cfg)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	worker := scheduler.NewWorker(cfg)
	defer func() { _ = worker.Close() }()

	server := &http.Server{
		Addr:              cfg.SchedulerAddr,
		Handler:           worker.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	app.LogServiceReady("scheduler-service", cfg.SchedulerAddr)
	go func() {
		log.Info().Str("addr", cfg.SchedulerAddr).Msg("scheduler management api listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("scheduler management api stopped")
			cancel()
		}
	}()

	log.Info().Str("service", "scheduler-service").Msg("background scheduler loop starting")
	go worker.Run(ctx)
	<-ctx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("scheduler management api shutdown failed")
		os.Exit(1)
	}
}
