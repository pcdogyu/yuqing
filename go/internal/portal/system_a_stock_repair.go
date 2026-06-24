package portal

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

type aStockRepairView struct {
	StrategyDate                string
	Period                      string
	IgnoreRecent                bool
	Selections                  model.AStockRecommendationSelectionListResult
	SelectionsJSON              string
	SelectionsCreatedAtText     string
	SelectionsUpdatedAtText     string
	Snapshot                    model.AStockRecommendationSnapshot
	SnapshotRecommendationsJSON string
	SnapshotBacktestsJSON       string
	SnapshotCreatedAtText       string
	SnapshotUpdatedAtText       string
	Error                       string
}

type aStockRepairSnapshotInput struct {
	StrategyDate             string
	Period                   string
	IgnoreRecent             bool
	RecommendationsJSON      string
	BacktestsJSON            string
	BacktestStatus           string
	GeneratedCount           int
	RecentFiltered           int
	SameDayMorningFiltered   int
	LimitUpFilterEnabled     bool
	LimitUpFiltered          int
	TodayMarketFilterEnabled bool
	NoTodayMarketCount       int
	MarketCandidateStatus    string
	MarketCandidateCount     int
	AuctionAmountLabel       string
	EmptyReason              string
}

func (s *Server) loadAStockRepairView(strategyDate string, period string, ignoreRecent bool) aStockRepairView {
	view := aStockRepairView{
		StrategyDate:                normalizeAStockStrategyDate(strategyDate),
		Period:                      normalizeAStockPeriod(period).Key,
		IgnoreRecent:                ignoreRecent,
		SelectionsJSON:              "[]",
		SelectionsCreatedAtText:     "--",
		SelectionsUpdatedAtText:     "--",
		SnapshotRecommendationsJSON: "[]",
		SnapshotBacktestsJSON:       "[]",
		SnapshotCreatedAtText:       "--",
		SnapshotUpdatedAtText:       "--",
	}
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		view.Error = "内容服务未配置"
		return view
	}

	errs := make([]string, 0, 2)

	selectionsURL := strings.TrimRight(s.cfg.ContentURL, "/") + "/api/v1/a-stock/recommendation-selections?date=" + url.QueryEscape(view.StrategyDate) + "&period=" + url.QueryEscape(view.Period)
	if err := s.getJSON(selectionsURL, &view.Selections); err != nil {
		errs = append(errs, "已选股票层读取失败："+err.Error())
	} else {
		view.SelectionsJSON = formatAStockRepairJSON(view.Selections.Items, "[]")
		view.SelectionsCreatedAtText = formatAStockRepairTime(view.Selections.CreatedAt)
		view.SelectionsUpdatedAtText = formatAStockRepairTime(view.Selections.UpdatedAt)
	}

	snapshotURL := strings.TrimRight(s.cfg.ContentURL, "/") + "/api/v1/a-stock/recommendations?date=" + url.QueryEscape(view.StrategyDate) + "&period=" + url.QueryEscape(view.Period)
	if view.IgnoreRecent {
		snapshotURL += "&ignore_recent=1"
	}
	if err := s.getJSON(snapshotURL, &view.Snapshot); err != nil {
		errs = append(errs, "推荐快照层读取失败："+err.Error())
	} else {
		view.SnapshotRecommendationsJSON = formatAStockRepairJSONText(view.Snapshot.RecommendationsJSON, "[]")
		view.SnapshotBacktestsJSON = formatAStockRepairJSONText(view.Snapshot.BacktestsJSON, "[]")
		view.SnapshotCreatedAtText = formatAStockRepairTime(view.Snapshot.CreatedAt)
		view.SnapshotUpdatedAtText = formatAStockRepairTime(view.Snapshot.UpdatedAt)
	}

	if len(errs) > 0 {
		view.Error = strings.Join(errs, "；")
	}
	return view
}

