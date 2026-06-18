package scheduler

import (
	"strings"
	"testing"
	"time"
)

func TestSinaFinanceSearchURLUsesStockSymbolSearch(t *testing.T) {
	got := sinaFinanceSearchURL("https://stock.finance.sina.com.cn/stock/go.php/vReport_List/kind/company/index.phtml", "002230")
	if !strings.Contains(got, "/stock/go.php/vReport_List/kind/search/index.phtml") || !strings.Contains(got, "t1=2") || !strings.Contains(got, "symbol=002230") {
		t.Fatalf("unexpected sina search url: %s", got)
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
