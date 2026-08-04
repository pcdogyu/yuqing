package eastmoneykuaixun

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"

	"github.com/pcdogyu/yuqing/go/internal/model"
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

func TestParseSearchResponse(t *testing.T) {
	body := []byte(`yuqing({"result":{"cmsArticleWebOld":[{"date":"2026-06-17 09:05:00","code":"20260617001","title":"<em>AI</em>算力订单增加","content":"人工智能产业链活跃","mediaName":"证券时报","url":"http://finance.eastmoney.com/a/20260617001.html"}]}})`)

	items, err := ParseSearchResponse(body, "https://kuaixun.eastmoney.com/", time.Now().UTC())
	if err != nil {
		t.Fatalf("ParseSearchResponse error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one search item, got %+v", items)
	}
	item := items[0]
	if item.SourceType != provider.SourceTypeEastMoneyKuaixun || item.Title != "AI算力订单增加" || item.Summary != "人工智能产业链活跃" {
		t.Fatalf("unexpected search item: %+v", item)
	}
	if item.PublishTime != "2026-06-17 09:05:00" || item.FromText != "证券时报" || item.DetailURL != "http://finance.eastmoney.com/a/20260617001.html" {
		t.Fatalf("unexpected search metadata: %+v", item)
	}
}

func TestParseDetailHTML(t *testing.T) {
	body := `<!doctype html><html><head><title>房地产板块震荡走弱 _ 东方财富网</title><meta name="description" content="房地产板块震荡走弱，京能置业触及跌停。[点击查看全文]"><link rel="canonical" href="https://finance.eastmoney.com/a/20260630001.html"></head><body><div id="ContentBody"><p>房地产板块震荡走弱，京能置业触及跌停。</p><p>*ST南置此前跌停，中洲控股、香江控股等跌幅居前。</p></div></body></html>`

	title, content, summary, sourceURL, source := parseDetailHTML(body)
	if title != "房地产板块震荡走弱" {
		t.Fatalf("unexpected detail title: %q", title)
	}
	if !strings.Contains(content, "京能置业触及跌停") || !strings.Contains(content, "*ST南置此前跌停") || strings.Contains(content, "点击查看全文") {
		t.Fatalf("unexpected detail content: %q", content)
	}
	if summary != "房地产板块震荡走弱，京能置业触及跌停。" {
		t.Fatalf("unexpected detail summary: %q", summary)
	}
	if sourceURL != "https://finance.eastmoney.com/a/20260630001.html" || source != "东方财富网" {
		t.Fatalf("unexpected detail metadata sourceURL=%q source=%q", sourceURL, source)
	}
}

func TestProviderEnrichesReadMoreDetail(t *testing.T) {
	detailCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		detailCalls++
		_, _ = w.Write([]byte(`<html><head><title>房地产板块震荡走弱 _ 东方财富网</title><meta name="description" content="房地产板块震荡走弱，京能置业触及跌停。"></head><body><div id="ContentBody">房地产板块震荡走弱，京能置业触及跌停，*ST南置此前跌停，中洲控股、香江控股等跌幅居前。</div></body></html>`))
	}))
	defer server.Close()

	prov := NewProvider(resty.New().SetRetryCount(0), "")
	items := prov.enrichItems(context.Background(), []model.Item{{
		SourceType:  provider.SourceTypeEastMoneyKuaixun,
		Title:       "房地产板块震荡走弱",
		Summary:     "房地产板块震荡走弱。[点击查看全文]",
		Content:     "房地产板块震荡走弱。[点击查看全文]",
		DetailURL:   server.URL + "/a/20260630001.html",
		PublishTime: "2026-06-30 13:58:00",
	}})
	if detailCalls != 1 {
		t.Fatalf("expected one detail request, got %d", detailCalls)
	}
	if len(items) != 1 || !strings.Contains(items[0].Content, "*ST南置此前跌停") || strings.Contains(items[0].Content, "点击查看全文") {
		t.Fatalf("expected detail content to replace read-more stub, got %+v", items)
	}
}

