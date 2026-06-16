package eastmoneykuaixun

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

func TestParseResponse(t *testing.T) {
	body := []byte(`{"code":"1","message":"success","data":{"fastNewsList":[{"summary":" 以下是中铝国际盘口异动快照。 ","code":"202606163772915624","titleColor":0,"realSort":"1781590197015624","showTime":"2026-06-16 14:09:57","title":"中铝国际6月16日快速回调","stockList":["1.601068"],"image":["https://example.com/a.png"]},{"summary":"重复","code":"202606163772915624","showTime":"2026-06-16 14:09:57","title":"中铝国际6月16日快速回调","stockList":["1.601068"],"image":[]}]}}`)

	items, err := ParseResponse(body, "https://kuaixun.eastmoney.com/", time.Date(2026, 6, 16, 6, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("ParseResponse error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected two distinct items because summary differs, got %+v", items)
	}
	item := items[0]
	if item.SourceType != provider.SourceTypeEastMoneyKuaixun || item.FromText != "东方财富网" {
		t.Fatalf("unexpected source metadata: %+v", item)
	}
	if item.Title != "中铝国际6月16日快速回调" || item.Summary != "以下是中铝国际盘口异动快照。" || item.Content != item.Summary {
		t.Fatalf("unexpected text fields: %+v", item)
	}
	if item.PublishTime != "2026-06-16 14:09:57" || item.PublishTimeText != item.PublishTime {
		t.Fatalf("unexpected publish time: %+v", item)
	}
	if item.DetailURL != "https://finance.eastmoney.com/a/202606163772915624.html" || item.SourceURL != item.DetailURL {
		t.Fatalf("unexpected urls: %+v", item)
	}
	if item.TagFlags != "1.601068" || !item.HasImage || item.ExternalSourceHost != "kuaixun.eastmoney.com" {
		t.Fatalf("unexpected metadata: %+v", item)
	}
}

func TestParseResponseJSONP(t *testing.T) {
	body := []byte(`callback({"code":"1","data":{"fastNewsList":[{"summary":"SpaceX上涨","code":"202606163772915593","showTime":"2026-06-16 14:07:49","title":"SpaceX股价上涨","stockList":["105.SPCX"],"image":[]}]}});`)

	items, err := ParseResponse(body, "https://kuaixun.eastmoney.com/", time.Now().UTC())
	if err != nil {
		t.Fatalf("ParseResponse jsonp error: %v", err)
	}
	if len(items) != 1 || items[0].Title != "SpaceX股价上涨" {
		t.Fatalf("unexpected jsonp items: %+v", items)
	}
}

func TestProviderFetchUsesEastMoneyListAPI(t *testing.T) {
	var gotPageSize, gotClient, gotBiz string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") == "" || !strings.Contains(r.Header.Get("Accept-Language"), "zh-CN") {
			t.Fatalf("expected browser-like headers, got %+v", r.Header)
		}
		gotPageSize = r.URL.Query().Get("pageSize")
		gotClient = r.URL.Query().Get("client")
		gotBiz = r.URL.Query().Get("biz")
		_, _ = w.Write([]byte(`{"code":"1","data":{"fastNewsList":[{"summary":"财经快讯正文","code":"20260616001","showTime":"2026-06-16 09:05:00","title":"东方财富快讯","stockList":[],"image":[]}]}}`))
	}))
	defer server.Close()

	prov := NewProvider(resty.New().SetRetryCount(0), server.URL+"/comm/web/getFastNewsList")
	items, err := prov.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if gotClient != "web" || gotBiz != "web_724" || gotPageSize != "50" {
		t.Fatalf("unexpected query params: client=%q biz=%q pageSize=%q", gotClient, gotBiz, gotPageSize)
	}
	if len(items) != 1 || items[0].Title != "东方财富快讯" {
		t.Fatalf("unexpected items: %+v", items)
	}
}
