package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"

	"github.com/pcdogyu/yuqing/go/internal/config"
)

func TestSinaFinanceSearchURLUsesStockSymbolSearch(t *testing.T) {
	got := sinaFinanceSearchURL("https://stock.finance.sina.com.cn/stock/go.php/vReport_List/kind/company/index.phtml", "002230")
	if !strings.Contains(got, "/stock/go.php/vReport_List/kind/search/index.phtml") || !strings.Contains(got, "t1=2") || !strings.Contains(got, "symbol=002230") {
		t.Fatalf("unexpected sina search url: %s", got)
	}
}

func TestParseSinaFinanceReportDocumentPrefersLinkTitleAttribute(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<html><body><table>
<tr><td>1</td><td><a title="科大讯飞(002230)：利润端表现良好 讯飞星火大模型持续落地推动公司发展" href="//stock.finance.sina.com.cn/stock/go.php/vReport_Show/kind/search/rptid/831866795943/index.phtml">科大讯飞(002230)：利润端表现良好 讯飞星火大模型持续...</a></td><td>公司</td><td>2026-05-11</td><td>平安证券股份有限公司</td><td>闫磊/黄韦涵/刘云坤</td></tr>
</table></body></html>`))
	if err != nil {
		t.Fatalf("new sina list document: %v", err)
	}
	items := parseSinaFinanceReportDocument(doc, "https://stock.finance.sina.com.cn/stock/go.php/vReport_List/kind/search/index.phtml?t1=2&symbol=002230", time.Date(2026, 6, 18, 1, 0, 0, 0, time.UTC))
	if len(items) != 1 {
		t.Fatalf("expected one sina item, got %+v", items)
	}
	if items[0].Title != "科大讯飞(002230)：利润端表现良好 讯飞星火大模型持续落地推动公司发展" {
		t.Fatalf("expected full title from title attribute, got %q", items[0].Title)
	}
}

func TestFetchSinaFinanceReportsUsesListRowsWithoutDetailFetch(t *testing.T) {
	var detailCalls int32
	sina := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/stock/go.php/vReport_List/kind/search/index.phtml":
			_, _ = w.Write([]byte(`<html><body><table>
<tr><th>序号</th><th>标题</th><th>报告类型</th><th>发布日期</th><th>机构</th><th>研究员</th></tr>
<tr><td>1</td><td><a title="科大讯飞(002230)：软硬一体打开成长空间" href="/stock/go.php/vReport_Show/kind/search/rptid/833792460291/index.phtml">科大讯飞(002230)：软硬一体打开成长空间</a></td><td>公司</td><td>2026-06-03</td><td>华泰证券股份有限公司</td><td>郭雅丽/袁泽世</td></tr>
</table></body></html>`))
		case "/stock/go.php/vReport_Show/kind/search/rptid/833792460291/index.phtml":
			atomic.AddInt32(&detailCalls, 1)
			_, _ = w.Write([]byte(`<html><body><div class="content"><h1>详情页不应被请求</h1></div></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer sina.Close()

	worker := NewWorker(config.Config{
		SinaFinanceReportURL: sina.URL + "/stock/go.php/vReport_List/kind/company/index.phtml",
		HTTPTimeout:          time.Second,
	})
	items, err := worker.fetchSinaFinanceReports(context.Background(), stockResearchCrawlOptions{Code: "002230", Start: "2026-01-01", End: "2026-12-31"})
	if err != nil {
		t.Fatalf("fetch sina reports: %v", err)
	}
	if atomic.LoadInt32(&detailCalls) != 0 {
		t.Fatalf("expected no detail page requests, got %d", detailCalls)
	}
	if len(items) != 1 {
		t.Fatalf("expected one sina item, got %+v", items)
	}
	if items[0].Code != "002230" || items[0].Name != "科大讯飞" || items[0].Title != "科大讯飞(002230)：软硬一体打开成长空间" || items[0].Institution != "华泰证券股份有限公司" || items[0].Analyst != "郭雅丽/袁泽世" || items[0].ResearchDate != "2026-06-03" || items[0].SourceType != "sina_finance_report" {
		t.Fatalf("unexpected sina item: %+v", items[0])
	}
}

func TestParseEastMoneyReportAPIForStockHistory(t *testing.T) {
	body := `{"hits":7,"size":1,"data":[{"title":"盈利能力提升，AI大模型商业化加速落地","stockName":"科大讯飞","stockCode":"002230","orgName":"信达证券股份有限公司","publishDate":"2026-05-08 00:00:00.000","infoCode":"AP202605081822079900","emRatingName":"买入","researcher":"傅晓烺","indvInduName":"软件开发"}]}`
	items, total, err := parseEastMoneyReportAPI(body, "https://data.eastmoney.com/report/stock.jshtml", time.Date(2026, 6, 18, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("parse eastmoney api: %v", err)
	}
	if total != 7 || len(items) != 1 || items[0].Code != "002230" || items[0].SourceKey != "AP202605081822079900" {
		t.Fatalf("unexpected eastmoney api items: total=%d items=%+v", total, items)
	}
}
