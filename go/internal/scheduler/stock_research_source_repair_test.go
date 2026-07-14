package scheduler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

func TestStockResearchSourceRepairDefaultRangeUsesRecentSevenShanghaiDays(t *testing.T) {
	start, end := stockResearchSourceRepairDefaultRange(time.Date(2026, 7, 12, 10, 30, 0, 0, aStockLocation()))

	if start != "2026-07-06" || end != "2026-07-12" {
		t.Fatalf("unexpected default source repair range: start=%s end=%s", start, end)
	}
}

func TestStockResearchSourceRepairRequiresServiceToken(t *testing.T) {
	worker := NewWorker(config.Config{ServiceToken: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scheduler/stock-research/source/repair", nil)
	rr := httptest.NewRecorder()

	worker.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized source repair without token, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRunStockResearchSourceRepairDryRunCountsAndSkipsWrites(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/stock-research":
			if r.URL.Query().Get("source") != "sina_finance_report" || r.URL.Query().Get("start") != "2026-07-06" || r.URL.Query().Get("end") != "2026-07-12" {
				t.Fatalf("unexpected source repair query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockResearchListResult{
				Page: 1, PageSize: 200, Total: 3,
				Items: []model.StockResearchSurvey{
					{ID: 1, SourceURL: "https://stock.finance.sina.com.cn/stock/go.php/vReport_Show/kind/company/rptid/1/index.phtml", SourceType: "sina_finance_report"},
					{ID: 2, SourceType: "sina_finance_report"},
					{ID: 3, SourceURL: "https://stock.finance.sina.com.cn/stock/go.php/vReport_Show/kind/company/rptid/3/index.phtml", SourceType: "sina_finance_report", SourceText: "已有正文"},
				},
			}})
		case r.Method == http.MethodPost:
			t.Fatalf("dry run should not write source update: %s", r.URL.Path)
		default:
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	worker := NewWorker(config.Config{ContentURL: content.URL, HTTPTimeout: time.Second})
	result, err := worker.runStockResearchSourceRepair(context.Background(), stockResearchSourceRepairOptions{
		Source: "sina_finance_report",
		Start:  "2026-07-06",
		End:    "2026-07-12",
		DryRun: true,
		Force:  false,
	})
	if err != nil {
		t.Fatalf("run source repair dry run: %v", err)
	}
	if !result.DryRun || result.Total != 3 || result.Repaired != 1 || result.Skipped != 2 || result.Failed != 0 {
		t.Fatalf("unexpected source repair dry-run result: %+v", result)
	}
}

func TestRunStockResearchSourceRepairAcceptsEastMoneyReport(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/stock-research":
			if r.URL.Query().Get("source") != "eastmoney_report" || r.URL.Query().Get("start") != "2026-07-12" || r.URL.Query().Get("end") != "2026-07-14" {
				t.Fatalf("unexpected eastmoney source repair query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockResearchListResult{
				Page: 1, PageSize: 200, Total: 1,
				Items: []model.StockResearchSurvey{{
					ID:         4927,
					SourceURL:  "https://data.eastmoney.com/report/info/AP202607121826913867.html",
					SourceType: "eastmoney_report",
				}},
			}})
		case r.Method == http.MethodPost:
			t.Fatalf("dry run should not write eastmoney source update: %s", r.URL.Path)
		default:
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	worker := NewWorker(config.Config{ContentURL: content.URL, HTTPTimeout: time.Second})
	result, err := worker.runStockResearchSourceRepair(context.Background(), stockResearchSourceRepairOptions{
		Source: "eastmoney_report",
		Start:  "2026-07-12",
		End:    "2026-07-14",
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("run eastmoney source repair dry run: %v", err)
	}
	if !result.DryRun || result.Total != 1 || result.Repaired != 1 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("unexpected eastmoney source repair result: %+v", result)
	}
}

func TestRunStockResearchSourceRepairForceWritesCleanedSinaText(t *testing.T) {
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body><article>
<h1>百隆东方(601339)：期待下半年价增驱动成长</h1>
<p>公司发布26 年半年度业绩预告</p>
<p>风险提示：外贸环境变化，原材料价格波动。</p>
</article></body></html>`))
	}))
	defer source.Close()

	var captured model.StockResearchSourceUpdate
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/stock-research":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockResearchListResult{
				Page: 1, PageSize: 200, Total: 1,
				Items: []model.StockResearchSurvey{{
					ID:         7,
					SourceURL:  source.URL + "/report",
					SourceType: "sina_finance_report",
					SourceText: "旧正文",
				}},
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/stock-research/7/source":
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Fatalf("decode source update: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": captured})
		default:
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	worker := NewWorker(config.Config{ContentURL: content.URL, HTTPTimeout: time.Second})
	result, err := worker.runStockResearchSourceRepair(context.Background(), stockResearchSourceRepairOptions{
		Source: "sina_finance_report",
		Start:  "2026-07-06",
		End:    "2026-07-12",
		Force:  true,
	})
	if err != nil {
		t.Fatalf("run source repair force: %v", err)
	}
	if result.Total != 1 || result.Repaired != 1 || result.Skipped != 0 || result.Failed != 0 {
		t.Fatalf("unexpected source repair result: %+v", result)
	}
	if !captured.Force || captured.SourceFetchStatus != "parsed" {
		t.Fatalf("expected forced parsed update, got %+v", captured)
	}
	for _, want := range []string{"百隆东方(601339)：期待下半年价增驱动成长", "公司发布26 年半年度业绩预告", "风险提示：外贸环境变化，原材料价格波动。"} {
		if !strings.Contains(captured.SourceText, want) {
			t.Fatalf("expected captured source text to contain %q, got %q", want, captured.SourceText)
		}
	}
}
