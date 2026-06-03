package main

import (
	"testing"

	"github.com/stonedt-yuqing/go-jin10/internal/testutil"
)

func TestMainWiresCrawlerService(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("crawler-service"`,
		`crawler := app.NewCrawler(cfg, store)`,
		`crawlerapi.NewService(cfg, crawler)`,
	)
}
