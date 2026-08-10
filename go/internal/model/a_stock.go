package model

import "time"

type AStockAuctionAmount struct {
	TradeDate     string    `json:"trade_date"`
	CaptureSlot   string    `json:"capture_slot"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	AuctionPrice  float64   `json:"auction_price"`
	AuctionVolume float64   `json:"auction_volume"`
	AuctionAmount float64   `json:"auction_amount"`
	Source        string    `json:"source"`
	Status        string    `json:"status"`
	FetchedAt     time.Time `json:"fetched_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type AStockAuctionFilter struct {
	Date        string `json:"date"`
	CaptureSlot string `json:"capture_slot"`
	Keyword     string `json:"keyword"`
	Page        int    `json:"page"`
	PageSize    int    `json:"page_size"`
	TrendDays   int    `json:"trend_days"`
}

type AStockAuctionListResult struct {
	Items        []AStockAuctionAmount           `json:"items"`
	Page         int                             `json:"page"`
	PageSize     int                             `json:"page_size"`
	Total        int                             `json:"total"`
	Date         string                          `json:"date"`
	CaptureSlot  string                          `json:"capture_slot"`
	Keyword      string                          `json:"keyword"`
	LatestDate   string                          `json:"latest_date"`
	Dates        []string                        `json:"dates"`
	SummaryCount int                             `json:"summary_count"`
	TotalAmount  float64                         `json:"total_amount"`
	MaxItem      *AStockAuctionAmount            `json:"max_item,omitempty"`
	FetchedAt    *time.Time                      `json:"fetched_at,omitempty"`
	Trend        []AStockAuctionTrend            `json:"trend"`
	TrendSeries  map[string][]AStockAuctionTrend `json:"trend_series,omitempty"`
}

type AStockAuctionUpsertResult struct {
	Date        string `json:"date"`
	CaptureSlot string `json:"capture_slot"`
	Inserted    int    `json:"inserted"`
	Updated     int    `json:"updated"`
	Total       int    `json:"total"`
}

