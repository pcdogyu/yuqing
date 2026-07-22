package portal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/config"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

func TestAStockEveningPeriodIsRecognized(t *testing.T) {
	period := normalizeAStockPeriod("evening")
	if period.Key != "evening" || period.Label != "晚间推荐" || period.WindowLabel != "15:00-18:30" {
		t.Fatalf("unexpected evening period: %+v", period)
	}
	if normalizeAStockPeriod("night").Key != "evening" {
		t.Fatalf("expected night alias to normalize to evening")
	}
	keys := aStockPeriodKeys()
	if !slices.Contains(keys, "evening") {
		t.Fatalf("expected period list to include evening, got %v", keys)
	}
}

func TestBuildAStockEveningRecommendationsStrictBoundariesAndLimit(t *testing.T) {
	candidates := []aStockEveningCandidate{
		eveningCandidate("600000", "浦发银行", 4.6, 11.2, 5.1, 4.0, 1_900_000_000, 0.2),
		eveningCandidate("000001", "平安银行", 4.4, 12.5, 5.2, 5.0, 1_500_000_000, 1.5),
		eveningCandidate("002230", "科大讯飞", 6.0, 43.2, 6.0, 6.0, 1_500_000_000, 1.0),
		eveningCandidate("300059", "东方财富", 5.1, 22.3, 4.9, 7.0, 1_200_000_000, 0.8),
		eveningCandidate("601318", "中国平安", 7.2, 56.7, 4.8, 8.0, 900_000_000, 0.6),
		eveningCandidate("000858", "五粮液", 4.3, 142.0, 4.7, 9.0, 800_000_000, 0.7),
		eveningCandidate("600010", "包钢股份", 3.0, 5.2, 4.5, 5.0, 700_000_000, 0.7),
		eveningCandidate("600011", "华能国际", 10.0, 7.2, 4.5, 5.0, 700_000_000, 0.7),
		eveningCandidate("600012", "皖通高速", 4.0, 8.2, 3.0, 5.0, 700_000_000, 0.7),
		eveningCandidate("600015", "华夏银行", 4.0, 8.2, 8.0, 5.0, 700_000_000, 0.7),
		eveningCandidate("000651", "格力电器", 4.0, 4.0, 4.5, 5.0, 700_000_000, 0.7),
		eveningCandidate("000333", "美的集团", 4.0, 72.0, 4.5, 2.0, 700_000_000, 0.7),
		eveningCandidate("002415", "海康威视", 4.0, 31.0, 4.5, 15.0, 700_000_000, 0.7),
		eveningCandidate("600519", "贵州茅台", 4.0, 1800.0, 4.5, 5.0, 200_000_000, 0.7),
		eveningCandidate("600036", "招商银行", 4.0, 40.0, 4.5, 5.0, 2_000_000_000, 0.7),
		eveningCandidate("000063", "中兴通讯", 4.0, 38.0, 4.5, 5.0, 700_000_000, 0),
	}

	recommendations := buildAStockEveningRecommendations("2026-07-22", candidates, 5)
	gotCodes := make([]string, 0, len(recommendations))
	for _, rec := range recommendations {
		gotCodes = append(gotCodes, rec.Code)
		if rec.Hotspot != aStockEveningHotspot {
			t.Fatalf("expected evening hotspot, got %+v", rec)
		}
		for _, fragment := range []string{"量比", "涨跌幅", "价格", "换手率", "成交额", "涨速"} {
			if !strings.Contains(rec.Reason, fragment) {
				t.Fatalf("expected reason to contain %s, got %q", fragment, rec.Reason)
			}
		}
	}
	wantCodes := []string{"600000", "000001", "002230", "601318", "000858"}
	if !slices.Equal(gotCodes, wantCodes) {
		t.Fatalf("expected ordered limited codes %v, got %v", wantCodes, gotCodes)
	}
}

func TestAStockEveningNoMatchPersistsEmptySnapshot(t *testing.T) {
	akshare := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-stock/evening-snapshot" {
			t.Fatalf("unexpected akshare request: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(aStockEveningSnapshotPayload{
			Date:  "2026-07-22",
			Items: []aStockEveningCandidate{eveningCandidate("600000", "浦发银行", 3.0, 11.2, 4.0, 4.0, 500_000_000, 0.2)},
			Count: 1,
		})
	}))
	defer akshare.Close()

	var savedSnapshot model.AStockRecommendationSnapshot
	var savedSelections model.AStockRecommendationSelectionSet
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/internal/a-stock/recommendation-selections":
			if err := json.NewDecoder(r.Body).Decode(&savedSelections); err != nil {
				t.Fatalf("decode selections: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		case "/api/v1/internal/a-stock/recommendations":
			if err := json.NewDecoder(r.Body).Decode(&savedSnapshot); err != nil {
				t.Fatalf("decode snapshot: %v", err)
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
	}))
	defer content.Close()

	srv := NewServer(config.Config{AStockAuctionURL: akshare.URL, ContentURL: content.URL, HTTPTimeout: time.Second})
	ctx := newAStockBaseContext("2026-07-22", "evening", 1, false, false, false, false, aStockRecommendationPhaseFinal)
	ctx = srv.loadAStockEveningRecommendationContext(ctx, newAStockRequestCache(), true, true)

	if len(ctx.Recommendations) != 0 || ctx.BacktestStatus != "无推荐股票" {
		t.Fatalf("expected empty evening context, got recs=%d status=%q", len(ctx.Recommendations), ctx.BacktestStatus)
	}
	if !strings.Contains(ctx.EmptyReason, aStockEveningEmptyReason) {
		t.Fatalf("expected evening empty reason, got %q", ctx.EmptyReason)
	}
	if savedSelections.StrategyDate != "2026-07-22" || savedSelections.Period != "evening" || len(savedSelections.Items) != 0 {
		t.Fatalf("expected empty selections persisted, got %+v", savedSelections)
	}
	if savedSnapshot.StrategyDate != "2026-07-22" || savedSnapshot.Period != "evening" || savedSnapshot.RecommendationsJSON != "[]" || !strings.Contains(savedSnapshot.EmptyReason, aStockEveningEmptyReason) {
		t.Fatalf("expected empty snapshot persisted, got %+v", savedSnapshot)
	}
}

