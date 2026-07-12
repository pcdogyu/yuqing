package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/astockcode"
	"github.com/pcdogyu/yuqing/go/internal/model"
)

const (
	aStockPerformanceStrategyOfficial = "official"
	aStockPerformanceMinSamples       = 20
)

type aStockPerformanceSnapshot struct {
	StrategyKey         string
	StrategyDate        string
	Period              string
	RecommendationsJSON string
	BacktestsJSON       string
}

type aStockPerformanceRecommendation struct {
	Rank         int
	Hotspot      string
	Code         string
	Name         string
	HotspotScore int
	MarketScore  int
	Change30     string
	Change60     string
	FundFlow5D   string
}

type aStockPerformanceBacktestCell struct {
	Return string
}

type aStockPerformanceBacktestRow struct {
	Stock string
	Days  []aStockPerformanceBacktestCell
}

func (s *Store) UpsertAStockRecommendationShadowSnapshot(ctx context.Context, snapshot model.AStockRecommendationShadowSnapshot) (model.AStockRecommendationSnapshotUpsertResult, error) {
	result := model.AStockRecommendationSnapshotUpsertResult{}
	strategyKey := normalizeAStockPerformanceStrategy(snapshot.StrategyKey)
	if strategyKey == aStockPerformanceStrategyOfficial {
		return result, nil
	}
	snapshot.StrategyDate = strings.TrimSpace(snapshot.StrategyDate)
	snapshot.Period = strings.TrimSpace(snapshot.Period)
	if strategyKey == "" || snapshot.StrategyDate == "" || snapshot.Period == "" {
		return result, nil
	}
	existed := false
	if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM a_stock_recommendation_shadow_snapshots WHERE strategy_key = ? AND strategy_date = ? AND period = ?`,
		strategyKey, snapshot.StrategyDate, snapshot.Period).Scan(new(int)); err == nil {
		existed = true
	} else if err != nil && !errorsIsNoRows(err) {
		return result, err
	}
	now := time.Now().UTC()
	createdAt := snapshot.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := snapshot.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO a_stock_recommendation_shadow_snapshots (
	strategy_key, strategy_date, period, recommendations_json, backtests_json, news_summary_json,
	backtest_status, generated_count, market_candidate_status, market_candidate_count,
	auction_amount_label, empty_reason, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(strategy_key, strategy_date, period) DO UPDATE SET
	recommendations_json = excluded.recommendations_json,
	backtests_json = excluded.backtests_json,
	news_summary_json = excluded.news_summary_json,
	backtest_status = excluded.backtest_status,
	generated_count = excluded.generated_count,
	market_candidate_status = excluded.market_candidate_status,
	market_candidate_count = excluded.market_candidate_count,
	auction_amount_label = excluded.auction_amount_label,
	empty_reason = excluded.empty_reason,
	updated_at = excluded.updated_at`,
		strategyKey,
		snapshot.StrategyDate,
		snapshot.Period,
		normalizeAStockRecommendationJSONArray(snapshot.RecommendationsJSON),
		normalizeAStockRecommendationJSONArray(snapshot.BacktestsJSON),
		strings.TrimSpace(snapshot.NewsSummaryJSON),
		strings.TrimSpace(snapshot.BacktestStatus),
		snapshot.GeneratedCount,
		strings.TrimSpace(snapshot.MarketCandidateStatus),
		snapshot.MarketCandidateCount,
		strings.TrimSpace(snapshot.AuctionAmountLabel),
		strings.TrimSpace(snapshot.EmptyReason),
		createdAt.UTC().Format(time.RFC3339),
		updatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return result, err
	}
	if existed {
		result.Updated = 1
	} else {
		result.Inserted = 1
	}
	return result, nil
}

func (s *Store) BuildAStockRecommendationPerformance(ctx context.Context, filter model.AStockRecommendationPerformanceFilter) (model.AStockRecommendationPerformanceSummary, error) {
	filter = normalizeAStockPerformanceFilter(filter)
	snapshots, err := s.loadAStockPerformanceSnapshots(ctx, filter)
	if err != nil {
		return model.AStockRecommendationPerformanceSummary{}, err
	}
	acc := newAStockPerformanceAccumulator(filter)
	for _, snapshot := range snapshots {
		if err := acc.addSnapshot(snapshot); err != nil {
			return model.AStockRecommendationPerformanceSummary{}, err
		}
	}
	return acc.summary(), nil
}

