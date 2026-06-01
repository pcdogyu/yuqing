package nlp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildTitleAndSummary(t *testing.T) {
	if got := buildTitle(""); got != "自动生成标题" {
		t.Fatalf("expected default title, got %q", got)
	}
	if got := buildSummary("  short text  "); got != "short text" {
		t.Fatalf("expected trimmed summary, got %q", got)
	}

	longTitle := strings.Repeat("a", 30)
	if got := buildTitle(longTitle); !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated title, got %q", got)
	}

	longSummary := strings.Repeat("中", 130)
	if got := buildSummary(longSummary); !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated summary, got %q", got)
	}
}

func TestExtractKeywords(t *testing.T) {
	keywords := extractKeywords("btc btc btc eth eth 宏观 宏观 新闻 market market")
	if len(keywords) == 0 {
		t.Fatal("expected extracted keywords")
	}
	if keywords[0] != "btc" {
		t.Fatalf("expected btc to rank first, got %+v", keywords)
	}
}

func TestHandleSummarize(t *testing.T) {
	svc := NewService()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/nlp/summarize", strings.NewReader(`{"text":"btc breaks higher and macro sentiment improves"}`))

	svc.handleSummarize(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var payload struct {
		Data struct {
			Title    string   `json:"title"`
			Summary  string   `json:"summary"`
			Keywords []string `json:"keywords"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if payload.Data.Title == "" || payload.Data.Summary == "" || len(payload.Data.Keywords) == 0 {
		t.Fatalf("expected populated response, got %+v", payload.Data)
	}
}