type AStockCodeName struct {
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Source    string    `json:"source"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AStockCodeNameListResult struct {
	Items []AStockCodeName `json:"items"`
	Total int              `json:"total"`
}

type AStockCodeNameUpsertResult struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
	Total    int `json:"total"`
}

type AStockAuctionTrend struct {
	Date         string                   `json:"date"`
	CaptureSlot  string                   `json:"capture_slot,omitempty"`
	StockCount   int                      `json:"stock_count"`
	TotalVolume  float64                  `json:"total_volume"`
	TotalAmount  float64                  `json:"total_amount"`
	MaxStockCode string                   `json:"max_stock_code"`
	MaxStockName string                   `json:"max_stock_name"`
	MarketTop    []AStockAuctionMarketTop `json:"market_top,omitempty"`
}

type AStockAuctionMarketTop struct {
	Market string                `json:"market"`
	Items  []AStockAuctionAmount `json:"items"`
}

type AStockRecommendationSnapshot struct {
	Found                       bool      `json:"found"`
	StrategyDate                string    `json:"strategy_date"`
	Period                      string    `json:"period"`
	IgnoreRecent                bool      `json:"ignore_recent"`
	RecommendationsJSON         string    `json:"recommendations_json"`
	FilteredRecommendationsJSON string    `json:"filtered_recommendations_json"`
	BacktestsJSON               string    `json:"backtests_json"`
	NewsSummaryJSON             string    `json:"news_summary_json"`
	BacktestStatus              string    `json:"backtest_status"`
	GeneratedCount              int       `json:"generated_count"`
	RecentFiltered              int       `json:"recent_filtered"`
	SameDayMorningFiltered      int       `json:"same_day_morning_filtered"`
	LimitUpFilterEnabled        bool      `json:"limit_up_filter_enabled"`
	LimitUpFiltered             int       `json:"limit_up_filtered"`
	TodayMarketFilterEnabled    bool      `json:"today_market_filter_enabled"`
	NoTodayMarketCount          int       `json:"no_today_market_count"`
	FundFlowFilterEnabled       bool      `json:"fund_flow_filter_enabled"`
	FundFlowFiltered            int       `json:"fund_flow_filtered"`
	FundFlowMissingCount        int       `json:"fund_flow_missing_count"`
	MarketCandidateStatus       string    `json:"market_candidate_status"`
	MarketCandidateCount        int       `json:"market_candidate_count"`
	AuctionAmountLabel          string    `json:"auction_amount_label"`
	EmptyReason                 string    `json:"empty_reason"`
	CreatedAt                   time.Time `json:"created_at"`
	UpdatedAt                   time.Time `json:"updated_at"`
}

type AStockRecommendationSnapshotFilter struct {
	IgnoreRecent                bool
	HasLimitUpFilterEnabled     bool
	LimitUpFilterEnabled        bool
	HasTodayMarketFilterEnabled bool
	TodayMarketFilterEnabled    bool
	HasFundFlowFilterEnabled    bool
	FundFlowFilterEnabled       bool
}

func (f AStockRecommendationSnapshotFilter) Exact() bool {
	return f.HasLimitUpFilterEnabled && f.HasTodayMarketFilterEnabled && f.HasFundFlowFilterEnabled
}

type AStockRecommendationSnapshotUpsertResult struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
}

type AStockRecommendationShadowSnapshot struct {
	StrategyKey string `json:"strategy_key"`
	AStockRecommendationSnapshot
}

type AStockRecommendationPerformanceFilter struct {
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Period    string `json:"period"`
	Strategy  string `json:"strategy"`
}

type AStockRecommendationPerformanceGroup struct {
	Dimension           string  `json:"dimension"`
	Key                 string  `json:"key"`
	RecommendationCount int     `json:"recommendation_count"`
	SampleCount         int     `json:"sample_count"`
	WinCount            int     `json:"win_count"`
	WinRate             float64 `json:"win_rate"`
	AverageReturn       float64 `json:"average_return"`
	RecommendationCover float64 `json:"recommendation_cover"`
	InsufficientSamples bool    `json:"insufficient_samples"`
}

type AStockRecommendationPerformanceSummary struct {
	Strategy            string                                 `json:"strategy"`
	StartDate           string                                 `json:"start_date"`
	EndDate             string                                 `json:"end_date"`
	Period              string                                 `json:"period"`
	RecommendationCount int                                    `json:"recommendation_count"`
	SampleCount         int                                    `json:"sample_count"`
	WinCount            int                                    `json:"win_count"`
	WinRate             float64                                `json:"win_rate"`
	AverageReturn       float64                                `json:"average_return"`
	RecommendationCover float64                                `json:"recommendation_cover"`
	InsufficientSamples bool                                   `json:"insufficient_samples"`
	Groups              []AStockRecommendationPerformanceGroup `json:"groups"`
}

type AStockRecommendationSelection struct {
	StrategyDate string    `json:"strategy_date"`
	Period       string    `json:"period"`
	Rank         int       `json:"rank"`
	Hotspot      string    `json:"hotspot"`
	Code         string    `json:"code"`
	Name         string    `json:"name"`
	HotspotScore int       `json:"hotspot_score"`
	MarketScore  int       `json:"market_score"`
	Reason       string    `json:"reason"`
	EntryTime    string    `json:"entry_time"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type AStockRecommendationSelectionSet struct {
	StrategyDate string                          `json:"strategy_date"`
	Period       string                          `json:"period"`
	Items        []AStockRecommendationSelection `json:"items"`
}

type AStockRecommendationSelectionListResult struct {
	Found        bool                            `json:"found"`
	StrategyDate string                          `json:"strategy_date"`
	Period       string                          `json:"period"`
	Items        []AStockRecommendationSelection `json:"items"`
	CreatedAt    time.Time                       `json:"created_at"`
	UpdatedAt    time.Time                       `json:"updated_at"`
}

type AStockRecommendationSelectionUpsertResult struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
	Total    int `json:"total"`
}

