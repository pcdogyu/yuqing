package model

import (
	"errors"
	"time"
)

type AStockRecommendationAlgorithmSettings struct {
	Version    int                                  `json:"version"`
	Auction    AStockRecommendationAuctionFactor    `json:"auction"`
	Candidate  AStockRecommendationCandidateFactor  `json:"candidate"`
	Emotion    AStockRecommendationEmotionFactor    `json:"emotion"`
	Sector     AStockRecommendationSectorFactor     `json:"sector"`
	Fund       AStockRecommendationFundFactor       `json:"fund"`
	Volatility AStockRecommendationVolatilityFactor `json:"volatility"`
	UpdatedAt  time.Time                            `json:"updated_at"`
}

type AStockRecommendationAuctionFactor struct {
	FactorScoreCap              int     `json:"factor_score_cap"`
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
	LowOpenThreshold1Pct        float64 `json:"low_open_threshold_1_pct"`
	LowOpenThreshold2Pct        float64 `json:"low_open_threshold_2_pct"`
	LowOpenThreshold3Pct        float64 `json:"low_open_threshold_3_pct"`
	LowOpenThreshold4Pct        float64 `json:"low_open_threshold_4_pct"`
	LowOpenThreshold5Pct        float64 `json:"low_open_threshold_5_pct"`
	LowOpenStrongThresholdPct   float64 `json:"low_open_strong_threshold_pct"`
	LowOpenPenalty1             int     `json:"low_open_penalty_1"`
	LowOpenPenalty2             int     `json:"low_open_penalty_2"`
	LowOpenPenalty3             int     `json:"low_open_penalty_3"`
	LowOpenPenalty4             int     `json:"low_open_penalty_4"`
	LowOpenPenalty5             int     `json:"low_open_penalty_5"`
	LowOpenStrongPenalty        int     `json:"low_open_strong_penalty"`
	LowOpenPenaltyThresholdPct  float64 `json:"low_open_penalty_threshold_pct"`
	LowOpenPenalty              int     `json:"low_open_penalty"`
}

type AStockRecommendationCandidateFactor struct {
	RequireHotspotLink             bool `json:"require_hotspot_link"`
	AuctionFallbackPerHotspot      int  `json:"auction_fallback_per_hotspot"`
	SectorCandidateLimitPerHotspot int  `json:"sector_candidate_limit_per_hotspot"`
	StockFundFlowCandidateLimit    int  `json:"stock_fund_flow_candidate_limit"`
	MaxStocksPerHotspotSoft        int  `json:"max_stocks_per_hotspot_soft"`
}

type AStockRecommendationEmotionFactor struct {
	FactorScoreCap        int `json:"factor_score_cap"`
	NewsEvidenceScore     int `json:"news_evidence_score"`
	NewsSourceScore       int `json:"news_source_score"`
	KeywordScore          int `json:"keyword_score"`
	NegativeNewsPenalty   int `json:"negative_news_penalty"`
	HotspotMinScore       int `json:"hotspot_min_score"`
	HotspotDisplayLimit   int `json:"hotspot_display_limit"`
	StockEvidenceScore    int `json:"stock_evidence_score"`
	StockNameKeywordScore int `json:"stock_name_keyword_score"`
	WeakEvidencePenalty   int `json:"weak_evidence_penalty"`
}

