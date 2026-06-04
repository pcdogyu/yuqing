package main

import (
	"testing"

	"github.com/stonedt-yuqing/go-jin10/internal/testutil"
)

func TestMainWiresHTTPServer(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("server"`,
		`jin10flash.NewProvider(httpClient, cfg.FlashURL)`,
		`jin10full.NewProvider(httpClient, store, jin10full.Options`,
		`jin10xnews.NewProvider(httpClient, cfg.HeadlineURL)`,
		`cryptosocial.NewXProvider(httpClient, cfg.CryptoXURL, cfg.CryptoXToken)`,
		`cryptosocial.NewTelegramProvider(httpClient, cfg.CryptoTelegramURL, cfg.CryptoTelegramToken)`,
		`httpapi.NewServer(cfg, crawler, store)`,
	)
}