func TestProviderSkipsDetailForCompleteListItem(t *testing.T) {
	detailCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		detailCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	prov := NewProvider(resty.New().SetRetryCount(0), "")
	items := prov.enrichItems(context.Background(), []model.Item{{
		SourceType:  provider.SourceTypeEastMoneyKuaixun,
		Title:       "新叶股份6月30日快速上涨",
		Summary:     "新叶股份盘中快速上涨，5分钟内涨幅超过2%。",
		Content:     "新叶股份盘中快速上涨，5分钟内涨幅超过2%。",
		DetailURL:   server.URL + "/a/20260630002.html",
		PublishTime: "2026-06-30 14:02:00",
	}})
	if detailCalls != 0 {
		t.Fatalf("expected no detail request for complete list item, got %d", detailCalls)
	}
	if len(items) != 1 || items[0].Content == "" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestProviderFetchUsesEastMoneyListAPI(t *testing.T) {
	var gotPageSize, gotClient, gotBiz string
	searchCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/comm/web/getFastNewsList":
			if r.Header.Get("Referer") == "" || !strings.Contains(r.Header.Get("Accept-Language"), "zh-CN") {
				t.Fatalf("expected browser-like headers, got %+v", r.Header)
			}
			gotPageSize = r.URL.Query().Get("pageSize")
			gotClient = r.URL.Query().Get("client")
			gotBiz = r.URL.Query().Get("biz")
			_, _ = w.Write([]byte(`{"code":"1","data":{"fastNewsList":[{"summary":"财经快讯正文","code":"20260616001","showTime":"2026-06-16 09:05:00","title":"东方财富快讯","stockList":[],"image":[]}]}}`))
		case "/search/jsonp":
			searchCalls++
			if r.URL.Query().Get("param") == "" || r.URL.Query().Get("cb") == "" {
				t.Fatalf("expected search params, got %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`yuqing({"result":{"cmsArticleWebOld":[{"date":"2026-06-16 09:08:00","title":"东方财富搜索新闻","content":"A股新闻搜索补充","mediaName":"东方财富网","url":"http://finance.eastmoney.com/a/20260616002.html"}]}})`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()

	prov := NewProvider(resty.New().SetRetryCount(0), server.URL+"/comm/web/getFastNewsList")
	prov.searchAPIURL = server.URL + "/search/jsonp"
	items, err := prov.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}
	if gotClient != "web" || gotBiz != "web_724" || gotPageSize != "50" {
		t.Fatalf("unexpected query params: client=%q biz=%q pageSize=%q", gotClient, gotBiz, gotPageSize)
	}
	if searchCalls != 0 {
		t.Fatalf("expected kuaixun provider not to call search endpoint after source split, got %d calls", searchCalls)
	}
	if len(items) != 1 || items[0].Title != "东方财富快讯" || items[0].SourceType != provider.SourceTypeEastMoneyKuaixun {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestFullProviderFetchUsesSearchSourceType(t *testing.T) {
	searchCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/jsonp":
			searchCalls++
			_, _ = w.Write([]byte(`yuqing({"result":{"cmsArticleWebOld":[{"date":"2026-06-16 09:08:00","title":"东方财富搜索新闻","content":"A股新闻搜索补充","mediaName":"东方财富网","url":"http://finance.eastmoney.com/a/20260616002.html"}]}})`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()

	prov := NewFullProvider(resty.New().SetRetryCount(0), server.URL+"/comm/web/getFastNewsList")
	prov.searchAPIURL = server.URL + "/search/jsonp"
	items, err := prov.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch full error: %v", err)
	}
	if searchCalls == 0 {
		t.Fatal("expected full provider to call search endpoint")
	}
	if len(items) != 1 || items[0].SourceType != provider.SourceTypeEastMoneyFull || items[0].Title != "东方财富搜索新闻" {
		t.Fatalf("unexpected full provider items: %+v", items)
	}
}

func TestProviderFetchWithOptionsPaginatesToHistoricalWindow(t *testing.T) {
	sortEnds := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/comm/web/getFastNewsList":
			sortEnd := r.URL.Query().Get("sortEnd")
			sortEnds = append(sortEnds, sortEnd)
			switch sortEnd {
			case "":
				_, _ = w.Write([]byte(`{"code":"1","data":{"fastNewsList":[{"summary":"今日新闻","code":"20260618001","realSort":"300","showTime":"2026-06-18 09:10:00","title":"今日A股新闻","stockList":[],"image":[]},{"summary":"今日新闻2","code":"20260618002","realSort":"200","showTime":"2026-06-18 09:00:00","title":"今日A股新闻2","stockList":[],"image":[]}]}}`))
			case "200":
				_, _ = w.Write([]byte(`{"code":"1","data":{"fastNewsList":[{"summary":"历史新闻","code":"20260616001","realSort":"100","showTime":"2026-06-16 09:05:00","title":"6月16日上午A股新闻","stockList":["1.601068"],"image":[]},{"summary":"历史新闻2","code":"20260616002","realSort":"050","showTime":"2026-06-16 07:59:00","title":"6月16日早间新闻","stockList":[],"image":[]}]}}`))
			default:
				_, _ = w.Write([]byte(`{"code":"1","data":{"fastNewsList":[]}}`))
			}
		case "/search/jsonp":
			_, _ = w.Write([]byte(`yuqing({"result":{"cmsArticleWebOld":[]}})`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
	}))
	defer server.Close()

	prov := NewProvider(resty.New().SetRetryCount(0), server.URL+"/comm/web/getFastNewsList")
	prov.searchAPIURL = server.URL + "/search/jsonp"
	items, err := prov.FetchWithOptions(context.Background(), model.CrawlOptions{
		Start:     "2026-06-16 08:00:00",
		End:       "2026-06-16 09:30:59",
		TimeField: "publish_time",
	})
	if err != nil {
		t.Fatalf("FetchWithOptions error: %v", err)
	}
	if strings.Join(sortEnds, ",") != ",200" {
		t.Fatalf("expected historical pagination with sortEnd, got %v", sortEnds)
	}
	if !containsEastMoneyTitle(items, "6月16日上午A股新闻") {
		t.Fatalf("expected historical morning item to be fetched, got %+v", items)
	}
}

func TestProviderFetchWithOptionsStopsOnCanceledContext(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"code":"1","data":{"fastNewsList":[]}}`))
	}))
	defer server.Close()

	prov := NewProvider(resty.New().SetRetryCount(0), server.URL+"/comm/web/getFastNewsList")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	items, err := prov.FetchWithOptions(ctx, model.CrawlOptions{
		Start:     "2026-06-16 08:00:00",
		End:       "2026-06-16 09:30:59",
		TimeField: "publish_time",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled error, got items=%+v err=%v", items, err)
	}
	if calls != 0 {
		t.Fatalf("expected no upstream request after context cancellation, got %d", calls)
	}
}

func containsEastMoneyTitle(items []model.Item, title string) bool {
	for _, item := range items {
		if item.Title == title {
			return true
		}
	}
	return false
}
