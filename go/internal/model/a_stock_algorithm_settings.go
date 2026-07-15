package model

import (
	"errors"
	"time"
)

type AStockRecommendationAlgorithmSettings struct {
	Version    int                                  `json:"version"`
	Auction    AStockRecommendationAuctionFactor    `json:"auction"`
	Emotion    AStockRecommendationEmotionFactor    `json:"emotion"`
	Sector     AStockRecommendationSectorFactor     `json:"sector"`
	Fund       AStockRecommendationFundFactor       `json:"fund"`
	Volatility AStockRecommendationVolatilityFactor `json:"volatility"`
	UpdatedAt  time.Time                            `json:"updated_at"`
}

type AStockRecommendationAuctionFactor struct {
	RecommendationLimit         int     `json:"recommendation_limit"`
	ReplacementPoolLimit        int     `json:"replacement_pool_limit"`
	ReplacementPerHotspot       int     `json:"replacement_per_hotspot"`
	HotspotTopStockLimit        int     `json:"hotspot_top_stock_limit"`
	HotspotScoredCandidateLimit int     `json:"hotspot_scored_candidate_limit"`
	HotspotLimit                int     `json:"hotspot_limit"`
	MarketCandidateLimit        int     `json:"market_candidate_limit"`
	MarketRankScoreBase         int     `json:"market_rank_score_base"`
	MarketRankScoreDivisor      int     `json:"market_rank_score_divisor"`
	StocksPerHotspot            int     `json:"stocks_per_hotspot"`
	HighOpenThreshold1Pct       float64 `json:"high_open_threshold_1_pct"`
	HighOpenThreshold2Pct       float64 `json:"high_open_threshold_2_pct"`
	HighOpenThreshold3Pct       float64 `json:"high_open_threshold_3_pct"`
	HighOpenThreshold4Pct       float64 `json:"high_open_threshold_4_pct"`
	HighOpenThreshold5Pct       float64 `json:"high_open_threshold_5_pct"`
	HighOpenStrongThresholdPct  float64 `json:"high_open_strong_threshold_pct"`
	HighOpenScore1              int     `json:"high_open_score_1"`
	HighOpenScore2              int     `json:"high_open_score_2"`
	HighOpenScore3              int     `json:"high_open_score_3"`
	HighOpenScore4              int     `json:"high_open_score_4"`
	HighOpenScore5              int     `json:"high_open_score_5"`
	HighOpenStrongScore         int     `json:"high_open_strong_score"`
	LowOpenPenaltyThresholdPct  float64 `json:"low_open_penalty_threshold_pct"`
	LowOpenPenalty              int     `json:"low_open_penalty"`
}

type AStockRecommendationEmotionFactor struct {
	NewsEvidenceScore     int `json:"news_evidence_score"`
	KeywordScore          int `json:"keyword_score"`
	NegativeNewsPenalty   int `json:"negative_news_penalty"`
	HotspotMinScore       int `json:"hotspot_min_score"`
	HotspotDisplayLimit   int `json:"hotspot_display_limit"`
	StockEvidenceScore    int `json:"stock_evidence_score"`
	StockNameKeywordScore int `json:"stock_name_keyword_score"`
	WeakEvidencePenalty   int `json:"weak_evidence_penalty"`
}

type AStockRecommendationSectorFactor struct {
	TrendMinDays                  int     `json:"trend_min_days"`
	TrendScoreMin                 int     `json:"trend_score_min"`
	TrendScoreMax                 int     `json:"trend_score_max"`
	TrendTurnThreshold            float64 `json:"trend_turn_threshold"`
	TrendContinuousInflowScore    int     `json:"trend_continuous_inflow_score"`
	TrendPartialInflowScore       int     `json:"trend_partial_inflow_score"`
	TrendWeakInflowScore          int     `json:"trend_weak_inflow_score"`
	TrendContinuousOutflowPenalty int     `json:"trend_continuous_outflow_penalty"`
	TrendPartialOutflowPenalty    int     `json:"trend_partial_outflow_penalty"`
	TrendWeakOutflowPenalty       int     `json:"trend_weak_outflow_penalty"`
	TrendTenDayAccelerationScore  int     `json:"trend_ten_day_acceleration_score"`
	TrendTenDayOutflowPenalty     int     `json:"trend_ten_day_outflow_penalty"`
	TrendRecent2DInflowScore      int     `json:"trend_recent_2d_inflow_score"`
	TrendRecent2DOutflowPenalty   int     `json:"trend_recent_2d_outflow_penalty"`
	TrendRankTop10Score           int     `json:"trend_rank_top10_score"`
	TrendRankTop30Score           int     `json:"trend_rank_top30_score"`
	TrendLatestOutflowDownPenalty int     `json:"trend_latest_outflow_down_penalty"`
	TrendChoppyPenalty            int     `json:"trend_choppy_penalty"`
	TopStockOneSectorScore        int     `json:"top_stock_one_sector_score"`
	TopStockTwoSectorScore        int     `json:"top_stock_two_sector_score"`
	TopStockThreeSectorScore      int     `json:"top_stock_three_sector_score"`
	TopStockLargeInflowThreshold  float64 `json:"top_stock_large_inflow_threshold"`
	TopStockLargeInflowScore      int     `json:"top_stock_large_inflow_score"`
	TopStockScoreCap              int     `json:"top_stock_score_cap"`
	TopStockOverheat30Cap         int     `json:"top_stock_overheat_30_cap"`
	TopStockEffectiveDate         string  `json:"top_stock_effective_date"`
	DrawdownPenalty               int     `json:"drawdown_penalty"`
}

