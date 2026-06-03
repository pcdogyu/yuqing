package main

import (
	"testing"

	"github.com/stonedt-yuqing/go-jin10/internal/testutil"
)

func TestMainWiresSchedulerWorker(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("scheduler-worker"`,
		`scheduler.NewWorker(cfg).Run(ctx)`,
	)
}
