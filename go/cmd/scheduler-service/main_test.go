package main

import (
	"testing"

	"github.com/stonedt-yuqing/go-jin10/internal/testutil"
)

func TestMainWiresSchedulerService(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("scheduler-service"`,
		`scheduler.NewWorker(cfg).Run(ctx)`,
	)
}