func normalizeAStockPerformanceFilter(filter model.AStockRecommendationPerformanceFilter) model.AStockRecommendationPerformanceFilter {
	filter.Strategy = normalizeAStockPerformanceStrategy(filter.Strategy)
	if filter.Strategy == "" {
		filter.Strategy = aStockPerformanceStrategyOfficial
	}
	filter.Period = strings.ToLower(strings.TrimSpace(filter.Period))
	if filter.Period == "" {
		filter.Period = "all"
	}
	filter.EndDate = strings.TrimSpace(filter.EndDate)
	filter.StartDate = strings.TrimSpace(filter.StartDate)
	if filter.EndDate == "" {
		filter.EndDate = time.Now().In(time.Local).Format("2006-01-02")
	}
	if filter.StartDate == "" {
		if end, err := time.Parse("2006-01-02", filter.EndDate); err == nil {
			filter.StartDate = end.AddDate(0, 0, -90).Format("2006-01-02")
		}
	}
	return filter
}

func normalizeAStockPerformanceStrategy(strategy string) string {
	strategy = strings.ToLower(strings.TrimSpace(strategy))
	if strategy == "" {
		return ""
	}
	if strategy == aStockPerformanceStrategyOfficial {
		return strategy
	}
	return strategy
}

func (s *Store) loadAStockPerformanceSnapshots(ctx context.Context, filter model.AStockRecommendationPerformanceFilter) ([]aStockPerformanceSnapshot, error) {
	if filter.Strategy == aStockPerformanceStrategyOfficial {
		return s.loadAStockOfficialPerformanceSnapshots(ctx, filter)
	}
	return s.loadAStockShadowPerformanceSnapshots(ctx, filter)
}

