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

func TestParsePageExtractsSinaHiddenList(t *testing.T) {
	body := []byte(`<div class='seaio_list'>
		<div newsdata-id="4942222" newsdata-time="20260618">
			<div>09:49:15</div>
			<a target="_blank" href="https://wap.cj.sina.cn/pc/7x24/4942222">A股三大股指快速拉升盘中转涨，培育钻石、稀土永磁、工业金属等板块涨幅居前。</a>
			<div class="btn-view">16.33万 阅读</div>
		</div>
	</div>`)

	items := ParsePage(body, "https://finance.sina.com.cn/7x24/?tag=10", provider.SourceTypeSinaFinance7x24, "新浪财经", time.Date(2026, 6, 18, 1, 50, 0, 0, time.UTC))
	if len(items) != 1 {
		t.Fatalf("expected one sina hidden item, got %+v", items)
	}
	if items[0].SourceKey != "4942222" || items[0].PublishTime != "2026-06-18 09:49:15" {
		t.Fatalf("unexpected sina metadata: %+v", items[0])
	}
}

func TestParsePageIgnoresFooterRegistrationText(t *testing.T) {
	body := []byte(`<html><body><a href="http://beian.miit.gov.cn">沪金信备 [2021] 2号</a></body></html>`)

	items := ParsePage(body, "https://www.cls.cn/telegraph", provider.SourceTypeCLSTelegraph, "财联社", time.Date(2026, 6, 18, 1, 50, 0, 0, time.UTC))
	if len(items) != 0 {
		t.Fatalf("expected footer registration text to be ignored, got %+v", items)
	}
}

func TestParseCLSNextDataExtractsRollData(t *testing.T) {
	body := []byte(`<script>
	__NEXT_DATA__ = {"props":{"initialState":{"roll_data":[{"id":2403172,"ctime":1781747715,"title":"钢铁板块异动拉升 抚顺特钢涨停","brief":"财联社6月18日电，钢铁板块盘中异动拉升。","shareurl":"https://api3.cls.cn/share/article/2403172","stock_list":[{"StockID":"sh600117","name":"西宁特钢"}]}]}}};
	</script>`)

	items := parseCLSNextData(body, "https://www.cls.cn/telegraph", time.Date(2026, 6, 18, 1, 50, 0, 0, time.UTC))
	if len(items) != 1 {
		t.Fatalf("expected one cls item, got %+v", items)
	}
	if items[0].SourceType != provider.SourceTypeCLSTelegraph || items[0].SourceKey != "2403172" || items[0].TagFlags != "sh600117" {
		t.Fatalf("unexpected cls item: %+v", items[0])
	}
}

func TestParseWallStreetCNLivesExtractsAStockItems(t *testing.T) {
	body := []byte(`{"code":20000,"message":"OK","data":{"items":[{"id":123,"content_text":"机器人概念股反复活跃，晋拓股份、天润工业涨停。","uri":"https://wallstreetcn.com/livenews/123","display_time":1781747577},{"id":124,"content_text":"美元指数小幅波动","display_time":1781747578}]}}`)

	items, err := parseWallStreetCNLives(body, "https://wallstreetcn.com/live/a-stock", time.Date(2026, 6, 18, 1, 50, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("parseWallStreetCNLives error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one wallstreetcn a-stock item, got %+v", items)
	}
	if items[0].SourceType != provider.SourceTypeWallStreetCNAStock || items[0].SourceKey != "123" {
		t.Fatalf("unexpected wallstreetcn item: %+v", items[0])
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