func (s *Server) saveAStockRepairSelections(strategyDate string, period string, raw string) error {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return fmt.Errorf("内容服务未配置")
	}
	items := make([]model.AStockRecommendationSelection, 0)
	normalizedJSON, err := normalizeAStockRepairJSON(raw, "[]")
	if err != nil {
		return fmt.Errorf("JSON 解析失败: %w", err)
	}
	if err := json.Unmarshal([]byte(normalizedJSON), &items); err != nil {
		return fmt.Errorf("JSON 解析失败: %w", err)
	}
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	period = normalizeAStockPeriod(period).Key
	for i := range items {
		items[i].StrategyDate = strategyDate
		items[i].Period = period
		items[i].Code = strings.TrimSpace(items[i].Code)
		items[i].Hotspot = strings.TrimSpace(items[i].Hotspot)
		items[i].Name = strings.TrimSpace(items[i].Name)
		items[i].Reason = strings.TrimSpace(items[i].Reason)
		if items[i].Rank <= 0 {
			items[i].Rank = i + 1
		}
	}
	resp, err := s.client.R().
		SetBody(model.AStockRecommendationSelectionSet{
			StrategyDate: strategyDate,
			Period:       period,
			Items:        items,
		}).
		Post(strings.TrimRight(s.cfg.ContentURL, "/") + "/api/v1/internal/a-stock/recommendation-selections")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf(responseErrorMessage(resp, nil))
	}
	return nil
}

func (s *Server) saveAStockRepairSnapshot(input aStockRepairSnapshotInput) error {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return fmt.Errorf("内容服务未配置")
	}
	recommendationsJSON, err := normalizeAStockRepairJSONArray(input.RecommendationsJSON, "[]")
	if err != nil {
		return fmt.Errorf("recommendations_json 解析失败: %w", err)
	}
	backtestsJSON, err := normalizeAStockRepairJSONArray(input.BacktestsJSON, "[]")
	if err != nil {
		return fmt.Errorf("backtests_json 解析失败: %w", err)
	}
	snapshot := model.AStockRecommendationSnapshot{
		StrategyDate:             normalizeAStockStrategyDate(input.StrategyDate),
		Period:                   normalizeAStockPeriod(input.Period).Key,
		IgnoreRecent:             input.IgnoreRecent,
		RecommendationsJSON:      recommendationsJSON,
		BacktestsJSON:            backtestsJSON,
		BacktestStatus:           strings.TrimSpace(input.BacktestStatus),
		GeneratedCount:           input.GeneratedCount,
		RecentFiltered:           input.RecentFiltered,
		SameDayMorningFiltered:   input.SameDayMorningFiltered,
		LimitUpFilterEnabled:     input.LimitUpFilterEnabled,
		LimitUpFiltered:          input.LimitUpFiltered,
		TodayMarketFilterEnabled: input.TodayMarketFilterEnabled,
		NoTodayMarketCount:       input.NoTodayMarketCount,
		MarketCandidateStatus:    strings.TrimSpace(input.MarketCandidateStatus),
		MarketCandidateCount:     input.MarketCandidateCount,
		AuctionAmountLabel:       strings.TrimSpace(input.AuctionAmountLabel),
		EmptyReason:              strings.TrimSpace(input.EmptyReason),
	}
	resp, err := s.client.R().
		SetBody(snapshot).
		Post(strings.TrimRight(s.cfg.ContentURL, "/") + "/api/v1/internal/a-stock/recommendations")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf(responseErrorMessage(resp, nil))
	}
	return nil
}

func normalizeAStockRepairJSON(raw string, fallback string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = fallback
	}
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", err
	}
	normalized, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(normalized), nil
}

func normalizeAStockRepairJSONArray(raw string, fallback string) (string, error) {
	normalized, err := normalizeAStockRepairJSON(raw, fallback)
	if err != nil {
		return "", err
	}
	var payload []any
	if err := json.Unmarshal([]byte(normalized), &payload); err != nil {
		return "", fmt.Errorf("必须是 JSON 数组")
	}
	return normalized, nil
}

func formatAStockRepairJSON(payload any, fallback string) string {
	if payload == nil {
		return fallback
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fallback
	}
	return string(encoded)
}

func formatAStockRepairJSONText(raw string, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return raw
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return raw
	}
	return string(encoded)
}

func formatAStockRepairTime(value time.Time) string {
	if value.IsZero() {
		return "--"
	}
	return value.In(aStockLocation()).Format("2006-01-02 15:04:05")
}
