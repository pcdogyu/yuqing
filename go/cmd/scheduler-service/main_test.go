package main

import (
	"testing"

	"github.com/pcdogyu/yuqing/go/internal/testutil"
)

func TestMainWiresSchedulerService(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("scheduler-service"`,
		`scheduler.NewWorker(cfg).Run(ctx)`,
	)
}