func TestBuildAStockBacktestRowsEveningUsesNextTradingDayOpen(t *testing.T) {
	recommendations := []aStockRecommendation{{Code: "600000", Name: "浦发银行"}}
	bars := map[string][]aStockMarketBar{
		"600000": {
			{Code: "600000", Date: "2026-07-17", Open: 10, Close: 10, Pct: 0},
			{Code: "600000", Date: "2026-07-20", Open: 11, Close: 12, Pct: 20},
			{Code: "600000", Date: "2026-07-21", Open: 12, Close: 13, Pct: 8.33},
		},
	}

	rows := buildAStockBacktestRows("2026-07-17", "evening", recommendations, bars)
	if len(rows) != 1 {
		t.Fatalf("expected one backtest row, got %d", len(rows))
	}
	if rows[0].EntryOpen != "11.00" || rows[0].T0Close != "12.00" || rows[0].T0Return != "+9.09%" {
		t.Fatalf("expected next trading day open/close return, got %+v", rows[0])
	}
	if len(rows[0].Days) == 0 || rows[0].Days[0].Return != "+18.18%" {
		t.Fatalf("expected T+1 to move after entry day, got %+v", rows[0].Days)
	}
}

func TestAStockBacktestContextsDisplayPreviousTradingDayEvening(t *testing.T) {
	content := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/a-stock/recommendations" {
			t.Fatalf("unexpected content request: %s", r.URL.String())
		}
		snapshot := model.AStockRecommendationSnapshot{
			Found:        false,
			StrategyDate: r.URL.Query().Get("date"),
			Period:       r.URL.Query().Get("period"),
		}
		if r.URL.Query().Get("date") == "2026-07-17" && r.URL.Query().Get("period") == "evening" {
			snapshot.Found = true
			snapshot.RecommendationsJSON = mustAStockTestJSON(t, []aStockRecommendation{
				{Rank: 1, Hotspot: "晚间量价筛选", Code: "600000", Name: "浦发银行", Reason: "量比 4.00"},
			})
			snapshot.BacktestsJSON = mustAStockTestJSON(t, []aStockBacktestRow{
				{Stock: "600000 浦发银行", EntryOpen: "11.00", T0Return: "+9.09%", T0Close: "12.00", T0ReturnClass: "astock-up", Days: []aStockBacktestCell{}, BestReturn: "--", BestReturnClass: "astock-flat", Status: "等待T+1行情"},
			})
			snapshot.BacktestStatus = "已锁定推荐股票，已回测 0/1"
			snapshot.GeneratedCount = 1
		}
		writeEnvelope(w, http.StatusOK, "ok", snapshot)
	}))
	defer content.Close()

	srv := NewServer(config.Config{ContentURL: content.URL, HTTPTimeout: time.Second})
	contexts := srv.loadAStockBacktestDisplayContextsWithCache("2026-07-20", "evening", 1, false, false, false, false, false, newAStockRequestCache())
	eveningCtx := aStockContextForPeriod(contexts, "evening")
	if eveningCtx.Date != "2026-07-17" || len(eveningCtx.Recommendations) != 1 {
		t.Fatalf("expected previous trading day evening context, got date=%s recs=%d status=%q empty=%q load=%q", eveningCtx.Date, len(eveningCtx.Recommendations), eveningCtx.BacktestStatus, eveningCtx.EmptyReason, eveningCtx.LoadMessage)
	}
	rows := combineAStockBacktestRows(contexts)
	if len(rows) != 1 || rows[0].PeriodKey != "evening" || rows[0].PeriodLabel != "晚间推荐" {
		t.Fatalf("expected evening backtest display row, got %+v", rows)
	}
}

func eveningCandidate(code string, name string, volumeRatio float64, price float64, changePct float64, turnoverPct float64, amount float64, speed float64) aStockEveningCandidate {
	return aStockEveningCandidate{
		TradeDate:   "2026-07-22",
		Code:        code,
		Name:        name,
		Price:       price,
		ChangePct:   changePct,
		VolumeRatio: volumeRatio,
		TurnoverPct: turnoverPct,
		Amount:      amount,
		Speed:       speed,
		Source:      "test",
		Status:      "ok",
	}
}