type AStockRecommendationLatestDate struct {
	Code       string `json:"code"`
	LatestDate string `json:"latest_date"`
}

type AStockRecommendationLatestDateListResult struct {
	StrategyDate string                           `json:"strategy_date"`
	Period       string                           `json:"period"`
	Items        []AStockRecommendationLatestDate `json:"items"`
}

type AStockSectorFundFlow struct {
	TradeDate              string    `json:"trade_date"`
	SectorType             string    `json:"sector_type"`
	Indicator              string    `json:"indicator"`
	Rank                   int       `json:"rank"`
	Name                   string    `json:"name"`
	ChangePct              float64   `json:"change_pct"`
	MainNetInflow          float64   `json:"main_net_inflow"`
	MainNetInflowPct       float64   `json:"main_net_inflow_pct"`
	SuperLargeNetInflow    float64   `json:"super_large_net_inflow"`
	SuperLargeNetInflowPct float64   `json:"super_large_net_inflow_pct"`
	LargeNetInflow         float64   `json:"large_net_inflow"`
	LargeNetInflowPct      float64   `json:"large_net_inflow_pct"`
	MediumNetInflow        float64   `json:"medium_net_inflow"`
	MediumNetInflowPct     float64   `json:"medium_net_inflow_pct"`
	SmallNetInflow         float64   `json:"small_net_inflow"`
	SmallNetInflowPct      float64   `json:"small_net_inflow_pct"`
	TopStock               string    `json:"top_stock"`
	SourceCount            int       `json:"source_count"`
	SourceTypes            string    `json:"source_types"`
	FieldCountsJSON        string    `json:"field_counts_json"`
	SourceType             string    `json:"source_type"`
	RawPayload             string    `json:"raw_payload"`
	FetchedAt              time.Time `json:"fetched_at"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type AStockSectorFundFlowFilter struct {
	Date       string `json:"date"`
	SectorType string `json:"sector_type"`
	Indicator  string `json:"indicator"`
	Keyword    string `json:"keyword"`
	SourceType string `json:"source_type"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
}

type AStockSectorFundFlowListResult struct {
	Items       []AStockSectorFundFlow `json:"items"`
	Page        int                    `json:"page"`
	PageSize    int                    `json:"page_size"`
	Total       int                    `json:"total"`
	Date        string                 `json:"date"`
	LatestDate  string                 `json:"latest_date"`
	SectorType  string                 `json:"sector_type"`
	Indicator   string                 `json:"indicator"`
	Keyword     string                 `json:"keyword"`
	SourceType  string                 `json:"source_type"`
	Dates       []string               `json:"dates"`
	SectorTypes []string               `json:"sector_types"`
	Indicators  []string               `json:"indicators"`
	FetchedAt   *time.Time             `json:"fetched_at,omitempty"`
}

type AStockSectorFundFlowUpsertResult struct {
	Date     string `json:"date"`
	Inserted int    `json:"inserted"`
	Updated  int    `json:"updated"`
	Total    int    `json:"total"`
}

type AStockSectorFundFlowIntradayFilter struct {
	Date       string `json:"date"`
	SectorType string `json:"sector_type"`
	Indicator  string `json:"indicator"`
	Limit      int    `json:"limit"`
}

type AStockSectorFundFlowIntradayPoint struct {
	Time          string  `json:"time"`
	MainNetInflow float64 `json:"main_net_inflow"`
	Rank          int     `json:"rank,omitempty"`
}

type AStockSectorFundFlowIntradaySeries struct {
	Name                string                              `json:"name"`
	LatestRank          int                                 `json:"latest_rank"`
	LatestMainNetInflow float64                             `json:"latest_main_net_inflow"`
	Points              []AStockSectorFundFlowIntradayPoint `json:"points"`
}

