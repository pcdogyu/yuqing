package main

import (
	"testing"

	"github.com/pcdogyu/yuqing/go/internal/testutil"
)

func TestMainWiresHTTPServer(t *testing.T) {
	testutil.AssertFileContains(t, "main.go",
		`app.LogStartup("server"`,
		`jin10flash.NewProvider(httpClient, cfg.FlashURL)`,
		`jin10full.NewProvider(httpClient, store, jin10full.Options`,
		`jin10xnews.NewProvider(httpClient, cfg.HeadlineURL)`,
		`cryptosocial.NewXProvider(httpClient, cfg.CryptoXURL, cfg.CryptoXToken)`,
		`cryptosocial.NewTelegramProvider(httpClient, cfg.CryptoTelegramURL, cfg.CryptoTelegramToken)`,
		`cryptonews.NewForesightNewsflashProvider(httpClient, cfg.ForesightNewsflashURL)`,
		`cryptonews.NewCoinDeskZHLatestProvider(httpClient, cfg.CoinDeskZHLatestURL)`,
		`cryptonews.NewPANewsNewsflashProvider(httpClient, cfg.PANewsNewsflashURL)`,
		`eastmoneykuaixun.NewProvider(httpClient, cfg.EastMoneyKuaixunURL)`,
		`httpapi.NewServer(cfg, crawler, store)`,
	)
}
