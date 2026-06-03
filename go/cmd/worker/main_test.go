package main

import (
	"testing"

	"github.com/stonedt-yuqing/go-jin10/internal/testutil"
)

func TestMainWiresWorkerLoops(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("worker"`,
		`runLoop(ctx, crawler, provider.SourceTypeFlash, cfg.FlashInterval)`,
		`runLoop(ctx, crawler, provider.SourceTypeHeadline, cfg.HeadlineInterval)`,
	)
}
