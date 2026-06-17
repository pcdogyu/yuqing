package publicfinance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/provider"
)

func TestParsePageExtractsFinanceLinksAndJSONText(t *testing.T) {
	body := []byte(`<html><body>
		<a href="/live/123">A股三大指数集体上涨，券商板块活跃</a>
		<a href="/about">关于我们</a>
		<script>window.__DATA__={"items":[{"title":"银行板块午后拉升，资金回流权重股","time":"2026-06-17 10:20:00"}]}</script>
	</body></html>`)

	items := ParsePage(body, "https://wallstreetcn.com/live/a-stock", provider.SourceTypeWallStreetCNAStock, "华尔街见闻", time.Date(2026, 6, 17, 2, 30, 0, 0, time.UTC))
	if len(items) != 2 {
		t.Fatalf("expected two finance items, got %+v", items)
	}
	if items[0].SourceType != provider.SourceTypeWallStreetCNAStock || items[0].FromText != "华尔街见闻" {
		t.Fatalf("unexpected source metadata: %+v", items[0])
	}
	if items[0].Title != "A股三大指数集体上涨，券商板块活跃" || items[0].DetailURL != "https://wallstreetcn.com/live/123" {
		t.Fatalf("unexpected link item: %+v", items[0])
	}
	if items[1].Title != "银行板块午后拉升，资金回流权重股" {
		t.Fatalf("unexpected json item: %+v", items[1])
	}
}

func TestProviderFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Language") == "" {
			t.Fatalf("expected browser-like headers, got %+v", r.Header)
		}
		_, _ = w.Write([]byte(`<a href="/telegraph/1">财联社：A股半导体板块震荡走强</a>`))
	}))
	defer server.Close()

	prov := NewCLSTelegraphProvider(resty.New().SetRetryCount(0), server.URL+"/telegraph")
	items, err := prov.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if len(items) != 1 || items[0].SourceType != provider.SourceTypeCLSTelegraph || items[0].FromText != "财联社" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestConstructorsUseDefaultURLs(t *testing.T) {
	if got := NewWallStreetCNAStockProvider(resty.New(), "").pageURL; got != defaultWallStreetCNAStockURL {
		t.Fatalf("unexpected wallstreetcn default url: %s", got)
	}
	if got := NewCLSTelegraphProvider(resty.New(), "").pageURL; got != defaultCLSTelegraphURL {
		t.Fatalf("unexpected cls default url: %s", got)
	}
	if got := NewSinaFinance7x24Provider(resty.New(), "").pageURL; got != defaultSinaFinance7x24URL {
		t.Fatalf("unexpected sina default url: %s", got)
	}
}
