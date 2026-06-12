package main

import (
	"testing"

	"github.com/pcdogyu/yuqing/go/internal/testutil"
)

func TestMainWiresWorkerLoops(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("worker"`,
		`runLoop(ctx, crawler, provider.SourceTypeFlash, cfg.FlashInterval)`,
		`runLoop(ctx, crawler, provider.SourceTypeHeadline, cfg.HeadlineInterval)`,
		`runLoop(ctx, crawler, provider.SourceTypeJin10Full, cfg.Jin10FullInterval)`,
		`runLoop(ctx, crawler, provider.SourceTypeCryptoX, cfg.CryptoXInterval)`,
		`runLoop(ctx, crawler, provider.SourceTypeCryptoTelegram, cfg.CryptoTelegramInterval)`,
	)
}
