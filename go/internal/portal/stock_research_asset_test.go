package portal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

func TestFormatStockResearchTargetPrice(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "", want: "--"},
		{name: "integer", value: "327", want: "327.00"},
		{name: "long decimal", value: "327.9000000000", want: "327.90"},
		{name: "comma number", value: "1,234.567", want: "1234.57"},
		{name: "plain text", value: "上调至合理区间", want: "上调至合理区间"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatStockResearchTargetPrice(tc.value); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestStockResearchPageRequestsTwentyItemsPerPage(t *testing.T) {
	var requests int
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/v1/stock-research" {
			t.Fatalf("unexpected stock research content path: %s", r.URL.String())
		}
		if r.URL.Query().Get("page") != "3" || r.URL.Query().Get("page_size") != "20" {
			t.Fatalf("expected portal to request page 3 with page_size=20, got %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": model.StockResearchListResult{
				Page:     3,
				PageSize: stockResearchPageSize,
				Total:    45,
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/stock-research?page=3&page_size=50", nil)
	rr := httptest.NewRecorder()
	srv.handleStockResearchPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected stock research page 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if requests != 1 {
		t.Fatalf("expected one stock research request, got %d", requests)
	}
	body := rr.Body.String()
	for _, want := range []string{"第 3/3 页，共 45 条", "page_size=20"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected stock research page to contain %q, got %s", want, body)
		}
	}
	if strings.Contains(body, "page_size=50") {
		t.Fatalf("expected pagination links to keep page_size=20, got %s", body)
	}
}

func TestStockResearchPageUsesPortalPDFLinks(t *testing.T) {
	var requests int
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/v1/stock-research" {
			t.Fatalf("unexpected stock research content path: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": model.StockResearchListResult{
				Page:     1,
				PageSize: 50,
				Total:    1,
				Items: []model.StockResearchSurvey{{
					ID:           7,
					Code:         "002230",
					Name:         "科大讯飞",
					Kind:         "report",
					Title:        "科大讯飞深度研究",
					ResearchDate: "2026-06-16",
					PDFFilePath:  "data/stock-research-pdfs/sina/sina-1.pdf",
					PDFStatus:    "parsed",
					PDFText:      "科大讯飞研报正文",
				}},
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/stock-research", nil)
	rr := httptest.NewRecorder()
	srv.handleStockResearchPage(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected stock research page 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if requests != 1 {
		t.Fatalf("expected one stock research request, got %d", requests)
	}
	body := rr.Body.String()
	for _, want := range []string{`href="/stock-research/7/pdf"`, `href="/stock-research/7/pdf/text"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected stock research page to contain %q, got %s", want, body)
		}
	}
	if strings.Contains(body, `/api/v1/stock-research/7/pdf`) {
		t.Fatalf("expected page links to stay on portal stock-research routes, got %s", body)
	}
}

func TestStockResearchPDFTextAssetRendersContent(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/stock-research/7/pdf/text" {
			t.Fatalf("unexpected pdf text content path: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": model.StockResearchSurvey{
				ID:          7,
				Code:        "002230",
				Name:        "科大讯飞",
				Title:       "科大讯飞深度研究",
				PDFURL:      "https://example.com/report.pdf",
				PDFText:     "请阅读最后一页免责声明\n科大讯飞研报正文",
				PDFParsedAt: "2026-06-18T07:21:23Z",
			},
		})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/stock-research/7/pdf/text", nil)
	rr := httptest.NewRecorder()
	srv.handleStockResearchAsset(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected pdf text page 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"研报 PDF 文本", "科大讯飞深度研究", "请阅读最后一页免责声明", "科大讯飞研报正文", "原始 PDF 链接"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected pdf text page to contain %q, got %s", want, body)
		}
	}
	if strings.Contains(body, `"pdf_text"`) {
		t.Fatalf("expected rendered text page, got raw json: %s", body)
	}
}

func TestStockResearchPDFAssetProxiesContentPDF(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/stock-research/7/pdf" {
			t.Fatalf("unexpected pdf content path: %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `inline; filename="stock-research.pdf"`)
		_, _ = w.Write([]byte("%PDF-1.7\nbody"))
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	req := httptest.NewRequest(http.MethodGet, "/stock-research/7/pdf", nil)
	rr := httptest.NewRecorder()
	srv.handleStockResearchAsset(rr, req, map[string]any{"id": 1})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected pdf proxy 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("expected application/pdf content type, got %q", got)
	}
	if got := rr.Header().Get("Content-Disposition"); got != `inline; filename="stock-research.pdf"` {
		t.Fatalf("expected content disposition to be proxied, got %q", got)
	}
	if got := rr.Body.String(); got != "%PDF-1.7\nbody" {
		t.Fatalf("expected proxied pdf bytes, got %q", got)
	}
}
