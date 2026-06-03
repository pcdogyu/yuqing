package main

import (
	"testing"

	"github.com/stonedt-yuqing/go-jin10/internal/testutil"
)

func TestMainWiresHTTPServer(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("server"`,
		`jin10flash.NewProvider(httpClient, cfg.FlashURL)`,
		`jin10xnews.NewProvider(httpClient, cfg.HeadlineURL)`,
		`httpapi.NewServer(cfg, crawler, store)`,
	)
}