func (s *Store) loadAStockOfficialPerformanceSnapshots(ctx context.Context, filter model.AStockRecommendationPerformanceFilter) ([]aStockPerformanceSnapshot, error) {
	query := `
SELECT strategy_date, period, recommendations_json, backtests_json
FROM a_stock_recommendation_snapshots
WHERE ignore_recent = 0 AND strategy_date >= ? AND strategy_date <= ?`
	args := []any{filter.StartDate, filter.EndDate}
	if filter.Period != "" && filter.Period != "all" {
		query += ` AND period = ?`
		args = append(args, filter.Period)
	}
	query += ` ORDER BY strategy_date ASC, period ASC, updated_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	snapshots := make([]aStockPerformanceSnapshot, 0)
	seen := make(map[string]struct{})
	for rows.Next() {
		snapshot := aStockPerformanceSnapshot{StrategyKey: aStockPerformanceStrategyOfficial}
		if err := rows.Scan(&snapshot.StrategyDate, &snapshot.Period, &snapshot.RecommendationsJSON, &snapshot.BacktestsJSON); err != nil {
			return nil, err
		}
		key := snapshot.StrategyDate + "|" + snapshot.Period
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, rows.Err()
}

func (s *Store) loadAStockShadowPerformanceSnapshots(ctx context.Context, filter model.AStockRecommendationPerformanceFilter) ([]aStockPerformanceSnapshot, error) {
	query := `
SELECT strategy_key, strategy_date, period, recommendations_json, backtests_json
FROM a_stock_recommendation_shadow_snapshots
WHERE strategy_key = ? AND strategy_date >= ? AND strategy_date <= ?`
	args := []any{filter.Strategy, filter.StartDate, filter.EndDate}
	if filter.Period != "" && filter.Period != "all" {
		query += ` AND period = ?`
		args = append(args, filter.Period)
	}
	query += ` ORDER BY strategy_date ASC, period ASC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	snapshots := make([]aStockPerformanceSnapshot, 0)
	for rows.Next() {
		snapshot := aStockPerformanceSnapshot{}
		if err := rows.Scan(&snapshot.StrategyKey, &snapshot.StrategyDate, &snapshot.Period, &snapshot.RecommendationsJSON, &snapshot.BacktestsJSON); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, rows.Err()
}

type aStockPerformanceAccumulator struct {
	filter model.AStockRecommendationPerformanceFilter
	total  aStockPerformanceBucket
	groups map[string]*aStockPerformanceBucket
}

type aStockPerformanceBucket struct {
	dimension           string
	key                 string
	recommendationCount int
	sampleCount         int
	winCount            int
	returnSum           float64
}

func newAStockPerformanceAccumulator(filter model.AStockRecommendationPerformanceFilter) *aStockPerformanceAccumulator {
	return &aStockPerformanceAccumulator{
		filter: filter,
		groups: make(map[string]*aStockPerformanceBucket),
	}
}

func (a *aStockPerformanceAccumulator) addSnapshot(snapshot aStockPerformanceSnapshot) error {
	var recommendations []aStockPerformanceRecommendation
	if err := json.Unmarshal([]byte(normalizeAStockRecommendationJSONArray(snapshot.RecommendationsJSON)), &recommendations); err != nil {
		return fmt.Errorf("parse recommendations %s %s: %w", snapshot.StrategyDate, snapshot.Period, err)
	}
	var backtests []aStockPerformanceBacktestRow
	if err := json.Unmarshal([]byte(normalizeAStockRecommendationJSONArray(snapshot.BacktestsJSON)), &backtests); err != nil {
		return fmt.Errorf("parse backtests %s %s: %w", snapshot.StrategyDate, snapshot.Period, err)
	}
	backtestsByCode := make(map[string]aStockPerformanceBacktestRow, len(backtests))
	for _, row := range backtests {
		code := aStockPerformanceBacktestRowCode(row)
		if code != "" {
			backtestsByCode[code] = row
		}
	}
	for _, rec := range recommendations {
		code := astockcode.Normalize(rec.Code)
		if code == "" {
			continue
		}
		t1Return, matured := aStockPerformanceT1Return(backtestsByCode[code])
		groups := aStockPerformanceRecommendationGroups(snapshot.Period, rec)
		a.total.add(t1Return, matured)
		for _, group := range groups {
			a.group(group.dimension, group.key).add(t1Return, matured)
		}
	}
	return nil
}

func (b *aStockPerformanceBucket) add(returnValue float64, matured bool) {
	b.recommendationCount++
	if !matured {
		return
	}
	b.sampleCount++
	b.returnSum += returnValue
	if returnValue > 0 {
		b.winCount++
	}
}

func (a *aStockPerformanceAccumulator) group(dimension string, key string) *aStockPerformanceBucket {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "未分组"
	}
	mapKey := dimension + "\x00" + key
	bucket := a.groups[mapKey]
	if bucket == nil {
		bucket = &aStockPerformanceBucket{dimension: dimension, key: key}
		a.groups[mapKey] = bucket
	}
	return bucket
}

func (a *aStockPerformanceAccumulator) summary() model.AStockRecommendationPerformanceSummary {
	summary := model.AStockRecommendationPerformanceSummary{
		Strategy:            a.filter.Strategy,
		StartDate:           a.filter.StartDate,
		EndDate:             a.filter.EndDate,
		Period:              a.filter.Period,
		RecommendationCount: a.total.recommendationCount,
		SampleCount:         a.total.sampleCount,
		WinCount:            a.total.winCount,
		WinRate:             aStockPerformanceRate(a.total.winCount, a.total.sampleCount),
		AverageReturn:       aStockPerformanceAverage(a.total.returnSum, a.total.sampleCount),
		RecommendationCover: aStockPerformanceRate(a.total.sampleCount, a.total.recommendationCount),
		InsufficientSamples: a.total.sampleCount < aStockPerformanceMinSamples,
	}
	groups := make([]model.AStockRecommendationPerformanceGroup, 0, len(a.groups))
	for _, bucket := range a.groups {
		groups = append(groups, model.AStockRecommendationPerformanceGroup{
			Dimension:           bucket.dimension,
			Key:                 bucket.key,
			RecommendationCount: bucket.recommendationCount,
			SampleCount:         bucket.sampleCount,
			WinCount:            bucket.winCount,
			WinRate:             aStockPerformanceRate(bucket.winCount, bucket.sampleCount),
			AverageReturn:       aStockPerformanceAverage(bucket.returnSum, bucket.sampleCount),
			RecommendationCover: aStockPerformanceRate(bucket.sampleCount, bucket.recommendationCount),
			InsufficientSamples: bucket.sampleCount < aStockPerformanceMinSamples,
		})
	}
	sort.SliceStable(groups, func(i, j int) bool {
		leftOrder := aStockPerformanceDimensionOrder(groups[i].Dimension)
		rightOrder := aStockPerformanceDimensionOrder(groups[j].Dimension)
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		if groups[i].SampleCount != groups[j].SampleCount {
			return groups[i].SampleCount > groups[j].SampleCount
		}
		return groups[i].Key < groups[j].Key
	})
	summary.Groups = groups
	return summary
}