type AStockRecommendationSectorFactor struct {
	FactorScoreCap                int     `json:"factor_score_cap"`
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
	FactorScoreCap           int     `json:"factor_score_cap"`
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
	FactorScoreCap              int     `json:"factor_score_cap"`
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
		Version: 5,
		Auction: AStockRecommendationAuctionFactor{
			FactorScoreCap:              120,
			RecommendationLimit:         5,
			ReplacementPoolLimit:        36,
			ReplacementPerHotspot:       12,
			HotspotTopStockLimit:        9,
			HotspotScoredCandidateLimit: 240,
			HotspotLimit:                3,
			MarketCandidateLimit:        5000,
			MarketRankScoreBase:         400,
			MarketRankScoreDivisor:      4,
			StocksPerHotspot:            3,
			HighOpenThreshold1Pct:       1.0,
			HighOpenThreshold2Pct:       2.0,
			HighOpenThreshold3Pct:       3.0,
			HighOpenThreshold4Pct:       4.0,
			HighOpenThreshold5Pct:       5.0,
			HighOpenStrongThresholdPct:  5.01,
			HighOpenScore1:              15,
			HighOpenScore2:              30,
			HighOpenScore3:              45,
			HighOpenScore4:              60,
			HighOpenScore5:              80,
			HighOpenStrongScore:         100,
			LowOpenThreshold1Pct:        1.0,
			LowOpenThreshold2Pct:        2.0,
			LowOpenThreshold3Pct:        3.0,
			LowOpenThreshold4Pct:        4.0,
			LowOpenThreshold5Pct:        5.0,
			LowOpenStrongThresholdPct:   5.01,
			LowOpenPenalty1:             10,
			LowOpenPenalty2:             20,
			LowOpenPenalty3:             30,
			LowOpenPenalty4:             40,
			LowOpenPenalty5:             50,
			LowOpenStrongPenalty:        65,
			LowOpenPenaltyThresholdPct:  -2.0,
			LowOpenPenalty:              60,
		},
		Candidate: AStockRecommendationCandidateFactor{
			RequireHotspotLink:             true,
			AuctionFallbackPerHotspot:      50,
			SectorCandidateLimitPerHotspot: 300,
			StockFundFlowCandidateLimit:    300,
			MaxStocksPerHotspotSoft:        2,
		},
		Emotion: AStockRecommendationEmotionFactor{
			FactorScoreCap:        300,
			NewsEvidenceScore:     12,
			NewsSourceScore:       80,
			KeywordScore:          4,
			NegativeNewsPenalty:   15,
			HotspotMinScore:       0,
			HotspotDisplayLimit:   8,
			StockEvidenceScore:    20,
			StockNameKeywordScore: 8,
			WeakEvidencePenalty:   25,
		},
		Sector: AStockRecommendationSectorFactor{
			FactorScoreCap:                220,
			TrendMinDays:                  3,
			TrendScoreMin:                 -120,
			TrendScoreMax:                 120,
			TrendTurnThreshold:            30000000,
			TrendContinuousInflowScore:    70,
			TrendPartialInflowScore:       45,
			TrendWeakInflowScore:          20,
			TrendContinuousOutflowPenalty: 90,
			TrendPartialOutflowPenalty:    60,
			TrendWeakOutflowPenalty:       25,
			TrendTenDayAccelerationScore:  25,
			TrendTenDayOutflowPenalty:     35,
			TrendRecent2DInflowScore:      20,
			TrendRecent2DOutflowPenalty:   30,
			TrendRankTop10Score:           25,
			TrendRankTop30Score:           12,
			TrendLatestOutflowDownPenalty: 30,
			TrendChoppyPenalty:            20,
			TopStockOneSectorScore:        25,
			TopStockTwoSectorScore:        45,
			TopStockThreeSectorScore:      65,
			TopStockLargeInflowThreshold:  5000000000,
			TopStockLargeInflowScore:      25,
			TopStockScoreCap:              90,
			TopStockOverheat30Cap:         20,
			TopStockEffectiveDate:         "2026-07-14",
			DrawdownPenalty:               40,
		},
		Fund: AStockRecommendationFundFactor{
			FactorScoreCap:           220,
			BonusThreshold:           30000000,
			StrongBonusThreshold:     100000000,
			VeryStrongThreshold:      500000000,
			ExtremeThreshold:         2000000000,
			BonusScore:               25,
			StrongBonusScore:         55,
			VeryStrongScore:          90,
			ExtremeScore:             120,
			NegativeDays2Penalty:     35,
			NegativeDays3Penalty:     70,
			NegativeDays4Penalty:     100,
			TenDayAccelerationRatio:  0.6,
			TenDayAccelerationScore:  25,
			TenDayRetreatPenalty:     70,
			TenDayRepairCap:          20,
			RecentTurnThreshold:      30000000,
			Recent2DInflowScore:      30,
			Recent2DOutflowPenalty:   50,
			LatestOutflowDownPenalty: 60,
			ScoreMin:                 -150,
			ScoreMax:                 150,
			SectorWeakCap:            20,
			SectorNetOutflowCap:      0,
			MedianPenalty:            50,
			Overheat30Cap:            20,
		},
		Volatility: AStockRecommendationVolatilityFactor{
			FactorScoreCap:              140,
			DrawdownFilterThreshold:     -15,
			Overheat30ThresholdPct:      30,
			Overheat60ThresholdPct:      60,
			PreviousLimitUpPenalty:      80,
			PreviousHighPctThreshold:    8,
			PreviousHighPctPenalty:      60,
			TodayHighPctFilterThreshold: 8,
			MomentumMinBars:             35,
			MomentumMaxScore:            100,
			MomentumADXScore:            30,
			MomentumBollingerScore:      25,
			MomentumMACDScore:           20,
			MomentumMAScore:             15,
			MomentumVolumeScore:         10,
			ExDividendWindowDays:        3,
		},
	}
}

