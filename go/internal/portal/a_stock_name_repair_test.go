package portal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

func TestAStockPersistedRecommendationsPreferAuthoritativeCodeNames(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/a-stock/code-names" {
			t.Fatalf("unexpected content path: %s", r.URL.String())
		}
		codes := r.URL.Query().Get("codes")
		if !strings.Contains(codes, "000100") || !strings.Contains(codes, "600313") {
			t.Fatalf("expected code-name lookup for both recommendations, got %s", r.URL.RawQuery)
		}
		writeEnvelope(w, http.StatusOK, "ok", model.AStockCodeNameListResult{Items: []model.AStockCodeName{
			{Code: "000100", Name: "TCL科技", Source: "akshare_code_name"},
			{Code: "600313", Name: "农发种业", Source: "akshare_code_name"},
		}})
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL})
	recommendations, skipped := srv.repairAStockPersistedRecommendationsWithCache("2026-08-20", []aStockRecommendation{
		{Rank: 1, Code: "000100", Name: "早间公告", Hotspot: "金融券商"},
		{Rank: 2, Code: "600313", Name: "经济日报", Hotspot: "金融券商"},
	}, newAStockRequestCache())
	if skipped != 0 || len(recommendations) != 2 {
		t.Fatalf("expected both recommendations to remain, skipped=%d recommendations=%+v", skipped, recommendations)
	}
	if recommendations[0].Code != "000100" || recommendations[0].Name != "TCL科技" {
		t.Fatalf("expected 000100 to use TCL科技, got %+v", recommendations[0])
	}
	if recommendations[1].Code != "600313" || recommendations[1].Name != "农发种业" {
		t.Fatalf("expected 600313 to use 农发种业, got %+v", recommendations[1])
	}

	backtests := filterAStockBacktestsForRecommendations([]aStockBacktestRow{
		{Stock: "000100 早间公告"},
		{Stock: "600313 经济日报"},
	}, recommendations)
	if len(backtests) != 2 || backtests[0].Stock != "000100 TCL科技" || backtests[1].Stock != "600313 农发种业" {
		t.Fatalf("expected backtests to use authoritative code names, got %+v", backtests)
	}
}
