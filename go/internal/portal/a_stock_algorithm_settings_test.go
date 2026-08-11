package portal

import (
	"testing"

	"github.com/pcdogyu/yuqing/go/internal/model"
)

func TestAStockAlgorithmSettingsForRecommendationPeriodCapitalMomentum(t *testing.T) {
	base := defaultAStockAlgorithmSettings()
	for _, period := range []string{"morning", "afternoon"} {
		t.Run(period, func(t *testing.T) {
			settings := aStockAlgorithmSettingsForRecommendationPeriod(period, base)
			if settings.Auction.FactorScoreCap != 180 {
				t.Fatalf("expected auction factor cap 180, got %d", settings.Auction.FactorScoreCap)
			}
			if settings.Auction.MarketRankScoreBase != 260 || settings.Auction.MarketRankScoreDivisor != 2 {
				t.Fatalf("unexpected auction rank settings: base=%d divisor=%d", settings.Auction.MarketRankScoreBase, settings.Auction.MarketRankScoreDivisor)
			}
			if settings.Fund.FactorScoreCap != 280 || settings.Fund.StrongBonusScore != 75 || settings.Fund.VeryStrongScore != 120 || settings.Fund.ExtremeScore != 160 || settings.Fund.Recent2DInflowScore != 45 {
				t.Fatalf("unexpected fund settings: %+v", settings.Fund)
			}
			if settings.Sector.FactorScoreCap != 260 || settings.Sector.TrendContinuousInflowScore != 90 {
				t.Fatalf("unexpected sector settings: %+v", settings.Sector)
			}
			if settings.Volatility.PreviousLimitUpPenalty != 40 || settings.Volatility.PreviousHighPctPenalty != 35 {
				t.Fatalf("unexpected volatility settings: %+v", settings.Volatility)
			}
			if settings.Emotion.NewsEvidenceScore != base.Emotion.NewsEvidenceScore || settings.Emotion.KeywordScore != base.Emotion.KeywordScore || settings.Emotion.NegativeNewsPenalty != base.Emotion.NegativeNewsPenalty || settings.Emotion.NewsSourceScore != 40 || settings.Emotion.StockEvidenceScore != 15 || settings.Emotion.StockNameKeywordScore != 6 {
				t.Fatalf("unexpected emotion settings: %+v", settings.Emotion)
			}
		})
	}
}

func TestAStockAlgorithmSettingsForRecommendationPeriodKeepsEveningBase(t *testing.T) {
	base := defaultAStockAlgorithmSettings()
	settings := aStockAlgorithmSettingsForRecommendationPeriod("evening", base)

	if settings.Auction.FactorScoreCap != base.Auction.FactorScoreCap ||
		settings.Auction.MarketRankScoreBase != base.Auction.MarketRankScoreBase ||
		settings.Auction.MarketRankScoreDivisor != base.Auction.MarketRankScoreDivisor {
		t.Fatalf("expected evening auction settings to remain unchanged: got %+v want %+v", settings.Auction, base.Auction)
	}
	if settings.Volatility.PreviousLimitUpPenalty != base.Volatility.PreviousLimitUpPenalty ||
		settings.Volatility.PreviousHighPctPenalty != base.Volatility.PreviousHighPctPenalty {
		t.Fatalf("expected evening volatility settings to remain unchanged: got %+v want %+v", settings.Volatility, base.Volatility)
	}
	if settings.Emotion.NewsEvidenceScore != base.Emotion.NewsEvidenceScore ||
		settings.Emotion.NewsSourceScore != base.Emotion.NewsSourceScore ||
		settings.Emotion.StockEvidenceScore != base.Emotion.StockEvidenceScore ||
		settings.Emotion.StockNameKeywordScore != base.Emotion.StockNameKeywordScore {
		t.Fatalf("expected evening emotion settings to remain unchanged: got %+v want %+v", settings.Emotion, base.Emotion)
	}
}

func TestAStockRecommendationScoreBreakdownUsesNewsSourceScore(t *testing.T) {
	settings := defaultAStockAlgorithmSettings()
	settings.Emotion.NewsSourceScore = 42

	components := buildAStockRecommendationScoreBreakdownWithSettings(aStockHotspot{
		Name:     "半导体",
		Keywords: []string{"芯片"},
		Evidence: 1,
	}, aStockMarketCandidate{
		Code:    "300999",
		Name:    "新闻点名",
		Rank:    1,
		Sources: []string{aStockCandidateSourceNews},
	}, settings)
	component, ok := findAStockScoreComponentByLabel(components, "候选来源")
	if !ok {
		t.Fatalf("expected news source component, got %+v", components)
	}
	if component.UnitValue != 42 || component.Score != 42 {
		t.Fatalf("expected news source score 42, got %+v", component)
	}
}

func TestAStockCapitalMomentumSettingsFavorAuctionFundCandidate(t *testing.T) {
	settings := aStockAlgorithmSettingsForRecommendationPeriod("morning", defaultAStockAlgorithmSettings())
	hotspot := aStockHotspot{
		Name:     "半导体",
		Keywords: []string{"芯片"},
		MatchedItems: []model.Item{{
			Title:    "半导体盘前消息，弱新闻获点名",
			Summary:  "芯片产业链关注度升温",
			TagFlags: "0.300999",
		}},
	}
	candidates := []aStockMarketCandidate{
		{
			Code:            "300001",
			Name:            "芯片强势",
			Rank:            1,
			AuctionAmount:   120000000,
			Sources:         []string{aStockCandidateSourceAuction, aStockCandidateSourceFund},
			PreFundScore:    settings.Fund.ExtremeScore,
			PreSectorScore:  settings.Sector.TrendContinuousInflowScore,
			PreFundDetail:   "5日主力资金净流入",
			PreSectorDetail: "板块连续净流入",
		},
		{
			Code:          "300999",
			Name:          "弱新闻",
			Rank:          5000,
			AuctionAmount: 1000000,
			Sources:       []string{aStockCandidateSourceNews},
		},
	}

	scored := scoreAStockMarketCandidatesWithSettings(hotspot, candidates, nil, settings)
	if len(scored) != 2 {
		t.Fatalf("expected two scored candidates, got %+v", scored)
	}
	if scored[0].Code != "300001" {
		t.Fatalf("expected auction/fund candidate first, got %+v", scored)
	}
	if scored[0].MatchedScore <= scored[1].MatchedScore {
		t.Fatalf("expected auction/fund score to beat news score, got strong=%d news=%d", scored[0].MatchedScore, scored[1].MatchedScore)
	}

	rec := applyAStockPreviousLimitUpPenaltyWithSettings(aStockRecommendation{}, 10, settings)
	component, ok := findAStockScoreComponentByLabel(rec.ScoreBreakdown, "昨日涨停")
	if !ok {
		t.Fatalf("expected previous limit-up component, got %+v", rec.ScoreBreakdown)
	}
	if component.Score != -40 {
		t.Fatalf("expected morning previous limit-up penalty -40, got %+v", component)
	}
}

func findAStockScoreComponentByLabel(components []aStockRecommendationScoreComponent, label string) (aStockRecommendationScoreComponent, bool) {
	for _, component := range components {
		if component.Label == label {
			return component, true
		}
	}
	return aStockRecommendationScoreComponent{}, false
}