func NormalizeAStockRecommendationAlgorithmSettings(settings AStockRecommendationAlgorithmSettings) AStockRecommendationAlgorithmSettings {
	defaults := DefaultAStockRecommendationAlgorithmSettings()
	loadedVersion := settings.Version
	if settings.Version <= 0 {
		settings.Version = defaults.Version
	}
	if loadedVersion > 0 && loadedVersion < defaults.Version {
		settings.Version = defaults.Version
		if loadedVersion < 4 {
			settings.Auction.FactorScoreCap = defaults.Auction.FactorScoreCap
			settings.Emotion.FactorScoreCap = defaults.Emotion.FactorScoreCap
			settings.Sector.FactorScoreCap = defaults.Sector.FactorScoreCap
			settings.Fund.FactorScoreCap = defaults.Fund.FactorScoreCap
			settings.Volatility.FactorScoreCap = defaults.Volatility.FactorScoreCap
		}
	}
	if settings.Auction.FactorScoreCap <= 0 {
		settings.Auction.FactorScoreCap = defaults.Auction.FactorScoreCap
	}
	if settings.Emotion.FactorScoreCap <= 0 {
		settings.Emotion.FactorScoreCap = defaults.Emotion.FactorScoreCap
	}
	if loadedVersion < 5 && settings.Emotion.NewsSourceScore == 0 {
		settings.Emotion.NewsSourceScore = defaults.Emotion.NewsSourceScore
	}
	if settings.Sector.FactorScoreCap <= 0 {
		settings.Sector.FactorScoreCap = defaults.Sector.FactorScoreCap
	}
	if settings.Fund.FactorScoreCap <= 0 {
		settings.Fund.FactorScoreCap = defaults.Fund.FactorScoreCap
	}
	if settings.Volatility.FactorScoreCap <= 0 {
		settings.Volatility.FactorScoreCap = defaults.Volatility.FactorScoreCap
	}
	if loadedVersion < defaults.Version || emptyAStockRecommendationCandidateFactor(settings.Candidate) {
		settings.Candidate.RequireHotspotLink = defaults.Candidate.RequireHotspotLink
	}
	if settings.Candidate.AuctionFallbackPerHotspot <= 0 {
		settings.Candidate.AuctionFallbackPerHotspot = defaults.Candidate.AuctionFallbackPerHotspot
	}
	if settings.Candidate.SectorCandidateLimitPerHotspot <= 0 {
		settings.Candidate.SectorCandidateLimitPerHotspot = defaults.Candidate.SectorCandidateLimitPerHotspot
	}
	if settings.Candidate.StockFundFlowCandidateLimit <= 0 {
		settings.Candidate.StockFundFlowCandidateLimit = defaults.Candidate.StockFundFlowCandidateLimit
	}
	if settings.Candidate.MaxStocksPerHotspotSoft <= 0 {
		settings.Candidate.MaxStocksPerHotspotSoft = defaults.Candidate.MaxStocksPerHotspotSoft
	}
	if settings.Auction.LowOpenThreshold1Pct <= 0 &&
		settings.Auction.LowOpenThreshold2Pct <= 0 &&
		settings.Auction.LowOpenThreshold3Pct <= 0 &&
		settings.Auction.LowOpenThreshold4Pct <= 0 &&
		settings.Auction.LowOpenThreshold5Pct <= 0 &&
		settings.Auction.LowOpenStrongThresholdPct <= 0 {
		settings.Auction.LowOpenThreshold1Pct = defaults.Auction.LowOpenThreshold1Pct
		settings.Auction.LowOpenThreshold2Pct = defaults.Auction.LowOpenThreshold2Pct
		settings.Auction.LowOpenThreshold3Pct = defaults.Auction.LowOpenThreshold3Pct
		settings.Auction.LowOpenThreshold4Pct = defaults.Auction.LowOpenThreshold4Pct
		settings.Auction.LowOpenThreshold5Pct = defaults.Auction.LowOpenThreshold5Pct
		settings.Auction.LowOpenStrongThresholdPct = defaults.Auction.LowOpenStrongThresholdPct
		settings.Auction.LowOpenPenalty1 = defaults.Auction.LowOpenPenalty1
		settings.Auction.LowOpenPenalty2 = defaults.Auction.LowOpenPenalty2
		settings.Auction.LowOpenPenalty3 = defaults.Auction.LowOpenPenalty3
		settings.Auction.LowOpenPenalty4 = defaults.Auction.LowOpenPenalty4
		settings.Auction.LowOpenPenalty5 = defaults.Auction.LowOpenPenalty5
		settings.Auction.LowOpenStrongPenalty = defaults.Auction.LowOpenStrongPenalty
	}
	return settings
}

