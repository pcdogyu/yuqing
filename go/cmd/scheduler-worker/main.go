package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/pcdogyu/yuqing/go/internal/app"
	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/logging"
	"github.com/pcdogyu/yuqing/go/internal/scheduler"
)

func main() {
	cfg := config.Load()
	logging.Setup(cfg.LogLevel, "scheduler-worker")
	app.LogStartup("scheduler-worker", cfg.SchedulerAddr, cfg)
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	worker := scheduler.NewWorker(cfg)
	defer func() { _ = worker.Close() }()
	app.LogServiceReady("scheduler-worker", cfg.SchedulerAddr)
	worker.Run(ctx)
}