type AStockRecommendationFundFactor struct {
	BonusThreshold           float64 `json:"bonus_threshold"`
	StrongBonusThreshold     float64 `json:"strong_bonus_threshold"`
	VeryStrongThreshold      float64 `json:"very_strong_threshold"`
	ExtremeThreshold         float64 `json:"extreme_threshold"`
	BonusScore               int     `json:"bonus_score"`
	StrongBonusScore         int     `json:"strong_bonus_score"`
	VeryStrongScore          int     `json:"very_strong_score"`
	ExtremeScore             int     `json:"extreme_score"`
	NegativeDays2Penalty     int     `json:"negative_days_2_penalty"`
	NegativeDays3Penalty     int     `json:"negative_days_3_penalty"`
	NegativeDays4Penalty     int     `json:"negative_days_4_penalty"`
	TenDayAccelerationRatio  float64 `json:"ten_day_acceleration_ratio"`
	TenDayAccelerationScore  int     `json:"ten_day_acceleration_score"`
	TenDayRetreatPenalty     int     `json:"ten_day_retreat_penalty"`
	TenDayRepairCap          int     `json:"ten_day_repair_cap"`
	RecentTurnThreshold      float64 `json:"recent_turn_threshold"`
	Recent2DInflowScore      int     `json:"recent_2d_inflow_score"`
	Recent2DOutflowPenalty   int     `json:"recent_2d_outflow_penalty"`
	LatestOutflowDownPenalty int     `json:"latest_outflow_down_penalty"`
	ScoreMin                 int     `json:"score_min"`
	ScoreMax                 int     `json:"score_max"`
	SectorWeakCap            int     `json:"sector_weak_cap"`
	SectorNetOutflowCap      int     `json:"sector_net_outflow_cap"`
	MedianPenalty            int     `json:"median_penalty"`
	Overheat30Cap            int     `json:"overheat_30_cap"`
}

type AStockRecommendationVolatilityFactor struct {
	DrawdownFilterThreshold     float64 `json:"drawdown_filter_threshold"`
	Overheat30ThresholdPct      float64 `json:"overheat_30_threshold_pct"`
	Overheat60ThresholdPct      float64 `json:"overheat_60_threshold_pct"`
	PreviousLimitUpPenalty      int     `json:"previous_limit_up_penalty"`
	PreviousHighPctThreshold    float64 `json:"previous_high_pct_threshold"`
	PreviousHighPctPenalty      int     `json:"previous_high_pct_penalty"`
	TodayHighPctFilterThreshold float64 `json:"today_high_pct_filter_threshold"`
	MomentumMinBars             int     `json:"momentum_min_bars"`
	MomentumMaxScore            int     `json:"momentum_max_score"`
	MomentumADXScore            int     `json:"momentum_adx_score"`
	MomentumBollingerScore      int     `json:"momentum_bollinger_score"`
	MomentumMACDScore           int     `json:"momentum_macd_score"`
	MomentumMAScore             int     `json:"momentum_ma_score"`
	MomentumVolumeScore         int     `json:"momentum_volume_score"`
	ExDividendWindowDays        int     `json:"ex_dividend_window_days"`
}

