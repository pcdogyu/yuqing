package portal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

func TestSystemTemplateIncludesAStockRepairSection(t *testing.T) {
	for _, snippet := range []string{
		`href="/system?section=stockrepair">推荐股票修复</a>`,
		`{{if eq .SectionKey "stockrepair"}}<section class="section-block"><h2>推荐股票修复</h2>`,
		`name="form_type" value="astock_repair_selections_save"`,
		`name="form_type" value="astock_repair_snapshot_save"`,
	} {
		if !strings.Contains(systemTemplate, snippet) {
			t.Fatalf("expected system template to contain %q", snippet)
		}
	}
}

func TestSystemAStockRepairSectionRendersLayers(t *testing.T) {
	srv, cleanup := newAStockRepairPortalTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/system?section=stockrepair&strategy_date=2026-06-24&period=morning", nil)
	rr := httptest.NewRecorder()
	srv.handleSystem(rr, req, map[string]any{"id": int64(1), "username": "admin"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected stock repair page 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, snippet := range []string{
		"推荐股票修复",
		"已选股票层",
		"推荐快照层",
		"603083",
		"测试已选股票",
		"测试快照推荐",
		"已回测 1/1",
	} {
		if !strings.Contains(body, snippet) {
			t.Fatalf("expected stock repair section to contain %q, got %s", snippet, body)
		}
	}
}

func TestSystemAStockRepairSelectionsSaveRedirectsWithSuccess(t *testing.T) {
	srv, cleanup := newAStockRepairPortalTestServer(t)
	defer cleanup()

	form := url.Values{}
	form.Set("section", "stockrepair")
	form.Set("form_type", "astock_repair_selections_save")
	form.Set("strategy_date", "2026-06-24")
	form.Set("period", "morning")
	form.Set("selection_items_json", `[{"rank":1,"hotspot":"半导体","code":"688981","name":"中芯国际","hotspot_score":95,"market_score":73,"reason":"人工修复已选股票"}]`)
	req := httptest.NewRequest(http.MethodPost, "/system?section=stockrepair&strategy_date=2026-06-24&period=morning", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	srv.handleSystem(rr, req, map[string]any{"id": int64(1), "username": "admin"})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after stock repair selection save, got %d body=%s", rr.Code, rr.Body.String())
	}
	location := rr.Header().Get("Location")
	if !strings.Contains(location, "section=stockrepair") || !strings.Contains(location, url.QueryEscape("推荐股票已选层已保存")) {
		t.Fatalf("expected stock repair selection save redirect, got %q", location)
	}

	viewReq := httptest.NewRequest(http.MethodGet, "/system?section=stockrepair&strategy_date=2026-06-24&period=morning", nil)
	viewRR := httptest.NewRecorder()
	srv.handleSystem(viewRR, viewReq, map[string]any{"id": int64(1), "username": "admin"})
	body := viewRR.Body.String()
	for _, snippet := range []string{"688981", "中芯国际", "人工修复已选股票"} {
		if !strings.Contains(body, snippet) {
			t.Fatalf("expected saved selection snippet %q in body %s", snippet, body)
		}
	}
}

func TestSystemAStockRepairSnapshotSaveRedirectsWithSuccess(t *testing.T) {
	srv, cleanup := newAStockRepairPortalTestServer(t)
	defer cleanup()

	form := url.Values{}
	form.Set("section", "stockrepair")
	form.Set("form_type", "astock_repair_snapshot_save")
	form.Set("strategy_date", "2026-06-24")
	form.Set("period", "morning")
	form.Set("snapshot_ignore_recent", "1")
	form.Set("snapshot_generated_count", "2")
	form.Set("snapshot_recent_filtered", "1")
	form.Set("snapshot_same_day_morning_filtered", "0")
	form.Set("snapshot_limit_up_filtered", "3")
	form.Set("snapshot_no_today_market_count", "4")
	form.Set("snapshot_market_candidate_count", "1234")
	form.Set("snapshot_limit_up_filter_enabled", "on")
	form.Set("snapshot_backtest_status", "已人工修复快照")
	form.Set("snapshot_market_candidate_status", "人工修复候选")
	form.Set("snapshot_auction_amount_label", "88.88亿")
	form.Set("snapshot_empty_reason", "人工补录快照说明")
	form.Set("snapshot_recommendations_json", `[{"Code":"600000","Name":"浦发银行"}]`)
	form.Set("snapshot_backtests_json", `[{"Stock":"600000 浦发银行","T0Return":"+2.10%","T0Close":"12.34"}]`)
	req := httptest.NewRequest(http.MethodPost, "/system?section=stockrepair&strategy_date=2026-06-24&period=morning&ignore_recent=1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	srv.handleSystem(rr, req, map[string]any{"id": int64(1), "username": "admin"})

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect after stock repair snapshot save, got %d body=%s", rr.Code, rr.Body.String())
	}
	location := rr.Header().Get("Location")
	if !strings.Contains(location, "section=stockrepair") || !strings.Contains(location, "ignore_recent=1") || !strings.Contains(location, url.QueryEscape("推荐股票快照层已保存")) {
		t.Fatalf("expected stock repair snapshot save redirect, got %q", location)
	}

	viewReq := httptest.NewRequest(http.MethodGet, "/system?section=stockrepair&strategy_date=2026-06-24&period=morning&ignore_recent=1", nil)
	viewRR := httptest.NewRecorder()
	srv.handleSystem(viewRR, viewReq, map[string]any{"id": int64(1), "username": "admin"})
	body := viewRR.Body.String()
	for _, snippet := range []string{"600000", "浦发银行", "已人工修复快照", "人工修复候选", "88.88亿", "人工补录快照说明"} {
		if !strings.Contains(body, snippet) {
			t.Fatalf("expected saved snapshot snippet %q in body %s", snippet, body)
		}
	}
}

func newAStockRepairPortalTestServer(t *testing.T) (*Server, func()) {
	t.Helper()

	type repairState struct {
		mu         sync.Mutex
		selections map[string]model.AStockRecommendationSelectionListResult
		snapshots  map[string]model.AStockRecommendationSnapshot
	}

	selectionKey := func(date string, period string) string {
		return strings.TrimSpace(date) + "|" + strings.TrimSpace(period)
	}
	snapshotKey := func(date string, period string, ignoreRecent bool) string {
		flag := "0"
		if ignoreRecent {
			flag = "1"
		}
		return strings.TrimSpace(date) + "|" + strings.TrimSpace(period) + "|" + flag
	}

	now := time.Now().UTC()
	state := repairState{
		selections: map[string]model.AStockRecommendationSelectionListResult{
			selectionKey("2026-06-24", "morning"): {
				Found:        true,
				StrategyDate: "2026-06-24",
				Period:       "morning",
				Items: []model.AStockRecommendationSelection{
					{StrategyDate: "2026-06-24", Period: "morning", Rank: 1, Hotspot: "机器人", Code: "603083", Name: "剑桥科技", HotspotScore: 88, MarketScore: 66, Reason: "测试已选股票"},
				},
				CreatedAt: now.Add(-4 * time.Hour),
				UpdatedAt: now.Add(-90 * time.Minute),
			},
		},
		snapshots: map[string]model.AStockRecommendationSnapshot{
			snapshotKey("2026-06-24", "morning", false): {
				Found:                 true,
				StrategyDate:          "2026-06-24",
				Period:                "morning",
				IgnoreRecent:          false,
				RecommendationsJSON:   `[{"Rank":1,"Hotspot":"机器人","Code":"603083","Name":"剑桥科技","HotspotScore":88,"MarketScore":66,"Reason":"测试快照推荐"}]`,
				BacktestsJSON:         `[{"Stock":"603083 剑桥科技","EntryOpen":"240.00","T0Return":"+1.70%","T0Close":"244.08","Status":"已回测"}]`,
				BacktestStatus:        "已回测 1/1",
				GeneratedCount:        1,
				MarketCandidateStatus: "已读取集合竞价 5500",
				MarketCandidateCount:  5500,
				AuctionAmountLabel:    "2329.54亿",
				CreatedAt:             now.Add(-3 * time.Hour),
				UpdatedAt:             now.Add(-45 * time.Minute),
			},
		},
	}

	writeEnvelope := func(w http.ResponseWriter, code int, message string, data any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    code,
			"message": message,
			"data":    data,
		})
	}

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthy":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "message": "ok"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/crawl-templates":
			writeEnvelope(w, http.StatusOK, "ok", []model.CrawlTemplate{})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/project-groups":
			writeEnvelope(w, http.StatusOK, "ok", []model.ProjectGroup{})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			writeEnvelope(w, http.StatusOK, "ok", []model.Project{})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/notices":
			writeEnvelope(w, http.StatusOK, "ok", []model.SystemNotice{})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/task-runs":
			writeEnvelope(w, http.StatusOK, "ok", []model.TaskRun{})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/audit-logs":
			writeEnvelope(w, http.StatusOK, "ok", []model.AuditLog{})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/operations":
			writeEnvelope(w, http.StatusOK, "ok", model.OperationsSummary{})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/preferences":
			writeEnvelope(w, http.StatusOK, "ok", model.UserPreference{})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/popup":
			writeEnvelope(w, http.StatusOK, "ok", model.PopupState{UserID: 1, Key: "system-announcement"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/mail-config":
			writeEnvelope(w, http.StatusOK, "ok", model.MailConfig{})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/system/database-config":
			writeEnvelope(w, http.StatusOK, "ok", model.DatabaseConfigStatus{Driver: "sqlite", ConfiguredDriver: "sqlite", RuntimeDriver: "sqlite", Status: "ok"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/tasks/crawl/runs":
			writeEnvelope(w, http.StatusOK, "ok", []model.CrawlRun{})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/scheduler/jobs":
			writeEnvelope(w, http.StatusOK, "ok", []model.OperationSchedulerJob{})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/recommendation-selections":
			key := selectionKey(r.URL.Query().Get("date"), r.URL.Query().Get("period"))
			state.mu.Lock()
			result, ok := state.selections[key]
			state.mu.Unlock()
			if !ok {
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionListResult{
					Found:        false,
					StrategyDate: strings.TrimSpace(r.URL.Query().Get("date")),
					Period:       strings.TrimSpace(r.URL.Query().Get("period")),
					Items:        []model.AStockRecommendationSelection{},
				})
				return
			}
			result.Found = len(result.Items) > 0
			writeEnvelope(w, http.StatusOK, "ok", result)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/a-stock/recommendation-selections":
			var payload model.AStockRecommendationSelectionSet
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				writeEnvelope(w, http.StatusBadRequest, err.Error(), nil)
				return
			}
			now := time.Now().UTC()
			key := selectionKey(payload.StrategyDate, payload.Period)
			state.mu.Lock()
			createdAt := now
			if existing, ok := state.selections[key]; ok && !existing.CreatedAt.IsZero() {
				createdAt = existing.CreatedAt
			}
			items := make([]model.AStockRecommendationSelection, 0, len(payload.Items))
			for idx, item := range payload.Items {
				item.StrategyDate = strings.TrimSpace(payload.StrategyDate)
				item.Period = strings.TrimSpace(payload.Period)
				if item.Rank <= 0 {
					item.Rank = idx + 1
				}
				if item.CreatedAt.IsZero() {
					item.CreatedAt = createdAt
				}
				item.UpdatedAt = now
				items = append(items, item)
			}
			state.selections[key] = model.AStockRecommendationSelectionListResult{
				Found:        len(items) > 0,
				StrategyDate: strings.TrimSpace(payload.StrategyDate),
				Period:       strings.TrimSpace(payload.Period),
				Items:        items,
				CreatedAt:    createdAt,
				UpdatedAt:    now,
			}
			state.mu.Unlock()
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSelectionUpsertResult{Inserted: len(items), Total: len(items)})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/a-stock/recommendations":
			key := snapshotKey(r.URL.Query().Get("date"), r.URL.Query().Get("period"), normalizeAStockBool(r.URL.Query().Get("ignore_recent")))
			state.mu.Lock()
			snapshot, ok := state.snapshots[key]
			state.mu.Unlock()
			if !ok {
				writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshot{
					Found:        false,
					StrategyDate: strings.TrimSpace(r.URL.Query().Get("date")),
					Period:       strings.TrimSpace(r.URL.Query().Get("period")),
				})
				return
			}
			snapshot.Found = true
			writeEnvelope(w, http.StatusOK, "ok", snapshot)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/a-stock/recommendations":
			var snapshot model.AStockRecommendationSnapshot
			if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
				writeEnvelope(w, http.StatusBadRequest, err.Error(), nil)
				return
			}
			now := time.Now().UTC()
			key := snapshotKey(snapshot.StrategyDate, snapshot.Period, snapshot.IgnoreRecent)
			state.mu.Lock()
			if existing, ok := state.snapshots[key]; ok && !existing.CreatedAt.IsZero() {
				snapshot.CreatedAt = existing.CreatedAt
			} else if snapshot.CreatedAt.IsZero() {
				snapshot.CreatedAt = now
			}
			snapshot.UpdatedAt = now
			snapshot.Found = true
			state.snapshots[key] = snapshot
			state.mu.Unlock()
			writeEnvelope(w, http.StatusOK, "ok", model.AStockRecommendationSnapshotUpsertResult{Updated: 1})
		default:
			writeEnvelope(w, http.StatusNotFound, "not found", nil)
		}
	}))

	srv := NewServer(config.Config{
		GatewayWebURL: backend.URL,
		AuthURL:       backend.URL,
		ContentURL:    backend.URL,
		CrawlerURL:    backend.URL,
		AnalysisURL:   backend.URL,
		NLPURL:        backend.URL,
		SchedulerURL:  backend.URL,
	})
	return srv, backend.Close
}
