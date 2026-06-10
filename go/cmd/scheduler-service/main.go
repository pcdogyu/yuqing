package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"

	"github.com/stonedt-yuqing/go-jin10/internal/app"
	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/logging"
	"github.com/stonedt-yuqing/go-jin10/internal/scheduler"
)

func main() {
	cfg := config.Load()
	logging.Setup(cfg.LogLevel)
	app.LogStartup("scheduler-service", cfg.SchedulerAddr, cfg)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	app.LogServiceReady("scheduler-service", cfg.SchedulerAddr)
	log.Info().Str("service", "scheduler-service").Msg("background scheduler loop starting")
	scheduler.NewWorker(cfg).Run(ctx)
}