func DefaultAStockRecommendationAlgorithmSettings() AStockRecommendationAlgorithmSettings {
	return AStockRecommendationAlgorithmSettings{
		Version: 1,
		Auction: AStockRecommendationAuctionFactor{
			RecommendationLimit:         5,
			ReplacementPoolLimit:        36,
			ReplacementPerHotspot:       12,
			HotspotTopStockLimit:        9,
			HotspotScoredCandidateLimit: 240,
			HotspotLimit:                3,
			MarketCandidateLimit:        5000,
			MarketRankScoreBase:         200,
			MarketRankScoreDivisor:      5,
			StocksPerHotspot:            3,
			HighOpenThreshold1Pct:       1.0,
			HighOpenThreshold2Pct:       2.0,
			HighOpenThreshold3Pct:       3.0,
			HighOpenThreshold4Pct:       4.0,
			HighOpenThreshold5Pct:       5.0,
			HighOpenStrongThresholdPct:  5.01,
			HighOpenScore1:              10,
			HighOpenScore2:              20,
			HighOpenScore3:              30,
			HighOpenScore4:              40,
			HighOpenScore5:              50,
			HighOpenStrongScore:         65,
			LowOpenPenaltyThresholdPct:  -2.0,
			LowOpenPenalty:              80,
		},
		Emotion: AStockRecommendationEmotionFactor{
			NewsEvidenceScore:     10,
			KeywordScore:          3,
			NegativeNewsPenalty:   30,
			HotspotMinScore:       1,
			HotspotDisplayLimit:   8,
			StockEvidenceScore:    25,
			StockNameKeywordScore: 12,
			WeakEvidencePenalty:   30,
		},
		Sector: AStockRecommendationSectorFactor{
			TrendMinDays:                  3,
			TrendScoreMin:                 -40,
			TrendScoreMax:                 40,
			TrendTurnThreshold:            30000000,
			TrendContinuousInflowScore:    25,
			TrendPartialInflowScore:       15,
			TrendWeakInflowScore:          8,
			TrendContinuousOutflowPenalty: 30,
			TrendPartialOutflowPenalty:    20,
			TrendWeakOutflowPenalty:       8,
			TrendTenDayAccelerationScore:  10,
			TrendTenDayOutflowPenalty:     10,
			TrendRecent2DInflowScore:      5,
			TrendRecent2DOutflowPenalty:   5,
			TrendRankTop10Score:           8,
			TrendRankTop30Score:           4,
			TrendLatestOutflowDownPenalty: 5,
			TrendChoppyPenalty:            5,
			TopStockOneSectorScore:        8,
			TopStockTwoSectorScore:        15,
			TopStockThreeSectorScore:      20,
			TopStockLargeInflowThreshold:  5000000000,
			TopStockLargeInflowScore:      5,
			TopStockScoreCap:              25,
			TopStockOverheat30Cap:         5,
			TopStockEffectiveDate:         "2026-07-14",
			DrawdownPenalty:               15,
		},
		Fund: AStockRecommendationFundFactor{
			BonusThreshold:           30000000,
			StrongBonusThreshold:     100000000,
			VeryStrongThreshold:      500000000,
			ExtremeThreshold:         2000000000,
			BonusScore:               10,
			StrongBonusScore:         25,
			VeryStrongScore:          40,
			ExtremeScore:             50,
			NegativeDays2Penalty:     20,
			NegativeDays3Penalty:     35,
			NegativeDays4Penalty:     50,
			TenDayAccelerationRatio:  0.6,
			TenDayAccelerationScore:  10,
			TenDayRetreatPenalty:     30,
			TenDayRepairCap:          10,
			RecentTurnThreshold:      30000000,
			Recent2DInflowScore:      10,
			Recent2DOutflowPenalty:   20,
			LatestOutflowDownPenalty: 20,
			ScoreMin:                 -50,
			ScoreMax:                 50,
			SectorWeakCap:            10,
			SectorNetOutflowCap:      0,
			MedianPenalty:            30,
			Overheat30Cap:            10,
		},
		Volatility: AStockRecommendationVolatilityFactor{
			DrawdownFilterThreshold:     -15,
			Overheat30ThresholdPct:      30,
			Overheat60ThresholdPct:      60,
			PreviousLimitUpPenalty:      50,
			PreviousHighPctThreshold:    8,
			PreviousHighPctPenalty:      40,
			TodayHighPctFilterThreshold: 8,
			MomentumMinBars:             35,
			MomentumMaxScore:            60,
			MomentumADXScore:            25,
			MomentumBollingerScore:      20,
			MomentumMACDScore:           15,
			MomentumMAScore:             10,
			MomentumVolumeScore:         10,
			ExDividendWindowDays:        3,
		},
	}
}

func ValidateAStockRecommendationAlgorithmSettings(settings AStockRecommendationAlgorithmSettings) error {
	if settings.Version <= 0 {
		return errors.New("version required")
	}
	if settings.Auction.RecommendationLimit < 0 || settings.Auction.HotspotLimit < 0 || settings.Auction.StocksPerHotspot < 0 {
		return errors.New("recommendation and hotspot limits cannot be negative")
	}
	if settings.Auction.MarketRankScoreDivisor <= 0 {
		return errors.New("market_rank_score_divisor must be greater than 0")
	}
	if settings.Fund.ScoreMin > settings.Fund.ScoreMax {
		return errors.New("fund score_min cannot be greater than score_max")
	}
	if settings.Sector.TrendScoreMin > settings.Sector.TrendScoreMax {
		return errors.New("sector trend score_min cannot be greater than score_max")
	}
	if settings.Volatility.MomentumMinBars < 0 || settings.Volatility.ExDividendWindowDays < 0 {
		return errors.New("days and bars cannot be negative")
	}
	return nil
}
