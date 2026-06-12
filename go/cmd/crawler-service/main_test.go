package main

import (
	"testing"

	"github.com/pcdogyu/yuqing/go/internal/testutil"
)

func TestMainWiresCrawlerService(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("crawler-service"`,
		`crawler := app.NewCrawler(cfg, store)`,
		`crawlerapi.NewService(cfg, crawler)`,
	)
}
