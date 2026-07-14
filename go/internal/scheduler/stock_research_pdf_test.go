package scheduler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

func TestRunStockResearchPDFParseFindsPDFDownloadsAndWritesContent(t *testing.T) {
	previousExtractor := stockResearchPDFTextExtractor
	stockResearchPDFTextExtractor = func(filePath string) (string, error) {
		if _, err := os.Stat(filePath); err != nil {
			t.Fatalf("expected downloaded pdf file: %v", err)
		}
		return "研报 PDF 文本\n第二段\n\n第三段", nil
	}
	t.Cleanup(func() { stockResearchPDFTextExtractor = previousExtractor })

	var externalURL string
	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/report-page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><body><a href="/research.pdf">PDF</a></body></html>`))
		case "/research.pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF-1.4\nfake\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer external.Close()
	externalURL = external.URL

	var captured model.StockResearchPDFUpdate
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/stock-research":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockResearchListResult{
				Page:     1,
				PageSize: 200,
				Total:    1,
				Items: []model.StockResearchSurvey{{
					ID:           7,
					Kind:         "report",
					Title:        "科大讯飞深度研究",
					SourceURL:    externalURL + "/report-page",
					SourceType:   "sina_finance_report",
					SourceKey:    "sina-pdf-1",
					ResearchDate: "2026-06-16",
				}},
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/stock-research/7/pdf":
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Fatalf("decode captured update: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": captured})
		default:
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		ContentURL:          content.URL,
		StockResearchPDFDir: t.TempDir(),
		HTTPTimeout:         time.Second,
		ExternalRetryWait:   time.Millisecond,
		ExternalRetryCount:  0,
		DatabasePath:        "",
		StockResearchURL:    "",
	})

	result, err := worker.runStockResearchPDFParse(context.Background(), stockResearchPDFParseOptions{Code: "002230", Start: "2026-06-01", End: "2026-06-17"})
	if err != nil {
		t.Fatalf("runStockResearchPDFParse error: %v", err)
	}
	if result.Total != 1 || result.Parsed != 1 || result.Failed != 0 {
		t.Fatalf("unexpected parse result: %+v", result)
	}
	if captured.PDFStatus != "parsed" || captured.PDFURL != external.URL+"/research.pdf" || captured.PDFText != "研报 PDF 文本\n第二段\n\n第三段" || captured.SourceText != captured.PDFText || captured.SourceFetchStatus != "parsed" || !strings.HasSuffix(captured.PDFFilePath, "sina-pdf-1.pdf") {
		t.Fatalf("unexpected captured update: %+v", captured)
	}
	if strings.Contains(captured.PDFText, "文本 第二段") {
		t.Fatalf("expected parsed PDF text to preserve line breaks, got %q", captured.PDFText)
	}
}

func TestRunStockResearchPDFParseDryRunCountsAndSkipsWrites(t *testing.T) {
	pdfPath := filepath.Join(t.TempDir(), "existing.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\nfake\n"), 0o644); err != nil {
		t.Fatalf("write existing pdf: %v", err)
	}
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/stock-research":
			if r.URL.Query().Get("kind") != "all" || r.URL.Query().Get("start") != "2026-07-06" || r.URL.Query().Get("end") != "2026-07-08" {
				t.Fatalf("unexpected stock research query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockResearchListResult{
				Page: 1, PageSize: 200, Total: 3,
				Items: []model.StockResearchSurvey{
					{ID: 1, PDFFilePath: pdfPath, SourceType: "sina_finance_report", SourceKey: "existing"},
					{ID: 2, SourceURL: "https://example.com/report", SourceType: "sina_finance_report", SourceKey: "download"},
					{ID: 3, SourceType: "sina_finance_report", SourceKey: "missing"},
				},
			}})
		case r.Method == http.MethodPost:
			t.Fatalf("dry run should not write content update: %s", r.URL.Path)
		default:
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		ContentURL:          content.URL,
		StockResearchPDFDir: t.TempDir(),
		HTTPTimeout:         time.Second,
		ExternalRetryWait:   time.Millisecond,
	})
	result, err := worker.runStockResearchPDFParse(context.Background(), stockResearchPDFParseOptions{Start: "2026-07-06", End: "2026-07-08", Kind: "all", DryRun: true, Force: true})
	if err != nil {
		t.Fatalf("runStockResearchPDFParse dry run error: %v", err)
	}
	if !result.DryRun || result.Total != 3 || result.ExistingPDF != 1 || result.NeedDownload != 1 || result.NoPDF != 1 || result.Parsed != 0 || result.Failed != 0 {
		t.Fatalf("unexpected dry-run result: %+v", result)
	}
}

func TestRunStockResearchPDFParseUsesExistingLocalPDFBeforeResolvingURL(t *testing.T) {
	pdfPath := filepath.Join(t.TempDir(), "local.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\nfake\n"), 0o644); err != nil {
		t.Fatalf("write existing pdf: %v", err)
	}
	previousExtractor := stockResearchPDFTextExtractor
	stockResearchPDFTextExtractor = func(filePath string) (string, error) {
		if filePath != pdfPath {
			t.Fatalf("expected extractor to use existing local pdf %q, got %q", pdfPath, filePath)
		}
		return "本地PDF第一段\n本地PDF第二段", nil
	}
	t.Cleanup(func() { stockResearchPDFTextExtractor = previousExtractor })

	var captured model.StockResearchPDFUpdate
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/stock-research/11":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockResearchSurvey{
				ID: 11, Code: "300476", Name: "胜宏科技", Kind: "survey", Title: "胜宏科技投资者关系活动记录",
				PDFFilePath: pdfPath, SourceURL: "https://invalid.example.com/no-page", SourceType: investorRelationsSourceType, SourceKey: "ir-local", ResearchDate: "2026-07-08",
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/stock-research/11/pdf":
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Fatalf("decode captured update: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": captured})
		default:
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		ContentURL:          content.URL,
		StockResearchPDFDir: t.TempDir(),
		HTTPTimeout:         time.Second,
		ExternalRetryWait:   time.Millisecond,
	})
	result, err := worker.runStockResearchPDFParse(context.Background(), stockResearchPDFParseOptions{ID: 11, Force: true})
	if err != nil {
		t.Fatalf("runStockResearchPDFParse local pdf error: %v", err)
	}
	if result.Parsed != 1 || result.ExistingPDF != 1 || result.NeedDownload != 0 {
		t.Fatalf("unexpected local pdf result: %+v", result)
	}
	if captured.PDFStatus != "parsed" || captured.PDFFilePath != pdfPath || !captured.Force || !strings.Contains(captured.PDFText, "本地PDF第一段\n本地PDF第二段") {
		t.Fatalf("unexpected local pdf update: %+v", captured)
	}
}

func TestRunStockResearchPDFParseTimesOutSingleStuckPDF(t *testing.T) {
	pdfPath := filepath.Join(t.TempDir(), "stuck.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4\nfake\n"), 0o644); err != nil {
		t.Fatalf("write existing pdf: %v", err)
	}
	previousExtractor := stockResearchPDFTextExtractor
	previousMinTimeout := stockResearchPDFParseItemMinTimeout
	stockResearchPDFTextExtractor = func(filePath string) (string, error) {
		time.Sleep(120 * time.Millisecond)
		return "too late", nil
	}
	stockResearchPDFParseItemMinTimeout = 10 * time.Millisecond
	t.Cleanup(func() {
		stockResearchPDFTextExtractor = previousExtractor
		stockResearchPDFParseItemMinTimeout = previousMinTimeout
	})

	var captured model.StockResearchPDFUpdate
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/stock-research/12":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockResearchSurvey{
				ID: 12, Code: "688048", Name: "长光华芯", Kind: "report", Title: "芯所往，光所至",
				PDFFilePath: pdfPath, PDFURL: "https://example.com/stuck.pdf", SourceType: "eastmoney_report", SourceKey: "stuck",
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/stock-research/12/pdf":
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Fatalf("decode captured update: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": captured})
		default:
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		ContentURL:          content.URL,
		StockResearchPDFDir: t.TempDir(),
		HTTPTimeout:         time.Millisecond,
		ExternalRetryWait:   time.Millisecond,
	})
	result, err := worker.runStockResearchPDFParse(context.Background(), stockResearchPDFParseOptions{ID: 12, Force: true})
	if err != nil {
		t.Fatalf("runStockResearchPDFParse timeout error: %v", err)
	}
	if result.Failed != 1 || captured.PDFStatus != "failed" || !strings.Contains(captured.PDFError, "PDF 解析超时") || !captured.Force {
		t.Fatalf("expected single PDF timeout to be written as failed, result=%+v update=%+v", result, captured)
	}
}

func TestRunStockResearchPDFParseMarksNoText(t *testing.T) {
	previousExtractor := stockResearchPDFTextExtractor
	stockResearchPDFTextExtractor = func(filePath string) (string, error) { return "", nil }
	t.Cleanup(func() { stockResearchPDFTextExtractor = previousExtractor })

	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4\nfake\n"))
	}))
	defer external.Close()

	var captured model.StockResearchPDFUpdate
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/stock-research/8":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockResearchSurvey{
				ID:         8,
				Kind:       "report",
				Title:      "图片型研报",
				PDFURL:     external.URL + "/image.pdf",
				SourceType: "akshare_stock_research",
				SourceKey:  "ak-pdf-1",
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/stock-research/8/pdf":
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Fatalf("decode captured update: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": captured})
		default:
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		ContentURL:          content.URL,
		StockResearchPDFDir: t.TempDir(),
		HTTPTimeout:         time.Second,
		ExternalRetryWait:   time.Millisecond,
	})

	result, err := worker.runStockResearchPDFParse(context.Background(), stockResearchPDFParseOptions{ID: 8})
	if err != nil {
		t.Fatalf("runStockResearchPDFParse error: %v", err)
	}
	if result.Total != 1 || result.NoText != 1 || captured.PDFStatus != "no_text" || !strings.Contains(captured.PDFError, "扫描版") {
		t.Fatalf("expected no_text result and update, result=%+v update=%+v", result, captured)
	}
}

func TestRunStockResearchPDFParseScoresInvestorRelations(t *testing.T) {
	previousExtractor := stockResearchPDFTextExtractor
	stockResearchPDFTextExtractor = func(filePath string) (string, error) {
		if _, err := os.Stat(filePath); err != nil {
			t.Fatalf("expected downloaded investor relations pdf file: %v", err)
		}
		return "公司AI订单增长，客户需求提升，盈利改善。", nil
	}
	t.Cleanup(func() { stockResearchPDFTextExtractor = previousExtractor })

	external := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4\nfake\n"))
	}))
	defer external.Close()

	nlp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/nlp/stock-score" {
			t.Fatalf("unexpected nlp request: %s %s", r.Method, r.URL.String())
		}
		var req model.NLPStockScoreRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode nlp request: %v", err)
		}
		if req.Code != "300250" || !strings.HasPrefix(req.Text, "# ") {
			t.Fatalf("unexpected nlp request payload: %+v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": model.NLPStockScoreResponse{Score: 78.5, Rating: "积极", Reason: "AI订单增长", Status: "ok"}})
	}))
	defer nlp.Close()

	var captured model.StockResearchPDFUpdate
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/stock-research":
			if r.URL.Query().Get("source") != investorRelationsSourceType || r.URL.Query().Get("kind") != "survey" {
				t.Fatalf("expected investor relation source filter, got %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockResearchListResult{
				Page: 1, PageSize: 200, Total: 1,
				Items: []model.StockResearchSurvey{{
					ID: 9, Code: "300250", Name: "初灵信息", Kind: "survey", Title: "初灵信息投资者关系活动记录",
					PDFURL: external.URL + "/ir.pdf", SourceType: investorRelationsSourceType, SourceKey: "ir-1", ResearchDate: "2026-06-18",
				}},
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/stock-research/9/pdf":
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Fatalf("decode captured update: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": captured})
		default:
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		ContentURL:          content.URL,
		NLPURL:              nlp.URL,
		StockResearchPDFDir: t.TempDir(),
		HTTPTimeout:         time.Second,
		ExternalRetryWait:   time.Millisecond,
	})
	result, err := worker.runStockResearchPDFParse(context.Background(), stockResearchPDFParseOptions{Source: investorRelationsSourceType, Code: "300250"})
	if err != nil {
		t.Fatalf("runStockResearchPDFParse error: %v", err)
	}
	if result.Parsed != 1 || captured.NLPScore != 78.5 || captured.NLPRating != "积极" || !strings.HasPrefix(captured.PDFText, "# 初灵信息") || captured.SourceText != captured.PDFText || captured.SourceFetchStatus != "parsed" {
		t.Fatalf("unexpected investor relation pdf parse result=%+v update=%+v", result, captured)
	}
}

func TestRunStockResearchNLPParseScoresSourceText(t *testing.T) {
	nlp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/nlp/stock-score" {
			t.Fatalf("unexpected nlp request: %s %s", r.Method, r.URL.String())
		}
		var req model.NLPStockScoreRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode nlp request: %v", err)
		}
		if req.Code != "600026" || req.Name != "中远海能" || req.Title != "油运景气持续 运价中枢大幅抬升" || !strings.Contains(req.Text, "油运景气持续") {
			t.Fatalf("unexpected nlp request payload: %+v", req)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": model.NLPStockScoreResponse{Score: 88, Rating: "积极", Reason: "运价提升", Status: "ok"}})
	}))
	defer nlp.Close()

	var captured model.StockResearchNLPUpdate
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/stock-research/4900":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockResearchSurvey{
				ID:           4900,
				Code:         "600026",
				Name:         "中远海能",
				Kind:         "report",
				Title:        "油运景气持续 运价中枢大幅抬升",
				SourceText:   "油运景气持续，运价中枢大幅抬升，盈利改善。",
				SourceType:   "sina_finance_report",
				PDFStatus:    "no_pdf",
				ResearchDate: "2026-07-13",
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/stock-research/4900/nlp":
			if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
				t.Fatalf("decode captured nlp update: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": captured})
		default:
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		ContentURL:          content.URL,
		NLPURL:              nlp.URL,
		HTTPTimeout:         time.Second,
		ExternalRetryWait:   time.Millisecond,
		StockResearchPDFDir: t.TempDir(),
	})
	result, err := worker.runStockResearchNLPParse(context.Background(), stockResearchNLPParseOptions{ID: 4900})
	if err != nil {
		t.Fatalf("runStockResearchNLPParse error: %v", err)
	}
	if result.Total != 1 || result.Scored != 1 || result.NoText != 0 || result.Failed != 0 {
		t.Fatalf("unexpected nlp parse result: %+v", result)
	}
	if captured.NLPScore != 88 || captured.NLPRating != "积极" || captured.NLPReason != "运价提升" || captured.NLPScoredAt == "" {
		t.Fatalf("unexpected nlp update: %+v", captured)
	}
}

func TestSchedulerStockResearchPDFParseEndpoint(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/stock-research" {
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockResearchListResult{Page: 1, PageSize: 200, Total: 0}})
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		ContentURL:          content.URL,
		ServiceToken:        "secret-token",
		HTTPTimeout:         time.Second,
		ExternalRetryWait:   time.Millisecond,
		DatabasePath:        "",
		DatabaseDriver:      "",
		StockResearchPDFDir: t.TempDir(),
	})
	router := worker.Router()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scheduler/stock-research/pdf/parse?code=002230", nil)
	req.Header.Set("X-Service-Token", "secret-token")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"total":0`) {
		t.Fatalf("expected scheduler pdf parse success, got status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestSchedulerStockResearchNLPParseEndpoint(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/stock-research" {
			t.Fatalf("unexpected content request: %s %s", r.Method, r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": model.StockResearchListResult{Page: 1, PageSize: 200, Total: 0}})
	}))
	defer content.Close()

	worker := NewWorker(config.Config{
		ContentURL:          content.URL,
		ServiceToken:        "secret-token",
		HTTPTimeout:         time.Second,
		ExternalRetryWait:   time.Millisecond,
		DatabasePath:        "",
		DatabaseDriver:      "",
		StockResearchPDFDir: t.TempDir(),
	})
	router := worker.Router()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/scheduler/stock-research/nlp/parse?code=002230", nil)
	req.Header.Set("X-Service-Token", "secret-token")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"total":0`) {
		t.Fatalf("expected scheduler nlp parse success, got status=%d body=%s", rr.Code, rr.Body.String())
	}
}