type AStockSectorFundFlowIntradayResult struct {
	Date       string                               `json:"date"`
	SectorType string                               `json:"sector_type"`
	Indicator  string                               `json:"indicator"`
	LatestTime string                               `json:"latest_time"`
	Times      []string                             `json:"times"`
	Series     []AStockSectorFundFlowIntradaySeries `json:"series"`
	Top        []AStockSectorFundFlow               `json:"top"`
	Total      int                                  `json:"total"`
}

type AStockStockFundFlow struct {
	TradeDate              string    `json:"trade_date"`
	Indicator              string    `json:"indicator"`
	Rank                   int       `json:"rank"`
	Code                   string    `json:"code"`
	Name                   string    `json:"name"`
	Price                  float64   `json:"price"`
	ChangePct              float64   `json:"change_pct"`
	TurnoverPct            float64   `json:"turnover_pct"`
	Amount                 float64   `json:"amount"`
	InAmount               float64   `json:"in_amount"`
	OutAmount              float64   `json:"out_amount"`
	MainNetInflow          float64   `json:"main_net_inflow"`
	MainNetInflowPct       float64   `json:"main_net_inflow_pct"`
	SuperLargeNetInflow    float64   `json:"super_large_net_inflow"`
	SuperLargeNetInflowPct float64   `json:"super_large_net_inflow_pct"`
	LargeNetInflow         float64   `json:"large_net_inflow"`
	LargeNetInflowPct      float64   `json:"large_net_inflow_pct"`
	MediumNetInflow        float64   `json:"medium_net_inflow"`
	MediumNetInflowPct     float64   `json:"medium_net_inflow_pct"`
	SmallNetInflow         float64   `json:"small_net_inflow"`
	SmallNetInflowPct      float64   `json:"small_net_inflow_pct"`
	SourceCount            int       `json:"source_count"`
	SourceTypes            string    `json:"source_types"`
	FieldCountsJSON        string    `json:"field_counts_json"`
	SourceType             string    `json:"source_type"`
	RawPayload             string    `json:"raw_payload"`
	FetchedAt              time.Time `json:"fetched_at"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type AStockStockFundFlowFilter struct {
	Date       string   `json:"date"`
	Indicator  string   `json:"indicator"`
	Keyword    string   `json:"keyword"`
	SourceType string   `json:"source_type"`
	Codes      []string `json:"codes"`
	Page       int      `json:"page"`
	PageSize   int      `json:"page_size"`
}

type AStockStockFundFlowListResult struct {
	Items      []AStockStockFundFlow `json:"items"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"page_size"`
	Total      int                   `json:"total"`
	Date       string                `json:"date"`
	LatestDate string                `json:"latest_date"`
	Indicator  string                `json:"indicator"`
	Keyword    string                `json:"keyword"`
	SourceType string                `json:"source_type"`
	Dates      []string              `json:"dates"`
	Indicators []string              `json:"indicators"`
	FetchedAt  *time.Time            `json:"fetched_at,omitempty"`
}

type AStockStockFundFlowUpsertResult struct {
	Date     string `json:"date"`
	Inserted int    `json:"inserted"`
	Updated  int    `json:"updated"`
	Total    int    `json:"total"`
}

type AStockSectorConstituent struct {
	SectorType string    `json:"sector_type"`
	SectorName string    `json:"sector_name"`
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	Source     string    `json:"source"`
	FetchedAt  time.Time `json:"fetched_at"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type AStockSectorConstituentFilter struct {
	SectorType string `json:"sector_type"`
	SectorName string `json:"sector_name"`
	Keyword    string `json:"keyword"`
	Limit      int    `json:"limit"`
}

type AStockSectorConstituentListResult struct {
	Items      []AStockSectorConstituent `json:"items"`
	Total      int                       `json:"total"`
	SectorType string                    `json:"sector_type"`
	SectorName string                    `json:"sector_name"`
	Keyword    string                    `json:"keyword"`
	FetchedAt  *time.Time                `json:"fetched_at,omitempty"`
}

type AStockSectorConstituentUpsertResult struct {
	SectorType string `json:"sector_type"`
	SectorName string `json:"sector_name"`
	Inserted   int    `json:"inserted"`
	Updated    int    `json:"updated"`
	Total      int    `json:"total"`
}

type AStockFundFlowTrendFilter struct {
	EndDate    string `json:"end_date"`
	SectorType string `json:"sector_type"`
	SectorName string `json:"sector_name"`
	Indicator  string `json:"indicator"`
	Code       string `json:"code"`
	Keyword    string `json:"keyword"`
	Days       int    `json:"days"`
}

type AStockSectorFundFlowTrendResult struct {
	Items      []AStockSectorFundFlow `json:"items"`
	Total      int                    `json:"total"`
	EndDate    string                 `json:"end_date"`
	SectorType string                 `json:"sector_type"`
	SectorName string                 `json:"sector_name"`
	Indicator  string                 `json:"indicator"`
	Days       int                    `json:"days"`
}

type AStockStockFundFlowTrendResult struct {
	Items     []AStockStockFundFlow `json:"items"`
	Total     int                   `json:"total"`
	EndDate   string                `json:"end_date"`
	Indicator string                `json:"indicator"`
	Code      string                `json:"code"`
	Keyword   string                `json:"keyword"`
	Days      int                   `json:"days"`
}

type AStockMarginSummary struct {
	TradeDate            string    `json:"trade_date"`
	Market               string    `json:"market"`
	MarketLabel          string    `json:"market_label"`
	MarginBuyAmount      *float64  `json:"margin_buy_amount"`
	MarginBalance        *float64  `json:"margin_balance"`
	ShortSellVolume      *float64  `json:"short_sell_volume"`
	ShortBalanceVolume   *float64  `json:"short_balance_volume"`
	ShortBalanceAmount   *float64  `json:"short_balance_amount"`
	MarginTradingBalance *float64  `json:"margin_trading_balance"`
	SourceType           string    `json:"source_type"`
	RawPayload           string    `json:"raw_payload"`
	FetchedAt            time.Time `json:"fetched_at"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type AStockMarginDetail struct {
	TradeDate            string    `json:"trade_date"`
	Market               string    `json:"market"`
	MarketLabel          string    `json:"market_label"`
	Rank                 int       `json:"rank"`
	Code                 string    `json:"code"`
	Name                 string    `json:"name"`
	MarginBuyAmount      *float64  `json:"margin_buy_amount"`
	MarginBalance        *float64  `json:"margin_balance"`
	MarginRepayAmount    *float64  `json:"margin_repay_amount"`
	ShortSellVolume      *float64  `json:"short_sell_volume"`
	ShortBalanceVolume   *float64  `json:"short_balance_volume"`
	ShortRepayVolume     *float64  `json:"short_repay_volume"`
	ShortBalanceAmount   *float64  `json:"short_balance_amount"`
	MarginTradingBalance *float64  `json:"margin_trading_balance"`
	SourceType           string    `json:"source_type"`
	RawPayload           string    `json:"raw_payload"`
	FetchedAt            time.Time `json:"fetched_at"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type AStockMarginFilter struct {
	Date     string `json:"date"`
	Market   string `json:"market"`
	Keyword  string `json:"keyword"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

type AStockMarginListResult struct {
	Summaries  []AStockMarginSummary `json:"summaries"`
	Details    []AStockMarginDetail  `json:"details"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"page_size"`
	Total      int                   `json:"total"`
	Date       string                `json:"date"`
	LatestDate string                `json:"latest_date"`
	Market     string                `json:"market"`
	Keyword    string                `json:"keyword"`
	Dates      []string              `json:"dates"`
	Markets    []string              `json:"markets"`
	FetchedAt  *time.Time            `json:"fetched_at,omitempty"`
}

type AStockMarginUpsertResult struct {
	Date      string `json:"date"`
	Summaries int    `json:"summaries"`
	Details   int    `json:"details"`
	Inserted  int    `json:"inserted"`
	Updated   int    `json:"updated"`
	Total     int    `json:"total"`
}
