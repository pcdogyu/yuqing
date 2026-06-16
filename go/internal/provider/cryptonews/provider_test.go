package cryptonews

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/provider"
)

func TestParseForesightHTMLFromNuxt(t *testing.T) {
	capturedAt := time.Date(2026, 6, 15, 8, 0, 0, 0, time.UTC)
	html := `<html><script>window.__NUXT__=(function(){return {data:[{list:[{news:[{id:106001,title:"某巨鲸买入 ETH",brief:"以太坊资金流入",content:"\u003Cp\u003EETH 突破阻力，Ethereum 链上活跃\u003C\u002Fp\u003E",tags:[{name:"以太坊"},{name:"ETH"}],source_link:"https:\u002F\u002Fx.com\u002Feth\u002F1",published_at:1781513172}]}]}]}})</script></html>`

	items, err := ParseForesightHTML(html, "https://foresightnews.pro/news", capturedAt)
	if err != nil {
		t.Fatalf("ParseForesightHTML error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %+v", items)
	}
	item := items[0]
	if item.SourceType != provider.SourceTypeForesightNewsflash || item.Title != "某巨鲸买入 ETH" {
		t.Fatalf("unexpected item: %+v", item)
	}
	if !strings.Contains(item.Content, "Ethereum") || !strings.Contains(item.TagFlags, "以太坊") {
		t.Fatalf("expected content and tags to be parsed, got %+v", item)
	}
	if item.DetailURL != "https://foresightnews.pro/news/detail/106001" || item.SourceURL != "https://x.com/eth/1" {
		t.Fatalf("unexpected URLs: %+v", item)
	}
}

func TestParseForesightHTMLSkipsBrokenObjects(t *testing.T) {
	items, err := ParseForesightHTML(`<script>window.__NUXT__={data:[{id:1,broken:true}]}</script>`, "https://foresightnews.pro/news", time.Now().UTC())
	if err != nil {
		t.Fatalf("ParseForesightHTML error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected broken objects to be skipped, got %+v", items)
	}
}