func emptyAStockRecommendationCandidateFactor(candidate AStockRecommendationCandidateFactor) bool {
	return !candidate.RequireHotspotLink &&
		candidate.AuctionFallbackPerHotspot == 0 &&
		candidate.SectorCandidateLimitPerHotspot == 0 &&
		candidate.StockFundFlowCandidateLimit == 0 &&
		candidate.MaxStocksPerHotspotSoft == 0
}

func ValidateAStockRecommendationAlgorithmSettings(settings AStockRecommendationAlgorithmSettings) error {
	if settings.Version <= 0 {
		return errors.New("version required")
	}
	if settings.Auction.RecommendationLimit < 0 || settings.Auction.HotspotLimit < 0 || settings.Auction.StocksPerHotspot < 0 {
		return errors.New("recommendation and hotspot limits cannot be negative")
	}
	if settings.Candidate.AuctionFallbackPerHotspot < 0 ||
		settings.Candidate.SectorCandidateLimitPerHotspot < 0 ||
		settings.Candidate.StockFundFlowCandidateLimit < 0 ||
		settings.Candidate.MaxStocksPerHotspotSoft < 0 {
		return errors.New("candidate limits cannot be negative")
	}
	for name, cap := range map[string]int{
		"auction.factor_score_cap":    settings.Auction.FactorScoreCap,
		"emotion.factor_score_cap":    settings.Emotion.FactorScoreCap,
		"sector.factor_score_cap":     settings.Sector.FactorScoreCap,
		"fund.factor_score_cap":       settings.Fund.FactorScoreCap,
		"volatility.factor_score_cap": settings.Volatility.FactorScoreCap,
	} {
		if cap <= 0 {
			return errors.New(name + " must be greater than 0")
		}
		if cap > 1000 {
			return errors.New(name + " cannot exceed 1000")
		}
	}
	if settings.Auction.MarketRankScoreDivisor <= 0 {
		return errors.New("market_rank_score_divisor must be greater than 0")
	}
	if settings.Emotion.NewsSourceScore < 0 {
		return errors.New("emotion.news_source_score cannot be negative")
	}
	if settings.Emotion.NewsSourceScore > 1000 {
		return errors.New("emotion.news_source_score cannot exceed 1000")
	}
	lowOpenThresholds := []float64{
		settings.Auction.LowOpenThreshold1Pct,
		settings.Auction.LowOpenThreshold2Pct,
		settings.Auction.LowOpenThreshold3Pct,
		settings.Auction.LowOpenThreshold4Pct,
		settings.Auction.LowOpenThreshold5Pct,
		settings.Auction.LowOpenStrongThresholdPct,
	}
	for i, threshold := range lowOpenThresholds {
		if threshold <= 0 {
			return errors.New("low open thresholds must be greater than 0")
		}
		if i > 0 && threshold < lowOpenThresholds[i-1] {
			return errors.New("low open thresholds must be ordered")
		}
	}
	for _, penalty := range []int{
		settings.Auction.LowOpenPenalty1,
		settings.Auction.LowOpenPenalty2,
		settings.Auction.LowOpenPenalty3,
		settings.Auction.LowOpenPenalty4,
		settings.Auction.LowOpenPenalty5,
		settings.Auction.LowOpenStrongPenalty,
	} {
		if penalty < 0 {
			return errors.New("low open penalties cannot be negative")
		}
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