type aStockPerformanceGroupKey struct {
	dimension string
	key       string
}

func aStockPerformanceRecommendationGroups(period string, rec aStockPerformanceRecommendation) []aStockPerformanceGroupKey {
	return []aStockPerformanceGroupKey{
		{dimension: "period", key: nonEmpty(strings.TrimSpace(period), "unknown")},
		{dimension: "hotspot", key: strings.TrimSpace(rec.Hotspot)},
		{dimension: "rank", key: aStockPerformanceRankBucket(rec.Rank)},
		{dimension: "score", key: aStockPerformanceScoreBucket(rec.MarketScore, rec.HotspotScore)},
		{dimension: "fund_flow", key: aStockPerformanceFundFlowBucket(rec.FundFlow5D)},
		{dimension: "drawdown", key: aStockPerformanceDrawdownBucket(rec.Change30, rec.Change60)},
	}
}

func aStockPerformanceT1Return(row aStockPerformanceBacktestRow) (float64, bool) {
	if len(row.Days) == 0 {
		return 0, false
	}
	return parseAStockPerformancePct(row.Days[0].Return)
}

func aStockPerformanceBacktestRowCode(row aStockPerformanceBacktestRow) string {
	fields := strings.Fields(strings.TrimSpace(row.Stock))
	if len(fields) == 0 {
		return ""
	}
	return astockcode.Normalize(fields[0])
}

func parseAStockPerformancePct(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" || value == "--" {
		return 0, false
	}
	value = strings.TrimSuffix(value, "%")
	value = strings.TrimPrefix(value, "+")
	value = strings.ReplaceAll(value, ",", "")
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func aStockPerformanceRankBucket(rank int) string {
	switch {
	case rank <= 0:
		return "未知"
	case rank <= 3:
		return "1-3"
	case rank <= 6:
		return "4-6"
	case rank <= 12:
		return "7-12"
	default:
		return "12+"
	}
}

func aStockPerformanceScoreBucket(marketScore int, hotspotScore int) string {
	score := marketScore
	if score == 0 {
		score = hotspotScore
	}
	switch {
	case score >= 250:
		return "250+"
	case score >= 200:
		return "200-249"
	case score >= 150:
		return "150-199"
	case score > 0:
		return "1-149"
	default:
		return "未知"
	}
}

func aStockPerformanceFundFlowBucket(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "--" {
		return "缺失"
	}
	if strings.HasPrefix(value, "-") {
		return "净流出"
	}
	if strings.HasPrefix(value, "0") || strings.HasPrefix(value, "+0") {
		return "持平"
	}
	return "净流入"
}

func aStockPerformanceDrawdownBucket(change30 string, change60 string) string {
	values := make([]float64, 0, 2)
	if value, ok := parseAStockPerformancePct(change30); ok {
		values = append(values, value)
	}
	if value, ok := parseAStockPerformancePct(change60); ok {
		values = append(values, value)
	}
	if len(values) == 0 {
		return "缺失"
	}
	lowest := values[0]
	for _, value := range values[1:] {
		if value < lowest {
			lowest = value
		}
	}
	switch {
	case lowest <= -10:
		return "回撤<-10%"
	case lowest < 0:
		return "回撤0~-10%"
	default:
		return "强势"
	}
}

func aStockPerformanceRate(numerator int, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func aStockPerformanceAverage(total float64, count int) float64 {
	if count <= 0 {
		return 0
	}
	return total / float64(count)
}

func aStockPerformanceDimensionOrder(dimension string) int {
	switch dimension {
	case "period":
		return 1
	case "hotspot":
		return 2
	case "rank":
		return 3
	case "score":
		return 4
	case "fund_flow":
		return 5
	case "drawdown":
		return 6
	default:
		return 99
	}
}
