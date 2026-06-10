package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/stonedt-yuqing/go-jin10/internal/app"
	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/logging"
	"github.com/stonedt-yuqing/go-jin10/internal/scheduler"
)

func main() {
	cfg := config.Load()
	logging.Setup(cfg.LogLevel)
	app.LogStartup("scheduler-worker", cfg.SchedulerAddr, cfg)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	worker := scheduler.NewWorker(cfg)
	defer func() { _ = worker.Close() }()
	worker.Run(ctx)
}
