package main

import (
	"context"
	"os/signal"
	"syscall"

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
	app.LogServiceReady("scheduler-service", cfg.SchedulerAddr)
	log.Info().Str("service", "scheduler-service").Msg("background scheduler loop starting")
	scheduler.NewWorker(cfg).Run(ctx)
}