func TestForesightProviderFallsBackToHomeOnBadGateway(t *testing.T) {
	var newsHits, homeHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Language") == "" || r.Header.Get("Referer") == "" {
			t.Fatalf("expected browser headers, got %+v", r.Header)
		}
		switch r.URL.Path {
		case "/news":
			newsHits++
			http.Error(w, "bad gateway", http.StatusBadGateway)
		case "/":
			homeHits++
			_, _ = w.Write([]byte(`<html><script>window.__NUXT__=(function(){return {data:[{list:[{news:[{id:106002,title:"Foresight 首页快讯",brief:"首页降级",content:"ETH news",published_at:1781513172}]}]}]}})</script></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	prov := NewForesightNewsflashProvider(resty.New().SetRetryCount(0), server.URL+"/news")
	items, err := prov.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if newsHits != 1 || homeHits != 1 {
		t.Fatalf("expected /news then / fallback, got news=%d home=%d", newsHits, homeHits)
	}
	if len(items) != 1 || items[0].Title != "Foresight 首页快讯" {
		t.Fatalf("unexpected fallback items: %+v", items)
	}
}

func TestForesightProviderSkipsTemporaryGatewayFailure(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	prov := NewForesightNewsflashProvider(resty.New().SetRetryCount(0), server.URL+"/news")
	items, err := prov.Fetch(context.Background())
	if err != nil {
		t.Fatalf("expected temporary Foresight gateway failure to be skipped, got %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected no items on skipped gateway failure, got %+v", items)
	}
	if hits != 2 {
		t.Fatalf("expected /news and / fallback attempts, got %d", hits)
	}
}

func TestParseCoinDeskHTML(t *testing.T) {
	html := `<html><body>
<div class="flex flex-col">
  <p class="mb-4"><a title="Markets" class="font-title text-subtle uppercase" href="/zh/markets">市场</a></p>
  <a class="text-default mb-4 hover:underline content-card-title" href="/zh/markets/2026/06/15/ethereum-wall-street">
    <h2 class="font-headline-xs font-normal">华尔街正逐步深入布局以太坊</h2>
  </a>
  <p class="font-body text-subtle mb-4">ETH 基础设施已基本建立，采用规模正在扩大。</p>
  <p class="flex gap-2 flex-col"><span class="font-metadata text-subtle">2小时前</span></p>
</div>
</body></html>`

	items, err := ParseCoinDeskHTML(html, "https://www.coindesk.com/zh/latest-crypto-news", time.Now().UTC())
	if err != nil {
		t.Fatalf("ParseCoinDeskHTML error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %+v", items)
	}
	item := items[0]
	if item.SourceType != provider.SourceTypeCoinDeskZHLatest || item.Title != "华尔街正逐步深入布局以太坊" {
		t.Fatalf("unexpected item: %+v", item)
	}
	if item.DetailURL != "https://www.coindesk.com/zh/markets/2026/06/15/ethereum-wall-street" || item.TagFlags != "市场" || item.PublishTimeText != "2小时前" {
		t.Fatalf("unexpected metadata: %+v", item)
	}
}

func TestParseCoinDeskRSS(t *testing.T) {
	rss := []byte(`<?xml version="1.0" encoding="UTF-8"?><rss><channel><item>
<title><![CDATA[Ethereum traders watch ETH ETF flow]]></title>
<link>https://www.coindesk.com/markets/2026/06/15/eth-etf-flow</link>
<guid>eth-rss-1</guid>
<pubDate>Mon, 15 Jun 2026 07:52:41 +0000</pubDate>
<description><![CDATA[ETH liquidity improves.]]></description>
<category>Markets</category>
</item></channel></rss>`)

	items, err := ParseCoinDeskRSS(rss, coindeskRSSURL, time.Now().UTC())
	if err != nil {
		t.Fatalf("ParseCoinDeskRSS error: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Ethereum traders watch ETH ETF flow" {
		t.Fatalf("unexpected rss items: %+v", items)
	}
	if items[0].PublishTime == "" || items[0].TagFlags != "Markets" {
		t.Fatalf("expected rss metadata, got %+v", items[0])
	}
}

func TestParsePANewsRSS(t *testing.T) {
	rss := []byte(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/"><channel><item>
<title><![CDATA[某巨鲸以5倍杠杆开设约3.2万枚ETH多单]]></title>
<description><![CDATA[PANews 6月15日消息，巨鲸开设 ETH 多头仓位。]]></description>
<link>https://www.panewslab.com/zh/articles/eth-long</link>
<guid isPermaLink="false">eth-long</guid>
<pubDate>Mon, 15 Jun 2026 09:06:00 GMT</pubDate>
<content:encoded><![CDATA[<img src="https://uploads.panewslab.com/eth.png"><p>PANews 6月15日消息，Ethereum 链上资金流入，ETH 交易活跃。</p>]]></content:encoded>
</item></channel></rss>`)

	items, err := ParsePANewsRSS(rss, "https://www.panewslab.com/rss.xml?lang=zh&type=NEWS", time.Now().UTC())
	if err != nil {
		t.Fatalf("ParsePANewsRSS error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %+v", items)
	}
	item := items[0]
	if item.SourceType != provider.SourceTypePANewsNewsflash || item.Title != "某巨鲸以5倍杠杆开设约3.2万枚ETH多单" {
		t.Fatalf("unexpected panews item: %+v", item)
	}
	if !strings.Contains(item.Content, "Ethereum") || item.FromText != "PANews" || !item.HasImage {
		t.Fatalf("expected panews content metadata, got %+v", item)
	}
	if item.PublishTime == "" || item.DetailURL != "https://www.panewslab.com/zh/articles/eth-long" {
		t.Fatalf("expected panews timing/url metadata, got %+v", item)
	}
}
