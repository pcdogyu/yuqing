package model

import "time"

type AStockAuctionAmount struct {
	TradeDate     string    `json:"trade_date"`
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
	Date      string `json:"date"`
	Keyword   string `json:"keyword"`
	Page      int    `json:"page"`
	PageSize  int    `json:"page_size"`
	TrendDays int    `json:"trend_days"`
}

type AStockAuctionListResult struct {
	Items        []AStockAuctionAmount `json:"items"`
	Page         int                   `json:"page"`
	PageSize     int                   `json:"page_size"`
	Total        int                   `json:"total"`
	Date         string                `json:"date"`
	Keyword      string                `json:"keyword"`
	LatestDate   string                `json:"latest_date"`
	Dates        []string              `json:"dates"`
	SummaryCount int                   `json:"summary_count"`
	TotalAmount  float64               `json:"total_amount"`
	MaxItem      *AStockAuctionAmount  `json:"max_item,omitempty"`
	FetchedAt    *time.Time            `json:"fetched_at,omitempty"`
	Trend        []AStockAuctionTrend  `json:"trend"`
}

type AStockAuctionUpsertResult struct {
	Date     string `json:"date"`
	Inserted int    `json:"inserted"`
	Updated  int    `json:"updated"`
	Total    int    `json:"total"`
}

type AStockAuctionTrend struct {
	Date         string                   `json:"date"`
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
	Found                    bool      `json:"found"`
	StrategyDate             string    `json:"strategy_date"`
	Period                   string    `json:"period"`
	IgnoreRecent             bool      `json:"ignore_recent"`
	RecommendationsJSON      string    `json:"recommendations_json"`
	BacktestsJSON            string    `json:"backtests_json"`
	BacktestStatus           string    `json:"backtest_status"`
	GeneratedCount           int       `json:"generated_count"`
	RecentFiltered           int       `json:"recent_filtered"`
	SameDayMorningFiltered   int       `json:"same_day_morning_filtered"`
	LimitUpFilterEnabled     bool      `json:"limit_up_filter_enabled"`
	LimitUpFiltered          int       `json:"limit_up_filtered"`
	TodayMarketFilterEnabled bool      `json:"today_market_filter_enabled"`
	NoTodayMarketCount       int       `json:"no_today_market_count"`
	MarketCandidateStatus    string    `json:"market_candidate_status"`
	MarketCandidateCount     int       `json:"market_candidate_count"`
	AuctionAmountLabel       string    `json:"auction_amount_label"`
	EmptyReason              string    `json:"empty_reason"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type AStockRecommendationSnapshotUpsertResult struct {
	Inserted int `json:"inserted"`
	Updated  int `json:"updated"`
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
