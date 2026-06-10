package main

import (
	"testing"

	"github.com/stonedt-yuqing/go-jin10/internal/testutil"
)

func TestMainWiresSchedulerWorker(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("scheduler-worker"`,
		`worker := scheduler.NewWorker(cfg)`,
		`defer func() { _ = worker.Close() }()`,
		`worker.Run(ctx)`,
	)
}
