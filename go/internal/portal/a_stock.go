package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pcdogyu/yuqing/go/internal/astockcalendar"
	"github.com/pcdogyu/yuqing/go/internal/astockcode"
	"github.com/pcdogyu/yuqing/go/internal/astocknews"
	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
)

type aStockContext struct {
	Date                         string
	Period                       string
	PeriodLabel                  string
	WindowLabel                  string
	NewsPage                     int
	NewsPageSize                 int
	NewsTotal                    int
	NewsTotalPages               int
	NewsWindowStart              time.Time
	NewsWindowEnd                time.Time
	WindowStart                  time.Time
	WindowEnd                    time.Time
	RecommendationWindowLabel    string
	RecommendationNewsTotal      int
	Articles                     []model.Item
	NewsArticles                 []model.Item
	PagedArticles                []model.Item
	Hotspots                     []aStockHotspot
	Recommendations              []aStockRecommendation
	FilteredRecommendations      []aStockFilteredRecommendation
	Backtests                    []aStockBacktestRow
	LoadMessage                  string
	BacktestStatus               string
	EmptyReason                  string
	RecentFiltered               int
	RecentReplenished            int
	RecentReplenishShortfall     bool
	IgnoreRecent                 bool
	IgnoreLimitUp                bool
	LimitUpFilterEnabled         bool
	LimitUpFiltered              int
	IgnoreFundFlow               bool
	FundFlowFilterExplicit       bool
	FundFlowFilterEnabled        bool
	FundFlowFiltered             int
	FundFlowMissingCount         int
	ExDividendFiltered           int
	NegativeNoEvidenceFiltered   int
	TodayMarketFilterEnabled     bool
	NoTodayMarketCount           int
	MarketCandidateStatus        string
	MarketCandidateCount         int
	GeneratedRecommendationCount int
	RecoveredCount               int
	AuctionAmountLabel           string
	SameDayMorningFiltered       int
	TradingDayBlocked            bool
	TradingDayMessage            string
	TradingDayReason             string
	SnapshotUpdatedAt            time.Time
	SourceRuns                   []aStockSourceRun
	FastReadOnly                 bool
}

type aStockSnapshotNewsSummary struct {
	Articles     []model.Item    `json:"articles"`
	NewsArticles []model.Item    `json:"news_articles"`
	Hotspots     []aStockHotspot `json:"hotspots"`
}

type aStockHotspot struct {
	Name                string
	Keywords            []string
	Score               int
	Evidence            int
	NegativeNewsCount   int
	NegativeNewsPenalty int
	MatchedItems        []model.Item
	TopStocks           []aStockHotspotStock
}

type aStockHotspotStock struct {
	Rank               int
	Code               string
	Name               string
	Score              int
	RecommendationDate string
}

type aStockRecommendation struct {
	Rank            int
	Hotspot         string
	Code            string
	Name            string
	HotspotScore    int
	MarketScore     int
	PrevClose       string
	PrevPct         string
	PrevPctClass    string
	Change30        string
	Change30Class   string
	Change60        string
	Change60Class   string
	CurrentPrice    string
	TodayPct        string
	TodayPctClass   string
	FundFlow5D      string
	FundFlow5DClass string
	HoldingSummary  string
	HoldingRatio    string
	Reason          string
	ScoreBreakdown  []aStockRecommendationScoreComponent `json:",omitempty"`
	EntryTime       string
	Recovered       bool   `json:",omitempty"`
	RecoveryReason  string `json:",omitempty"`
}

type aStockFilteredRecommendation struct {
	Reason         string               `json:"reason"`
	Recommendation aStockRecommendation `json:"recommendation"`
}

type aStockRecommendationScoreComponent struct {
	Factor    string `json:",omitempty"`
	Label     string
	Detail    string
	UnitValue int `json:",omitempty"`
	Score     int
}

const (
	aStockScoreFactorAuction    = "auction"
	aStockScoreFactorEmotion    = "emotion"
	aStockScoreFactorSector     = "sector"
	aStockScoreFactorFund       = "fund"
	aStockScoreFactorVolatility = "volatility"
	aStockScoreFactorHistory    = "history"
)

const (
	aStockFilteredReasonTodayHighPct = "today_high_pct"
	aStockFilteredReasonLimitUp      = "limit_up"
)

type aStockMarketBar struct {
	Code                string
	Date                string
	Open                float64
	High                float64
	Low                 float64
	Close               float64
	Pct                 float64
	Volume              float64
	Amount              float64
	EntryPrice          float64
	AfternoonEntryPrice float64
	SessionPrices       map[string]float64
}

type aStockMarketViewResult struct {
	Recommendations         []aStockRecommendation
	Backtests               []aStockBacktestRow
	Status                  string
	LimitUpFiltered         int
	NoTodayMarketCount      int
	FilteredRecommendations []aStockFilteredRecommendation
}

type aStockBacktestCell struct {
	Close          string
	Return         string
	ReturnClass    string
	MarketPct      string
	MarketPctClass string
}

type aStockBacktestRow struct {
	Stock                 string
	EntryOpen             string
	AfternoonOpen         string
	T0Return              string
	T0Close               string
	T0ReturnClass         string
	CurrentPrice          string
	CurrentReturn         string
	CurrentReturnClass    string
	CurrentMarketPct      string
	CurrentMarketPctClass string
	Days                  []aStockBacktestCell
	BestReturn            string
	BestReturnClass       string
	Status                string
}

type aStockTopicRule struct {
	Name     string
	Keywords []string
}

type aStockMarketCandidate struct {
	Code              string
	Name              string
	TradeDate         string
	Rank              int
	AuctionAmount     float64
	AuctionVolume     float64
	AuctionAmount0920 float64
	AuctionAmount0925 float64
	AuctionAmount0929 float64
	AuctionVolume0920 float64
	AuctionVolume0925 float64
	AuctionVolume0929 float64
	MatchedScore      int
	Evidence          int
	StrongEvidence    int
	WeakEvidence      int
	WeakPenalty       int
	BadEvidence       int
	Keywords          []string
	Fallback          bool
	Sources           []string
	PreFundScore      int
	PreFundDetail     string
	PreSectorScore    int
	PreSectorDetail   string
}

type aStockEveningCandidate struct {
	TradeDate   string    `json:"trade_date"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Price       float64   `json:"price"`
	ChangePct   float64   `json:"change_pct"`
	VolumeRatio float64   `json:"volume_ratio"`
	TurnoverPct float64   `json:"turnover_pct"`
	Amount      float64   `json:"amount"`
	Speed       float64   `json:"speed"`
	Source      string    `json:"source"`
	Status      string    `json:"status"`
	FetchedAt   time.Time `json:"fetched_at"`
}

type aStockEveningSnapshotPayload struct {
	Date    string                   `json:"date"`
	Items   []aStockEveningCandidate `json:"items"`
	Count   int                      `json:"count"`
	OK      int                      `json:"ok"`
	Warning string                   `json:"warning"`
	Message string                   `json:"message"`
}

type aStockDividendEvent struct {
	Code         string `json:"code"`
	Name         string `json:"name"`
	ExDate       string `json:"ex_date"`
	DividendDate string `json:"dividend_date"`
	RecordDate   string `json:"record_date"`
	Description  string `json:"description"`
	Source       string `json:"source"`
}

type aStockDividendEventResult struct {
	Items     []aStockDividendEvent `json:"items"`
	Count     int                   `json:"count"`
	Date      string                `json:"date"`
	Start     string                `json:"start"`
	End       string                `json:"end"`
	Warning   string                `json:"warning"`
	FetchedAt string                `json:"fetched_at"`
}

type aStockHotspotSectorGate struct {
	codesByHotspot map[string]map[string]struct{}
	gatedHotspots  map[string]struct{}
}

type aStockHotspotSectorAlias struct {
	SectorType string
	SectorName string
}

type aStockHotspotTopStockCandidateSet struct {
	scored   []aStockMarketCandidate
	fallback []aStockMarketCandidate
}

type aStockTradingDayStatus struct {
	Date               string `json:"date"`
	IsTradingDay       bool   `json:"is_trading_day"`
	LatestTradingDay   string `json:"latest_trading_day"`
	PreviousTradingDay string `json:"previous_trading_day"`
	NextTradingDay     string `json:"next_trading_day"`
	Source             string `json:"source"`
	Reason             string `json:"reason"`
	Message            string `json:"message"`
}

type aStockRequestCache struct {
	tradingDay                map[string]aStockTradingDayCacheEntry
	recentCodes               map[string]map[string]struct{}
	latestRecommendationDates map[string]map[string]string
	articles                  map[string]aStockArticlesCacheEntry
	snapshots                 map[string]aStockRecommendationSnapshotCacheEntry
	selections                map[string]aStockRecommendationSelectionCacheEntry
	holdingSummaries          map[string]aStockHoldingSummaryCacheEntry
	stockFundFlowTrends       map[string]aStockStockFundFlowTrendCacheEntry
	stockFundFlows            map[string]aStockStockFundFlowListCacheEntry
	sectorFundFlows           map[string]aStockSectorFundFlowCacheEntry
	sectorFundFlowTrends      map[string]aStockSectorFundFlowTrendCacheEntry
	dividendEvents            map[string]aStockDividendEventCacheEntry
	auctionResults            map[string]aStockAuctionResultCacheEntry
	sectorConstituents        map[string]aStockSectorConstituentCodesCacheEntry
	codeNames                 map[string]map[string]string
	sourceRuns                []aStockSourceRun
	algorithmSettings         model.AStockRecommendationAlgorithmSettings
	algorithmSettingsLoaded   bool
}

type aStockTradingDayCacheEntry struct {
	status aStockTradingDayStatus
	err    error
}

type aStockArticlesCacheEntry struct {
	items []model.Item
	err   error
}

type aStockRecommendationSnapshotCacheEntry struct {
	snapshot model.AStockRecommendationSnapshot
	found    bool
}

type aStockRecommendationSelectionCacheEntry struct {
	result model.AStockRecommendationSelectionListResult
	found  bool
}

type aStockHoldingSummaryCacheEntry struct {
	summary model.StockInstitutionHoldingSummary
	err     error
}

type aStockStockFundFlowTrendCacheEntry struct {
	result model.AStockStockFundFlowTrendResult
	err    error
}

type aStockStockFundFlowListCacheEntry struct {
	result model.AStockStockFundFlowListResult
	err    error
}

type aStockSectorFundFlowCacheEntry struct {
	result model.AStockSectorFundFlowListResult
	err    error
}

type aStockSectorFundFlowTrendCacheEntry struct {
	result model.AStockSectorFundFlowTrendResult
	err    error
}

type aStockDividendEventCacheEntry struct {
	result aStockDividendEventResult
	err    error
}

type aStockAuctionResultCacheEntry struct {
	result model.AStockAuctionListResult
	found  bool
}

type aStockSectorConstituentCodesCacheEntry struct {
	codes map[string]struct{}
	names map[string]string
	found bool
}

type aStockServerAuctionCacheEntry struct {
	result    model.AStockAuctionListResult
	found     bool
	expiresAt time.Time
}

type aStockServerArticlesCacheEntry struct {
	items     []model.Item
	err       error
	date      string
	expiresAt time.Time
}

type aStockServerFragmentCacheEntry struct {
	payload   aStockPartialPayload
	expiresAt time.Time
}

type aStockServerContextCacheEntry struct {
	ctx       aStockContext
	expiresAt time.Time
}

type aStockPartialPayload struct {
	HTML         string `json:"html"`
	CanonicalURL string `json:"canonical_url"`
	Date         string `json:"date"`
	Period       string `json:"period"`
	Message      string `json:"message"`
}

type aStockPeriod struct {
	Key         string
	Label       string
	WindowLabel string
}

type aStockNewsSourceCount struct {
	SourceType            string
	Label                 string
	URL                   string
	Count                 int
	Run                   aStockSourceRun
	CrawlEnabled          bool
	RecommendationEnabled bool
}

type aStockSourceRun struct {
	SourceType    string
	Status        string
	FetchedCount  int
	InsertedCount int
	UpdatedCount  int
	ErrorText     string
	StartedAt     time.Time
}

type aStockRecommendationGenerateResult struct {
	StrategyDate             string `json:"strategy_date"`
	Period                   string `json:"period"`
	Phase                    string `json:"phase"`
	DryRun                   bool   `json:"dry_run"`
	SkipShadow               bool   `json:"skip_shadow"`
	RecommendationCount      int    `json:"recommendation_count"`
	GeneratedCount           int    `json:"generated_count"`
	BacktestStatus           string `json:"backtest_status"`
	RecentFiltered           int    `json:"recent_filtered"`
	RecentLookbackDays       int    `json:"recent_lookback_days"`
	SameDayMorningFiltered   int    `json:"same_day_morning_filtered"`
	LimitUpFiltered          int    `json:"limit_up_filtered"`
	RecoveredCount           int    `json:"recovered_count"`
	TodayMarketFilterEnabled bool   `json:"today_market_filter_enabled"`
	NoTodayMarketCount       int    `json:"no_today_market_count"`
	FundFlowFilterEnabled    bool   `json:"fund_flow_filter_enabled"`
	FundFlowFiltered         int    `json:"fund_flow_filtered"`
	FundFlowMissingCount     int    `json:"fund_flow_missing_count"`
	LoadMessage              string `json:"load_message"`
}

type aStockRecommendationRefreshMode string

const (
	aStockRecommendationPreserveLocked aStockRecommendationRefreshMode = "preserve_locked"
	aStockRecommendationRebuild        aStockRecommendationRefreshMode = "rebuild"
)

const (
	aStockCandidateSourceNews    = "新闻点名"
	aStockCandidateSourceSector  = "热点板块成分股"
	aStockCandidateSourceFund    = "个股资金流入池"
	aStockCandidateSourceAuction = "集合竞价确认"
)

type aStockPopupRecommendation struct {
	Rank    int    `json:"rank"`
	Code    string `json:"code"`
	Name    string `json:"name"`
	Hotspot string `json:"hotspot"`
	Reason  string `json:"reason"`
}

type aStockPopupPayload struct {
	Show            bool                        `json:"show"`
	Key             string                      `json:"key"`
	Title           string                      `json:"title"`
	Meta            string                      `json:"meta"`
	StrategyDate    string                      `json:"strategy_date"`
	Period          string                      `json:"period"`
	UpdatedAt       string                      `json:"updated_at"`
	Recommendations []aStockPopupRecommendation `json:"recommendations"`
}

const (
	aStockDrawdownFilterThreshold                = -15.0
	aStockSectorDrawdownPenalty                  = 15
	aStockFundFlowBonusThreshold                 = 30000000.0
	aStockFundFlowStrongBonusThreshold           = 100000000.0
	aStockFundFlowVeryStrongThreshold            = 500000000.0
	aStockFundFlowExtremeThreshold               = 2000000000.0
	aStockFundFlowScoreMin                       = -50
	aStockFundFlowScoreMax                       = 50
	aStockFundFlowRecentTurnThreshold            = 30000000.0
	aStockSectorFundTrendMinDays                 = 3
	aStockSectorFundTrendScoreMin                = -40
	aStockSectorFundTrendScoreMax                = 40
	aStockSectorFundTrendTurnThreshold           = 30000000.0
	aStockSectorTopStockResonanceInflowThreshold = 5000000000.0
	aStockSectorTopStockResonanceScoreCap        = 25
	aStockSectorTopStockResonanceOverheat30Cap   = 5
	aStockSectorTopStockResonanceEffectiveDate   = "2026-07-14"
	aStockOverheat30ThresholdPct                 = 30.0
	aStockOverheat60ThresholdPct                 = 60.0
	aStockNegativeNewsPenalty                    = 30
	aStockPrevLimitUpPenalty                     = 50
	aStockPrevHighPctThreshold                   = 8.0
	aStockPrevHighPctPenalty                     = 40
	aStockTodayHighPctFilterThreshold            = 8.0
	aStockHighOpenThreshold1Pct                  = 1.0
	aStockHighOpenThreshold2Pct                  = 2.0
	aStockHighOpenThreshold3Pct                  = 3.0
	aStockHighOpenThreshold4Pct                  = 4.0
	aStockHighOpenThreshold5Pct                  = 5.0
	aStockHighOpenStrongThresholdPct             = 5.01
	aStockMomentumMinBars                        = 35
	aStockMomentumMaxScore                       = 60
	aStockMomentumADXScore                       = 25
	aStockMomentumBollingerScore                 = 20
	aStockMomentumMACDScore                      = 15
	aStockMomentumMAScore                        = 10
	aStockMomentumVolumeScore                    = 10
	aStockLowOpenPenaltyThresholdPct             = -2.0
	aStockLowOpenPenalty                         = 80
	aStockWeakEvidencePenalty                    = 30
	aStockFundFlowMedianPenalty                  = 30
	aStockNewsPageSize                           = 10
	aStockArticleFetchPageSize                   = 1000
	aStockArticleFetchMaxPages                   = 100
	aStockRecentLookbackDays                     = 90
	aStockFundFlowFilterCookieName               = "yuqing_astock_fund_flow_filter"
	aStockFundFlowFilterCookieEnabled            = "enabled"
	aStockFundFlowFilterCookieDisabled           = "disabled"
	aStockAuctionCandidateCacheTTL               = 5 * time.Minute
	aStockMarketCandidateLimit                   = 5000
	aStockDailyRecommendationLimit               = 5
	aStockRecommendationLimit                    = aStockDailyRecommendationLimit
	aStockEveningRecommendationLimit             = aStockDailyRecommendationLimit
	aStockEveningHotspot                         = "晚间量价筛选"
	aStockEveningEmptyReason                     = "晚间量价筛选无符合条件股票"
	aStockReplacementPoolLimit                   = 36
	aStockReplacementPerHotspot                  = 12
	aStockHotspotTopStockLimit                   = 9
	aStockHotspotScoredCandidateLimit            = 240
	aStockHotspotLimit                           = 3
	aStockMarketRankScoreBase                    = 200
	aStockStocksPerHotspot                       = 3
	aStockExDividendWindowDays                   = 3
	aStockT1ShadowStrategyKey                    = "t1_shadow_v1"
	aStockT1ShadowRecommendationLimit            = aStockDailyRecommendationLimit
	aStockT1ShadowStocksPerHotspot               = 2
	aStockT1ShadowDrawdownThreshold              = -10.0
	aStockAuctionStrengthStrategyKey             = "auction_strength_v1"
	aStockAuctionStrengthRecommendationLimit     = aStockDailyRecommendationLimit
	aStockAuctionStrengthStocksPerHotspot        = 2
	aStockAuctionStrengthFadeRatio               = 0.70
	aStockRecommendationPhasePreopen             = "preopen"
	aStockRecommendationPhaseFinal               = "final"
	aStockRealtimeQuoteCacheTTL                  = time.Minute
)

var (
	aStockNow               = time.Now
	aStockEastmoneyKlineURL = "https://push2his.eastmoney.com/api/qt/stock/kline/get"
	aStockEastmoneyQuoteURL = "https://push2.eastmoney.com/api/qt/stock/get"
	aStockTencentMinuteURL  = "https://web.ifzq.gtimg.cn/appstock/app/minute/query"
	aStockSinaMinuteURL     = "https://quotes.sina.cn/cn/api/jsonp_v2.php/=/CN_MarketDataService.getKLineData"
	aStockYahooChartURL     = "https://query1.finance.yahoo.com/v8/finance/chart/"
)

func (s *Server) handleAStockPage(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method == http.MethodPost {
		s.handleAStockPageAction(w, r, "/a-stock")
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	strategyDate := normalizeAStockStrategyDate(r.URL.Query().Get("date"))
	period := normalizeAStockPeriod(r.URL.Query().Get("period"))
	newsPage := normalizeAStockNewsPage(r.URL.Query().Get("news_page"))
	ignoreRecent := normalizeAStockIgnoreRecent(r.URL.Query())
	ignoreLimitUp := normalizeAStockIgnoreLimitUp(r.URL.Query())
	ignoreFundFlow, fundFlowExplicit := normalizeAStockIgnoreFundFlowFromRequest(r, strategyDate)
	setAStockFundFlowFilterCookie(w, ignoreFundFlow, fundFlowExplicit)
	filterTodayMarket := normalizeAStockFilterTodayMarket(r.URL.Query())
	forceRecommendationRefresh := normalizeAStockBool(r.URL.Query().Get("refresh_recommendations"))
	refreshAllBacktests := normalizeAStockBool(r.URL.Query().Get("refresh_all_backtests"))
	if refreshAllBacktests {
		forceRecommendationRefresh = true
	}
	partial := normalizeAStockBool(r.URL.Query().Get("partial"))
	cacheable := isAStockPageFragmentCacheable(r.URL.Query(), forceRecommendationRefresh)
	fragmentKey := aStockPageFragmentCacheKey(strategyDate, period.Key, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
	var payload aStockPartialPayload
	if cacheable {
		if cached, ok := s.loadCachedAStockPageFragment(fragmentKey); ok {
			payload = cached
		}
	}
	if payload.HTML == "" {
		payload = s.buildAStockPageFragment(strategyDate, period.Key, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, fundFlowExplicit, filterTodayMarket, forceRecommendationRefresh, strings.TrimSpace(r.URL.Query().Get("msg")))
		if cacheable && strings.TrimSpace(payload.Message) == "" {
			s.storeCachedAStockPageFragment(fragmentKey, payload)
		}
	}
	if partial {
		writeRawJSON(w, http.StatusOK, payload)
		return
	}

	var b strings.Builder
	b.WriteString(`<style>
		body[data-page='a-stock'] main{max-width:none;width:100%;box-sizing:border-box}
		body[data-page='a-stock'] .site-footer{max-width:none;width:100%;box-sizing:border-box}
		body[data-page='a-stock'] section{width:100%;box-sizing:border-box}
		body[data-page='a-stock'] table{width:100%;min-width:100%;font-size:13px}
		.astock-card{padding:18px;border:1px solid #ece7dc;border-radius:14px;background:#fff}
		.astock-overview-header{display:flex;align-items:flex-start;justify-content:space-between;gap:24px;margin-bottom:14px}
		.astock-overview-header h2{margin:0}
		.astock-overview-summary{display:flex;justify-content:flex-start;gap:24px;flex-wrap:wrap;text-align:left;font-size:12px}
		.astock-overview-summary .astock-muted{display:block;margin-bottom:4px;white-space:nowrap;word-break:keep-all}
		.astock-overview-summary strong{display:block;font-size:18px;line-height:1.25;white-space:nowrap}
		.astock-overview-table{width:100%;min-width:1920px;table-layout:fixed;font-size:12px}
		.astock-overview-table th,.astock-overview-table td{vertical-align:top}
		.astock-overview-table .astock-muted{display:block;margin-bottom:7px;font-size:12px;white-space:nowrap;word-break:keep-all}
		.astock-overview-table .astock-overview-sub-label{margin-top:16px}
		.astock-overview-period{width:6.4%;min-width:110px}
		.astock-overview-period strong{white-space:nowrap}
		.astock-overview-metric{width:6.1%;min-width:82px}
		.astock-overview-metric .astock-muted,.astock-overview-metric strong{white-space:nowrap}
		.astock-overview-recent-filter{width:7.2%;min-width:128px}
		.astock-overview-recent-filter .astock-muted,.astock-overview-recent-filter strong{white-space:nowrap}
		.astock-overview-limit-filter{width:6.6%;min-width:116px}
		.astock-overview-limit-filter .astock-muted,.astock-overview-limit-filter strong{white-space:nowrap}
		.astock-overview-market-filter{width:7.8%;min-width:138px}
		.astock-overview-market-filter .astock-muted,.astock-overview-market-filter strong{white-space:nowrap}
		.astock-overview-fund-filter{width:7.8%;min-width:138px}
		.astock-overview-fund-filter .astock-muted,.astock-overview-fund-filter strong{white-space:nowrap}
		.astock-overview-recalculate{width:8.2%;min-width:146px}
		.astock-overview-recalculate .astock-muted,.astock-overview-recalculate strong{white-space:nowrap}
		.astock-overview-window{width:11.2%;min-width:196px}
		.astock-overview-window strong{white-space:nowrap}
		.astock-overview-table strong{display:block;font-size:18px;line-height:1.25}
		.astock-overview-status{width:23.1%;min-width:380px}
		.astock-overview-status strong{white-space:normal;word-break:break-word}
		.astock-filter-toggle-form{margin:0}
		.astock-filter-toggle{display:inline-flex;align-items:center;justify-content:center;max-width:100%;box-sizing:border-box;margin-top:8px;padding:6px 10px;border:1px solid #d6ccbb;border-radius:8px;color:#214e34;text-align:center;text-decoration:none;background:#fff;font-size:12px;font-weight:600;font-family:inherit;line-height:1.2;white-space:normal;cursor:pointer}
		.astock-overview-table .astock-filter-toggle{width:100%;min-height:28px;white-space:nowrap}
		.astock-actions{width:100%}
		.astock-action-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:10px;align-items:stretch}
		.astock-actions form{margin:0}
		.astock-actions button{display:inline-flex;align-items:center;justify-content:center;width:100%;min-height:40px;margin:0;padding:8px 12px;border:1px solid #d6ccbb;border-radius:8px;background:#fff;color:#214e34;text-align:center;font-size:14px;font-weight:700;line-height:1.2;white-space:nowrap}
		.astock-actions button:hover{background:#eef4ec;border-color:#b7c9b8;color:#153823}
		.astock-actions button.astock-action-running{background:#8f6a20;border-color:#8f6a20;color:#fff;cursor:progress}
		.astock-actions button:disabled{opacity:.78;cursor:wait}
		.astock-muted{color:#6a6257}
		.astock-empty{padding:18px;border:1px dashed #d0c8b8;border-radius:12px;background:#fff;color:#6a6257}
		.astock-badge{display:inline-flex;align-items:center;padding:5px 9px;border-radius:999px;background:#eef4ec;color:#214e34;font-size:13px;margin-right:6px}
		.astock-source-list{display:flex;gap:8px;flex-wrap:wrap;margin-top:12px}
		.astock-news-grid{display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:16px;align-items:start}
		.astock-news-window{min-width:0}
		.astock-news-table{table-layout:fixed}
		.astock-news-table th,.astock-news-table td{vertical-align:top}
		.astock-news-counts{display:inline-flex;align-items:center;gap:6px;white-space:nowrap}
		.astock-news-diagnostic{margin-top:6px;color:#8a5a17;font-size:12px;line-height:1.35}
		.astock-hotspot-stocks{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:6px 14px;align-items:start;line-height:1.55;white-space:normal}
		.astock-hotspot-stock{display:block;min-width:0;white-space:nowrap}
		.astock-hotspot-date{color:#7a7064;font-size:12px}
		.astock-help{display:inline-flex;align-items:center;justify-content:center;width:18px;height:18px;border-radius:50%;background:#eef4ec;color:#214e34;font-size:12px;font-weight:700;line-height:1;cursor:help;position:relative}
		.astock-help-text{position:absolute;right:0;top:calc(100% + 8px);z-index:10;display:none;width:max-content;max-width:260px;padding:8px 10px;border:1px solid #d6ccbb;border-radius:8px;background:#fff;color:#2b261f;box-shadow:0 12px 28px rgba(31,40,34,.14);font-size:12px;font-weight:400;line-height:1.4;white-space:normal}
		.astock-help:hover .astock-help-text,.astock-help:focus .astock-help-text{display:block}
		.astock-table{min-width:960px}
		.astock-table th{white-space:nowrap}
		.astock-scroll{width:100%;overflow:auto}
		.astock-recommendation-table{width:100%;min-width:1460px;table-layout:fixed}
		.astock-recommendation-table th,.astock-recommendation-table td{vertical-align:top}
		.astock-recommendation-table th:nth-child(2),.astock-recommendation-table td:nth-child(2){width:7.5%;white-space:nowrap}
		.astock-recommendation-table th:last-child,.astock-recommendation-table td:last-child{width:47%}
		.astock-recommendation-detail-cell{white-space:normal}
		.astock-recommendation-fundflow{margin-bottom:6px;font-weight:700;line-height:1.25;white-space:nowrap}
		.astock-simulation-status{margin-top:10px;color:#214e34;font-weight:700;line-height:1.45}
		.astock-simulation-status[hidden]{display:none}
		.astock-score-total{display:flex;align-items:center;justify-content:space-between;gap:10px;margin-bottom:6px;color:#214e34;font-weight:700;line-height:1.25}
		.astock-score-table{width:100%;min-width:0!important;table-layout:auto;border-collapse:collapse;font-size:12px;line-height:1.35}
		.astock-score-table th,.astock-score-table td{padding:4px 6px;border:1px solid #ece7dc;vertical-align:top}
		.astock-score-table th{background:#faf8f2;color:#554b40;font-weight:700;white-space:nowrap}
		.astock-score-table td:first-child{white-space:nowrap}
		.astock-score-table th:nth-child(3),.astock-score-table td:nth-child(3){width:42%}
		.astock-score-table th:last-child,.astock-score-table td:last-child{text-align:right;white-space:nowrap;font-weight:700}
		.astock-score-category{white-space:nowrap;color:#214e34;font-weight:700}
		.astock-score-value{text-align:right;white-space:nowrap}
		.astock-score-detail{white-space:normal}
		.astock-score-summary{display:flex;flex-wrap:wrap;gap:8px 14px;margin-top:6px;color:#214e34;font-size:12px;font-weight:700;line-height:1.45}
		.astock-score-summary span{white-space:nowrap}
		.astock-score-reason{margin-top:6px;color:#6a6257;font-size:12px;line-height:1.45}
		.astock-date-tabs{display:flex;gap:8px;flex-wrap:nowrap;margin:14px 0 18px;overflow-x:auto;padding-bottom:6px;scrollbar-width:thin}
		.astock-tabs{display:flex;gap:8px;flex-wrap:wrap;margin-top:12px}
		.astock-tab{display:inline-flex;align-items:center;flex:0 0 auto;padding:8px 12px;border:1px solid #d6ccbb;border-radius:8px;color:#214e34;text-decoration:none;background:#fff}
		.astock-tab.active{background:#214e34;color:#fff;border-color:#214e34}
		.astock-tab.disabled{color:#9a9388;border-color:#ece7dc;background:#faf8f2;pointer-events:none}
		.astock-history-actions{display:flex;gap:10px;flex-wrap:wrap;align-items:stretch;margin:0 0 18px}
		.astock-history-actions form{display:flex;margin:0}
		.astock-history-actions button,.astock-history-actions .astock-filter-toggle{display:inline-flex;align-items:center;justify-content:center;box-sizing:border-box;min-height:38px;margin:0;padding:8px 12px;font-family:inherit;font-size:14px;font-weight:700;line-height:1.2}
		.astock-history-actions .astock-filter-toggle{background:#214e34;color:#fff;border-color:#214e34;text-decoration:none}
		.astock-pagination{display:flex;gap:8px;flex-wrap:wrap;align-items:center;margin-top:14px}
		.astock-up{color:#b3261e;font-weight:700}
		.astock-down{color:#1b7f3a;font-weight:700}
		.astock-flat{color:#6a6257}
		.astock-section-divider{margin:18px 0;border:0;border-top:1px solid #ece7dc}
		.astock-popup-mask{position:fixed;inset:0;z-index:1000;display:flex;align-items:center;justify-content:center;padding:24px;background:rgba(20,27,22,.42)}
		.astock-popup-mask[hidden]{display:none}
		.astock-popup{width:min(1080px,100%);max-height:min(86vh,860px);overflow:hidden;border-radius:16px;background:#fff;box-shadow:0 24px 72px rgba(23,35,27,.24);display:flex;flex-direction:column}
		.astock-popup-header{display:flex;gap:12px;align-items:flex-start;justify-content:space-between;padding:20px 24px 12px;border-bottom:1px solid #ece7dc}
		.astock-popup-header h3{margin:0}
		.astock-popup-close{padding:8px 12px;border:1px solid #d6ccbb;border-radius:8px;background:#fff;color:#214e34;font-weight:700;cursor:pointer}
		.astock-popup-body{padding:0 24px 24px}
		.astock-popup-scroll{overflow:auto;max-height:60vh}
		.astock-popup-table{min-width:760px}
		.astock-page-content.astock-loading{opacity:.62;pointer-events:none}
		@media (max-width:1100px){.astock-news-grid{grid-template-columns:1fr}.astock-news-table{min-width:720px}}
	</style>`)
	renderAStockPopupShell(&b)
	writeAStockPageScript(&b, payload.Date)
	b.WriteString(payload.HTML)

	_ = s.writeSimplePage(w, "a-stock", "A股", b.String())
}

func (s *Server) handleAStockTestPage(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	strategyDate := normalizeAStockStrategyDate(r.URL.Query().Get("date"))
	period := normalizeAStockPeriod(r.URL.Query().Get("period"))
	phase := normalizeAStockRecommendationPhase(r.URL.Query().Get("phase"))
	ignoreRecent := normalizeAStockBool(r.URL.Query().Get("ignore_recent"))
	ignoreLimitUp := normalizeAStockBool(r.URL.Query().Get("ignore_limit_up"))
	ignoreFundFlow := normalizeAStockBool(r.URL.Query().Get("ignore_fund_flow"))
	filterTodayMarket := normalizeAStockBool(r.URL.Query().Get("filter_today_market"))
	simulate := normalizeAStockBool(r.URL.Query().Get("simulate"))

	var b strings.Builder
	b.WriteString(`<style>
		body[data-page='a-stock-test'] main{max-width:none;width:98vw;box-sizing:border-box;padding-left:12px;padding-right:12px}
		body[data-page='a-stock-test'] .site-footer{max-width:none;width:98vw;box-sizing:border-box}
		body[data-page='a-stock-test'] section{width:100%;box-sizing:border-box}
		body[data-page='a-stock-test'] table{width:100%;min-width:100%;font-size:13px}
		body[data-page='a-stock-test'] .astock-scroll{width:100%;overflow:auto}
		body[data-page='a-stock-test'] .astock-recommendation-table{min-width:1800px;table-layout:auto}
		body[data-page='a-stock-test'] .astock-recommendation-table th,body[data-page='a-stock-test'] .astock-recommendation-table td{vertical-align:top}
		body[data-page='a-stock-test'] .astock-recommendation-table th:nth-child(-n+10),body[data-page='a-stock-test'] .astock-recommendation-table td:nth-child(-n+10){white-space:nowrap;word-break:keep-all}
		body[data-page='a-stock-test'] .astock-score-table th:nth-child(3),body[data-page='a-stock-test'] .astock-score-table td:nth-child(3){width:33.6%}
		body[data-page='a-stock-test'] [data-astock-simulation-form='1'] button[type='submit']:disabled{background:#9aa0a6;border-color:#8a8f96;color:#fff;cursor:wait;opacity:1;box-shadow:none}
	</style>`)
	b.WriteString(`<section><h2>A股模拟生成</h2><p class="astock-muted">模拟生成，不写快照/数据库。此页面只计算推荐、回测和过滤状态，不保存推荐快照、已选股票或T+1影子快照。</p>`)
	b.WriteString(`<form method="get" action="/a-stock/test" class="astock-action-grid" data-astock-simulation-form="1">`)
	b.WriteString(`<label>策略日期<input type="date" name="date" value="`)
	b.WriteString(html.EscapeString(strategyDate))
	b.WriteString(`"></label>`)
	b.WriteString(`<label>推荐窗口<select name="period">`)
	for _, option := range aStockPeriods() {
		b.WriteString(`<option value="`)
		b.WriteString(html.EscapeString(option.Key))
		b.WriteString(`"`)
		if option.Key == period.Key {
			b.WriteString(` selected`)
		}
		b.WriteString(`>`)
		b.WriteString(html.EscapeString(option.Label))
		b.WriteString(`</option>`)
	}
	b.WriteString(`</select></label>`)
	b.WriteString(`<label>生成阶段<select name="phase"><option value="final"`)
	if phase == aStockRecommendationPhaseFinal {
		b.WriteString(` selected`)
	}
	b.WriteString(`>正式窗口</option><option value="preopen"`)
	if phase == aStockRecommendationPhasePreopen {
		b.WriteString(` selected`)
	}
	b.WriteString(`>盘前窗口</option></select></label>`)
	writeAStockSimulationCheckbox(&b, "ignore_recent", "忽略90个交易日内过滤", ignoreRecent)
	writeAStockSimulationCheckbox(&b, "ignore_limit_up", "忽略涨停过滤", ignoreLimitUp)
	writeAStockSimulationCheckbox(&b, "ignore_fund_flow", "忽略资金过滤", ignoreFundFlow)
	writeAStockSimulationCheckbox(&b, "filter_today_market", "过滤当日行情缺失", filterTodayMarket)
	b.WriteString(`<input type="hidden" name="simulate" value="1"><button type="submit" name="simulate" value="1">模拟生成</button></form><div id="astock-simulation-status" class="astock-simulation-status" hidden>正在模拟生成，可能需要1-2分钟，请不要重复点击。</div></section>`)

	if simulate {
		ctx := s.loadAStockSimulationContext(strategyDate, period.Key, 1, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, phase, newAStockRequestCache())
		if strings.TrimSpace(ctx.LoadMessage) != "" {
			b.WriteString(`<section><p style="color:#8a4b16">`)
			b.WriteString(html.EscapeString(ctx.LoadMessage))
			b.WriteString(`</p></section>`)
		}
		b.WriteString(`<section><h2>模拟结果</h2><table class="astock-overview"><tr>`)
		writeAStockSimulationMetric(&b, "策略日期", ctx.Date)
		writeAStockSimulationMetric(&b, "推荐窗口", ctx.PeriodLabel)
		writeAStockSimulationMetric(&b, "候选热点数", fmt.Sprint(ctx.MarketCandidateCount))
		writeAStockSimulationMetric(&b, "生成数量", fmt.Sprintf("%d/%d", len(ctx.Recommendations), ctx.GeneratedRecommendationCount))
		writeAStockSimulationMetric(&b, "回测状态", nonEmpty(ctx.BacktestStatus, "无"))
		b.WriteString(`</tr><tr>`)
		writeAStockSimulationMetric(&b, aStockRecentLookbackLabel()+"内过滤", fmt.Sprint(ctx.RecentFiltered))
		writeAStockSimulationMetric(&b, "涨停过滤", fmt.Sprint(ctx.LimitUpFiltered))
		writeAStockSimulationMetric(&b, "当日行情过滤", fmt.Sprint(ctx.NoTodayMarketCount))
		writeAStockSimulationMetric(&b, "资金过滤", fmt.Sprint(ctx.FundFlowFiltered))
		writeAStockSimulationMetric(&b, "资金缺失", fmt.Sprint(ctx.FundFlowMissingCount))
		b.WriteString(`</tr></table></section><section><h2>推荐股票</h2>`)
		renderAStockRecommendationSubsection(&b, ctx)
		b.WriteString(`</section>`)
	} else {
		b.WriteString(`<section><h2>模拟结果</h2><div class="astock-empty">选择参数后点击模拟生成。</div></section>`)
	}

	writeAStockSimulationScript(&b)
	_ = s.writeSimplePage(w, "a-stock-test", "A股模拟生成", b.String())
}

func writeAStockSimulationScript(b *strings.Builder) {
	b.WriteString(`<script>(function(){var form=document.querySelector("[data-astock-simulation-form='1']");if(!form){return}var button=form.querySelector("button[type='submit']");var originalText=button?button.textContent:"模拟生成";function setRunning(running){if(button){button.disabled=!!running;if(running){button.setAttribute("aria-busy","true");button.setAttribute("aria-disabled","true");button.textContent="模拟生成中..."}else{button.removeAttribute("aria-busy");button.removeAttribute("aria-disabled");button.textContent=originalText}}var status=document.getElementById("astock-simulation-status");if(status){status.hidden=!running;if(running){status.textContent="正在模拟生成，可能需要1-2分钟，请不要重复点击。"}}}form.addEventListener("submit",function(){setRunning(true)});window.addEventListener("pageshow",function(){setRunning(false)})})();</script>`)
}

func writeAStockSimulationCheckbox(b *strings.Builder, name string, label string, checked bool) {
	b.WriteString(`<label><input type="checkbox" name="`)
	b.WriteString(html.EscapeString(name))
	b.WriteString(`" value="1"`)
	if checked {
		b.WriteString(` checked`)
	}
	b.WriteString(`>`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</label>`)
}

func writeAStockSimulationMetric(b *strings.Builder, label string, value string) {
	b.WriteString(`<td><span class="astock-muted">`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</span><strong>`)
	b.WriteString(html.EscapeString(value))
	b.WriteString(`</strong></td>`)
}

func (s *Server) buildAStockPageFragment(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, fundFlowExplicit bool, filterTodayMarket bool, forceRecommendationRefresh bool, message string) aStockPartialPayload {
	requestCache := newAStockRequestCache()
	ctx := s.loadAStockContextReadOnlyWithCache(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh, requestCache)
	ctx.FundFlowFilterExplicit = aStockFundFlowRenderExplicit(ctx.Date, ctx.IgnoreFundFlow, fundFlowExplicit)
	periodContexts := make([]aStockContext, 0, len(aStockPeriods()))
	for _, option := range aStockPeriods() {
		periodCtx := ctx
		if option.Key != ctx.Period {
			periodCtx = s.loadAStockCompanionContextReadOnlyWithCache(strategyDate, option.Key, 1, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh, requestCache)
			periodCtx.FundFlowFilterExplicit = aStockFundFlowRenderExplicit(periodCtx.Date, periodCtx.IgnoreFundFlow, fundFlowExplicit)
		}
		if !periodCtx.FastReadOnly {
			s.applyAStockRecommendationFundFlow5DToContext(&periodCtx, requestCache)
		}
		periodContexts = append(periodContexts, periodCtx)
	}
	if strings.TrimSpace(message) == "" {
		message = ctx.LoadMessage
	}

	var b strings.Builder
	b.WriteString(`<div id="astock-page-content" class="astock-page-content" data-astock-date="`)
	b.WriteString(html.EscapeString(ctx.Date))
	b.WriteString(`" data-astock-period="`)
	b.WriteString(html.EscapeString(ctx.Period))
	b.WriteString(`">`)
	if message != "" {
		b.WriteString(`<section><p style="color:#214e34">`)
		b.WriteString(html.EscapeString(message))
		b.WriteString(`</p></section>`)
	}
	renderAStockDateTabs(&b, ctx.Date, ctx.Period, ctx.IgnoreRecent, ctx.IgnoreLimitUp, ctx.IgnoreFundFlow, ctx.FundFlowFilterExplicit, ctx.TodayMarketFilterEnabled, false)
	renderAStockOverviewSection(&b, periodContexts)
	renderAStockRecommendationSection(&b, periodContexts)
	renderAStockActionSection(&b, ctx)
	renderAStockHotspotSection(&b, ctx.Hotspots)
	b.WriteString(`</div>`)

	return aStockPartialPayload{
		HTML:         b.String(),
		CanonicalURL: aStockCanonicalPageURL(ctx.Date, ctx.Period, ctx.NewsPage, ctx.IgnoreRecent, ctx.IgnoreLimitUp, ctx.IgnoreFundFlow, ctx.FundFlowFilterExplicit, ctx.TodayMarketFilterEnabled),
		Date:         ctx.Date,
		Period:       ctx.Period,
		Message:      strings.TrimSpace(message),
	}
}

func renderAStockActionSection(b *strings.Builder, ctx aStockContext) {
	b.WriteString(`<section><h2>操作区</h2><div class="astock-actions"><div class="astock-action-grid">`)
	actions := []struct {
		Name   string
		Label  string
		Period string
	}{
		{Name: "crawl", Label: "抓取全部财经信息", Period: ctx.Period},
		{Name: "backfill_window_news", Label: "补录上午新闻", Period: "morning"},
		{Name: "generate_morning_stock", Label: "重新生成上午推荐", Period: "morning"},
		{Name: "sync_market", Label: "同步行情", Period: ctx.Period},
		{Name: "backfill_auction", Label: "补录集合竞价", Period: ctx.Period},
		{Name: "refresh_backtest", Label: "刷新回测结果", Period: ctx.Period},
		{Name: "generate", Label: "生成全部推荐股票", Period: ctx.Period},
		{Name: "backfill_window_news", Label: "补录下午新闻", Period: "afternoon"},
		{Name: "generate_afternoon_stock", Label: "重新生成下午推荐", Period: "afternoon"},
		{Name: "generate_evening_stock", Label: "重新生产晚间推荐", Period: "evening"},
	}
	for _, action := range actions {
		b.WriteString(`<form class="astock-action-form" method="post"><input type="hidden" name="date" value="`)
		b.WriteString(html.EscapeString(ctx.Date))
		b.WriteString(`"><input type="hidden" name="period" value="`)
		b.WriteString(html.EscapeString(action.Period))
		b.WriteString(`"><input type="hidden" name="action" value="`)
		b.WriteString(html.EscapeString(action.Name))
		b.WriteString(`">`)
		if ctx.IgnoreRecent {
			b.WriteString(`<input type="hidden" name="ignore_recent" value="1">`)
		}
		if ctx.IgnoreLimitUp {
			b.WriteString(`<input type="hidden" name="ignore_limit_up" value="1">`)
		}
		writeAStockFundFlowPreserveInput(b, ctx.Date, ctx.IgnoreFundFlow, ctx.FundFlowFilterExplicit)
		if ctx.TodayMarketFilterEnabled {
			b.WriteString(`<input type="hidden" name="filter_today_market" value="1">`)
		}
		b.WriteString(`<button type="submit">`)
		b.WriteString(html.EscapeString(action.Label))
		b.WriteString(`</button></form>`)
	}
	b.WriteString(`</div></div><p class="astock-muted">已接入已有新闻抓取链路：抓取按钮会触发金十快讯、金十资讯、金十全站信息、东方财富快讯、东方财富全站、华尔街见闻、财联社和新浪财经，页面按策略日期和推荐窗口聚合财经新闻。行情接口读取 `)
	b.WriteString(aStockMarketConfigHint())
	b.WriteString(`，用于展示昨日收盘价、现价、涨跌幅和行情收益。</p><div class="astock-source-list"><span class="astock-badge">jin10_kuaixun: https://www.jin10.com/</span><span class="astock-badge">jin10_资讯: https://xnews.jin10.com/</span><span class="astock-badge">jin10_full: 金十全站</span><span class="astock-badge">eastmoney_kuaixun: 东方财富快讯</span><span class="astock-badge">eastmoney_full: 东方财富全站</span><span class="astock-badge">wallstreetcn_a_stock: 华尔街见闻</span><span class="astock-badge">cls_telegraph: 财联社</span><span class="astock-badge">sina_finance_7x24: 新浪财经</span></div></section>`)
}

func aStockCanonicalPageURL(strategyDate string, period string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, fundFlowExplicit bool, filterTodayMarket bool) string {
	query := url.Values{}
	query.Set("date", normalizeAStockStrategyDate(strategyDate))
	query.Set("period", normalizeAStockPeriod(period).Key)
	if newsPage > 1 {
		query.Set("news_page", fmt.Sprint(newsPage))
	}
	if ignoreRecent {
		query.Set("ignore_recent", "1")
	}
	if ignoreLimitUp {
		query.Set("ignore_limit_up", "1")
	}
	setAStockFundFlowFilterQuery(query, strategyDate, ignoreFundFlow, fundFlowExplicit)
	if filterTodayMarket {
		query.Set("filter_today_market", "1")
	}
	return "/a-stock?" + query.Encode()
}

func isAStockPageFragmentCacheable(query url.Values, forceRecommendationRefresh bool) bool {
	if forceRecommendationRefresh {
		return false
	}
	if strings.TrimSpace(query.Get("msg")) != "" {
		return false
	}
	if normalizeAStockBool(query.Get("refresh_recommendations")) || normalizeAStockBool(query.Get("refresh_all_backtests")) {
		return false
	}
	return true
}

func aStockPageFragmentCacheKey(strategyDate string, period string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) string {
	return strings.Join([]string{
		normalizeAStockStrategyDate(strategyDate),
		normalizeAStockPeriod(period).Key,
		fmt.Sprint(maxInt(newsPage, 1)),
		fmt.Sprint(ignoreRecent),
		fmt.Sprint(ignoreLimitUp),
		fmt.Sprint(ignoreFundFlow),
		fmt.Sprint(filterTodayMarket),
	}, "|")
}

func (s *Server) loadCachedAStockPageFragment(cacheKey string) (aStockPartialPayload, bool) {
	if s == nil || strings.TrimSpace(cacheKey) == "" {
		return aStockPartialPayload{}, false
	}
	s.aStockCacheMu.Lock()
	defer s.aStockCacheMu.Unlock()
	entry, ok := s.aStockFragments[cacheKey]
	if !ok {
		return aStockPartialPayload{}, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(s.aStockFragments, cacheKey)
		return aStockPartialPayload{}, false
	}
	return entry.payload, true
}

func (s *Server) storeCachedAStockPageFragment(cacheKey string, payload aStockPartialPayload) {
	if s == nil || strings.TrimSpace(cacheKey) == "" || strings.TrimSpace(payload.HTML) == "" {
		return
	}
	s.aStockCacheMu.Lock()
	defer s.aStockCacheMu.Unlock()
	if s.aStockFragments == nil {
		s.aStockFragments = make(map[string]aStockServerFragmentCacheEntry)
	}
	s.aStockFragments[cacheKey] = aStockServerFragmentCacheEntry{
		payload:   payload,
		expiresAt: time.Now().Add(aStockPageFragmentCacheTTL(payload.Date)),
	}
}

func aStockReadOnlyContextCacheKey(strategyDate string, period string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, forceRecommendationRefresh bool, includeHotspotTopStocks bool) string {
	return strings.Join([]string{
		normalizeAStockStrategyDate(strategyDate),
		normalizeAStockPeriod(period).Key,
		fmt.Sprint(maxInt(newsPage, 1)),
		fmt.Sprint(ignoreRecent),
		fmt.Sprint(ignoreLimitUp),
		fmt.Sprint(ignoreFundFlow),
		fmt.Sprint(filterTodayMarket),
		fmt.Sprint(forceRecommendationRefresh),
		fmt.Sprint(includeHotspotTopStocks),
	}, "|")
}

func (s *Server) loadCachedAStockReadOnlyContext(cacheKey string) (aStockContext, bool) {
	if s == nil || strings.TrimSpace(cacheKey) == "" {
		return aStockContext{}, false
	}
	s.aStockCacheMu.Lock()
	defer s.aStockCacheMu.Unlock()
	entry, ok := s.aStockContexts[cacheKey]
	if !ok {
		return aStockContext{}, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(s.aStockContexts, cacheKey)
		return aStockContext{}, false
	}
	return entry.ctx, true
}

func (s *Server) storeCachedAStockReadOnlyContext(cacheKey string, ctx aStockContext) {
	if s == nil || strings.TrimSpace(cacheKey) == "" || normalizeAStockStrategyDate(ctx.Date) != aStockTodayDate() {
		return
	}
	s.aStockCacheMu.Lock()
	defer s.aStockCacheMu.Unlock()
	if s.aStockContexts == nil {
		s.aStockContexts = make(map[string]aStockServerContextCacheEntry)
	}
	s.aStockContexts[cacheKey] = aStockServerContextCacheEntry{
		ctx:       ctx,
		expiresAt: time.Now().Add(aStockPageFragmentCacheTTL(ctx.Date)),
	}
}

func aStockPageFragmentCacheTTL(strategyDate string) time.Duration {
	if normalizeAStockStrategyDate(strategyDate) == aStockTodayDate() {
		return 2 * time.Minute
	}
	return 10 * time.Minute
}

func aStockArticleWindowCacheTTL(strategyDate string) time.Duration {
	if normalizeAStockStrategyDate(strategyDate) == aStockTodayDate() {
		return 15 * time.Second
	}
	return 6 * time.Hour
}

func aStockWindowCacheDate(start time.Time) string {
	if start.IsZero() {
		return ""
	}
	return start.In(aStockLocation()).Format("2006-01-02")
}

func aStockArticlePagesCacheKey(timeField string, start time.Time, end time.Time) string {
	return strings.TrimSpace(timeField) + "|" + start.UTC().Format(time.RFC3339Nano) + "|" + end.UTC().Format(time.RFC3339Nano)
}

func (s *Server) loadCachedAStockArticlePages(cacheKey string) ([]model.Item, error, bool) {
	if s == nil || strings.TrimSpace(cacheKey) == "" {
		return nil, nil, false
	}
	s.aStockCacheMu.Lock()
	defer s.aStockCacheMu.Unlock()
	entry, ok := s.aStockArticles[cacheKey]
	if !ok {
		return nil, nil, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(s.aStockArticles, cacheKey)
		return nil, nil, false
	}
	return append([]model.Item(nil), entry.items...), entry.err, true
}

func (s *Server) storeCachedAStockArticlePages(cacheKey string, strategyDate string, items []model.Item) {
	if s == nil || strings.TrimSpace(cacheKey) == "" {
		return
	}
	s.aStockCacheMu.Lock()
	defer s.aStockCacheMu.Unlock()
	if s.aStockArticles == nil {
		s.aStockArticles = make(map[string]aStockServerArticlesCacheEntry)
	}
	s.aStockArticles[cacheKey] = aStockServerArticlesCacheEntry{
		items:     append([]model.Item(nil), items...),
		date:      normalizeAStockStrategyDate(strategyDate),
		expiresAt: time.Now().Add(aStockArticleWindowCacheTTL(strategyDate)),
	}
}

func (s *Server) clearAStockPageCaches(strategyDates ...string) {
	if s == nil {
		return
	}
	s.aStockCacheMu.Lock()
	defer s.aStockCacheMu.Unlock()
	if len(strategyDates) == 0 {
		s.aStockFragments = make(map[string]aStockServerFragmentCacheEntry)
		s.aStockArticles = make(map[string]aStockServerArticlesCacheEntry)
		s.aStockAuctions = make(map[string]aStockServerAuctionCacheEntry)
		s.aStockContexts = make(map[string]aStockServerContextCacheEntry)
		return
	}
	dates := make(map[string]struct{}, len(strategyDates))
	for _, raw := range strategyDates {
		date := normalizeAStockStrategyDate(raw)
		if date != "" {
			dates[date] = struct{}{}
		}
	}
	for key, entry := range s.aStockFragments {
		if _, ok := dates[normalizeAStockStrategyDate(entry.payload.Date)]; ok {
			delete(s.aStockFragments, key)
		}
	}
	for key, entry := range s.aStockContexts {
		if _, ok := dates[normalizeAStockStrategyDate(entry.ctx.Date)]; ok {
			delete(s.aStockContexts, key)
		}
	}
	for key, entry := range s.aStockArticles {
		if _, ok := dates[normalizeAStockStrategyDate(entry.date)]; ok {
			delete(s.aStockArticles, key)
		}
	}
	for date := range dates {
		delete(s.aStockAuctions, aStockAuctionCandidateCacheKey(date))
	}
	delete(s.aStockAuctions, "__latest__")
}

func (s *Server) handleAStockBacktestPage(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method == http.MethodPost {
		s.handleAStockPageAction(w, r, "/a-stock/backtest")
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	strategyDate := normalizeAStockStrategyDate(r.URL.Query().Get("date"))
	period := normalizeAStockPeriod(r.URL.Query().Get("period"))
	newsPage := normalizeAStockNewsPage(r.URL.Query().Get("news_page"))
	ignoreRecent := normalizeAStockIgnoreRecent(r.URL.Query())
	ignoreLimitUp := normalizeAStockIgnoreLimitUp(r.URL.Query())
	ignoreFundFlow, fundFlowExplicit := normalizeAStockIgnoreFundFlowFromRequest(r, strategyDate)
	setAStockFundFlowFilterCookie(w, ignoreFundFlow, fundFlowExplicit)
	filterTodayMarket := normalizeAStockFilterTodayMarket(r.URL.Query())
	requestCache := newAStockRequestCache()
	periodContexts := s.loadAStockBacktestDisplayContextsWithCache(strategyDate, period.Key, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, fundFlowExplicit, filterTodayMarket, requestCache)
	ctx := aStockContextForPeriod(periodContexts, period.Key)
	if strings.TrimSpace(ctx.Date) == "" {
		ctx = newAStockBaseContext(strategyDate, period.Key, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, aStockRecommendationPhaseFinal)
		ctx.FundFlowFilterExplicit = aStockFundFlowRenderExplicit(ctx.Date, ctx.IgnoreFundFlow, fundFlowExplicit)
	}
	message := strings.TrimSpace(r.URL.Query().Get("msg"))
	if message == "" {
		message = ctx.LoadMessage
	}
	detail := strings.TrimSpace(r.URL.Query().Get("detail"))
	officialPerformance, officialPerformanceOK := s.loadAStockRecommendationPerformance(strategyDate, "all", "official")
	shadowPerformance, shadowPerformanceOK := s.loadAStockRecommendationPerformance(strategyDate, "all", aStockT1ShadowStrategyKey)
	auctionStrengthPerformance, auctionStrengthPerformanceOK := s.loadAStockRecommendationPerformance(strategyDate, "all", aStockAuctionStrengthStrategyKey)

	var b strings.Builder
	b.WriteString(`<style>
		body[data-page='a-stock-backtest'] main{max-width:none;width:100%;box-sizing:border-box}
		body[data-page='a-stock-backtest'] .site-footer{max-width:none;width:100%;box-sizing:border-box}
		body[data-page='a-stock-backtest'] section{width:100%;box-sizing:border-box}
		body[data-page='a-stock-backtest'] table{width:100%;min-width:100%;font-size:13px}
		.astock-muted{color:#6a6257}
		.astock-empty{padding:18px;border:1px dashed #d0c8b8;border-radius:12px;background:#fff;color:#6a6257}
		.astock-scroll{width:100%;overflow:auto}
		.astock-table{min-width:960px}
		.astock-table th{white-space:nowrap}
		.astock-date-tabs{display:flex;gap:8px;flex-wrap:nowrap;margin:14px 0 18px;overflow-x:auto;padding-bottom:6px;scrollbar-width:thin}
		.astock-tabs{display:flex;gap:8px;flex-wrap:wrap;margin:12px 0 18px}
		.astock-tab{display:inline-flex;align-items:center;flex:0 0 auto;padding:8px 12px;border:1px solid #d6ccbb;border-radius:8px;color:#214e34;text-decoration:none;background:#fff}
		.astock-tab.active{background:#214e34;color:#fff;border-color:#214e34}
		.astock-history-actions{display:flex;gap:10px;flex-wrap:wrap;align-items:stretch;margin:0 0 18px}
		.astock-history-actions form{display:flex;margin:0}
		.astock-history-actions button,.astock-history-actions .astock-filter-toggle{display:inline-flex;align-items:center;justify-content:center;box-sizing:border-box;min-height:38px;margin:0;padding:8px 12px;border:1px solid #214e34;border-radius:8px;background:#214e34;color:#fff;font-family:inherit;font-size:14px;font-weight:700;line-height:1.2;text-decoration:none}
		.astock-up{color:#b3261e;font-weight:700}
		.astock-down{color:#1b7f3a;font-weight:700}
		.astock-flat{color:#6a6257}
		.astock-action-detail{white-space:pre-wrap;line-height:1.55;margin:10px 0 0;color:#3d3a34;font-family:Consolas,Menlo,monospace;font-size:13px}
	</style>`)
	if message != "" {
		b.WriteString(`<section><h3>推荐行情收益</h3><p style="color:#214e34">`)
		b.WriteString(html.EscapeString(message))
		b.WriteString(`</p>`)
		if detail != "" {
			b.WriteString(`<pre class="astock-action-detail">`)
			b.WriteString(html.EscapeString(detail))
			b.WriteString(`</pre>`)
		}
		b.WriteString(`</section>`)
	}
	renderAStockT1PerformanceSection(&b, officialPerformance, officialPerformanceOK, shadowPerformance, shadowPerformanceOK, auctionStrengthPerformance, auctionStrengthPerformanceOK)
	renderAStockBacktestSectionForPath(&b, "/a-stock/backtest", strategyDate, ctx.Period, ctx.IgnoreRecent, ctx.IgnoreLimitUp, ctx.IgnoreFundFlow, ctx.FundFlowFilterExplicit, ctx.TodayMarketFilterEnabled, periodContexts)

	_ = s.writeSimplePage(w, "a-stock-backtest", "A股回测", b.String())
}

func (s *Server) loadAStockBacktestDisplayContextsWithCache(strategyDate string, activePeriod string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, fundFlowExplicit bool, filterTodayMarket bool, cache *aStockRequestCache) []aStockContext {
	displayDate := normalizeAStockStrategyDate(strategyDate)
	contexts := make([]aStockContext, 0, len(aStockPeriods()))
	for _, option := range aStockPeriods() {
		loadDate := displayDate
		if option.Key == "evening" {
			if previous := s.aStockPreviousTradingDayForBacktestWithCache(displayDate, cache); previous != "" {
				loadDate = previous
			}
		}
		page := 1
		if option.Key == normalizeAStockPeriod(activePeriod).Key {
			page = newsPage
		}
		ctx := s.loadAStockBacktestSnapshotContextWithCache(loadDate, option.Key, page, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, cache)
		ctx.FundFlowFilterExplicit = aStockFundFlowRenderExplicit(ctx.Date, ctx.IgnoreFundFlow, fundFlowExplicit)
		contexts = append(contexts, ctx)
	}
	return contexts
}

func (s *Server) aStockPreviousTradingDayForBacktestWithCache(strategyDate string, cache *aStockRequestCache) string {
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	if strategyDate == "" {
		return ""
	}
	if status, err := s.loadAStockTradingDayStatusWithCache(strategyDate, cache); err == nil {
		if previous := strings.TrimSpace(status.PreviousTradingDay); previous != "" {
			return previous
		}
	}
	return localAStockAdjacentTradingDay(strategyDate, -1)
}

func aStockContextForPeriod(contexts []aStockContext, period string) aStockContext {
	normalized := normalizeAStockPeriod(period).Key
	for _, ctx := range contexts {
		if ctx.Period == normalized {
			return ctx
		}
	}
	return aStockContext{}
}

func (s *Server) handleAStockPageAction(w http.ResponseWriter, r *http.Request, redirectPath string) {
	_ = r.ParseForm()
	redirectPath = aStockPagePath(redirectPath)
	query := url.Values{}
	strategyDate := normalizeAStockStrategyDate(r.FormValue("date"))
	if strategyDate != "" {
		query.Set("date", strategyDate)
	}
	period := normalizeAStockPeriod(r.FormValue("period"))
	query.Set("period", period.Key)
	ignoreRecent := normalizeAStockBool(r.FormValue("ignore_recent"))
	ignoreLimitUp := normalizeAStockBool(r.FormValue("ignore_limit_up"))
	ignoreFundFlow, fundFlowExplicit := normalizeAStockIgnoreFundFlowFromForm(r, strategyDate)
	setAStockFundFlowFilterCookie(w, ignoreFundFlow, fundFlowExplicit)
	filterTodayMarket := normalizeAStockBool(r.FormValue("filter_today_market"))
	if ignoreRecent {
		query.Set("ignore_recent", "1")
	}
	if ignoreLimitUp {
		query.Set("ignore_limit_up", "1")
	}
	setAStockFundFlowFilterQuery(query, strategyDate, ignoreFundFlow, fundFlowExplicit)
	if filterTodayMarket {
		query.Set("filter_today_market", "1")
	}
	action := strings.TrimSpace(r.FormValue("action"))
	if aStockActionRequiresTradingDay(action) {
		if blocked, message := s.aStockRecommendationBlockedMessage(strategyDate); blocked {
			query.Set("msg", message)
			http.Redirect(w, r, redirectPath+"?"+query.Encode(), http.StatusSeeOther)
			return
		}
	}
	persistRecommendation := false
	persistAllBacktests := false
	recommendationRefreshMode := aStockRecommendationPreserveLocked
	switch action {
	case "crawl":
		query.Set("msg", s.triggerAStockCrawl())
	case "backfill_window_news":
		query.Set("msg", s.triggerAStockWindowCrawl(strategyDate, period.Key))
		persistRecommendation = true
		recommendationRefreshMode = aStockRecommendationRebuild
	case "backfill_morning_stock":
		period = normalizeAStockPeriod("morning")
		query.Set("period", period.Key)
		query.Set("msg", s.triggerAStockWindowCrawl(strategyDate, period.Key)+"上午推荐已按补录后的新闻窗口重新计算。")
		persistRecommendation = true
		recommendationRefreshMode = aStockRecommendationRebuild
	case "generate_morning_stock":
		period = normalizeAStockPeriod("morning")
		query.Set("period", period.Key)
		query.Set("msg", "已切换到上午窗口，按 08:00-09:26:59 推荐生成窗口重新计算推荐。")
		persistRecommendation = true
		recommendationRefreshMode = aStockRecommendationRebuild
	case "generate_afternoon_stock":
		period = normalizeAStockPeriod("afternoon")
		query.Set("period", period.Key)
		query.Set("msg", "已切换到下午窗口，按 09:30-13:00:59 推荐生成窗口重新计算推荐。")
		persistRecommendation = true
		recommendationRefreshMode = aStockRecommendationRebuild
	case "generate_evening_stock":
		period = normalizeAStockPeriod("evening")
		query.Set("period", period.Key)
		query.Set("msg", "已切换到晚间窗口，按 15:00-18:30:59 量价筛选重新计算推荐。")
		persistRecommendation = true
		recommendationRefreshMode = aStockRecommendationRebuild
	case "generate_ignore_recent_stock":
		query.Set("ignore_recent", "1")
		ignoreRecent = true
		query.Set("msg", period.Label+"已忽略"+aStockRecentLookbackStatusPrefix()+"推荐过滤，按当前新闻窗口重新计算推荐。")
		persistRecommendation = true
		recommendationRefreshMode = aStockRecommendationRebuild
	case "generate":
		query.Set("msg", period.Label+"热点已按当前新闻窗口重新计算。")
		persistRecommendation = true
		recommendationRefreshMode = aStockRecommendationRebuild
	case "sync_market":
		query.Set("msg", "行情已按当前策略日期刷新，页面已重新计算昨日收盘价、现价、涨跌幅和回测。")
		persistRecommendation = true
	case "refresh_current_backtest":
		result := s.refreshAStockCurrentBacktestAction(strategyDate, period.Key, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, redirectPath == "/a-stock/backtest")
		query.Set("msg", result.Summary)
		if result.Detail != "" {
			query.Set("detail", result.Detail)
		}
	case "repair_stock_names":
		query.Set("msg", s.repairAStockActionRecommendationNames(strategyDate, period.Key, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, redirectPath == "/a-stock/backtest"))
	case "backfill_auction":
		query.Set("msg", s.triggerAStockAuctionBackfillDate(strategyDate))
		persistRecommendation = true
	case "refresh_backtest":
		query.Set("msg", "上午、下午和晚间消息回测已按当前推荐股票、13:01价格和行情收益重新刷新。")
		persistRecommendation = true
		persistAllBacktests = true
	case "recalculate":
		query.Set("msg", period.Label+"已按当前过滤开关重新计算推荐和回测。")
		persistRecommendation = true
		recommendationRefreshMode = aStockRecommendationRebuild
	default:
		query.Set("msg", "未知操作")
	}
	if persistRecommendation {
		if loadMessage := s.persistAStockActionRecommendation(strategyDate, period.Key, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, persistAllBacktests, recommendationRefreshMode); loadMessage != "" {
			existing := strings.TrimSpace(query.Get("msg"))
			if existing != "" {
				query.Set("msg", existing+" "+loadMessage)
			} else {
				query.Set("msg", loadMessage)
			}
		}
	}
	s.clearAStockPageCaches(strategyDate)
	http.Redirect(w, r, redirectPath+"?"+query.Encode(), http.StatusSeeOther)
}

func (s *Server) persistAStockActionRecommendation(strategyDate string, periodKey string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, allPeriods bool, refreshMode aStockRecommendationRefreshMode) string {
	cache := newAStockRequestCache()
	periods := []string{normalizeAStockPeriod(periodKey).Key}
	if allPeriods {
		periods = aStockPeriodKeys()
	}
	messages := make([]string, 0, len(periods))
	for _, period := range periods {
		entryTimeOverride := aStockManualRecommendationEntryTime(strategyDate, period, refreshMode)
		ctx := s.loadAStockContextWithRecommendationPhasePersistenceModeEntryTime(strategyDate, period, 1, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, true, aStockRecommendationPhaseFinal, cache, true, true, refreshMode, entryTimeOverride)
		if message := strings.TrimSpace(ctx.LoadMessage); message != "" {
			messages = append(messages, fmt.Sprintf("%s：%s", ctx.PeriodLabel, message))
		}
	}
	return strings.Join(messages, " ")
}

type aStockBacktestRefreshActionResult struct {
	Summary string
	Detail  string
}

func (s *Server) refreshAStockCurrentBacktestAction(strategyDate string, periodKey string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, allVisiblePeriods bool) aStockBacktestRefreshActionResult {
	cache := newAStockRequestCache()
	periods := []string{normalizeAStockPeriod(periodKey).Key}
	if allVisiblePeriods {
		periods = aStockPeriodKeys()
	}
	summaries := make([]string, 0, len(periods))
	details := make([]string, 0, len(periods))
	for _, period := range periods {
		summary, detail := s.refreshAStockCurrentBacktestPeriod(strategyDate, period, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, cache)
		if summary != "" {
			summaries = append(summaries, summary)
		}
		if detail != "" {
			details = append(details, detail)
		}
	}
	return aStockBacktestRefreshActionResult{
		Summary: strings.Join(summaries, " "),
		Detail:  strings.Join(details, "\n\n"),
	}
}

func (s *Server) refreshAStockCurrentBacktestPeriod(strategyDate string, periodKey string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, cache *aStockRequestCache) (string, string) {
	ctx, ok, message := s.loadAStockRecommendationNameRepairContext(strategyDate, periodKey, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, cache)
	if message != "" {
		return message, message
	}
	if !ok || len(ctx.Recommendations) == 0 {
		summary := ctx.PeriodLabel + "行情收益未补齐：未找到当前推荐股票。"
		return summary, summary
	}
	previousRows := append([]aStockBacktestRow(nil), ctx.Backtests...)
	previousByCode := aStockBacktestRowsByCode(previousRows)
	repaired, skipped := s.repairAStockPersistedRecommendationsForPeriodWithCache(ctx.Date, ctx.Period, ctx.Recommendations, cache)
	ctx.Recommendations = repaired
	if len(ctx.Recommendations) == 0 {
		summary := ctx.PeriodLabel + "行情收益未补齐：当前推荐股票名称无效。"
		if skipped > 0 {
			summary = fmt.Sprintf("%s 跳过无有效名称股票 %d 只。", strings.TrimSuffix(summary, "。"), skipped)
		}
		return summary, summary
	}
	ctx.Recommendations, ctx.Backtests, ctx.BacktestStatus, ctx.LimitUpFiltered = s.loadAStockLockedMarketView(ctx.Date, ctx.Period, ctx.Recommendations)
	if merged, changed := mergeAStockBacktestRowsFromSnapshot(ctx.Backtests, previousRows); changed {
		ctx.Backtests = merged
		ctx.BacktestStatus = formatAStockLockedBacktestStatus(ctx.Period, ctx.Backtests)
	}
	ctx.Backtests = s.enrichAStockBacktestsWithRealtimeQuotes(ctx.Date, ctx.Period, ctx.Backtests)
	ctx.BacktestStatus = formatAStockLockedBacktestStatus(ctx.Period, ctx.Backtests)
	ctx.EmptyReason = aStockRecommendationEmptyReason(ctx)
	if err := s.saveAStockRecommendationSnapshot(ctx); err != nil {
		summary := ctx.PeriodLabel + "行情收益保存失败：" + err.Error()
		return summary, summary
	}
	updatedCells := countAStockBacktestUpdatedCells(previousByCode, ctx.Backtests)
	summary := fmt.Sprintf("%s行情收益已按当前推荐股票重新补齐：读取 %d 只，写入 %d 行，补齐/更新 %d 个行情/收益字段。", ctx.PeriodLabel, len(ctx.Recommendations), len(ctx.Backtests), updatedCells)
	if skipped > 0 {
		summary = fmt.Sprintf("%s 跳过无有效名称股票 %d 只。", summary, skipped)
	}
	detail := formatAStockBacktestRefreshDetail(ctx, previousByCode, updatedCells)
	return summary, detail
}

type aStockBacktestRefreshPriceRequest struct {
	Date   string `json:"date"`
	Period string `json:"period"`
	Code   string `json:"code"`
}

type aStockBacktestRefreshPriceResult struct {
	Summary  string                             `json:"summary"`
	Detail   string                             `json:"detail"`
	Snapshot model.AStockRecommendationSnapshot `json:"snapshot"`
}

func (s *Server) handleAStockBacktestRefreshPrice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeRawJSON(w, http.StatusMethodNotAllowed, map[string]any{"code": http.StatusMethodNotAllowed, "message": "method not allowed"})
		return
	}
	payload := aStockBacktestRefreshPriceRequest{
		Date:   r.URL.Query().Get("date"),
		Period: r.URL.Query().Get("period"),
		Code:   r.URL.Query().Get("code"),
	}
	if r.Body != nil && r.ContentLength != 0 {
		var bodyPayload aStockBacktestRefreshPriceRequest
		if err := json.NewDecoder(r.Body).Decode(&bodyPayload); err != nil {
			writeRawJSON(w, http.StatusBadRequest, map[string]any{"code": http.StatusBadRequest, "message": "invalid json"})
			return
		}
		if strings.TrimSpace(bodyPayload.Date) != "" {
			payload.Date = bodyPayload.Date
		}
		if strings.TrimSpace(bodyPayload.Period) != "" {
			payload.Period = bodyPayload.Period
		}
		if strings.TrimSpace(bodyPayload.Code) != "" {
			payload.Code = bodyPayload.Code
		}
	}
	result, status, message := s.refreshAStockBacktestPrice(payload)
	if status != http.StatusOK {
		writeRawJSON(w, status, map[string]any{"code": status, "message": message})
		return
	}
	writeRawJSON(w, http.StatusOK, map[string]any{"code": http.StatusOK, "message": "ok", "data": result})
}

func (s *Server) refreshAStockBacktestPrice(payload aStockBacktestRefreshPriceRequest) (aStockBacktestRefreshPriceResult, int, string) {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return aStockBacktestRefreshPriceResult{}, http.StatusBadGateway, "content url required"
	}
	strategyDate := normalizeAStockStrategyDate(payload.Date)
	period := normalizeAStockPeriod(payload.Period)
	code := normalizeAStockCode(payload.Code)
	if strategyDate == "" || period.Key == "" || code == "" {
		return aStockBacktestRefreshPriceResult{}, http.StatusBadRequest, "date, period and code required"
	}
	cache := newAStockRequestCache()
	ctx, ok, message := s.loadAStockRecommendationNameRepairContext(strategyDate, period.Key, false, false, false, false, cache)
	if message != "" {
		return aStockBacktestRefreshPriceResult{}, http.StatusInternalServerError, message
	}
	if !ok || len(ctx.Recommendations) == 0 {
		return aStockBacktestRefreshPriceResult{}, http.StatusNotFound, period.Label + "未找到推荐快照。"
	}
	recIndex := aStockRecommendationIndexByCode(ctx.Recommendations, code)
	if recIndex < 0 {
		return aStockBacktestRefreshPriceResult{}, http.StatusNotFound, period.Label + "未找到当前股票 " + code + "。"
	}
	previousRows := append([]aStockBacktestRow(nil), ctx.Backtests...)
	previousByCode := aStockBacktestRowsByCode(previousRows)
	previousRow := previousByCode[code]
	targetRecommendation := ctx.Recommendations[recIndex]
	refreshedRecommendations, refreshedRows, status, _ := s.loadAStockLockedMarketView(ctx.Date, ctx.Period, []aStockRecommendation{targetRecommendation})
	if strings.Contains(status, "行情读取失败") {
		return aStockBacktestRefreshPriceResult{}, http.StatusBadGateway, status
	}
	if len(refreshedRecommendations) == 0 || len(refreshedRows) == 0 {
		return aStockBacktestRefreshPriceResult{}, http.StatusBadGateway, period.Label + code + "未返回有效行情。"
	}
	refreshedRow := refreshedRows[0]
	if previousRow.Stock != "" {
		if merged, _ := mergeAStockBacktestRowFromSnapshot(refreshedRow, previousRow); merged.Stock != "" {
			refreshedRow = merged
		}
	}
	refreshedRows = s.enrichAStockBacktestsWithRealtimeQuotes(ctx.Date, ctx.Period, []aStockBacktestRow{refreshedRow})
	refreshedRow = refreshedRows[0]
	if !aStockBacktestRowHasRefreshPrice(refreshedRow) {
		return aStockBacktestRefreshPriceResult{}, http.StatusBadGateway, period.Label + code + "未返回有效价格。"
	}
	refreshedRecommendation := refreshedRecommendations[0]
	if !aStockBacktestValueMissing(refreshedRow.CurrentPrice) {
		refreshedRecommendation.CurrentPrice = refreshedRow.CurrentPrice
	}
	if !aStockBacktestValueMissing(refreshedRow.CurrentReturn) {
		refreshedRecommendation.TodayPct = refreshedRow.CurrentReturn
		refreshedRecommendation.TodayPctClass = refreshedRow.CurrentReturnClass
	}
	if !aStockBacktestValueMissing(refreshedRow.CurrentMarketPct) {
		refreshedRecommendation.TodayPct = refreshedRow.CurrentMarketPct
		refreshedRecommendation.TodayPctClass = refreshedRow.CurrentMarketPctClass
	}
	ctx.Recommendations[recIndex] = refreshedRecommendation
	ctx.Backtests = replaceAStockBacktestRowByCode(ctx.Backtests, code, refreshedRow)
	ctx.BacktestStatus = formatAStockLockedBacktestStatus(ctx.Period, ctx.Backtests)
	ctx.EmptyReason = aStockRecommendationEmptyReason(ctx)
	if err := s.saveAStockRecommendationSnapshot(ctx); err != nil {
		return aStockBacktestRefreshPriceResult{}, http.StatusInternalServerError, period.Label + "行情价格保存失败：" + err.Error()
	}
	snapshot, err := s.buildAStockRecommendationSnapshot(ctx)
	if err != nil {
		return aStockBacktestRefreshPriceResult{}, http.StatusInternalServerError, period.Label + "行情价格返回失败：" + err.Error()
	}
	updatedCells := countAStockBacktestRowUpdatedCells(previousRow, refreshedRow)
	summary := fmt.Sprintf("%s%s行情价格已刷新：补齐/更新 %d 个行情/收益字段。", period.Label, code, updatedCells)
	detailCtx := ctx
	detailCtx.Recommendations = []aStockRecommendation{refreshedRecommendation}
	detailCtx.Backtests = []aStockBacktestRow{refreshedRow}
	detailCtx.BacktestStatus = refreshedRow.Status
	detail := formatAStockBacktestRefreshDetail(detailCtx, previousByCode, updatedCells)
	return aStockBacktestRefreshPriceResult{Summary: summary, Detail: detail, Snapshot: snapshot}, http.StatusOK, "ok"
}

func aStockRecommendationIndexByCode(recommendations []aStockRecommendation, code string) int {
	code = normalizeAStockCode(code)
	for i, rec := range recommendations {
		if normalizeAStockCode(rec.Code) == code {
			return i
		}
	}
	return -1
}

func replaceAStockBacktestRowByCode(rows []aStockBacktestRow, code string, row aStockBacktestRow) []aStockBacktestRow {
	code = normalizeAStockCode(code)
	result := append([]aStockBacktestRow(nil), rows...)
	for i := range result {
		if aStockBacktestRowCode(result[i]) == code {
			result[i] = row
			return result
		}
	}
	return append(result, row)
}

func aStockBacktestRowHasRefreshPrice(row aStockBacktestRow) bool {
	for _, value := range []string{row.EntryOpen, row.AfternoonOpen, row.T0Close, row.T0Return, row.CurrentPrice, row.CurrentReturn, row.CurrentMarketPct, row.BestReturn} {
		if !aStockBacktestValueMissing(value) {
			return true
		}
	}
	for _, day := range row.Days {
		if !aStockBacktestValueMissing(day.Close) || !aStockBacktestValueMissing(day.Return) || !aStockBacktestValueMissing(day.MarketPct) {
			return true
		}
	}
	return false
}

func aStockBacktestRowsByCode(rows []aStockBacktestRow) map[string]aStockBacktestRow {
	result := make(map[string]aStockBacktestRow, len(rows))
	for _, row := range rows {
		code := aStockBacktestRowCode(row)
		if code != "" {
			result[code] = row
		}
	}
	return result
}

func countAStockBacktestUpdatedCells(previousByCode map[string]aStockBacktestRow, rows []aStockBacktestRow) int {
	total := 0
	for _, row := range rows {
		total += countAStockBacktestRowUpdatedCells(previousByCode[aStockBacktestRowCode(row)], row)
	}
	return total
}

func countAStockBacktestRowUpdatedCells(previous aStockBacktestRow, current aStockBacktestRow) int {
	count := 0
	if aStockBacktestRefreshFieldChanged(previous.EntryOpen, current.EntryOpen) {
		count++
	}
	if aStockBacktestRefreshFieldChanged(previous.AfternoonOpen, current.AfternoonOpen) {
		count++
	}
	if aStockBacktestRefreshFieldChanged(previous.T0Return, current.T0Return) {
		count++
	}
	if aStockBacktestRefreshFieldChanged(previous.T0Close, current.T0Close) {
		count++
	}
	if aStockBacktestRefreshFieldChanged(previous.CurrentPrice, current.CurrentPrice) {
		count++
	}
	if aStockBacktestRefreshFieldChanged(previous.CurrentReturn, current.CurrentReturn) {
		count++
	}
	if aStockBacktestRefreshFieldChanged(previous.CurrentMarketPct, current.CurrentMarketPct) {
		count++
	}
	for i := 0; i < 5; i++ {
		var previousValue, previousMarketPct string
		if i < len(previous.Days) {
			previousValue = previous.Days[i].Return
			previousMarketPct = previous.Days[i].MarketPct
		}
		var currentValue, currentMarketPct string
		if i < len(current.Days) {
			currentValue = current.Days[i].Return
			currentMarketPct = current.Days[i].MarketPct
		}
		if aStockBacktestRefreshFieldChanged(previousValue, currentValue) {
			count++
		}
		if aStockBacktestRefreshFieldChanged(previousMarketPct, currentMarketPct) {
			count++
		}
	}
	if aStockBacktestRefreshFieldChanged(previous.BestReturn, current.BestReturn) {
		count++
	}
	return count
}

func aStockBacktestRefreshFieldChanged(previous string, current string) bool {
	previous = strings.TrimSpace(previous)
	current = strings.TrimSpace(current)
	if aStockBacktestValueMissing(current) {
		return false
	}
	if aStockBacktestValueMissing(previous) {
		return true
	}
	return previous != current
}

func formatAStockBacktestRefreshDetail(ctx aStockContext, previousByCode map[string]aStockBacktestRow, updatedCells int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s行情收益\n", ctx.PeriodLabel)
	fmt.Fprintf(&b, "获取：date=%s period=%s codes=%s source=%s；%s\n", ctx.Date, ctx.Period, strings.Join(aStockRecommendationCodes(ctx.Recommendations), ","), aStockMarketConfigHint(), aStockRealtimeQuoteConfigHint(ctx.Date))
	fmt.Fprintf(&b, "写入：recommendations=%d backtests=%d status=%s updated_fields=%d\n", len(ctx.Recommendations), len(ctx.Backtests), ctx.BacktestStatus, updatedCells)
	for _, row := range ctx.Backtests {
		code := aStockBacktestRowCode(row)
		previous := previousByCode[code]
		fmt.Fprintf(&b, "补齐：%s 原状态=%s -> 新状态=%s；%s\n", row.Stock, nonEmpty(previous.Status, "无旧行"), nonEmpty(row.Status, "--"), formatAStockBacktestRefreshRowValues(previous, row))
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatAStockBacktestRefreshRowValues(previous aStockBacktestRow, current aStockBacktestRow) string {
	parts := []string{
		formatAStockBacktestRefreshField("开盘", previous.EntryOpen, current.EntryOpen),
		formatAStockBacktestRefreshField("下午开盘", previous.AfternoonOpen, current.AfternoonOpen),
		formatAStockBacktestRefreshField("现价", previous.CurrentPrice, current.CurrentPrice),
		formatAStockBacktestRefreshField("当前收益", previous.CurrentReturn, current.CurrentReturn),
		formatAStockBacktestRefreshField("今日涨跌", previous.CurrentMarketPct, current.CurrentMarketPct),
		formatAStockBacktestRefreshField("T+0", previous.T0Return, current.T0Return),
		formatAStockBacktestRefreshField("T+0收盘", previous.T0Close, current.T0Close),
	}
	for i := 0; i < 5; i++ {
		var previousValue, previousMarketPct string
		if i < len(previous.Days) {
			previousValue = previous.Days[i].Return
			previousMarketPct = previous.Days[i].MarketPct
		}
		var currentValue, currentMarketPct string
		if i < len(current.Days) {
			currentValue = current.Days[i].Return
			currentMarketPct = current.Days[i].MarketPct
		}
		parts = append(parts, formatAStockBacktestRefreshField(fmt.Sprintf("T+%d", i+1), previousValue, currentValue))
		parts = append(parts, formatAStockBacktestRefreshField(fmt.Sprintf("T+%d市场", i+1), previousMarketPct, currentMarketPct))
	}
	parts = append(parts, formatAStockBacktestRefreshField("五日最高", previous.BestReturn, current.BestReturn))
	return strings.Join(parts, "，")
}

func formatAStockBacktestRefreshField(label string, previous string, current string) string {
	previous = nonEmpty(strings.TrimSpace(previous), "--")
	current = nonEmpty(strings.TrimSpace(current), "--")
	if previous == current {
		return label + "=" + current
	}
	return fmt.Sprintf("%s=%s->%s", label, previous, current)
}

func aStockManualRecommendationEntryTime(strategyDate string, period string, refreshMode aStockRecommendationRefreshMode) string {
	if normalizeAStockPeriod(period).Key != "afternoon" || refreshMode != aStockRecommendationRebuild || normalizeAStockStrategyDate(strategyDate) != aStockTodayDate() {
		return ""
	}
	now := aStockNow().In(aStockLocation())
	minutes := now.Hour()*60 + now.Minute()
	inTradingSession := (minutes >= 9*60+30 && minutes <= 11*60+30) || (minutes >= 13*60 && minutes <= 15*60)
	if !inTradingSession {
		return ""
	}
	return normalizeAStockGeneratedEntryTime("afternoon", fmt.Sprintf("%02d:%02d", now.Hour(), now.Minute()))
}

func (s *Server) repairAStockActionRecommendationNames(strategyDate string, periodKey string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, allVisiblePeriods bool) string {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return "内容服务未配置，无法补股票名称。"
	}
	cache := newAStockRequestCache()
	periods := []string{normalizeAStockPeriod(periodKey).Key}
	if allVisiblePeriods {
		periods = aStockPeriodKeys()
	}
	messages := make([]string, 0, len(periods))
	for _, period := range periods {
		message := s.repairAStockActionRecommendationNamesPeriod(strategyDate, period, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, cache)
		if strings.TrimSpace(message) != "" {
			messages = append(messages, message)
		}
	}
	return strings.Join(messages, " ")
}

func (s *Server) repairAStockActionRecommendationNamesPeriod(strategyDate string, periodKey string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, cache *aStockRequestCache) string {
	ctx, ok, loadMessage := s.loadAStockRecommendationNameRepairContext(strategyDate, periodKey, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, cache)
	if loadMessage != "" {
		return loadMessage
	}
	if !ok || len(ctx.Recommendations) == 0 {
		return ctx.PeriodLabel + "暂无推荐股票可补名称。"
	}
	originalNames := aStockRecommendationNameMap(ctx.Recommendations)
	originalTextRepairs := countAStockRecommendationTextRepairs(ctx.Recommendations)
	recommendations, skipped := s.repairAStockPersistedRecommendationsForPeriodWithCache(ctx.Date, ctx.Period, ctx.Recommendations, cache)
	if len(recommendations) == 0 {
		return ctx.PeriodLabel + "未从集合竞价名称库找到可补齐的股票名称。"
	}
	ctx.Recommendations = recommendations
	ctx.Backtests = filterAStockBacktestsForRecommendations(ctx.Backtests, recommendations)
	if ctx.GeneratedRecommendationCount <= 0 {
		ctx.GeneratedRecommendationCount = len(recommendations)
	}
	if ctx.BacktestStatus == "" {
		ctx.BacktestStatus = "已补齐股票名称"
	}
	if isAStockOfficialSelectionContextForDate(strategyDate, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket) {
		if err := s.saveAStockRecommendationSelections(ctx); err != nil {
			return ctx.PeriodLabel + "股票名称补齐失败：" + err.Error()
		}
	}
	if err := s.saveAStockRecommendationSnapshot(ctx); err != nil {
		return ctx.PeriodLabel + "股票名称补齐失败：" + err.Error()
	}
	changed := countAStockRecommendationNameChanges(originalNames, recommendations)
	message := fmt.Sprintf("%s已修复：名称 %d 只，乱码热点/理由 %d 只", ctx.PeriodLabel, changed, originalTextRepairs)
	if skipped > 0 {
		message = fmt.Sprintf("%s，跳过无有效名称股票 %d 只", message, skipped)
	}
	return message + "。"
}

func (s *Server) loadAStockRecommendationNameRepairContext(strategyDate string, periodKey string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, cache *aStockRequestCache) (aStockContext, bool, string) {
	period := normalizeAStockPeriod(periodKey)
	ctx := aStockContext{
		Date:                         normalizeAStockStrategyDate(strategyDate),
		Period:                       period.Key,
		PeriodLabel:                  period.Label,
		WindowLabel:                  period.WindowLabel,
		IgnoreRecent:                 ignoreRecent,
		IgnoreLimitUp:                ignoreLimitUp,
		IgnoreFundFlow:               ignoreFundFlow,
		FundFlowFilterEnabled:        !ignoreFundFlow,
		TodayMarketFilterEnabled:     filterTodayMarket,
		LimitUpFilterEnabled:         period.Key == "afternoon" && !ignoreLimitUp,
		BacktestStatus:               "已补齐股票名称",
		GeneratedRecommendationCount: 0,
	}
	if snapshot, ok := s.loadAStockRecommendationSnapshotForContextWithCache(ctx, cache); ok {
		var recommendations []aStockRecommendation
		if err := json.Unmarshal([]byte(normalizeAStockSnapshotJSONArray(snapshot.RecommendationsJSON)), &recommendations); err != nil {
			return ctx, false, ctx.PeriodLabel + "推荐快照解析失败：" + err.Error()
		}
		var backtests []aStockBacktestRow
		if err := json.Unmarshal([]byte(normalizeAStockSnapshotJSONArray(snapshot.BacktestsJSON)), &backtests); err != nil {
			return ctx, false, ctx.PeriodLabel + "回测快照解析失败：" + err.Error()
		}
		ctx.Recommendations = recommendations
		ctx.Backtests = backtests
		ctx.BacktestStatus = nonEmpty(snapshot.BacktestStatus, ctx.BacktestStatus)
		ctx.GeneratedRecommendationCount = snapshot.GeneratedCount
		ctx.RecentFiltered = snapshot.RecentFiltered
		ctx.SameDayMorningFiltered = snapshot.SameDayMorningFiltered
		ctx.LimitUpFilterEnabled = snapshot.LimitUpFilterEnabled
		ctx.LimitUpFiltered = snapshot.LimitUpFiltered
		ctx.FundFlowFilterEnabled = snapshot.FundFlowFilterEnabled
		ctx.IgnoreFundFlow = !snapshot.FundFlowFilterEnabled
		ctx.FundFlowFilterExplicit = ctx.IgnoreFundFlow != aStockDefaultIgnoreFundFlow(ctx.Date)
		ctx.FundFlowFiltered = snapshot.FundFlowFiltered
		ctx.FundFlowMissingCount = snapshot.FundFlowMissingCount
		ctx.TodayMarketFilterEnabled = snapshot.TodayMarketFilterEnabled
		ctx.NoTodayMarketCount = snapshot.NoTodayMarketCount
		ctx.MarketCandidateStatus = snapshot.MarketCandidateStatus
		ctx.MarketCandidateCount = snapshot.MarketCandidateCount
		ctx.AuctionAmountLabel = snapshot.AuctionAmountLabel
		ctx.EmptyReason = snapshot.EmptyReason
		return ctx, true, ""
	}
	if !ignoreRecent {
		if result, ok := s.loadAStockRecommendationSelectionsWithCache(ctx.Date, ctx.Period, cache); ok {
			ctx.Recommendations = aStockRecommendationSelectionsToRecommendations(result.Items, ctx.Period)
			ctx.GeneratedRecommendationCount = len(ctx.Recommendations)
			if len(ctx.Recommendations) > 0 {
				ctx.Recommendations, ctx.Backtests, ctx.BacktestStatus, ctx.LimitUpFiltered = s.loadAStockLockedMarketView(ctx.Date, ctx.Period, ctx.Recommendations)
			}
			return ctx, len(ctx.Recommendations) > 0, ""
		}
	}
	return ctx, false, ""
}

func renderAStockPopupShell(b *strings.Builder) {
	b.WriteString(`<div id="astock-popup-mask" class="astock-popup-mask" hidden><div class="astock-popup" role="dialog" aria-modal="true" aria-labelledby="astock-popup-title"><div class="astock-popup-header"><div><h3 id="astock-popup-title">盘前推荐股票</h3><p id="astock-popup-meta" class="astock-muted"></p></div><button id="astock-popup-dismiss" class="astock-popup-close" type="button">关闭</button></div><div class="astock-popup-body"><div class="astock-popup-scroll"><table class="astock-table astock-popup-table"><thead><tr><th>排名</th><th>股票代码</th><th>股票名称</th><th>热点</th><th>推荐理由</th></tr></thead><tbody id="astock-popup-body"></tbody></table></div></div></div></div>`)
}

func writeAStockPageScript(b *strings.Builder, strategyDate string) {
	popupEligible := normalizeAStockStrategyDate(strategyDate) == aStockTodayDate()
	b.WriteString(`<script>(function(){`)
	fmt.Fprintf(b, `var scrollKey=%q;`, "astock-scroll-y")
	fmt.Fprintf(b, `var todayDate=%q;`, aStockTodayDate())
	fmt.Fprintf(b, `var popupDate=%q;`, normalizeAStockStrategyDate(strategyDate))
	fmt.Fprintf(b, `var popupEligible=%t;`, popupEligible)
	b.WriteString(`var popupKey="";var popupVisible=false;var popupTimer=0;var popupExactTimers=[];var partialLoading=false;`)
	b.WriteString(`function popupMask(){return document.getElementById("astock-popup-mask");}`)
	b.WriteString(`function popupBody(){return document.getElementById("astock-popup-body");}`)
	b.WriteString(`function hidePopup(){var mask=popupMask();if(mask){mask.hidden=true;}popupVisible=false;}`)
	b.WriteString(`function renderPopupRows(items){var body=popupBody();if(!body){return;}body.innerHTML="";(items||[]).forEach(function(item){var row=document.createElement("tr");["rank","code","name","hotspot","reason"].forEach(function(field){var cell=document.createElement("td");cell.textContent=item&&item[field]!==undefined&&item[field]!==null?String(item[field]):"";row.appendChild(cell);});body.appendChild(row);});}`)
	b.WriteString(`function showPopup(data){var mask=popupMask();if(!mask||!data||!data.show){return;}if(popupVisible&&popupKey===data.key){return;}var title=document.getElementById("astock-popup-title");var meta=document.getElementById("astock-popup-meta");if(title){title.textContent=data.title||"盘前推荐股票";}if(meta){meta.textContent=data.meta||"";}renderPopupRows(data.recommendations||[]);popupKey=data.key||"";mask.hidden=false;popupVisible=true;}`)
	b.WriteString(`function fetchPopup(){if(!popupEligible||!popupDate){return;}fetch("/a-stock/popup?date="+encodeURIComponent(popupDate),{credentials:"same-origin"}).then(function(resp){if(!resp.ok){return null;}return resp.json();}).then(function(data){if(!data){return;}if(data.show){showPopup(data);return;}if(!data.show&&popupVisible){hidePopup();}}).catch(function(){});}`)
	b.WriteString(`function clearPopupTimers(){if(popupTimer){window.clearInterval(popupTimer);popupTimer=0;}popupExactTimers.forEach(function(timer){window.clearTimeout(timer);});popupExactTimers=[];}`)
	b.WriteString(`function schedulePopupChecks(){clearPopupTimers();fetchPopup();if(!popupEligible){return;}popupTimer=window.setInterval(fetchPopup,5000);var now=new Date();[[9,27],[9,30],[9,31],[12,57],[13,1]].forEach(function(parts){var target=new Date();target.setHours(parts[0],parts[1],0,0);if(now<target){popupExactTimers.push(window.setTimeout(fetchPopup,Math.max(0,target.getTime()-now.getTime()+100)));}});}`)
	b.WriteString(`function content(){return document.getElementById("astock-page-content");}`)
	b.WriteString(`function partialURL(raw){var u=new URL(raw,window.location.origin);u.searchParams.set("partial","1");return u.toString();}`)
	b.WriteString(`function isPartialLink(anchor){if(!anchor||!anchor.href){return false;}if(anchor.target&&anchor.target!=="_self"){return false;}var u;try{u=new URL(anchor.href,window.location.origin);}catch(e){return false;}if(u.origin!==window.location.origin||u.pathname!=="/a-stock"){return false;}if(u.searchParams.get("refresh_recommendations")||u.searchParams.get("refresh_all_backtests")){return false;}return true;}`)
	b.WriteString(`function fallback(raw){window.location.href=raw;}`)
	b.WriteString(`function setLoading(on,el){var c=content();if(c){c.classList.toggle("astock-loading",!!on);}if(el){if(on){el.setAttribute("aria-busy","true");el.setAttribute("aria-disabled","true");}else{el.removeAttribute("aria-busy");el.removeAttribute("aria-disabled");}}}`)
	b.WriteString(`function applyPartial(data,push){if(!data||!data.html){throw new Error("empty partial");}var c=content();if(!c){throw new Error("missing content");}c.outerHTML=data.html;popupDate=data.date||popupDate;popupEligible=popupDate===todayDate;if(push&&data.canonical_url){history.pushState({astock:true},"",data.canonical_url);}bindAStockPage();schedulePopupChecks();}`)
	b.WriteString(`function loadPartial(raw,push,el){if(partialLoading){return;}partialLoading=true;setLoading(true,el);fetch(partialURL(raw),{credentials:"same-origin",headers:{"Accept":"application/json"}}).then(function(resp){var ct=resp.headers.get("Content-Type")||"";if(!resp.ok||ct.indexOf("application/json")<0){throw new Error("bad partial response");}return resp.json();}).then(function(data){applyPartial(data,push);}).catch(function(){fallback(raw);}).finally(function(){partialLoading=false;setLoading(false,el);});}`)
	b.WriteString(`function bindAStockPage(){document.querySelectorAll("[data-preserve-scroll='1']").forEach(function(el){el.addEventListener("click",function(){sessionStorage.setItem(scrollKey,String(window.scrollY||0));});});document.querySelectorAll("#astock-page-content a").forEach(function(anchor){if(anchor.dataset.astockPartialBound==="1"){return;}anchor.dataset.astockPartialBound="1";if(!isPartialLink(anchor)){return;}anchor.addEventListener("click",function(event){if(event.defaultPrevented||event.metaKey||event.ctrlKey||event.shiftKey||event.altKey){return;}event.preventDefault();loadPartial(anchor.href,true,anchor);});});document.querySelectorAll("#astock-page-content form[method='get']").forEach(function(form){if(form.dataset.astockPartialFormBound==="1"){return;}form.dataset.astockPartialFormBound="1";form.addEventListener("submit",function(event){var action=form.getAttribute("action")||window.location.pathname;var target=new URL(action,window.location.origin);if(target.origin!==window.location.origin||target.pathname!=="/a-stock"){return;}event.preventDefault();var data=new FormData(form);data.forEach(function(value,key){target.searchParams.set(key,String(value));});loadPartial(target.toString(),true,form.querySelector("button[type='submit']"));});});document.querySelectorAll(".astock-action-form").forEach(function(form){if(form.dataset.astockSubmitBound==="1"){return;}form.dataset.astockSubmitBound="1";form.addEventListener("submit",function(event){if(form.dataset.submitting==="1"){event.preventDefault();return;}form.dataset.submitting="1";var current=form.querySelector("button[type='submit']");document.querySelectorAll(".astock-action-form button[type='submit']").forEach(function(button){button.disabled=true;button.setAttribute("aria-disabled","true");});if(current){current.classList.add("astock-action-running");current.setAttribute("aria-busy","true");}});});}`)
	b.WriteString(`window.addEventListener("DOMContentLoaded",function(){var y=sessionStorage.getItem(scrollKey);if(y!==null){sessionStorage.removeItem(scrollKey);var n=parseInt(y,10);if(!isNaN(n)){window.scrollTo(0,n);}}bindAStockPage();var dismiss=document.getElementById("astock-popup-dismiss");if(dismiss){dismiss.addEventListener("click",function(){if(!popupKey){hidePopup();return;}fetch("/a-stock/popup/dismiss",{method:"POST",credentials:"same-origin",headers:{"Content-Type":"application/json"},body:JSON.stringify({key:popupKey})}).catch(function(){}).finally(function(){hidePopup();});});}schedulePopupChecks();});`)
	b.WriteString(`window.addEventListener("popstate",function(){loadPartial(window.location.href,false,null);});`)
	b.WriteString(`window.addEventListener("pageshow",function(){document.querySelectorAll(".astock-action-form").forEach(function(form){form.dataset.submitting="";});document.querySelectorAll(".astock-action-form button[type='submit']").forEach(function(button){button.disabled=false;button.removeAttribute("aria-disabled");button.removeAttribute("aria-busy");button.classList.remove("astock-action-running");});bindAStockPage();schedulePopupChecks();});`)
	b.WriteString(`window.addEventListener("beforeunload",function(){if(popupTimer){window.clearInterval(popupTimer);popupTimer=0;}popupExactTimers.forEach(function(timer){window.clearTimeout(timer);});popupExactTimers=[];});`)
	b.WriteString(`})();</script>`)
}

func renderAStockOverviewSection(b *strings.Builder, contexts []aStockContext) {
	auctionLabels := make([]string, 0, len(contexts))
	for _, ctx := range contexts {
		auctionLabels = append(auctionLabels, ctx.AuctionAmountLabel)
	}
	auctionAmount := firstNonEmpty(auctionLabels...)
	if strings.TrimSpace(auctionAmount) == "" {
		auctionAmount = "--"
	}
	b.WriteString(`<section><div class="astock-overview-header"><h2>顶部概览</h2><div class="astock-overview-summary">`)
	writeAStockOverviewSummaryItem(b, "策略日期", aStockContextsDisplayDate(contexts))
	writeAStockOverviewSummaryItem(b, "集合竞价金额", auctionAmount)
	b.WriteString(`</div></div><div class="astock-scroll"><table class="astock-overview-table"><tr>`)
	for i, ctx := range contexts {
		if i > 0 {
			b.WriteString(`</tr><tr>`)
		}
		writeAStockOverviewPeriodCells(b, ctx)
	}
	b.WriteString(`</tr></table></div></section>`)
}

func aStockContextsDisplayDate(contexts []aStockContext) string {
	for _, ctx := range contexts {
		if date := strings.TrimSpace(ctx.Date); date != "" {
			return date
		}
	}
	return aStockTodayDate()
}

func writeAStockOverviewSummaryItem(b *strings.Builder, label string, value string) {
	if strings.TrimSpace(value) == "" {
		value = "--"
	}
	b.WriteString(`<div><span class="astock-muted">`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</span><strong>`)
	b.WriteString(html.EscapeString(value))
	b.WriteString(`</strong></div>`)
}

func writeAStockOverviewPeriodCells(b *strings.Builder, ctx aStockContext) {
	writeAStockOverviewCell(b, "推荐窗口", ctx.PeriodLabel, ` class="astock-overview-period"`)
	writeAStockOverviewCell(b, "推荐生成窗口", nonEmpty(ctx.RecommendationWindowLabel, ctx.WindowLabel), ` class="astock-overview-window"`)
	writeAStockOverviewCell(b, "新闻统计窗口", ctx.WindowLabel, ` class="astock-overview-window"`)
	writeAStockOverviewNewsCountCell(b, ctx)
	writeAStockOverviewCell(b, "候选热点数", fmt.Sprintf("%d", len(ctx.Hotspots)), ` class="astock-overview-metric"`)
	writeAStockOverviewCell(b, "推荐股票数", fmt.Sprintf("%d", len(ctx.Recommendations)), ` class="astock-overview-metric"`)
	writeAStockOverviewFilterCell(b, ctx)
	writeAStockOverviewLimitUpFilterCell(b, ctx)
	writeAStockOverviewTodayMarketFilterCell(b, ctx)
	writeAStockOverviewFundFlowFilterCell(b, ctx)
	writeAStockOverviewRecalculateCell(b, ctx)
	writeAStockOverviewCell(b, "回测状态", aStockOverviewBacktestStatus(ctx), ` class="astock-overview-status"`)
}

func writeAStockOverviewNewsCountCell(b *strings.Builder, ctx aStockContext) {
	writeAStockOverviewCell(b, "财经新闻数", fmt.Sprintf("%d", aStockNewsCount(ctx)), ` class="astock-overview-metric"`)
}

func writeAStockOverviewFilterCell(b *strings.Builder, ctx aStockContext) {
	filterStatus := "已启用"
	if ctx.IgnoreRecent {
		filterStatus = "已关闭"
	}
	b.WriteString(`<td class="astock-overview-recent-filter"><span class="astock-muted">`)
	b.WriteString(html.EscapeString(aStockRecentLookbackLabel() + "内过滤"))
	b.WriteString(`</span><strong>`)
	b.WriteString(html.EscapeString(filterStatus))
	b.WriteString(`</strong><a class="astock-filter-toggle" data-preserve-scroll="1" href="`)
	b.WriteString(html.EscapeString(aStockFilterToggleHref(ctx.Date, ctx.Period, ctx.NewsPage, ctx.IgnoreRecent, ctx.IgnoreLimitUp, ctx.IgnoreFundFlow, ctx.TodayMarketFilterEnabled)))
	b.WriteString(`">`)
	b.WriteString(html.EscapeString(aStockFilterToggleLabel(ctx.IgnoreRecent)))
	b.WriteString(`</a></td>`)
}

func writeAStockOverviewTodayMarketFilterCell(b *strings.Builder, ctx aStockContext) {
	status := "不过滤"
	if ctx.TodayMarketFilterEnabled {
		status = "已启用"
		if ctx.NoTodayMarketCount > 0 {
			status = fmt.Sprintf("已过滤 %d", ctx.NoTodayMarketCount)
		}
	} else if ctx.NoTodayMarketCount > 0 {
		status = fmt.Sprintf("允许缺失 %d", ctx.NoTodayMarketCount)
	}
	b.WriteString(`<td class="astock-overview-market-filter"><span class="astock-muted">当日行情</span><strong>`)
	b.WriteString(html.EscapeString(status))
	b.WriteString(`</strong><form class="astock-filter-toggle-form" method="get" action="/a-stock"><input type="hidden" name="date" value="`)
	b.WriteString(html.EscapeString(ctx.Date))
	b.WriteString(`"><input type="hidden" name="period" value="`)
	b.WriteString(html.EscapeString(ctx.Period))
	b.WriteString(`">`)
	if ctx.NewsPage > 1 {
		b.WriteString(`<input type="hidden" name="news_page" value="`)
		b.WriteString(html.EscapeString(fmt.Sprintf("%d", ctx.NewsPage)))
		b.WriteString(`">`)
	}
	if ctx.IgnoreRecent {
		b.WriteString(`<input type="hidden" name="ignore_recent" value="1">`)
	}
	if ctx.IgnoreLimitUp {
		b.WriteString(`<input type="hidden" name="ignore_limit_up" value="1">`)
	}
	writeAStockFundFlowPreserveInput(b, ctx.Date, ctx.IgnoreFundFlow, ctx.FundFlowFilterExplicit)
	if !ctx.TodayMarketFilterEnabled {
		b.WriteString(`<input type="hidden" name="filter_today_market" value="1">`)
	}
	b.WriteString(`<button class="astock-filter-toggle" type="submit" data-preserve-scroll="1">`)
	b.WriteString(html.EscapeString(aStockTodayMarketFilterToggleLabel(ctx.TodayMarketFilterEnabled)))
	b.WriteString(`</button></form></td>`)
}

func writeAStockOverviewRecalculateCell(b *strings.Builder, ctx aStockContext) {
	b.WriteString(`<td class="astock-overview-recalculate"><span class="astock-muted">重新计算</span><strong>当前窗口</strong><form class="astock-filter-toggle-form" method="post"><input type="hidden" name="date" value="`)
	b.WriteString(html.EscapeString(ctx.Date))
	b.WriteString(`"><input type="hidden" name="period" value="`)
	b.WriteString(html.EscapeString(ctx.Period))
	b.WriteString(`"><input type="hidden" name="action" value="recalculate">`)
	if ctx.IgnoreRecent {
		b.WriteString(`<input type="hidden" name="ignore_recent" value="1">`)
	}
	if ctx.IgnoreLimitUp {
		b.WriteString(`<input type="hidden" name="ignore_limit_up" value="1">`)
	}
	writeAStockFundFlowPreserveInput(b, ctx.Date, ctx.IgnoreFundFlow, ctx.FundFlowFilterExplicit)
	if ctx.TodayMarketFilterEnabled {
		b.WriteString(`<input type="hidden" name="filter_today_market" value="1">`)
	}
	b.WriteString(`<button class="astock-filter-toggle" type="submit" data-preserve-scroll="1">重新计算</button></form></td>`)
}

func writeAStockOverviewFundFlowFilterCell(b *strings.Builder, ctx aStockContext) {
	status := "已启用"
	if ctx.IgnoreFundFlow {
		status = "已关闭"
	} else if ctx.FundFlowFiltered > 0 && ctx.FundFlowMissingCount > 0 {
		status = fmt.Sprintf("过滤 %d / 缺失 %d", ctx.FundFlowFiltered, ctx.FundFlowMissingCount)
	} else if ctx.FundFlowFiltered > 0 {
		status = fmt.Sprintf("已过滤 %d", ctx.FundFlowFiltered)
	} else if ctx.FundFlowMissingCount > 0 {
		status = fmt.Sprintf("缺失 %d", ctx.FundFlowMissingCount)
	}
	b.WriteString(`<td class="astock-overview-fund-filter"><span class="astock-muted">资金过滤</span><strong>`)
	b.WriteString(html.EscapeString(status))
	b.WriteString(`</strong><form class="astock-filter-toggle-form" method="get" action="/a-stock"><input type="hidden" name="date" value="`)
	b.WriteString(html.EscapeString(ctx.Date))
	b.WriteString(`"><input type="hidden" name="period" value="`)
	b.WriteString(html.EscapeString(ctx.Period))
	b.WriteString(`">`)
	if ctx.NewsPage > 1 {
		b.WriteString(`<input type="hidden" name="news_page" value="`)
		b.WriteString(html.EscapeString(fmt.Sprintf("%d", ctx.NewsPage)))
		b.WriteString(`">`)
	}
	if ctx.IgnoreRecent {
		b.WriteString(`<input type="hidden" name="ignore_recent" value="1">`)
	}
	if ctx.IgnoreLimitUp {
		b.WriteString(`<input type="hidden" name="ignore_limit_up" value="1">`)
	}
	writeAStockFundFlowToggleInput(b, !ctx.IgnoreFundFlow)
	if ctx.TodayMarketFilterEnabled {
		b.WriteString(`<input type="hidden" name="filter_today_market" value="1">`)
	}
	b.WriteString(`<button class="astock-filter-toggle" type="submit" data-preserve-scroll="1">`)
	b.WriteString(html.EscapeString(aStockFundFlowFilterToggleLabel(ctx.IgnoreFundFlow)))
	b.WriteString(`</button></form></td>`)
}

func writeAStockOverviewLimitUpFilterCell(b *strings.Builder, ctx aStockContext) {
	status := "不适用"
	if ctx.Period == "afternoon" && !ctx.LimitUpFilterEnabled {
		status = "已关闭"
	} else if ctx.LimitUpFilterEnabled {
		status = "已启用"
		if ctx.LimitUpFiltered > 0 {
			status = fmt.Sprintf("已过滤 %d", ctx.LimitUpFiltered)
		}
	}
	b.WriteString(`<td class="astock-overview-limit-filter"><span class="astock-muted">涨停过滤</span><strong>`)
	b.WriteString(html.EscapeString(status))
	b.WriteString(`</strong>`)
	if ctx.Period == "afternoon" {
		b.WriteString(`<form class="astock-filter-toggle-form" method="get" action="/a-stock"><input type="hidden" name="date" value="`)
		b.WriteString(html.EscapeString(ctx.Date))
		b.WriteString(`"><input type="hidden" name="period" value="afternoon">`)
		if ctx.NewsPage > 1 {
			b.WriteString(`<input type="hidden" name="news_page" value="`)
			b.WriteString(html.EscapeString(fmt.Sprintf("%d", ctx.NewsPage)))
			b.WriteString(`">`)
		}
		if ctx.IgnoreRecent {
			b.WriteString(`<input type="hidden" name="ignore_recent" value="1">`)
		}
		writeAStockFundFlowPreserveInput(b, ctx.Date, ctx.IgnoreFundFlow, ctx.FundFlowFilterExplicit)
		if ctx.TodayMarketFilterEnabled {
			b.WriteString(`<input type="hidden" name="filter_today_market" value="1">`)
		}
		if !ctx.IgnoreLimitUp {
			b.WriteString(`<input type="hidden" name="ignore_limit_up" value="1">`)
		}
		b.WriteString(`<button class="astock-filter-toggle" type="submit" data-preserve-scroll="1">`)
		b.WriteString(html.EscapeString(aStockLimitUpFilterToggleLabel(ctx.IgnoreLimitUp)))
		b.WriteString(`</button></form>`)
	}
	b.WriteString(`</td>`)
}

func aStockOverviewBacktestStatus(ctx aStockContext) string {
	status := strings.TrimSpace(ctx.BacktestStatus)
	if status == "" {
		status = "--"
	}
	reasons := make([]string, 0, 2)
	if ctx.RecentFiltered > 0 && !strings.Contains(status, aStockRecentLookbackStatusPrefix()) {
		reasons = append(reasons, fmt.Sprintf("%s过滤股票 %d", aStockRecentLookbackStatusPrefix(), ctx.RecentFiltered))
	}
	if ctx.SameDayMorningFiltered > 0 {
		reasons = append(reasons, fmt.Sprintf("过滤上午同股票/热点/日内名额 %d", ctx.SameDayMorningFiltered))
	}
	if ctx.ExDividendFiltered > 0 && !strings.Contains(status, "除权除息过滤") {
		reasons = append(reasons, fmt.Sprintf("除权除息过滤股票 %d", ctx.ExDividendFiltered))
	}
	if ctx.LimitUpFiltered > 0 {
		reasons = append(reasons, fmt.Sprintf("涨停过滤股票 %d", ctx.LimitUpFiltered))
	}
	if ctx.FundFlowFiltered > 0 && !strings.Contains(status, "资金过滤") {
		reasons = append(reasons, fmt.Sprintf("资金过滤股票 %d", ctx.FundFlowFiltered))
	}
	if ctx.FundFlowMissingCount > 0 && !strings.Contains(status, "资金数据缺失") {
		reasons = append(reasons, fmt.Sprintf("资金数据缺失 %d", ctx.FundFlowMissingCount))
	}
	if ctx.NoTodayMarketCount > 0 && !strings.Contains(status, "当日行情") {
		if ctx.TodayMarketFilterEnabled {
			reasons = append(reasons, fmt.Sprintf("过滤无当日行情股票 %d", ctx.NoTodayMarketCount))
		} else {
			reasons = append(reasons, fmt.Sprintf("允许无当日行情股票 %d", ctx.NoTodayMarketCount))
		}
	}
	if len(reasons) > 0 {
		status += "，" + strings.Join(reasons, "，")
	}
	return status
}

func writeAStockOverviewCell(b *strings.Builder, label string, value string, attrs string) {
	b.WriteString(`<td`)
	b.WriteString(attrs)
	b.WriteString(`><span class="astock-muted">`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</span><strong>`)
	b.WriteString(html.EscapeString(value))
	b.WriteString(`</strong>`)
	b.WriteString(`</td>`)
}

func aStockNewsCount(ctx aStockContext) int {
	if ctx.NewsTotal > 0 {
		return ctx.NewsTotal
	}
	return len(aStockNewsArticles(ctx))
}

func aStockNewsArticles(ctx aStockContext) []model.Item {
	if ctx.NewsArticles != nil {
		return ctx.NewsArticles
	}
	return ctx.Articles
}

func renderAStockNewsSections(b *strings.Builder, contexts []aStockContext) {
	b.WriteString(`<section>`)
	renderAStockNewsStatsContent(b, contexts)
	b.WriteString(`</section>`)
}

func renderAStockNewsStatsContent(b *strings.Builder, contexts []aStockContext) {
	b.WriteString(`<h2>财经新闻来源统计</h2><div class="astock-news-grid">`)
	for _, ctx := range contexts {
		renderAStockNewsWindow(b, ctx)
	}
	b.WriteString(`</div>`)
}

func renderAStockNewsWindow(b *strings.Builder, ctx aStockContext) {
	newsArticles := aStockNewsArticles(ctx)
	b.WriteString(`<div class="astock-news-window"><h3>`)
	b.WriteString(html.EscapeString(ctx.WindowLabel))
	b.WriteString(` 财经新闻</h3>`)
	if len(newsArticles) == 0 {
		if aStockNewsWindowPending(ctx) {
			b.WriteString(`<div class="astock-empty">暂无数据：`)
			b.WriteString(html.EscapeString(ctx.Date))
			b.WriteString(` `)
			b.WriteString(html.EscapeString(ctx.WindowLabel))
			b.WriteString(` 窗口尚未开始，开始后自动抓取/入库的数据会计入这里。</div>`)
		} else {
			b.WriteString(`<div class="astock-empty">暂无数据：请点击“抓取 A 股新闻”，或确认 `)
			b.WriteString(html.EscapeString(ctx.Date))
			b.WriteString(` `)
			b.WriteString(html.EscapeString(ctx.WindowLabel))
			b.WriteString(` 窗口内已有财经新闻源入库。</div>`)
		}
	}
	b.WriteString(`<div class="astock-scroll"><table class="astock-news-table"><tr><th>来源</th><th>原始地址</th><th>新闻条数</th><th>最近抓取</th><th>抓取/入库/更新 `)
	renderAStockTooltip(b, "源站抓取数 / 入库新增数 / 更新数")
	b.WriteString(`</th><th>开关</th><th>推荐</th></tr>`)
	for _, source := range summarizeAStockNewsSources(newsArticles, ctx.SourceRuns) {
		note := strings.TrimSpace(formatAStockCrawlRunNote(source.Run))
		diagnostic := strings.TrimSpace(formatAStockNewsSourceDiagnostic(source, ctx))
		if diagnostic != "" {
			if note != "" {
				note += "；" + diagnostic
			} else {
				note = diagnostic
			}
		}
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(source.Label))
		b.WriteString(`</td><td>`)
		renderAStockNewsSourceURL(b, source)
		b.WriteString(`</td><td>`)
		b.WriteString(fmt.Sprintf("%d条", source.Count))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(formatAStockCrawlRunStatus(source.Run)))
		b.WriteString(`</td><td>`)
		b.WriteString(`<span class="astock-news-counts">`)
		b.WriteString(html.EscapeString(formatAStockCrawlRunCounts(source.Run)))
		if note != "" {
			renderAStockTooltip(b, note)
		}
		b.WriteString(`</span>`)
		if diagnostic != "" {
			b.WriteString(`<div class="astock-news-diagnostic">`)
			b.WriteString(html.EscapeString(diagnostic))
			b.WriteString(`</div>`)
		}
		b.WriteString(`</td><td>`)
		renderAStockNewsSourceToggle(b, ctx.Date, source, "crawl", source.CrawlEnabled)
		b.WriteString(`</td><td>`)
		renderAStockNewsSourceToggle(b, ctx.Date, source, "recommendation", source.RecommendationEnabled)
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></div></div>`)
}

func renderAStockNewsSourceURL(b *strings.Builder, source aStockNewsSourceCount) {
	urlText := strings.TrimSpace(source.URL)
	if urlText == "" {
		b.WriteString(`--`)
		return
	}
	b.WriteString(`<a class="astock-source-link" href="`)
	b.WriteString(html.EscapeString(urlText))
	b.WriteString(`" target="_blank" rel="noreferrer">原始地址</a>`)
}

func renderAStockNewsSourceToggle(b *strings.Builder, strategyDate string, source aStockNewsSourceCount, field string, enabled bool) {
	next := "1"
	label := "关"
	className := "astock-source-toggle off"
	if enabled {
		next = "0"
		label = "开"
		className = "astock-source-toggle on"
	}
	b.WriteString(`<form class="astock-source-toggle-form" method="post" action="/system"><input type="hidden" name="form_type" value="astock_news_source_setting"><input type="hidden" name="section" value="newsstats"><input type="hidden" name="strategy_date" value="`)
	b.WriteString(html.EscapeString(normalizeAStockStrategyDate(strategyDate)))
	b.WriteString(`"><input type="hidden" name="source_type" value="`)
	b.WriteString(html.EscapeString(source.SourceType))
	b.WriteString(`"><input type="hidden" name="setting" value="`)
	b.WriteString(html.EscapeString(field))
	b.WriteString(`"><input type="hidden" name="enabled" value="`)
	b.WriteString(next)
	b.WriteString(`"><button class="`)
	b.WriteString(className)
	b.WriteString(`" type="submit">`)
	b.WriteString(label)
	b.WriteString(`</button></form>`)
}

const aStockSourceCoverageStaleAfter = 30 * time.Minute

func formatAStockNewsSourceDiagnostic(source aStockNewsSourceCount, ctx aStockContext) string {
	if source.Count > 0 || strings.TrimSpace(source.Run.SourceType) == "" || source.Run.StartedAt.IsZero() || strings.TrimSpace(source.Run.ErrorText) != "" {
		return ""
	}
	deadline, ok := aStockNewsCoverageDeadline(ctx)
	if !ok {
		return ""
	}
	if source.Run.StartedAt.Before(deadline.Add(-aStockSourceCoverageStaleAfter)) {
		return "最近抓取早于统计截止，可能未覆盖后续新闻"
	}
	return ""
}

func aStockNewsWindowPending(ctx aStockContext) bool {
	start := ctx.NewsWindowStart
	if start.IsZero() {
		start = ctx.WindowStart
	}
	if start.IsZero() {
		return false
	}
	return aStockNow().Before(start)
}

func aStockNewsCoverageDeadline(ctx aStockContext) (time.Time, bool) {
	start := ctx.NewsWindowStart
	end := ctx.NewsWindowEnd
	if start.IsZero() || end.IsZero() {
		start = ctx.WindowStart
		end = ctx.WindowEnd
	}
	if start.IsZero() || end.IsZero() {
		return time.Time{}, false
	}
	now := aStockNow()
	if now.Before(start) {
		return time.Time{}, false
	}
	if now.After(start) && now.Before(end) {
		return now, true
	}
	return end, true
}

func renderAStockTooltip(b *strings.Builder, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	escaped := html.EscapeString(text)
	b.WriteString(`<span class="astock-help" tabindex="0" title="`)
	b.WriteString(escaped)
	b.WriteString(`" aria-label="`)
	b.WriteString(escaped)
	b.WriteString(`">?<span class="astock-help-text" role="tooltip">`)
	b.WriteString(escaped)
	b.WriteString(`</span></span>`)
}

func summarizeAStockNewsSources(items []model.Item, runs []aStockSourceRun) []aStockNewsSourceCount {
	counts := make(map[string]int)
	for _, item := range items {
		sourceType := provider.CanonicalSourceType(item.SourceType)
		if sourceType == "" {
			sourceType = "unknown"
		}
		counts[sourceType]++
	}
	runBySource := make(map[string]aStockSourceRun, len(runs))
	for _, run := range runs {
		sourceType := provider.CanonicalSourceType(run.SourceType)
		if sourceType != "" {
			run.SourceType = sourceType
			runBySource[sourceType] = run
		}
	}
	settings, _ := astocknews.LoadSettings("")
	sources := astocknews.Sources()
	summary := make([]aStockNewsSourceCount, 0, len(sources))
	for _, source := range sources {
		sourceType := source.Type
		setting := settings[sourceType]
		summary = append(summary, aStockNewsSourceCount{
			SourceType:            sourceType,
			Label:                 source.Label,
			URL:                   source.URL,
			Count:                 counts[sourceType],
			Run:                   runBySource[sourceType],
			CrawlEnabled:          setting.CrawlEnabled,
			RecommendationEnabled: setting.RecommendationEnabled,
		})
	}
	return summary
}

func renderAStockNewsPagination(b *strings.Builder, ctx aStockContext) {
	if ctx.NewsTotalPages <= 1 {
		return
	}
	b.WriteString(`<div class="astock-pagination"><span class="astock-muted">新闻分页：`)
	b.WriteString(fmt.Sprintf("%d/%d，共%d条", ctx.NewsPage, ctx.NewsTotalPages, ctx.NewsTotal))
	b.WriteString(`</span>`)
	renderAStockNewsPageLink(b, ctx, ctx.NewsPage-1, "上一页", ctx.NewsPage <= 1)
	for page := 1; page <= ctx.NewsTotalPages; page++ {
		renderAStockNewsPageLink(b, ctx, page, fmt.Sprintf("%d", page), false)
	}
	renderAStockNewsPageLink(b, ctx, ctx.NewsPage+1, "下一页", ctx.NewsPage >= ctx.NewsTotalPages)
	b.WriteString(`</div>`)
}

func renderAStockNewsPageLink(b *strings.Builder, ctx aStockContext, page int, label string, disabled bool) {
	b.WriteString(`<a class="astock-tab`)
	if page == ctx.NewsPage && !disabled {
		b.WriteString(` active`)
	}
	if disabled {
		b.WriteString(` disabled`)
	}
	b.WriteString(`" href="`)
	if disabled {
		b.WriteString(`#`)
	} else {
		b.WriteString(aStockPageHref(ctx.Date, ctx.Period, page, ctx.IgnoreRecent, ctx.IgnoreLimitUp, ctx.IgnoreFundFlow, ctx.TodayMarketFilterEnabled))
	}
	b.WriteString(`">`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</a>`)
}

func renderAStockHotspotSection(b *strings.Builder, hotspots []aStockHotspot) {
	b.WriteString(`<section><h2>热点归纳</h2><p><span class="astock-badge">规则+词典</span><span class="astock-badge">可复现回测</span></p>`)
	if len(hotspots) == 0 {
		b.WriteString(`<div class="astock-empty">暂无数据：当前新闻窗口未命中 A 股热点词典。</div><table><tr><th>热点</th><th>关键词</th><th>热度分</th><th>证据新闻数</th><th>推荐排名前9股票</th></tr><tr><td colspan="5">暂无热点</td></tr></table></section>`)
		return
	}
	b.WriteString(`<table><tr><th>热点</th><th>关键词</th><th>热度分</th><th>证据新闻数</th><th>推荐排名前9股票</th></tr>`)
	for _, hotspot := range hotspots {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(hotspot.Name))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(strings.Join(hotspot.Keywords, "、")))
		b.WriteString(`</td><td>`)
		b.WriteString(fmt.Sprintf("%d", hotspot.Score))
		b.WriteString(`</td><td>`)
		b.WriteString(fmt.Sprintf("%d", hotspot.Evidence))
		b.WriteString(`</td><td>`)
		renderAStockHotspotTopStocks(b, hotspot.TopStocks)
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></section>`)
}

func renderAStockHotspotTopStocks(b *strings.Builder, stocks []aStockHotspotStock) {
	if len(stocks) == 0 {
		b.WriteString(`--`)
		return
	}
	b.WriteString(`<div class="astock-hotspot-stocks">`)
	for _, stock := range stocks {
		b.WriteString(`<span class="astock-hotspot-stock">`)
		b.WriteString(html.EscapeString(strings.TrimSpace(stock.Code)))
		if name := strings.TrimSpace(stock.Name); name != "" {
			b.WriteString(` `)
			b.WriteString(html.EscapeString(name))
		}
		if recommendationDate := strings.TrimSpace(stock.RecommendationDate); recommendationDate != "" {
			b.WriteString(`<span class="astock-hotspot-date">（`)
			b.WriteString(html.EscapeString(recommendationDate))
			b.WriteString(`）</span>`)
		}
		b.WriteString(`</span>`)
	}
	b.WriteString(`</div>`)
}

func renderAStockRecommendationSection(b *strings.Builder, contexts []aStockContext) {
	b.WriteString(`<section><h2>推荐股票</h2>`)
	for i, ctx := range contexts {
		if i > 0 {
			b.WriteString(`<hr class="astock-section-divider">`)
		}
		renderAStockRecommendationSubsection(b, ctx)
	}
	b.WriteString(`</section>`)
}

func renderAStockRecommendationSubsection(b *strings.Builder, ctx aStockContext) {
	b.WriteString(`<h3>`)
	b.WriteString(html.EscapeString(ctx.PeriodLabel))
	b.WriteString(`</h3>`)
	recommendations := ctx.Recommendations
	if len(recommendations) == 0 {
		reason := strings.TrimSpace(ctx.EmptyReason)
		if reason == "" {
			reason = "暂无数据：当前新闻窗口未生成热点映射股票，或候选股票被行情、当日开盘价、30/60天跌幅过滤。"
		}
		b.WriteString(`<div class="astock-empty">`)
		b.WriteString(html.EscapeString(reason))
		b.WriteString(`</div><div class="astock-scroll"><table class="astock-table astock-recommendation-table"><tr><th>排名</th><th>热点</th><th>股票代码</th><th>股票名称</th><th>昨日收盘价</th><th>昨日涨跌幅</th><th>30天涨跌幅</th><th>60天涨跌幅</th><th>现价</th><th>今日涨跌幅</th><th>5日资金动向</th><th>推荐理由</th></tr><tr><td colspan="12">暂无推荐股票</td></tr></table></div>`)
		return
	}
	b.WriteString(`<div class="astock-scroll"><table class="astock-table astock-recommendation-table"><tr><th>排名</th><th>热点</th><th>股票代码</th><th>股票名称</th><th>昨日收盘价</th><th>昨日涨跌幅</th><th>30天涨跌幅</th><th>60天涨跌幅</th><th>现价</th><th>今日涨跌幅</th><th>5日资金动向</th><th>推荐理由</th></tr>`)
	for _, rec := range recommendations {
		b.WriteString(`<tr><td>`)
		b.WriteString(fmt.Sprintf("%d", rec.Rank))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(rec.Hotspot))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(rec.Code))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(rec.Name))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(rec.PrevClose))
		b.WriteString(`</td><td><span class="`)
		b.WriteString(html.EscapeString(rec.PrevPctClass))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(rec.PrevPct))
		b.WriteString(`</span>`)
		b.WriteString(`</td><td><span class="`)
		b.WriteString(html.EscapeString(rec.Change30Class))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(rec.Change30))
		b.WriteString(`</span>`)
		b.WriteString(`</td><td><span class="`)
		b.WriteString(html.EscapeString(rec.Change60Class))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(rec.Change60))
		b.WriteString(`</span>`)
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(rec.CurrentPrice))
		b.WriteString(`</td><td><span class="`)
		b.WriteString(html.EscapeString(rec.TodayPctClass))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(rec.TodayPct))
		b.WriteString(`</span>`)
		b.WriteString(`</td>`)
		writeAStockRecommendationDetailCell(b, rec)
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></div>`)
}

func writeAStockRecommendationDetailCell(b *strings.Builder, rec aStockRecommendation) {
	b.WriteString(`<td class="astock-recommendation-detail-cell" colspan="2"><div class="astock-recommendation-fundflow"><span class="`)
	b.WriteString(html.EscapeString(rec.FundFlow5DClass))
	b.WriteString(`">`)
	b.WriteString(html.EscapeString(rec.FundFlow5D))
	b.WriteString(`</span></div>`)
	writeAStockRecommendationReasonCell(b, rec)
}

func writeAStockRecommendationReasonCell(b *strings.Builder, rec aStockRecommendation) {
	components := aStockRecommendationScoreBreakdown(rec)
	if len(components) == 0 {
		b.WriteString(html.EscapeString(rec.Reason))
		return
	}
	components = aStockRecommendationDisplayScoreBreakdown(rec, components)
	rows := aStockRecommendationScoreRows(rec, components)
	total := aStockRecommendationScoreTotal(rec, components)
	b.WriteString(`<div class="astock-score-total"><span>总分</span><span>`)
	b.WriteString(fmt.Sprintf("%d/1000 分", total))
	b.WriteString(`</span></div><table class="astock-score-table"><tr><th>类别</th><th>项目</th><th>命中/依据</th><th>分值</th><th>小计</th></tr>`)
	summary := newAStockScoreCategorySummary()
	for _, row := range rows {
		if row.Kind == aStockScoreRowComponent {
			summary.add(row.Category, row.Score)
		}
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(row.Category.Label))
		b.WriteString(`</td><td class="astock-score-category">`)
		b.WriteString(html.EscapeString(row.Label))
		b.WriteString(`</td><td class="astock-score-detail">`)
		b.WriteString(html.EscapeString(row.Detail))
		b.WriteString(`</td><td class="astock-score-value">`)
		b.WriteString(html.EscapeString(row.Value))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(formatAStockScoreComponentScore(row.Score)))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table>`)
	writeAStockScoreCategorySummary(b, summary, total, rec)
	if strings.TrimSpace(rec.Reason) != "" {
		b.WriteString(`<div class="astock-score-reason">`)
		b.WriteString(html.EscapeString(rec.Reason))
		b.WriteString(`</div>`)
	}
}

func formatAStockScoreComponentScore(score int) string {
	return fmt.Sprintf("%d 分", score)
}

type aStockScoreComponentCategory struct {
	Key   string
	Label string
}

type aStockScoreCategorySummary struct {
	order  []aStockScoreComponentCategory
	totals map[string]int
}

type aStockScoreRowKind int

const (
	aStockScoreRowComponent aStockScoreRowKind = iota
	aStockScoreRowSubtotal
)

type aStockScoreRow struct {
	Kind     aStockScoreRowKind
	Category aStockScoreComponentCategory
	Label    string
	Detail   string
	Value    string
	Score    int
}

func newAStockScoreCategorySummary() aStockScoreCategorySummary {
	return aStockScoreCategorySummary{
		order: []aStockScoreComponentCategory{
			{Key: aStockScoreFactorSector, Label: "版块资金因子"},
			{Key: aStockScoreFactorFund, Label: "个股资金因子"},
			{Key: aStockScoreFactorEmotion, Label: "情绪因子"},
			{Key: aStockScoreFactorAuction, Label: "竞价因子"},
			{Key: aStockScoreFactorVolatility, Label: "波动因子"},
			{Key: "history", Label: "历史修正"},
		},
		totals: make(map[string]int, 6),
	}
}

func (summary aStockScoreCategorySummary) add(category aStockScoreComponentCategory, score int) {
	if summary.totals == nil {
		return
	}
	summary.totals[category.Key] += score
}

func writeAStockScoreCategorySummary(b *strings.Builder, summary aStockScoreCategorySummary, total int, rec aStockRecommendation) {
	settings := defaultAStockAlgorithmSettings()
	b.WriteString(`<div class="astock-score-summary">`)
	calculatedTotal := 0
	for _, category := range summary.order {
		rawValue := summary.totals[category.Key]
		if category.Key == aStockScoreFactorHistory && rawValue == 0 {
			continue
		}
		value := rawValue
		suffix := ""
		if isAStockPrimaryScoreFactor(category.Key) {
			cap := aStockScoreFactorCapWithSettings(category.Key, settings)
			value = clampAStockScoreFactorSubtotal(rawValue, cap)
			calculatedTotal += value
			suffix = fmt.Sprintf("/%d", cap)
		}
		b.WriteString(`<span>`)
		b.WriteString(html.EscapeString(category.Label))
		b.WriteString(`：`)
		if suffix != "" {
			b.WriteString(html.EscapeString(fmt.Sprintf("%d%s 分", value, suffix)))
		} else {
			b.WriteString(html.EscapeString(formatAStockScoreComponentScore(value)))
		}
		b.WriteString(`</span>`)
	}
	if calculatedTotal < 0 {
		calculatedTotal = 0
	}
	if calculatedTotal > 1000 {
		calculatedTotal = 1000
	}
	_ = rec
	b.WriteString(`<span>合计：`)
	b.WriteString(html.EscapeString(fmt.Sprintf("%d/1000 分", total)))
	b.WriteString(`</span></div>`)
}

func aStockRecommendationScoreRows(rec aStockRecommendation, components []aStockRecommendationScoreComponent) []aStockScoreRow {
	grouped := make(map[string][]aStockScoreRow, len(components))
	other := make([]aStockScoreRow, 0)
	for _, component := range components {
		category := aStockScoreComponentCategoryFor(component)
		row := aStockScoreRow{
			Kind:     aStockScoreRowComponent,
			Category: category,
			Label:    component.Label,
			Detail:   component.Detail,
			Value:    formatAStockScoreComponentUnitValueText(component),
			Score:    component.Score,
		}
		if category.Key == "" {
			other = append(other, row)
		} else {
			grouped[category.Key] = append(grouped[category.Key], row)
		}
	}
	_ = rec
	rows := make([]aStockScoreRow, 0, len(components)+8)
	for _, category := range newAStockScoreCategorySummary().order {
		rows = append(rows, grouped[category.Key]...)
		delete(grouped, category.Key)
	}
	for _, categoryRows := range grouped {
		rows = append(rows, categoryRows...)
	}
	rows = append(rows, other...)
	return rows
}

func insertAStockReasonStageTableRows(rows []aStockScoreRow, stageRows []aStockScoreRow) []aStockScoreRow {
	for _, stage := range stageRows {
		switch stage.Label {
		case "热度分":
			rows = insertAStockScoreRowAfter(rows, stage, func(row aStockScoreRow) bool {
				return isAStockHotspotScoreRow(row.Label)
			})
		case "匹配分", "行情分":
			rows = insertAStockScoreRowAfter(rows, stage, func(row aStockScoreRow) bool {
				return isAStockMatchScoreRow(row.Label)
			})
		case "综合分":
			rows = insertAStockScoreRowAfter(rows, stage, func(row aStockScoreRow) bool {
				return row.Label == "匹配分" || row.Label == "行情分" || isAStockMatchScoreRow(row.Label)
			})
		default:
			rows = append(rows, stage)
		}
	}
	return rows
}

func insertAStockScoreRowAfter(rows []aStockScoreRow, row aStockScoreRow, match func(aStockScoreRow) bool) []aStockScoreRow {
	index := -1
	for i, candidate := range rows {
		if match(candidate) {
			index = i
		}
	}
	if index < 0 {
		return append(rows, row)
	}
	rows = append(rows, aStockScoreRow{})
	copy(rows[index+2:], rows[index+1:])
	rows[index+1] = row
	return rows
}

func isAStockHotspotScoreRow(label string) bool {
	switch strings.TrimSpace(label) {
	case "新闻热度", "热点关键词", "热度修正", "热点热度":
		return true
	default:
		return false
	}
}

func isAStockMatchScoreRow(label string) bool {
	switch strings.TrimSpace(label) {
	case "行情排名", "个股证据", "股票名命中", "弱证据", "匹配修正", "匹配差额":
		return true
	default:
		return false
	}
}

func aStockReasonStageTableRows(reason string) []aStockScoreRow {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil
	}
	sectorCategory := aStockScoreComponentCategory{Key: "sector-stage", Label: "板块"}
	stockCategory := aStockScoreComponentCategory{Key: "stock-stage", Label: "个股"}
	rows := make([]aStockScoreRow, 0, 3)
	hotspotScore := 0
	hasHotspotScore := false
	if matches := aStockReasonHotspotScorePattern.FindStringSubmatch(reason); len(matches) == 4 {
		keywords := strings.TrimSpace(matches[1])
		newsCount := atoiAStockScorePart(matches[2])
		hotspotScore = atoiAStockScorePart(matches[3])
		hasHotspotScore = true
		rows = append(rows, aStockScoreRow{
			Kind:     aStockScoreRowSubtotal,
			Category: sectorCategory,
			Label:    "热度分",
			Detail:   fmt.Sprintf("命中 %s；证据新闻 %d 条；热度分 %d", keywords, newsCount, hotspotScore),
			Value:    "--",
			Score:    hotspotScore,
		})
	}
	matchLabel := ""
	matchScore := 0
	matchEvidence := 0
	if matches := aStockReasonMarketScorePattern.FindStringSubmatch(reason); len(matches) == 4 {
		rank := atoiAStockScorePart(matches[1])
		matchEvidence = atoiAStockScorePart(matches[2])
		matchScore = atoiAStockScorePart(matches[3])
		matchLabel = "行情分"
		rows = append(rows, aStockScoreRow{
			Kind:     aStockScoreRowSubtotal,
			Category: stockCategory,
			Label:    matchLabel,
			Detail:   fmt.Sprintf("行情排名 %d；个股证据 %d 条；行情分 %d", rank, matchEvidence, matchScore),
			Value:    "--",
			Score:    matchScore,
		})
	} else if matches := aStockReasonMatchedScorePattern.FindStringSubmatch(reason); len(matches) == 3 {
		matchEvidence = atoiAStockScorePart(matches[1])
		matchScore = atoiAStockScorePart(matches[2])
		matchLabel = "匹配分"
		detail := fmt.Sprintf("个股证据 %d 条；匹配分 %d", matchEvidence, matchScore)
		if strings.Contains(reason, "使用实时新闻明确提及股票") {
			detail = "使用实时新闻明确提及股票，" + detail
		}
		rows = append(rows, aStockScoreRow{
			Kind:     aStockScoreRowSubtotal,
			Category: stockCategory,
			Label:    matchLabel,
			Detail:   detail,
			Value:    "--",
			Score:    matchScore,
		})
	}
	if matches := aStockReasonComprehensiveScorePattern.FindStringSubmatch(reason); len(matches) == 2 {
		comprehensiveScore := atoiAStockScorePart(matches[1])
		detail := fmt.Sprintf("综合分 %d", comprehensiveScore)
		if hasHotspotScore && matchLabel != "" {
			detail = fmt.Sprintf("热度分 %d + %s %d = 综合分 %d", hotspotScore, matchLabel, matchScore, comprehensiveScore)
		}
		rows = append(rows, aStockScoreRow{
			Kind:     aStockScoreRowSubtotal,
			Category: stockCategory,
			Label:    "综合分",
			Detail:   detail,
			Value:    "--",
			Score:    comprehensiveScore,
		})
	}
	return rows
}

func aStockReasonStageSubtotalRows(reason string) []aStockScoreRow {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil
	}
	subtotalCategory := aStockScoreComponentCategory{Key: "subtotal", Label: "小计"}
	rows := make([]aStockScoreRow, 0, 2)
	if matches := aStockReasonMarketScorePattern.FindStringSubmatch(reason); len(matches) == 4 {
		rows = append(rows, aStockScoreRow{
			Kind:     aStockScoreRowSubtotal,
			Category: subtotalCategory,
			Label:    "行情分",
			Detail:   "个股阶段小计",
			Value:    "--",
			Score:    atoiAStockScorePart(matches[3]),
		})
	} else if matches := aStockReasonMatchedScorePattern.FindStringSubmatch(reason); len(matches) == 3 {
		rows = append(rows, aStockScoreRow{
			Kind:     aStockScoreRowSubtotal,
			Category: subtotalCategory,
			Label:    "匹配分",
			Detail:   "个股阶段小计",
			Value:    "--",
			Score:    atoiAStockScorePart(matches[2]),
		})
	}
	if matches := aStockReasonComprehensiveScorePattern.FindStringSubmatch(reason); len(matches) == 2 {
		rows = append(rows, aStockScoreRow{
			Kind:     aStockScoreRowSubtotal,
			Category: subtotalCategory,
			Label:    "综合分",
			Detail:   "基础阶段小计",
			Value:    "--",
			Score:    atoiAStockScorePart(matches[1]),
		})
	}
	return rows
}

func aStockRecommendationDisplayScoreBreakdown(rec aStockRecommendation, components []aStockRecommendationScoreComponent) []aStockRecommendationScoreComponent {
	total := aStockRecommendationScoreTotal(rec, components)
	if total == 0 {
		return components
	}
	components = mergeAStockRecommendationDisplayScoreComponents(components, parseAStockRecommendationScoreBreakdown(rec))
	calculated := aStockRecommendationDisplayCalculatedTotal(components, defaultAStockAlgorithmSettings())
	if calculated == total {
		return components
	}
	display := make([]aStockRecommendationScoreComponent, 0, len(components)+1)
	display = append(display, components...)
	display = append(display, newAStockScoreComponentWithFactor(aStockScoreFactorHistory, "总分修正", fmt.Sprintf("保存总分 %d", total), total-calculated, total-calculated))
	return display
}

func aStockRecommendationDisplayCalculatedTotal(components []aStockRecommendationScoreComponent, settings model.AStockRecommendationAlgorithmSettings) int {
	total := aStockRecommendationFactorScoreTotalWithSettings(components, settings)
	for _, component := range components {
		if aStockScoreComponentFactor(component) == aStockScoreFactorHistory {
			total += component.Score
		}
	}
	return total
}

func mergeAStockRecommendationDisplayScoreComponents(saved []aStockRecommendationScoreComponent, parsed []aStockRecommendationScoreComponent) []aStockRecommendationScoreComponent {
	merged := make([]aStockRecommendationScoreComponent, 0, len(saved)+len(parsed))
	seen := make(map[string]struct{}, len(saved)+len(parsed))
	savedLabels := make(map[string]struct{}, len(saved))
	appendComponent := func(component aStockRecommendationScoreComponent, fromSaved bool) {
		label := strings.TrimSpace(component.Label)
		if label == "" || label == "总分修正" {
			return
		}
		component.Label = label
		key := aStockScoreComponentKey(component)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		if fromSaved {
			savedLabels[label] = struct{}{}
		}
		merged = append(merged, component)
	}
	for _, component := range saved {
		appendComponent(component, true)
	}
	for _, component := range parsed {
		label := strings.TrimSpace(component.Label)
		if shouldSkipParsedAStockScoreComponent(label, savedLabels) {
			continue
		}
		appendComponent(component, false)
	}
	return merged
}

func shouldSkipParsedAStockScoreComponent(label string, seenLabels map[string]struct{}) bool {
	if _, exists := seenLabels[label]; !exists {
		if label == "匹配修正" || label == "匹配差额" {
			for _, sourceLabel := range []string{"行情排名", "个股证据", "股票名命中", "弱证据"} {
				if _, sourceExists := seenLabels[sourceLabel]; sourceExists {
					return true
				}
			}
		}
		if label == "热度修正" {
			for _, sourceLabel := range []string{"新闻热度", "热点关键词", "热点热度"} {
				if _, sourceExists := seenLabels[sourceLabel]; sourceExists {
					return true
				}
			}
		}
		return false
	}
	switch label {
	case "新闻热度", "热点关键词", "负面新闻", "热度修正", "热点热度", "行情排名", "个股证据", "股票名命中", "弱证据", "匹配修正", "匹配差额", "机构持仓", "资金动向", "资金强度", "开盘盘口", "当日高开", "昨日涨停", "昨日涨幅过高", "动能趋势", "板块资金趋势", "板块资金共振", "板块回撤":
		return true
	default:
		return false
	}
}

func aStockScoreComponentCategoryFor(component aStockRecommendationScoreComponent) aStockScoreComponentCategory {
	switch aStockScoreComponentFactor(component) {
	case aStockScoreFactorAuction:
		return aStockScoreComponentCategory{Key: aStockScoreFactorAuction, Label: "竞价因子"}
	case aStockScoreFactorEmotion:
		return aStockScoreComponentCategory{Key: aStockScoreFactorEmotion, Label: "情绪因子"}
	case aStockScoreFactorSector:
		return aStockScoreComponentCategory{Key: aStockScoreFactorSector, Label: "版块资金因子"}
	case aStockScoreFactorFund:
		return aStockScoreComponentCategory{Key: aStockScoreFactorFund, Label: "个股资金因子"}
	case aStockScoreFactorVolatility:
		return aStockScoreComponentCategory{Key: aStockScoreFactorVolatility, Label: "波动因子"}
	case aStockScoreFactorHistory:
		return aStockScoreComponentCategory{Key: aStockScoreFactorHistory, Label: "历史修正"}
	default:
		return aStockScoreComponentCategory{Key: aStockScoreFactorHistory, Label: "历史修正"}
	}
}

func aStockScoreComponentFactor(component aStockRecommendationScoreComponent) string {
	if factor := normalizeAStockScoreFactor(component.Factor); factor != "" {
		return factor
	}
	switch strings.TrimSpace(component.Label) {
	case "行情排名", "开盘盘口", "当日高开", "当日低开":
		return aStockScoreFactorAuction
	case "新闻热度", "热点关键词", "负面新闻", "热度修正", "热点热度", "个股证据", "股票名命中", "弱证据", "机构持仓":
		return aStockScoreFactorEmotion
	case "板块资金趋势", "板块资金共振", "板块回撤":
		return aStockScoreFactorSector
	case "资金动向", "资金强度":
		return aStockScoreFactorFund
	case "昨日涨停", "昨日涨幅过高", "动能趋势", "30日过热", "60日过热", "回撤过滤":
		return aStockScoreFactorVolatility
	case "保存总分", "总分修正", "递补排序", "扣分调整", "匹配修正", "匹配差额":
		return aStockScoreFactorHistory
	default:
		return aStockScoreFactorHistory
	}
}

func normalizeAStockScoreFactor(factor string) string {
	switch strings.TrimSpace(factor) {
	case aStockScoreFactorAuction, "竞价因子":
		return aStockScoreFactorAuction
	case aStockScoreFactorEmotion, "情绪因子":
		return aStockScoreFactorEmotion
	case aStockScoreFactorSector, "sector_fund", "版块资金因子", "板块资金因子":
		return aStockScoreFactorSector
	case aStockScoreFactorFund, "stock_fund", "个股资金因子":
		return aStockScoreFactorFund
	case aStockScoreFactorVolatility, "波动因子":
		return aStockScoreFactorVolatility
	case aStockScoreFactorHistory, "历史修正":
		return aStockScoreFactorHistory
	default:
		return ""
	}
}

func isAStockPrimaryScoreFactor(factor string) bool {
	switch factor {
	case aStockScoreFactorAuction, aStockScoreFactorEmotion, aStockScoreFactorSector, aStockScoreFactorFund, aStockScoreFactorVolatility:
		return true
	default:
		return false
	}
}

func aStockScoreFactorCapWithSettings(factor string, settings model.AStockRecommendationAlgorithmSettings) int {
	switch factor {
	case aStockScoreFactorAuction:
		return positiveOrDefaultInt(settings.Auction.FactorScoreCap, 200)
	case aStockScoreFactorEmotion:
		return positiveOrDefaultInt(settings.Emotion.FactorScoreCap, 200)
	case aStockScoreFactorSector:
		return positiveOrDefaultInt(settings.Sector.FactorScoreCap, 200)
	case aStockScoreFactorFund:
		return positiveOrDefaultInt(settings.Fund.FactorScoreCap, 200)
	case aStockScoreFactorVolatility:
		return positiveOrDefaultInt(settings.Volatility.FactorScoreCap, 200)
	default:
		return 0
	}
}

func positiveOrDefaultInt(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func clampAStockScoreFactorSubtotal(value int, cap int) int {
	if cap <= 0 {
		return value
	}
	if value > cap {
		return cap
	}
	if value < -cap {
		return -cap
	}
	return value
}

func formatAStockScoreComponentUnitValueText(component aStockRecommendationScoreComponent) string {
	value, ok := aStockScoreComponentUnitValue(component)
	if !ok {
		return "--"
	}
	switch strings.TrimSpace(component.Label) {
	case "新闻热度", "个股证据":
		return fmt.Sprintf("每条 %d 分", value)
	case "热点关键词", "股票名命中":
		return fmt.Sprintf("每个 %d 分", value)
	default:
		return formatAStockScoreComponentScore(value)
	}
}

func formatAStockScoreComponentUnitValue(component aStockRecommendationScoreComponent) string {
	value, ok := aStockScoreComponentUnitValue(component)
	if !ok {
		return "--"
	}
	return strconv.Itoa(value)
}

func aStockScoreComponentUnitValue(component aStockRecommendationScoreComponent) (int, bool) {
	if component.UnitValue != 0 {
		return component.UnitValue, true
	}
	if value, ok := inferAStockScoreComponentUnitValue(component); ok {
		return value, true
	}
	if component.Score != 0 {
		return component.Score, true
	}
	return 0, false
}

func inferAStockScoreComponentUnitValue(component aStockRecommendationScoreComponent) (int, bool) {
	label := strings.TrimSpace(component.Label)
	detail := strings.TrimSpace(component.Detail)
	switch label {
	case "新闻热度":
		return divideAStockScoreByDetailCount(component.Score, detail, `证据新闻\s*(\d+)\s*条`)
	case "热点关键词", "股票名命中":
		return divideAStockScoreByDetailCount(component.Score, detail, `命中关键词\s*(\d+)\s*个`)
	case "个股证据":
		if value, ok := divideAStockScoreByDetailCount(component.Score, detail, `有效证据\s*(\d+)\s*条`); ok {
			return value, true
		}
		return divideAStockScoreByDetailCount(component.Score, detail, `个股证据\s*(\d+)\s*条`)
	}
	return 0, false
}

func divideAStockScoreByDetailCount(score int, detail string, pattern string) (int, bool) {
	matches := regexp.MustCompile(pattern).FindStringSubmatch(detail)
	if len(matches) != 2 {
		return 0, false
	}
	count := atoiAStockScorePart(matches[1])
	if count == 0 {
		return 0, false
	}
	if score%count != 0 {
		return 0, false
	}
	return score / count, true
}

func renderAStockBacktestSection(b *strings.Builder, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, contexts []aStockContext) {
	renderAStockBacktestSectionForPath(b, "/a-stock", strategyDate, period, ignoreRecent, ignoreLimitUp, ignoreFundFlow, false, filterTodayMarket, contexts)
}

func renderAStockBacktestSectionForPath(b *strings.Builder, targetPath string, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, fundFlowExplicit bool, filterTodayMarket bool, contexts []aStockContext) {
	if strings.TrimSpace(strategyDate) == "" {
		strategyDate = aStockContextsDisplayDate(contexts)
	}
	b.WriteString(`<section><h2>消息回测</h2><p class="astock-muted">上午推荐按上午开盘价计算，下午推荐按下午开盘价计算；晚间推荐按后一个交易日开盘价计算。实时价展示当前价格，T+0 到 T+5 及五日内最高收益按对应推荐窗口的基准价回测。</p>`)
	renderAStockRecommendationHistoryTabsForPath(b, targetPath, strategyDate, period, ignoreRecent, ignoreLimitUp, ignoreFundFlow, fundFlowExplicit, filterTodayMarket)
	renderAStockRecommendationHistoryActionsForPath(b, targetPath, strategyDate, period, ignoreRecent, ignoreLimitUp, ignoreFundFlow, fundFlowExplicit, filterTodayMarket)
	mergedRows := combineAStockBacktestRows(contexts)
	b.WriteString(`<div class="astock-scroll"><table class="astock-table"><tr><th>推荐窗口</th><th>股票</th><th>上午开盘价</th><th>下午开盘价</th><th>实时价</th><th>T+0 收益</th><th>T+1 收益</th><th>T+2 收益</th><th>T+3 收益</th><th>T+4 收益</th><th>T+5 收益</th><th>五日内最高收益</th><th>命中状态</th></tr>`)
	if len(mergedRows) == 0 {
		b.WriteString(`<tr><td colspan="13">`)
		b.WriteString(html.EscapeString(aStockBacktestEmptyReason(period, contexts)))
		b.WriteString(`</td></tr>`)
		b.WriteString(`</table></div></section>`)
		return
	}
	for _, display := range mergedRows {
		row := display.Row
		morningOpen, afternoonOpen := aStockBacktestDisplayOpenPrices(display.PeriodKey, row)
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(display.PeriodLabel))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(row.Stock))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(morningOpen))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(afternoonOpen))
		b.WriteString(`</td><td><span class="`)
		b.WriteString(html.EscapeString(aStockBacktestCurrentReturnClass(row)))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(nonEmpty(strings.TrimSpace(row.CurrentPrice), "--")))
		b.WriteString(`</span></td><td><span class="`)
		b.WriteString(html.EscapeString(row.T0ReturnClass))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(row.T0Return))
		b.WriteString(`</span></td>`)
		for i := 0; i < 5; i++ {
			cell := aStockBacktestCell{Close: "--", Return: "--", ReturnClass: "astock-flat"}
			if i < len(row.Days) {
				cell = row.Days[i]
			}
			b.WriteString(`<td><span class="`)
			b.WriteString(html.EscapeString(cell.ReturnClass))
			b.WriteString(`">`)
			b.WriteString(html.EscapeString(cell.Return))
			b.WriteString(`</span></td>`)
		}
		b.WriteString(`<td><span class="`)
		b.WriteString(html.EscapeString(row.BestReturnClass))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(row.BestReturn))
		b.WriteString(`</span></td><td>`)
		b.WriteString(html.EscapeString(row.Status))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></div></section>`)
}

func renderAStockT1PerformanceSection(b *strings.Builder, official model.AStockRecommendationPerformanceSummary, officialOK bool, shadow model.AStockRecommendationPerformanceSummary, shadowOK bool, auctionStrength model.AStockRecommendationPerformanceSummary, auctionStrengthOK bool) {
	b.WriteString(`<section><h2>T+1 胜率评估</h2>`)
	if !officialOK && !shadowOK && !auctionStrengthOK {
		b.WriteString(`<div class="astock-empty">暂无 T+1 胜率统计，请先生成推荐快照并刷新 T+1 回测。</div></section>`)
		return
	}
	b.WriteString(`<p class="astock-muted">按 T+1 收益大于 0 计胜，仅统计已有 T+1 回测收益的成熟样本；影子策略不替换正式推荐。</p>`)
	b.WriteString(`<div class="astock-scroll"><table class="astock-table"><tr><th>策略</th><th>区间</th><th>推荐数</th><th>成熟样本</th><th>胜数</th><th>T+1胜率</th><th>平均T+1</th><th>覆盖率</th><th>状态</th></tr>`)
	renderAStockT1PerformanceRow(b, "正式推荐", official, officialOK)
	renderAStockT1PerformanceRow(b, aStockT1ShadowStrategyKey, shadow, shadowOK)
	renderAStockT1PerformanceRow(b, aStockAuctionStrengthStrategyKey, auctionStrength, auctionStrengthOK)
	b.WriteString(`</table></div>`)
	renderAStockT1PerformancePeriodGroups(b, official, officialOK, shadow, shadowOK, auctionStrength, auctionStrengthOK)
	b.WriteString(`</section>`)
}

func renderAStockT1PerformanceRow(b *strings.Builder, label string, summary model.AStockRecommendationPerformanceSummary, ok bool) {
	if !ok {
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(label))
		b.WriteString(`</td><td colspan="8">暂无统计</td></tr>`)
		return
	}
	status := "可观察"
	if summary.InsufficientSamples {
		status = "样本不足"
	}
	b.WriteString(`<tr><td>`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</td><td>`)
	b.WriteString(html.EscapeString(summary.StartDate + " ~ " + summary.EndDate))
	b.WriteString(`</td><td>`)
	b.WriteString(fmt.Sprint(summary.RecommendationCount))
	b.WriteString(`</td><td>`)
	b.WriteString(fmt.Sprint(summary.SampleCount))
	b.WriteString(`</td><td>`)
	b.WriteString(fmt.Sprint(summary.WinCount))
	b.WriteString(`</td><td>`)
	b.WriteString(html.EscapeString(formatAStockPerformanceRatio(summary.WinRate)))
	b.WriteString(`</td><td><span class="`)
	b.WriteString(html.EscapeString(aStockPctClass(summary.AverageReturn)))
	b.WriteString(`">`)
	b.WriteString(html.EscapeString(formatAStockPct(summary.AverageReturn)))
	b.WriteString(`</span></td><td>`)
	b.WriteString(html.EscapeString(formatAStockPerformanceRatio(summary.RecommendationCover)))
	b.WriteString(`</td><td>`)
	b.WriteString(html.EscapeString(status))
	b.WriteString(`</td></tr>`)
}

func renderAStockT1PerformancePeriodGroups(b *strings.Builder, official model.AStockRecommendationPerformanceSummary, officialOK bool, shadow model.AStockRecommendationPerformanceSummary, shadowOK bool, auctionStrength model.AStockRecommendationPerformanceSummary, auctionStrengthOK bool) {
	b.WriteString(`<h3>窗口拆分</h3><div class="astock-scroll"><table class="astock-table"><tr><th>策略</th><th>窗口</th><th>成熟样本</th><th>胜数</th><th>T+1胜率</th><th>平均T+1</th></tr>`)
	renderAStockT1PerformancePeriodGroupRows(b, "正式推荐", official, officialOK)
	renderAStockT1PerformancePeriodGroupRows(b, aStockT1ShadowStrategyKey, shadow, shadowOK)
	renderAStockT1PerformancePeriodGroupRows(b, aStockAuctionStrengthStrategyKey, auctionStrength, auctionStrengthOK)
	b.WriteString(`</table></div>`)
}

func renderAStockT1PerformancePeriodGroupRows(b *strings.Builder, label string, summary model.AStockRecommendationPerformanceSummary, ok bool) {
	if !ok {
		return
	}
	for _, group := range summary.Groups {
		if group.Dimension != "period" {
			continue
		}
		b.WriteString(`<tr><td>`)
		b.WriteString(html.EscapeString(label))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(aStockPerformancePeriodLabel(group.Key)))
		b.WriteString(`</td><td>`)
		b.WriteString(fmt.Sprint(group.SampleCount))
		b.WriteString(`</td><td>`)
		b.WriteString(fmt.Sprint(group.WinCount))
		b.WriteString(`</td><td>`)
		b.WriteString(html.EscapeString(formatAStockPerformanceRatio(group.WinRate)))
		b.WriteString(`</td><td><span class="`)
		b.WriteString(html.EscapeString(aStockPctClass(group.AverageReturn)))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(formatAStockPct(group.AverageReturn)))
		b.WriteString(`</span></td></tr>`)
	}
}

func formatAStockPerformanceRatio(value float64) string {
	return fmt.Sprintf("%.2f%%", value*100)
}

func aStockPerformancePeriodLabel(period string) string {
	switch normalizeAStockPeriod(period).Key {
	case "morning":
		return "上午推荐"
	case "afternoon":
		return "下午推荐"
	default:
		return period
	}
}

func renderAStockRecommendationHistoryTabs(b *strings.Builder, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) {
	renderAStockRecommendationHistoryTabsForPath(b, "/a-stock", strategyDate, period, ignoreRecent, ignoreLimitUp, ignoreFundFlow, false, filterTodayMarket)
}

func renderAStockRecommendationHistoryTabsForPath(b *strings.Builder, targetPath string, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, fundFlowArgs ...bool) {
	fundFlowExplicit, filterTodayMarket := parseAStockFundFlowRenderArgs(fundFlowArgs...)
	renderAStockDateTabsForPath(b, targetPath, strategyDate, period, ignoreRecent, ignoreLimitUp, ignoreFundFlow, fundFlowExplicit, filterTodayMarket, true)
}

func renderAStockDatePeriodTabs(b *strings.Builder, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, withHeading bool) {
	renderAStockDateTabsForPath(b, "/a-stock", strategyDate, period, ignoreRecent, ignoreLimitUp, ignoreFundFlow, false, filterTodayMarket, withHeading)
}

func renderAStockDateTabs(b *strings.Builder, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, fundFlowExplicit bool, filterTodayMarket bool, withHeading bool) {
	renderAStockDateTabsForPath(b, "/a-stock", strategyDate, period, ignoreRecent, ignoreLimitUp, ignoreFundFlow, fundFlowExplicit, filterTodayMarket, withHeading)
}

func renderAStockDateTabsForPath(b *strings.Builder, targetPath string, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, fundFlowExplicit bool, filterTodayMarket bool, withHeading bool) {
	if withHeading {
		b.WriteString(`<h3>推荐历史</h3>`)
	}
	b.WriteString(`<div class="astock-date-tabs">`)
	normalizedPeriod := normalizeAStockPeriod(period).Key
	tabs := aStockDateTabs(strategyDate)
	today := aStockTodayDate()
	if len(tabs) == 0 {
		writeAStockDateTab(b, targetPath, "今日", today, normalizedPeriod, strategyDate == today, ignoreRecent, ignoreLimitUp, ignoreFundFlow, fundFlowExplicit, filterTodayMarket)
		b.WriteString(`</div>`)
		return
	}
	hasToday := false
	for _, tab := range tabs {
		if tab.Date == today {
			hasToday = true
		}
		active := strategyDate == tab.Date
		writeAStockDateTab(b, targetPath, tab.Label, tab.Date, normalizedPeriod, active, ignoreRecent, ignoreLimitUp, ignoreFundFlow, fundFlowExplicit, filterTodayMarket)
	}
	if !hasToday {
		writeAStockDateTab(b, targetPath, "今日", today, normalizedPeriod, strategyDate == today, ignoreRecent, ignoreLimitUp, ignoreFundFlow, fundFlowExplicit, filterTodayMarket)
	}
	b.WriteString(`</div>`)
}

func renderAStockRecommendationHistoryActions(b *strings.Builder, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) {
	renderAStockRecommendationHistoryActionsForPath(b, "/a-stock", strategyDate, period, ignoreRecent, ignoreLimitUp, ignoreFundFlow, false, filterTodayMarket)
}

func renderAStockPeriodSwitchTabsForPath(b *strings.Builder, targetPath string, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, fundFlowExplicit bool, filterTodayMarket bool) {
	current := normalizeAStockPeriod(period).Key
	b.WriteString(`<div class="astock-tabs">`)
	for _, option := range aStockPeriods() {
		b.WriteString(`<a class="astock-tab`)
		if option.Key == current {
			b.WriteString(` active`)
		}
		b.WriteString(`" data-preserve-scroll="1" href="`)
		b.WriteString(aStockPageHrefForPathWithFundFlowExplicit(targetPath, strategyDate, option.Key, 1, ignoreRecent, ignoreLimitUp, ignoreFundFlow, fundFlowExplicit, filterTodayMarket))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(option.Label))
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)
}

func renderAStockRecommendationHistoryActionsForPath(b *strings.Builder, targetPath string, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, fundFlowArgs ...bool) {
	fundFlowExplicit, filterTodayMarket := parseAStockFundFlowRenderArgs(fundFlowArgs...)
	targetPath = aStockPagePath(targetPath)
	b.WriteString(`<div class="astock-history-actions">`)
	b.WriteString(`<a class="astock-filter-toggle" data-preserve-scroll="1" href="`)
	b.WriteString(html.EscapeString(aStockFilterToggleHrefForPath(targetPath, strategyDate, period, 1, ignoreRecent, ignoreLimitUp, ignoreFundFlow, fundFlowExplicit, filterTodayMarket)))
	b.WriteString(`">`)
	b.WriteString(html.EscapeString(aStockFilterToggleLabel(ignoreRecent)))
	b.WriteString(`</a>`)
	for _, action := range []struct {
		Period string
		Name   string
		Label  string
	}{
		{Period: "morning", Name: "backfill_window_news", Label: "补抓上午新闻"},
		{Period: "morning", Name: "generate_morning_stock", Label: "重新生成上午推荐"},
		{Period: "afternoon", Name: "backfill_window_news", Label: "补抓下午新闻"},
		{Period: "afternoon", Name: "generate_afternoon_stock", Label: "重新生成下午推荐"},
		{Period: "evening", Name: "generate_evening_stock", Label: "重新生成晚间推荐"},
		{Period: "morning", Name: "backfill_auction", Label: "补录集合竞价"},
		{Period: normalizeAStockPeriod(period).Key, Name: "repair_stock_names", Label: "补股票名称"},
		{Period: normalizeAStockPeriod(period).Key, Name: "refresh_current_backtest", Label: "补行情收益"},
		{Period: "morning", Name: "refresh_backtest", Label: "刷新全部回测"},
	} {
		b.WriteString(`<form method="post" action="`)
		b.WriteString(html.EscapeString(targetPath))
		b.WriteString(`"><input type="hidden" name="date" value="`)
		b.WriteString(html.EscapeString(strategyDate))
		b.WriteString(`"><input type="hidden" name="period" value="`)
		b.WriteString(html.EscapeString(action.Period))
		b.WriteString(`">`)
		if ignoreRecent {
			b.WriteString(`<input type="hidden" name="ignore_recent" value="1">`)
		}
		if ignoreLimitUp {
			b.WriteString(`<input type="hidden" name="ignore_limit_up" value="1">`)
		}
		writeAStockFundFlowPreserveInput(b, strategyDate, ignoreFundFlow, fundFlowExplicit)
		if filterTodayMarket {
			b.WriteString(`<input type="hidden" name="filter_today_market" value="1">`)
		}
		b.WriteString(`<input type="hidden" name="action" value="`)
		b.WriteString(html.EscapeString(action.Name))
		b.WriteString(`"><button type="submit" data-preserve-scroll="1">`)
		b.WriteString(html.EscapeString(action.Label))
		b.WriteString(`</button></form>`)
	}
	b.WriteString(`</div>`)
}

func parseAStockFundFlowRenderArgs(args ...bool) (bool, bool) {
	switch len(args) {
	case 0:
		return false, false
	case 1:
		return false, args[0]
	default:
		return args[0], args[1]
	}
}

func writeAStockDateTab(b *strings.Builder, targetPath string, label string, date string, period string, active bool, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, fundFlowExplicit bool, filterTodayMarket bool) {
	b.WriteString(`<a class="astock-tab`)
	if active {
		b.WriteString(` active`)
	}
	b.WriteString(`" data-preserve-scroll="1" href="`)
	b.WriteString(aStockPageHrefForPathWithFundFlowExplicit(targetPath, date, period, 1, ignoreRecent, ignoreLimitUp, ignoreFundFlow, fundFlowExplicit, filterTodayMarket))
	b.WriteString(`">`)
	b.WriteString(html.EscapeString(label))
	b.WriteString(`</a>`)
}

type aStockDateTab struct {
	Label string
	Date  string
}

type aStockBacktestDisplayRow struct {
	PeriodLabel string
	PeriodKey   string
	Row         aStockBacktestRow
}

func aStockDateTabs(strategyDate string) []aStockDateTab {
	centerDate := normalizeAStockStrategyDate(strategyDate)
	if centerDate == "" {
		centerDate = aStockTodayDate()
	}
	latestDate := localAStockAdjacentTradingDay(aStockTodayDate(), 0)
	if latestDate == "" {
		latestDate = aStockTodayDate()
	}
	if strings.TrimSpace(centerDate) > strings.TrimSpace(latestDate) {
		centerDate = latestDate
	}
	tabs := make([]aStockDateTab, 0, 5)
	prevOne := localAStockAdjacentTradingDay(centerDate, -1)
	prevTwo := localAStockAdjacentTradingDay(prevOne, -1)
	if prevTwo != "" {
		tabs = append(tabs, aStockDateTab{Label: aStockDateTabLabel(prevTwo), Date: prevTwo})
	}
	if prevOne != "" {
		tabs = append(tabs, aStockDateTab{Label: aStockDateTabLabel(prevOne), Date: prevOne})
	}
	tabs = append(tabs, aStockDateTab{Label: aStockDateTabLabel(centerDate), Date: centerDate})
	nextOne := localAStockAdjacentTradingDay(centerDate, 1)
	if nextOne != "" && nextOne <= latestDate {
		tabs = append(tabs, aStockDateTab{Label: aStockDateTabLabel(nextOne), Date: nextOne})
		nextTwo := localAStockAdjacentTradingDay(nextOne, 1)
		if nextTwo != "" && nextTwo <= latestDate {
			tabs = append(tabs, aStockDateTab{Label: aStockDateTabLabel(nextTwo), Date: nextTwo})
		}
	}
	return tabs
}

func aStockDateTabLabel(date string) string {
	date = normalizeAStockStrategyDate(date)
	if date == "" {
		return ""
	}
	if date == aStockTodayDate() {
		return "今日"
	}
	parsed, err := time.ParseInLocation("2006-01-02", date, aStockLocation())
	if err != nil {
		return date
	}
	weekday := [...]string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}[parsed.Weekday()]
	return date + " " + weekday
}

func combineAStockBacktestRows(contexts []aStockContext) []aStockBacktestDisplayRow {
	total := 0
	for _, ctx := range contexts {
		total += len(aStockBacktestRowsForDisplay(ctx))
	}
	rows := make([]aStockBacktestDisplayRow, 0, total)
	for _, ctx := range contexts {
		for _, row := range aStockBacktestRowsForDisplay(ctx) {
			rows = append(rows, aStockBacktestDisplayRow{
				PeriodLabel: ctx.PeriodLabel,
				PeriodKey:   ctx.Period,
				Row:         row,
			})
		}
	}
	return rows
}

func aStockBacktestEmptyReason(period string, contexts []aStockContext) string {
	current := normalizeAStockPeriod(period).Key
	for _, ctx := range contexts {
		if ctx.Period == current {
			if reason := strings.TrimSpace(ctx.EmptyReason); reason != "" {
				return reason
			}
			break
		}
	}
	for _, ctx := range contexts {
		if reason := strings.TrimSpace(ctx.EmptyReason); reason != "" {
			return reason
		}
	}
	return "暂无回测结果，等待行情同步。"
}

func aStockBacktestRowsForDisplay(ctx aStockContext) []aStockBacktestRow {
	if len(ctx.Recommendations) == 0 {
		return ctx.Backtests
	}
	rowsByCode := make(map[string]aStockBacktestRow, len(ctx.Backtests))
	for _, row := range ctx.Backtests {
		if code := aStockBacktestRowCode(row); code != "" {
			rowsByCode[code] = row
		}
	}
	rows := make([]aStockBacktestRow, 0, len(ctx.Recommendations))
	for _, rec := range ctx.Recommendations {
		code := normalizeAStockCode(rec.Code)
		if row, ok := rowsByCode[code]; ok {
			rows = append(rows, row)
			continue
		}
		rows = append(rows, aStockBacktestPlaceholderRow(rec))
	}
	return rows
}

func aStockBacktestPlaceholderRow(rec aStockRecommendation) aStockBacktestRow {
	return aStockBacktestRow{
		Stock:              aStockRecommendationStockLabel(rec),
		EntryOpen:          "--",
		AfternoonOpen:      "--",
		T0Return:           "--",
		T0Close:            "--",
		T0ReturnClass:      "astock-flat",
		CurrentPrice:       "--",
		CurrentReturn:      "--",
		CurrentReturnClass: "astock-flat",
		BestReturn:         "--",
		BestReturnClass:    "astock-flat",
		Status:             "等待行情同步",
	}
}

func aStockRecommendationStockLabel(rec aStockRecommendation) string {
	code := normalizeAStockCode(rec.Code)
	name := astockcode.DisplayName(code, rec.Name)
	if code != "" && name != "" && name != code {
		return code + " " + name
	}
	if code != "" {
		return code
	}
	return strings.TrimSpace(rec.Name)
}

func aStockBacktestDisplayOpenPrices(period string, row aStockBacktestRow) (string, string) {
	morningOpen := nonEmpty(strings.TrimSpace(row.EntryOpen), "--")
	afternoonOpen := nonEmpty(strings.TrimSpace(row.AfternoonOpen), "--")
	if normalizeAStockPeriod(period).Key == "afternoon" {
		if (afternoonOpen == "" || afternoonOpen == "--") && morningOpen != "" && morningOpen != "--" {
			afternoonOpen = morningOpen
		}
		return "--", nonEmpty(afternoonOpen, "--")
	}
	if (morningOpen == "" || morningOpen == "--") && afternoonOpen != "" && afternoonOpen != "--" {
		morningOpen = afternoonOpen
	}
	return nonEmpty(morningOpen, "--"), "--"
}

func aStockBacktestCurrentReturnClass(row aStockBacktestRow) string {
	if !aStockBacktestValueMissing(row.CurrentReturn) {
		return nonEmpty(strings.TrimSpace(row.CurrentReturnClass), "astock-flat")
	}
	return nonEmpty(strings.TrimSpace(row.CurrentMarketPctClass), "astock-flat")
}

func (s *Server) loadAStockContext(strategyDate string, periodKey string, newsPage int, ignoreRecent bool) aStockContext {
	return s.loadAStockContextWithCache(strategyDate, periodKey, newsPage, ignoreRecent, false, false, false, false, newAStockRequestCache())
}

func (s *Server) handleAStockRecommendationGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	strategyDate := normalizeAStockStrategyDate(r.URL.Query().Get("date"))
	period := normalizeAStockPeriod(r.URL.Query().Get("period"))
	phase := normalizeAStockRecommendationPhase(r.URL.Query().Get("phase"))
	ignoreRecent := normalizeAStockBool(r.URL.Query().Get("ignore_recent"))
	ignoreLimitUp := normalizeAStockBool(r.URL.Query().Get("ignore_limit_up"))
	ignoreFundFlow := normalizeAStockBool(r.URL.Query().Get("ignore_fund_flow"))
	filterTodayMarket := normalizeAStockBool(r.URL.Query().Get("filter_today_market"))
	dryRun := normalizeAStockBool(r.URL.Query().Get("dry_run"))
	skipShadow := normalizeAStockBool(r.URL.Query().Get("skip_shadow"))
	refreshMode := aStockRecommendationRebuild
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("refresh_mode")), string(aStockRecommendationPreserveLocked)) {
		refreshMode = aStockRecommendationPreserveLocked
	}
	ctx := aStockContext{}
	if dryRun {
		ctx = s.loadAStockSimulationContext(strategyDate, period.Key, 1, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, phase, newAStockRequestCache())
	} else {
		ctx = s.loadAStockContextWithRecommendationPhasePersistenceMode(strategyDate, period.Key, 1, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, true, phase, newAStockRequestCache(), true, true, refreshMode)
		if !skipShadow {
			_ = s.saveAStockT1ShadowRecommendationSnapshot(ctx)
			_ = s.saveAStockAuctionStrengthShadowRecommendationSnapshot(ctx)
		}
	}
	writeRawJSON(w, http.StatusOK, map[string]any{
		"code":    http.StatusOK,
		"message": "ok",
		"data": aStockRecommendationGenerateResult{
			StrategyDate:             ctx.Date,
			Period:                   ctx.Period,
			Phase:                    phase,
			DryRun:                   dryRun,
			SkipShadow:               skipShadow,
			RecommendationCount:      len(ctx.Recommendations),
			GeneratedCount:           ctx.GeneratedRecommendationCount,
			BacktestStatus:           ctx.BacktestStatus,
			RecentFiltered:           ctx.RecentFiltered,
			RecentLookbackDays:       aStockRecentLookbackDays,
			SameDayMorningFiltered:   ctx.SameDayMorningFiltered,
			LimitUpFiltered:          ctx.LimitUpFiltered,
			RecoveredCount:           ctx.RecoveredCount,
			TodayMarketFilterEnabled: ctx.TodayMarketFilterEnabled,
			NoTodayMarketCount:       ctx.NoTodayMarketCount,
			FundFlowFilterEnabled:    ctx.FundFlowFilterEnabled,
			FundFlowFiltered:         ctx.FundFlowFiltered,
			FundFlowMissingCount:     ctx.FundFlowMissingCount,
			LoadMessage:              ctx.LoadMessage,
		},
	})
}

func (s *Server) handleAStockPopup(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeRawJSON(w, http.StatusOK, s.buildAStockPreopenPopup(userIDFromMap(user), r.URL.Query().Get("date")))
}

func (s *Server) handleAStockPopupDismiss(w http.ResponseWriter, r *http.Request, user any) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeRawJSON(w, http.StatusBadRequest, map[string]any{"message": "invalid body"})
		return
	}
	key := strings.TrimSpace(payload.Key)
	if key == "" {
		writeRawJSON(w, http.StatusBadRequest, map[string]any{"message": "popup key required"})
		return
	}
	userID := userIDFromMap(user)
	if userID <= 0 {
		writeRawJSON(w, http.StatusForbidden, map[string]any{"message": "unauthorized"})
		return
	}
	current, ok, err := s.getPopupState(userID, key)
	if err != nil {
		writeRawJSON(w, http.StatusBadGateway, map[string]any{"message": err.Error()})
		return
	}
	count := 1
	if ok {
		count = current.Count + 1
	}
	updated, err := s.putPopupState(model.PopupState{
		UserID:    userID,
		Key:       key,
		Dismissed: true,
		Count:     count,
	})
	if err != nil {
		writeRawJSON(w, http.StatusBadGateway, map[string]any{"message": err.Error()})
		return
	}
	s.recordAStockPopupDismissTask(r.Context(), userID, key, updated.Count)
	writeRawJSON(w, http.StatusOK, map[string]any{"ok": true, "key": key, "dismissed": updated.Dismissed})
}

func (s *Server) recordAStockPopupDismissTask(ctx context.Context, userID int64, key string, count int) {
	eventTime := time.Now().UTC()
	finishedAt := eventTime
	period := aStockPopupPeriodFromKey(key)
	_ = s.recordSystemTaskRun(ctx, model.TaskRun{
		TaskName:   "a-stock-popup-dismiss:" + period,
		Status:     "success",
		Message:    fmt.Sprintf("key=%s user_id=%d dismiss_count=%d", key, userID, count),
		StartedAt:  eventTime,
		FinishedAt: &finishedAt,
	})
}

func aStockPopupPeriodFromKey(key string) string {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch {
	case strings.Contains(normalized, "-morning-"):
		return "morning"
	case strings.Contains(normalized, "-afternoon-"):
		return "afternoon"
	default:
		return "unknown"
	}
}

func newAStockRequestCache() *aStockRequestCache {
	return &aStockRequestCache{
		tradingDay:                make(map[string]aStockTradingDayCacheEntry),
		recentCodes:               make(map[string]map[string]struct{}),
		latestRecommendationDates: make(map[string]map[string]string),
		articles:                  make(map[string]aStockArticlesCacheEntry),
		snapshots:                 make(map[string]aStockRecommendationSnapshotCacheEntry),
		selections:                make(map[string]aStockRecommendationSelectionCacheEntry),
		holdingSummaries:          make(map[string]aStockHoldingSummaryCacheEntry),
		stockFundFlowTrends:       make(map[string]aStockStockFundFlowTrendCacheEntry),
		stockFundFlows:            make(map[string]aStockStockFundFlowListCacheEntry),
		sectorFundFlows:           make(map[string]aStockSectorFundFlowCacheEntry),
		sectorFundFlowTrends:      make(map[string]aStockSectorFundFlowTrendCacheEntry),
		dividendEvents:            make(map[string]aStockDividendEventCacheEntry),
		auctionResults:            make(map[string]aStockAuctionResultCacheEntry),
		sectorConstituents:        make(map[string]aStockSectorConstituentCodesCacheEntry),
		codeNames:                 make(map[string]map[string]string),
	}
}

func (s *Server) loadAStockContextWithCache(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, forceRecommendationRefresh bool, cache *aStockRequestCache) aStockContext {
	return s.loadAStockContextWithRecommendationPhasePersistence(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh, aStockRecommendationPhaseFinal, cache, true, true)
}

func (s *Server) loadAStockFastReadOnlyContextWithCache(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, includeHotspotTopStocks bool, cache *aStockRequestCache) (aStockContext, bool) {
	period := normalizeAStockPeriod(periodKey)
	ctx := newAStockBaseContext(strategyDate, period.Key, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, aStockRecommendationPhaseFinal)
	ctx.FastReadOnly = true
	ctx.SourceRuns = s.loadAStockSourceRunsWithCache(cache)
	if err := s.populateAStockContextArticleStatsWithCache(&ctx, newsPage, cache); err != nil {
		ctx.LoadMessage = err.Error()
		return ctx, false
	}
	settings := aStockAlgorithmSettingsForRecommendationPeriod(period.Key, s.loadAStockAlgorithmSettingsWithCache(cache))
	ctx.AuctionAmountLabel = s.loadAStockAuctionAmountLabelWithCache(ctx.Date, cache)
	if blocked, message, reason := s.aStockRecommendationBlockedStatusWithCache(ctx.Date, cache); blocked {
		ctx.TradingDayBlocked = true
		ctx.TradingDayMessage = message
		ctx.TradingDayReason = reason
		ctx.BacktestStatus = message
		ctx.EmptyReason = aStockRecommendationEmptyReason(ctx)
		return ctx, false
	}
	if len(ctx.Hotspots) == 0 {
		ctx.BacktestStatus = "无推荐股票"
		ctx.EmptyReason = aStockRecommendationEmptyReason(ctx)
		return ctx, false
	}
	candidates, candidateStatus, auctionResult := s.loadAStockMarketCandidatesWithStatusWithCache(ctx.Date, cache)
	ctx.MarketCandidateStatus = candidateStatus
	if auctionLabel := normalizeAStockAuctionSummaryLabel(formatAStockAuctionSummaryAmount(auctionResult)); auctionLabel != "" {
		ctx.AuctionAmountLabel = auctionLabel
	}
	if includeHotspotTopStocks {
		ctx.Hotspots = buildAStockHotspotsWithTopStocksWithSettings(ctx.Hotspots, candidates, settings.Auction.HotspotTopStockLimit, settings)
		ctx.Hotspots = s.applyAStockHotspotRecommendationDatesWithCache(ctx.Hotspots, ctx.Date, ctx.Period, cache)
	}
	sectorGate := s.loadAStockHotspotSectorGateWithCache(ctx.Hotspots, cache)
	recommendationCandidates := s.buildAStockPriorityCandidatePoolWithSettings(ctx.Date, ctx.Hotspots, candidates, sectorGate, cache, settings)
	ctx.MarketCandidateCount = len(recommendationCandidates)
	recommendationTarget := 0
	var recentCodes map[string]struct{}
	recentReplacementStatus := ""
	negativeFilteredCodes := make(map[string]struct{})
	negativeFilterStatus := ""
	baseRecommendations := buildAStockSnapshotRecommendationsWithPhaseAndLimitAndSectorGateWithSettings(ctx.Date, ctx.Period, aStockRecommendationPhaseFinal, ctx.Articles, recommendationCandidates, settings.Auction.ReplacementPoolLimit, settings.Auction.ReplacementPerHotspot, sectorGate, settings)
	if filtered, skipped := filterAStockNegativeNoEvidenceRecommendations(baseRecommendations, negativeFilteredCodes); skipped > 0 {
		baseRecommendations = filtered
		ctx.NegativeNoEvidenceFiltered += skipped
		negativeFilterStatus = formatAStockNegativeNoEvidenceFilterStatus(ctx.NegativeNoEvidenceFiltered)
	}
	recommendationTarget = minInt(settings.Auction.RecommendationLimit, len(baseRecommendations))
	ctx.GeneratedRecommendationCount = recommendationTarget
	ctx.Recommendations = baseRecommendations
	exDividendStatus := ""
	dailyLimitStatus := ""
	if ctx.Period == "afternoon" && len(ctx.Recommendations) > 0 {
		remaining := s.applyAStockAfternoonSameDayDuplicateCapsWithCache(&ctx, recommendationCandidates, cache)
		if remaining <= 0 {
			_, skipped := limitAStockRecommendationsByCount(ctx.Recommendations, 0)
			ctx.Recommendations = nil
			ctx.SameDayMorningFiltered += skipped
			dailyLimitStatus = formatAStockDailyRecommendationLimitStatus(skipped)
			recommendationTarget = 0
		} else if recommendationTarget > remaining {
			recommendationTarget = remaining
		}
	}
	if len(ctx.Recommendations) > 0 && !ctx.IgnoreRecent {
		recentCodes = s.loadRecentAStockRecommendationCodesForPeriodWithCache(ctx.Date, ctx.Period, aStockRecentLookbackDays, cache)
		recentResult := filterRecentAStockRecommendationsWithReplenishment(ctx.Recommendations, nil, recentCodes, len(ctx.Recommendations))
		ctx.Recommendations = recentResult.Recommendations
		ctx.RecentFiltered = recentResult.Filtered
		recentReplacementStatus = formatAStockRecentReplenishmentStatusWithStocks(ctx.RecentFiltered, recentResult.FilteredStocks, 0, false)
	}
	if exDividendSkipped := s.applyAStockExDividendFilterWithCache(&ctx, cache); exDividendSkipped > 0 {
		exDividendStatus = formatAStockExDividendFilterStatus(exDividendSkipped)
		recommendationTarget = minInt(recommendationTarget, len(ctx.Recommendations))
	}
	if ctx.Period == "afternoon" && len(ctx.Recommendations) > 0 {
		recommendationTarget = minInt(recommendationTarget, len(ctx.Recommendations))
	}
	if recommendationTarget > 0 {
		ctx.Recommendations = limitAStockRecommendationsByScoreWithSettings(ctx.Recommendations, recommendationTarget, settings)
		recommendationTarget = minInt(recommendationTarget, len(ctx.Recommendations))
	}
	ctx.Recommendations = withAStockRecommendationEntryTimes(ctx.Recommendations, ctx.Period, "")
	ctx.Recommendations = initializeAStockRecommendationMarket(ctx.Recommendations)
	ctx.Backtests = buildAStockBacktestRows(ctx.Date, ctx.Period, ctx.Recommendations, nil)
	ctx.BacktestStatus = "只读快速推荐，等待行情同步"
	if negativeFilterStatus != "" {
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, negativeFilterStatus)
	}
	if recentReplacementStatus != "" {
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, recentReplacementStatus)
	}
	if exDividendStatus != "" {
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, exDividendStatus)
	}
	if dailyLimitStatus != "" {
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, dailyLimitStatus)
	}
	ctx.EmptyReason = aStockRecommendationEmptyReason(ctx)
	ctx.LoadMessage = appendAStockLoadMessage(ctx.LoadMessage, "今日推荐使用只读快速结果，未写入推荐快照。")
	return ctx, hasAStockBacktestDisplayData(ctx)
}

func (s *Server) loadAStockContextReadOnlyWithCache(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, forceRecommendationRefresh bool, cache *aStockRequestCache) aStockContext {
	cacheKey := aStockReadOnlyContextCacheKey(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh, true)
	if !forceRecommendationRefresh {
		if ctx, ok := s.loadCachedAStockReadOnlyContext(cacheKey); ok {
			return ctx
		}
	}
	if !forceRecommendationRefresh {
		if ctx, ok := s.loadAStockReadOnlySnapshotContextWithCache(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, cache); ok {
			s.storeCachedAStockReadOnlyContext(cacheKey, ctx)
			return ctx
		}
		ctx := aStockMissingExactSnapshotContext(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
		s.storeCachedAStockReadOnlyContext(cacheKey, ctx)
		return ctx
	}
	ctx := s.loadAStockContextWithRecommendationPhasePersistence(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh, aStockRecommendationPhaseFinal, cache, true, false)
	s.storeCachedAStockReadOnlyContext(cacheKey, ctx)
	return ctx
}

func (s *Server) loadAStockCompanionContextWithCache(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, forceRecommendationRefresh bool, cache *aStockRequestCache) aStockContext {
	return s.loadAStockContextWithRecommendationPhasePersistence(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh, aStockRecommendationPhaseFinal, cache, false, true)
}

func (s *Server) loadAStockCompanionContextReadOnlyWithCache(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, forceRecommendationRefresh bool, cache *aStockRequestCache) aStockContext {
	cacheKey := aStockReadOnlyContextCacheKey(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh, false)
	if !forceRecommendationRefresh {
		if ctx, ok := s.loadCachedAStockReadOnlyContext(cacheKey); ok {
			return ctx
		}
	}
	if !forceRecommendationRefresh {
		ctx, ok := s.loadAStockReadOnlySnapshotContextWithCache(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, cache)
		if ok {
			s.storeCachedAStockReadOnlyContext(cacheKey, ctx)
			return ctx
		}
		ctx = aStockMissingExactSnapshotContext(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
		s.storeCachedAStockReadOnlyContext(cacheKey, ctx)
		return ctx
	}
	ctx := s.loadAStockContextWithRecommendationPhasePersistence(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh, aStockRecommendationPhaseFinal, cache, false, false)
	s.storeCachedAStockReadOnlyContext(cacheKey, ctx)
	return ctx
}

func aStockMissingExactSnapshotContext(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) aStockContext {
	ctx := newAStockBaseContext(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, aStockRecommendationPhaseFinal)
	ctx.BacktestStatus = "无推荐快照"
	ctx.EmptyReason = fmt.Sprintf("暂无推荐股票：%s精确快照待生成，请稍后刷新。", ctx.PeriodLabel)
	return ctx
}

func (s *Server) loadAStockReadOnlySnapshotContextWithCache(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, cache *aStockRequestCache) (aStockContext, bool) {
	ctx := newAStockBaseContext(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, aStockRecommendationPhaseFinal)
	if s.applyAStockBacktestSnapshotOnlyWithCache(&ctx, cache) {
		if !applyAStockSnapshotNewsSummary(&ctx, newsPage, s.currentAStockSnapshotNewsSummary(ctx, cache)) {
			if err := s.populateAStockContextArticleStatsWithCache(&ctx, newsPage, cache); err != nil && ctx.LoadMessage == "" {
				ctx.LoadMessage = err.Error()
			}
		}
		return ctx, true
	}
	return ctx, false
}

func (s *Server) currentAStockSnapshotNewsSummary(ctx aStockContext, cache *aStockRequestCache) string {
	snapshot, ok := s.loadAStockRecommendationSnapshotForContextWithCache(ctx, cache)
	if !ok {
		return ""
	}
	return snapshot.NewsSummaryJSON
}

func applyAStockSnapshotNewsSummary(ctx *aStockContext, newsPage int, raw string) bool {
	if ctx == nil {
		return false
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	var summary aStockSnapshotNewsSummary
	if err := json.Unmarshal([]byte(raw), &summary); err != nil {
		return false
	}
	if len(summary.Articles) == 0 && len(summary.NewsArticles) == 0 && len(summary.Hotspots) == 0 {
		return false
	}
	ctx.Articles = summary.Articles
	ctx.NewsArticles = summary.NewsArticles
	ctx.NewsTotal = len(ctx.NewsArticles)
	ctx.RecommendationNewsTotal = len(ctx.Articles)
	ctx.PagedArticles, ctx.NewsPage, ctx.NewsTotalPages = paginateAStockNews(ctx.NewsArticles, newsPage, aStockNewsPageSize)
	ctx.Hotspots = summary.Hotspots
	if ctx.Hotspots == nil {
		ctx.Hotspots = buildAStockHotspots(ctx.Articles)
	}
	return true
}

func buildAStockSnapshotNewsSummaryJSON(ctx aStockContext) (string, error) {
	summary := aStockSnapshotNewsSummary{
		Articles:     ctx.Articles,
		NewsArticles: ctx.NewsArticles,
		Hotspots:     ctx.Hotspots,
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (s *Server) loadAStockBacktestSnapshotContextWithCache(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, cache *aStockRequestCache) aStockContext {
	ctx := newAStockBaseContext(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, aStockRecommendationPhaseFinal)
	if s.applyAStockBacktestSnapshotOnlyWithCache(&ctx, cache) {
		return ctx
	}
	ctx.BacktestStatus = "无推荐快照"
	ctx.EmptyReason = fmt.Sprintf("暂无推荐股票：%s精确快照待生成，请稍后刷新。", ctx.PeriodLabel)
	return ctx
}

func hasAStockBacktestDisplayData(ctx aStockContext) bool {
	return len(ctx.Recommendations) > 0 || len(ctx.Backtests) > 0
}

func normalizeAStockSnapshotJSONArray(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "null") {
		return "[]"
	}
	return raw
}

func decodeAStockFilteredRecommendations(raw string) []aStockFilteredRecommendation {
	raw = normalizeAStockSnapshotJSONArray(raw)
	var filtered []aStockFilteredRecommendation
	if err := json.Unmarshal([]byte(raw), &filtered); err != nil {
		return nil
	}
	return normalizeAStockFilteredRecommendations(filtered)
}

func normalizeAStockFilteredRecommendations(filtered []aStockFilteredRecommendation) []aStockFilteredRecommendation {
	if len(filtered) == 0 {
		return nil
	}
	normalized := make([]aStockFilteredRecommendation, 0, len(filtered))
	seen := make(map[string]struct{})
	for _, item := range filtered {
		rec := item.Recommendation
		code := normalizeAStockCode(rec.Code)
		if code == "" {
			continue
		}
		key := code + "|" + strings.TrimSpace(item.Reason)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		rec.Code = code
		item.Reason = strings.TrimSpace(item.Reason)
		item.Recommendation = rec
		normalized = append(normalized, item)
	}
	return normalized
}

func mergeAStockFilteredRecommendations(base []aStockFilteredRecommendation, additions []aStockFilteredRecommendation) []aStockFilteredRecommendation {
	if len(base) == 0 {
		return normalizeAStockFilteredRecommendations(additions)
	}
	if len(additions) == 0 {
		return normalizeAStockFilteredRecommendations(base)
	}
	merged := make([]aStockFilteredRecommendation, 0, len(base)+len(additions))
	merged = append(merged, base...)
	merged = append(merged, additions...)
	return normalizeAStockFilteredRecommendations(merged)
}

func shouldUseAStockFastReadOnlyFallback(strategyDate string) bool {
	return normalizeAStockStrategyDate(strategyDate) == aStockTodayDate()
}

func appendAStockLoadMessage(current string, addition string) string {
	current = strings.TrimSpace(current)
	addition = strings.TrimSpace(addition)
	if addition == "" || strings.Contains(current, addition) {
		return current
	}
	if current == "" {
		return addition
	}
	return current + " " + addition
}

func (s *Server) loadTodayAStockBacktestFallbackContextWithCache(ctx aStockContext, cache *aStockRequestCache) (aStockContext, bool) {
	if normalizeAStockStrategyDate(ctx.Date) != aStockTodayDate() {
		return aStockContext{}, false
	}
	cacheKey := aStockReadOnlyContextCacheKey(ctx.Date, ctx.Period, ctx.NewsPage, ctx.IgnoreRecent, ctx.IgnoreLimitUp, ctx.IgnoreFundFlow, ctx.TodayMarketFilterEnabled, false, true)
	if cached, ok := s.loadCachedAStockReadOnlyContext(cacheKey); ok && hasAStockBacktestDisplayData(cached) {
		cached.LoadMessage = appendAStockLoadMessage(cached.LoadMessage, "今日回测使用只读实时推荐结果，未写入推荐快照。")
		return cached, true
	}
	if shouldUseAStockFastReadOnlyFallback(ctx.Date) {
		if fast, ok := s.loadAStockFastReadOnlyContextWithCache(ctx.Date, ctx.Period, ctx.NewsPage, ctx.IgnoreRecent, ctx.IgnoreLimitUp, ctx.IgnoreFundFlow, ctx.TodayMarketFilterEnabled, true, cache); ok {
			fast.LoadMessage = appendAStockLoadMessage(fast.LoadMessage, "今日回测使用只读实时推荐结果，未写入推荐快照。")
			s.storeCachedAStockReadOnlyContext(cacheKey, fast)
			return fast, true
		}
	}
	return aStockContext{}, false
}

func (s *Server) applyAStockBacktestSnapshotOnlyWithCache(ctx *aStockContext, cache *aStockRequestCache) bool {
	if ctx == nil || strings.TrimSpace(s.cfg.ContentURL) == "" {
		return false
	}
	snapshot, ok := s.loadAStockRecommendationSnapshotForContextWithCache(*ctx, cache)
	if !ok {
		return false
	}
	recommendationsJSON := normalizeAStockSnapshotJSONArray(snapshot.RecommendationsJSON)
	backtestsJSON := normalizeAStockSnapshotJSONArray(snapshot.BacktestsJSON)
	var recommendations []aStockRecommendation
	if err := json.Unmarshal([]byte(recommendationsJSON), &recommendations); err != nil {
		return false
	}
	var backtests []aStockBacktestRow
	if err := json.Unmarshal([]byte(backtestsJSON), &backtests); err != nil {
		return false
	}
	if strings.TrimSpace(recommendationsJSON) == "[]" && strings.TrimSpace(backtestsJSON) == "[]" && len(recommendations) == 0 && len(backtests) == 0 {
		ctx.Recommendations = nil
		ctx.Backtests = nil
		ctx.BacktestStatus = nonEmpty(snapshot.BacktestStatus, "无推荐股票")
		applyAStockSnapshotMetadata(ctx, snapshot)
		ctx.EmptyReason = nonEmpty(snapshot.EmptyReason, aStockRecommendationEmptyReason(*ctx))
		return true
	}
	if len(recommendations) == 0 {
		return false
	}
	ctx.Recommendations, _ = s.repairAStockPersistedRecommendationsForPeriodWithCache(ctx.Date, ctx.Period, recommendations, cache)
	if len(ctx.Recommendations) == 0 {
		return false
	}
	ctx.Backtests = filterAStockBacktestsForSnapshotRecommendations(backtests, ctx.Recommendations)
	ctx.BacktestStatus = nonEmpty(snapshot.BacktestStatus, "已读取推荐快照")
	s.enrichAStockCurrentSnapshotMarket(ctx)
	applyAStockSnapshotMetadata(ctx, snapshot)
	ctx.EmptyReason = snapshot.EmptyReason
	if ctx.EmptyReason == "" {
		ctx.EmptyReason = aStockRecommendationEmptyReason(*ctx)
	}
	return true
}

func applyAStockSnapshotMetadata(ctx *aStockContext, snapshot model.AStockRecommendationSnapshot) {
	if ctx == nil {
		return
	}
	ctx.GeneratedRecommendationCount = snapshot.GeneratedCount
	ctx.FilteredRecommendations = decodeAStockFilteredRecommendations(snapshot.FilteredRecommendationsJSON)
	ctx.RecoveredCount = countAStockRecoveredRecommendations(ctx.Recommendations)
	ctx.RecentFiltered = snapshot.RecentFiltered
	ctx.SameDayMorningFiltered = snapshot.SameDayMorningFiltered
	ctx.LimitUpFilterEnabled = snapshot.LimitUpFilterEnabled
	ctx.LimitUpFiltered = snapshot.LimitUpFiltered
	ctx.FundFlowFilterEnabled = snapshot.FundFlowFilterEnabled
	ctx.IgnoreFundFlow = !snapshot.FundFlowFilterEnabled
	ctx.FundFlowFilterExplicit = ctx.IgnoreFundFlow != aStockDefaultIgnoreFundFlow(ctx.Date)
	ctx.FundFlowFiltered = snapshot.FundFlowFiltered
	ctx.FundFlowMissingCount = snapshot.FundFlowMissingCount
	ctx.TodayMarketFilterEnabled = snapshot.TodayMarketFilterEnabled
	ctx.NoTodayMarketCount = snapshot.NoTodayMarketCount
	ctx.MarketCandidateStatus = snapshot.MarketCandidateStatus
	ctx.MarketCandidateCount = snapshot.MarketCandidateCount
	if auctionLabel := normalizeAStockAuctionSummaryLabel(snapshot.AuctionAmountLabel); auctionLabel != "" {
		ctx.AuctionAmountLabel = auctionLabel
	}
	ctx.SnapshotUpdatedAt = snapshot.UpdatedAt
}

func (s *Server) loadAStockContextWithRecommendationPhase(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, forceRecommendationRefresh bool, recommendationPhase string, cache *aStockRequestCache) aStockContext {
	return s.loadAStockContextWithRecommendationPhasePersistence(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh, recommendationPhase, cache, true, true)
}

func (s *Server) loadAStockContextWithRecommendationPhaseOptions(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, forceRecommendationRefresh bool, recommendationPhase string, cache *aStockRequestCache, includeHotspotTopStocks bool) aStockContext {
	return s.loadAStockContextWithRecommendationPhasePersistence(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh, recommendationPhase, cache, includeHotspotTopStocks, true)
}

func (s *Server) loadAStockContextWithRecommendationPhasePersistence(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, forceRecommendationRefresh bool, recommendationPhase string, cache *aStockRequestCache, includeHotspotTopStocks bool, persist bool) aStockContext {
	return s.loadAStockContextWithRecommendationPhasePersistenceMode(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh, recommendationPhase, cache, includeHotspotTopStocks, persist, aStockRecommendationPreserveLocked)
}

func (s *Server) loadAStockContextWithRecommendationPhasePersistenceMode(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, forceRecommendationRefresh bool, recommendationPhase string, cache *aStockRequestCache, includeHotspotTopStocks bool, persist bool, refreshMode aStockRecommendationRefreshMode) aStockContext {
	return s.loadAStockContextWithRecommendationPhasePersistenceModeEntryTime(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh, recommendationPhase, cache, includeHotspotTopStocks, persist, refreshMode, "")
}

func (s *Server) loadAStockSimulationContext(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, recommendationPhase string, cache *aStockRequestCache) aStockContext {
	return s.loadAStockContextWithRecommendationPhasePersistenceModeEntryTime(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, true, recommendationPhase, cache, true, false, aStockRecommendationRebuild, "")
}

func newAStockBaseContext(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, recommendationPhase string) aStockContext {
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	period := normalizeAStockPeriod(periodKey)
	phase := normalizeAStockRecommendationPhase(recommendationPhase)
	if newsPage < 1 {
		newsPage = 1
	}
	recommendationStart, recommendationEnd, recommendationWindowLabel := aStockRecommendationPhaseWindow(strategyDate, period.Key, phase)
	newsStart, newsEnd := aStockWindow(strategyDate, period.Key)
	ctx := aStockContext{
		Date:                      strategyDate,
		Period:                    period.Key,
		PeriodLabel:               period.Label,
		WindowLabel:               period.WindowLabel,
		NewsPage:                  newsPage,
		NewsPageSize:              aStockNewsPageSize,
		NewsWindowStart:           newsStart,
		NewsWindowEnd:             newsEnd,
		WindowStart:               recommendationStart,
		WindowEnd:                 recommendationEnd,
		RecommendationWindowLabel: recommendationWindowLabel,
		BacktestStatus:            "等待行情接口",
		IgnoreRecent:              ignoreRecent,
		IgnoreLimitUp:             ignoreLimitUp,
		IgnoreFundFlow:            ignoreFundFlow,
		FundFlowFilterEnabled:     !ignoreFundFlow,
		TodayMarketFilterEnabled:  filterTodayMarket,
	}
	ctx.LimitUpFilterEnabled = period.Key == "afternoon" && !ignoreLimitUp
	return ctx
}

func (s *Server) populateAStockContextArticleStatsWithCache(ctx *aStockContext, newsPage int, cache *aStockRequestCache) error {
	if ctx == nil {
		return nil
	}
	recommendationStart, recommendationEnd := ctx.WindowStart, ctx.WindowEnd
	newsStart, newsEnd := ctx.NewsWindowStart, ctx.NewsWindowEnd
	var articles []model.Item
	var newsArticles []model.Item
	var err error
	if sameAStockWindow(recommendationStart, recommendationEnd, newsStart, newsEnd) {
		articles, err = s.loadAStockWindowArticlesWithCache(recommendationStart, recommendationEnd, cache)
		if err != nil {
			return fmt.Errorf("A股新闻读取失败：%w", err)
		}
		newsArticles = articles
	} else if aStockWindowContains(newsStart, newsEnd, recommendationStart, recommendationEnd) {
		newsArticles, err = s.loadAStockWindowArticlesWithCache(newsStart, newsEnd, cache)
		if err != nil {
			return fmt.Errorf("A股新闻统计读取失败：%w", err)
		}
		articles = filterAStockArticlesByPublishWindow(newsArticles, recommendationStart, recommendationEnd)
	} else {
		articles, err = s.loadAStockWindowArticlesWithCache(recommendationStart, recommendationEnd, cache)
		if err != nil {
			return fmt.Errorf("A股新闻读取失败：%w", err)
		}
		newsArticles, err = s.loadAStockWindowArticlesWithCache(newsStart, newsEnd, cache)
		if err != nil {
			return fmt.Errorf("A股新闻统计读取失败：%w", err)
		}
	}
	settings, _ := astocknews.LoadSettings("")
	articles = astocknews.FilterRecommendationItems(articles, settings)
	ctx.Articles = articles
	ctx.NewsArticles = newsArticles
	ctx.NewsTotal = len(ctx.NewsArticles)
	ctx.RecommendationNewsTotal = len(ctx.Articles)
	ctx.PagedArticles, ctx.NewsPage, ctx.NewsTotalPages = paginateAStockNews(ctx.NewsArticles, newsPage, aStockNewsPageSize)
	ctx.Hotspots = buildAStockHotspotsWithSettings(ctx.Articles, s.loadAStockAlgorithmSettingsWithCache(cache))
	return nil
}

func (s *Server) loadAStockNewsStatsContextWithCache(strategyDate string, periodKey string, cache *aStockRequestCache) aStockContext {
	period := normalizeAStockPeriod(periodKey)
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	ctx := newAStockBaseContext(strategyDate, period.Key, 1, false, false, false, false, aStockRecommendationPhaseFinal)
	ctx.SourceRuns = s.loadAStockSourceRunsWithCache(cache)
	if period.Key != "evening" {
		if err := s.populateAStockContextArticleStatsWithCache(&ctx, 1, cache); err != nil {
			ctx.LoadMessage = err.Error()
		}
	}
	return ctx
}

func (s *Server) loadAStockNewsStatsContextsWithCache(strategyDate string, cache *aStockRequestCache) []aStockContext {
	contexts := make([]aStockContext, 0, len(aStockPeriods()))
	for _, period := range aStockPeriods() {
		contexts = append(contexts, s.loadAStockNewsStatsContextWithCache(strategyDate, period.Key, cache))
	}
	return contexts
}

func appendAStockLoadMessages(messages []string, contexts []aStockContext) []string {
	for _, ctx := range contexts {
		if msg := strings.TrimSpace(ctx.LoadMessage); msg != "" {
			messages = append(messages, msg)
		}
	}
	return messages
}

func (s *Server) renderAStockNewsStatsHTML(strategyDate string) string {
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	cache := newAStockRequestCache()
	contexts := s.loadAStockNewsStatsContextsWithCache(strategyDate, cache)
	var b strings.Builder
	for _, msg := range appendAStockLoadMessages(nil, contexts) {
		msg = strings.TrimSpace(msg)
		if msg == "" {
			continue
		}
		b.WriteString(`<div class="astock-empty">`)
		b.WriteString(html.EscapeString(msg))
		b.WriteString(`</div>`)
	}
	renderAStockNewsStatsContent(&b, contexts)
	return b.String()
}

func (s *Server) loadAStockContextWithRecommendationPhasePersistenceModeEntryTime(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, forceRecommendationRefresh bool, recommendationPhase string, cache *aStockRequestCache, includeHotspotTopStocks bool, persist bool, refreshMode aStockRecommendationRefreshMode, entryTimeOverride string) aStockContext {
	period := normalizeAStockPeriod(periodKey)
	phase := normalizeAStockRecommendationPhase(recommendationPhase)
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	ctx := newAStockBaseContext(strategyDate, period.Key, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, phase)
	settings := aStockAlgorithmSettingsForRecommendationPeriod(period.Key, s.loadAStockAlgorithmSettingsWithCache(cache))
	ctx.SourceRuns = s.loadAStockSourceRunsWithCache(cache)
	if period.Key != "evening" {
		if err := s.populateAStockContextArticleStatsWithCache(&ctx, newsPage, cache); err != nil {
			ctx.LoadMessage = err.Error()
			return ctx
		}
	}
	ctx.AuctionAmountLabel = s.loadAStockAuctionAmountLabelWithCache(strategyDate, cache)
	var marketCandidates []aStockMarketCandidate
	if len(ctx.Hotspots) > 0 && includeHotspotTopStocks {
		candidates, candidateStatus, auctionResult := s.loadAStockMarketCandidatesWithStatusWithCache(strategyDate, cache)
		marketCandidates = candidates
		ctx.Hotspots = buildAStockHotspotsWithTopStocksWithSettings(ctx.Hotspots, marketCandidates, settings.Auction.HotspotTopStockLimit, settings)
		ctx.Hotspots = s.applyAStockHotspotRecommendationDatesWithCache(ctx.Hotspots, strategyDate, period.Key, cache)
		ctx.MarketCandidateStatus = candidateStatus
		if auctionLabel := normalizeAStockAuctionSummaryLabel(formatAStockAuctionSummaryAmount(auctionResult)); auctionLabel != "" {
			ctx.AuctionAmountLabel = auctionLabel
		}
	}
	if blocked, message, reason := s.aStockRecommendationBlockedStatusWithCache(strategyDate, cache); blocked {
		ctx.TradingDayBlocked = true
		ctx.TradingDayMessage = message
		ctx.TradingDayReason = reason
		ctx.BacktestStatus = message
		ctx.EmptyReason = aStockRecommendationEmptyReason(ctx)
		return ctx
	}
	allowPersistedRecommendations := phase == aStockRecommendationPhaseFinal && isAStockOfficialSelectionContextForDate(strategyDate, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
	rebuildRecommendations := refreshMode == aStockRecommendationRebuild
	if allowPersistedRecommendations && !forceRecommendationRefresh && s.applyAStockRecommendationSnapshotWithCache(&ctx, cache) {
		return ctx
	}
	if period.Key == "evening" {
		if allowPersistedRecommendations && !rebuildRecommendations && s.applyAStockEveningPersistedRecommendationsWithCache(&ctx, cache) {
			ctx.Recommendations, ctx.Backtests, ctx.BacktestStatus, ctx.LimitUpFiltered = s.loadAStockLockedMarketView(strategyDate, ctx.Period, ctx.Recommendations)
			if forceRecommendationRefresh {
				s.restoreAStockBacktestsFromSnapshotWithCache(&ctx, cache)
			}
			ctx.EmptyReason = aStockEveningRecommendationEmptyReason(ctx)
			if persist && ctx.LoadMessage == "" {
				if err := s.saveAStockRecommendationSnapshot(ctx); err != nil {
					ctx.LoadMessage = "A股推荐保存失败：" + err.Error()
				}
			}
			return ctx
		}
		return s.loadAStockEveningRecommendationContext(ctx, cache, persist, rebuildRecommendations)
	}
	if allowPersistedRecommendations && !rebuildRecommendations && s.applyAStockRecommendationSelectionsWithCache(&ctx, cache) {
		if snapshot, ok := s.loadAStockRecommendationSnapshotForContextWithCache(ctx, cache); ok {
			ctx.FilteredRecommendations = decodeAStockFilteredRecommendations(snapshot.FilteredRecommendationsJSON)
		}
		if ctx.Period == "afternoon" && len(ctx.Recommendations) > 0 {
			s.applyAStockAfternoonDailyLimitOnlyWithCache(&ctx, cache)
		}
		ctx.Recommendations = s.applyAStockHoldingSummariesWithSettings(ctx.Recommendations, cache, settings)
		fundFlowStatus := ""
		if ctx.FundFlowFilterEnabled && len(ctx.Recommendations) > 0 {
			result := s.applyAStockRecommendationFundFlowFilterWithSettings(strategyDate, ctx.Recommendations, nil, nil, len(ctx.Recommendations), cache, false, settings)
			ctx.Recommendations = result.Recommendations
			ctx.FundFlowFiltered = result.Filtered
			ctx.FundFlowMissingCount = result.Missing
			fundFlowStatus = formatAStockFundFlowFilterStatusWithStocks(result.Filtered, result.FilteredStocks, result.Replenished, result.Missing, result.Shortfall)
		}
		recoveryStatus := s.recoverAStockFilteredRecommendationsAfterClose(&ctx, cache)
		ctx.Recommendations, ctx.Backtests, ctx.BacktestStatus, ctx.LimitUpFiltered = s.loadAStockLockedMarketView(strategyDate, ctx.Period, ctx.Recommendations)
		if forceRecommendationRefresh {
			s.restoreAStockBacktestsFromSnapshotWithCache(&ctx, cache)
		}
		if recoveryStatus != "" {
			ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, recoveryStatus)
		}
		if fundFlowStatus != "" {
			ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, fundFlowStatus)
		}
		ctx.EmptyReason = aStockRecommendationEmptyReason(ctx)
		if persist && ctx.LoadMessage == "" {
			if err := s.saveAStockRecommendationSnapshot(ctx); err != nil {
				ctx.LoadMessage = "A股推荐保存失败：" + err.Error()
			}
		}
		return ctx
	}
	if allowPersistedRecommendations && !rebuildRecommendations && s.applyAStockRecommendationSnapshotRecommendationsWithCache(&ctx, cache) {
		s.applyAStockAfternoonSameDayCapsWithCache(&ctx, nil, cache)
		if persist && ctx.LoadMessage == "" {
			if err := s.saveAStockRecommendationSelections(ctx); err != nil {
				ctx.LoadMessage = "A股已选股票保存失败：" + err.Error()
			}
		}
		if !forceRecommendationRefresh {
			if s.applyAStockRecommendationSnapshotWithCache(&ctx, cache) {
				return ctx
			}
		}
		ctx.Recommendations = s.applyAStockHoldingSummariesWithSettings(ctx.Recommendations, cache, settings)
		recoveryStatus := s.recoverAStockFilteredRecommendationsAfterClose(&ctx, cache)
		ctx.Recommendations, ctx.Backtests, ctx.BacktestStatus, ctx.LimitUpFiltered = s.loadAStockLockedMarketView(strategyDate, ctx.Period, ctx.Recommendations)
		if forceRecommendationRefresh {
			s.restoreAStockBacktestsFromSnapshotWithCache(&ctx, cache)
		}
		if recoveryStatus != "" {
			ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, recoveryStatus)
		}
		ctx.EmptyReason = aStockRecommendationEmptyReason(ctx)
		if persist && ctx.LoadMessage == "" {
			if err := s.saveAStockRecommendationSnapshot(ctx); err != nil {
				ctx.LoadMessage = "A股推荐保存失败：" + err.Error()
			}
		}
		return ctx
	}
	if !forceRecommendationRefresh && phase == aStockRecommendationPhaseFinal {
		if s.applyAStockRecommendationSnapshotWithCache(&ctx, cache) {
			return ctx
		}
	}
	recommendationTarget := 0
	var fundFlowReplacementPool []aStockRecommendation
	var recentCodes map[string]struct{}
	recentReplacementStatus := ""
	fundFlowStatus := ""
	exDividendStatus := ""
	dailyLimitStatus := ""
	negativeFilteredCodes := make(map[string]struct{})
	negativeFilterStatus := ""
	if len(ctx.Hotspots) > 0 {
		if marketCandidates == nil {
			candidates, candidateStatus, auctionResult := s.loadAStockMarketCandidatesWithStatusWithCache(strategyDate, cache)
			marketCandidates = candidates
			ctx.MarketCandidateStatus = candidateStatus
			if auctionLabel := normalizeAStockAuctionSummaryLabel(formatAStockAuctionSummaryAmount(auctionResult)); auctionLabel != "" {
				ctx.AuctionAmountLabel = auctionLabel
			}
		}
		sectorGate := s.loadAStockHotspotSectorGateWithCache(ctx.Hotspots, cache)
		candidates := s.buildAStockPriorityCandidatePoolWithSettings(strategyDate, ctx.Hotspots, marketCandidates, sectorGate, cache, settings)
		ctx.MarketCandidateCount = len(candidates)
		baseRecommendations := buildAStockSnapshotRecommendationsWithPhaseAndLimitAndSectorGateWithSettings(strategyDate, period.Key, phase, ctx.Articles, candidates, settings.Auction.ReplacementPoolLimit, settings.Auction.ReplacementPerHotspot, sectorGate, settings)
		if filtered, skipped := filterAStockNegativeNoEvidenceRecommendations(baseRecommendations, negativeFilteredCodes); skipped > 0 {
			baseRecommendations = filtered
			ctx.NegativeNoEvidenceFiltered += skipped
			negativeFilterStatus = formatAStockNegativeNoEvidenceFilterStatus(ctx.NegativeNoEvidenceFiltered)
		}
		recommendationTarget = minInt(settings.Auction.RecommendationLimit, len(baseRecommendations))
		ctx.GeneratedRecommendationCount = recommendationTarget
		ctx.Recommendations = baseRecommendations
		if ctx.FundFlowFilterEnabled && recommendationTarget > 0 {
			fundFlowReplacementPool = buildAStockSnapshotReplacementRecommendationsWithSectorGateWithSettings(strategyDate, period.Key, phase, ctx.Articles, candidates, sectorGate, settings)
			if filtered, skipped := filterAStockNegativeNoEvidenceRecommendations(fundFlowReplacementPool, negativeFilteredCodes); skipped > 0 {
				fundFlowReplacementPool = filtered
				ctx.NegativeNoEvidenceFiltered += skipped
				negativeFilterStatus = formatAStockNegativeNoEvidenceFilterStatus(ctx.NegativeNoEvidenceFiltered)
			}
		}
		if ctx.LimitUpFilterEnabled && recommendationTarget > 0 {
			replacementPool := buildAStockSnapshotRecommendationsWithPhaseAndLimitAndSectorGateWithSettings(strategyDate, period.Key, phase, ctx.Articles, candidates, settings.Auction.ReplacementPoolLimit, settings.Auction.ReplacementPerHotspot, sectorGate, settings)
			if filtered, skipped := filterAStockNegativeNoEvidenceRecommendations(replacementPool, negativeFilteredCodes); skipped > 0 {
				replacementPool = filtered
				ctx.NegativeNoEvidenceFiltered += skipped
				negativeFilterStatus = formatAStockNegativeNoEvidenceFilterStatus(ctx.NegativeNoEvidenceFiltered)
			}
			ctx.Recommendations = mergeAStockLimitUpReplacementPool(ctx.Recommendations, replacementPool)
		}
		if period.Key == "afternoon" && len(ctx.Recommendations) > 0 {
			remaining := s.applyAStockAfternoonSameDayDuplicateCapsWithCache(&ctx, candidates, cache)
			if remaining <= 0 {
				_, skipped := limitAStockRecommendationsByCount(ctx.Recommendations, 0)
				ctx.Recommendations = nil
				ctx.SameDayMorningFiltered += skipped
				dailyLimitStatus = formatAStockDailyRecommendationLimitStatus(skipped)
				recommendationTarget = 0
			} else if recommendationTarget > remaining {
				recommendationTarget = remaining
			}
		}
	}
	if len(ctx.Recommendations) > 0 && !ctx.IgnoreRecent {
		recentCodes = s.loadRecentAStockRecommendationCodesForPeriodWithCache(strategyDate, period.Key, aStockRecentLookbackDays, cache)
		recentResult := filterRecentAStockRecommendationsWithReplenishment(ctx.Recommendations, nil, recentCodes, len(ctx.Recommendations))
		ctx.Recommendations = recentResult.Recommendations
		ctx.RecentFiltered = recentResult.Filtered
		recentReplacementStatus = formatAStockRecentReplenishmentStatusWithStocks(ctx.RecentFiltered, recentResult.FilteredStocks, 0, false)
	}
	ctx.Recommendations = withAStockRecommendationEntryTimes(ctx.Recommendations, period.Key, entryTimeOverride)
	ctx.Recommendations = s.applyAStockHoldingSummariesWithSettings(ctx.Recommendations, cache, settings)
	if ctx.FundFlowFilterEnabled && len(ctx.Recommendations) > 0 {
		if len(fundFlowReplacementPool) > 0 {
			fundFlowReplacementPool = withAStockRecommendationEntryTimes(fundFlowReplacementPool, period.Key, entryTimeOverride)
			fundFlowReplacementPool = s.applyAStockHoldingSummariesWithSettings(fundFlowReplacementPool, cache, settings)
		}
		result := s.applyAStockRecommendationFundFlowFilterWithSettings(strategyDate, ctx.Recommendations, fundFlowReplacementPool, recentCodes, len(ctx.Recommendations), cache, false, settings)
		ctx.Recommendations = result.Recommendations
		ctx.FundFlowFiltered = result.Filtered
		ctx.FundFlowMissingCount = result.Missing
		fundFlowStatus = formatAStockFundFlowFilterStatusWithStocks(result.Filtered, result.FilteredStocks, result.Replenished, result.Missing, false)
	}
	if exDividendSkipped := s.applyAStockExDividendFilterWithCache(&ctx, cache); exDividendSkipped > 0 {
		exDividendStatus = formatAStockExDividendFilterStatus(exDividendSkipped)
		recommendationTarget = minInt(recommendationTarget, len(ctx.Recommendations))
	}
	if period.Key == "afternoon" && len(ctx.Recommendations) > 0 {
		recommendationTarget = minInt(recommendationTarget, len(ctx.Recommendations))
	}
	if recommendationTarget > 0 {
		ctx.Recommendations = limitAStockRecommendationsByScoreWithSettings(ctx.Recommendations, recommendationTarget, settings)
		recommendationTarget = minInt(recommendationTarget, len(ctx.Recommendations))
	}
	marketView := s.loadAStockMarketViewDetailed(strategyDate, ctx.Period, ctx.Recommendations, ctx.LimitUpFilterEnabled, ctx.TodayMarketFilterEnabled, recommendationTarget)
	ctx.Recommendations = marketView.Recommendations
	ctx.Backtests = marketView.Backtests
	ctx.BacktestStatus = marketView.Status
	ctx.LimitUpFiltered = marketView.LimitUpFiltered
	ctx.NoTodayMarketCount = marketView.NoTodayMarketCount
	ctx.FilteredRecommendations = mergeAStockFilteredRecommendations(ctx.FilteredRecommendations, marketView.FilteredRecommendations)
	if forceRecommendationRefresh {
		s.restoreAStockBacktestsFromSnapshotWithCache(&ctx, cache)
	}
	if negativeFilterStatus != "" {
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, negativeFilterStatus)
	}
	if recentReplacementStatus != "" {
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, recentReplacementStatus)
	}
	if fundFlowStatus != "" {
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, fundFlowStatus)
	}
	if exDividendStatus != "" {
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, exDividendStatus)
	}
	if dailyLimitStatus != "" {
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, dailyLimitStatus)
	}
	if persist && shouldPersistAStockRecommendationSelectionsForDate(strategyDate, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh) && (len(ctx.Recommendations) > 0 || rebuildRecommendations) {
		if err := s.saveAStockRecommendationSelections(ctx); err != nil && ctx.LoadMessage == "" {
			ctx.LoadMessage = "A股已选股票保存失败：" + err.Error()
		}
	}
	ctx.EmptyReason = aStockRecommendationEmptyReason(ctx)
	if persist {
		if err := s.saveAStockRecommendationSnapshot(ctx); err != nil {
			if ctx.LoadMessage == "" {
				ctx.LoadMessage = "A股推荐保存失败：" + err.Error()
			}
		}
	}
	return ctx
}

func (s *Server) loadAStockEveningRecommendationContext(ctx aStockContext, cache *aStockRequestCache, persist bool, rebuildRecommendations bool) aStockContext {
	candidates, candidateStatus := s.loadAStockEveningCandidates(ctx.Date)
	ctx.MarketCandidateStatus = candidateStatus
	ctx.MarketCandidateCount = len(candidates)
	eligibleRecommendations := eligibleAStockEveningRecommendations(ctx.Date, candidates)
	ctx.GeneratedRecommendationCount = len(eligibleRecommendations)
	ctx.Recommendations = eligibleRecommendations
	recentReplacementStatus := ""
	if len(ctx.Recommendations) > 0 && !ctx.IgnoreRecent {
		recentCodes := s.loadRecentAStockRecommendationCodesForPeriodWithCache(ctx.Date, ctx.Period, aStockRecentLookbackDays, cache)
		recentResult := filterRecentAStockRecommendationsWithReplenishment(ctx.Recommendations, nil, recentCodes, len(ctx.Recommendations))
		ctx.Recommendations = recentResult.Recommendations
		ctx.RecentFiltered = recentResult.Filtered
		recentReplacementStatus = formatAStockRecentReplenishmentStatusWithStocks(ctx.RecentFiltered, recentResult.FilteredStocks, 0, false)
	}
	ctx.Recommendations = buildAStockEveningRecommendationsFromEligible(ctx.Recommendations, aStockEveningRecommendationLimit)
	ctx.Recommendations = withAStockRecommendationEntryTimes(ctx.Recommendations, ctx.Period, "")
	ctx.Recommendations = initializeAStockRecommendationMarket(ctx.Recommendations)
	ctx.Recommendations, ctx.Backtests, ctx.BacktestStatus, ctx.LimitUpFiltered = s.loadAStockLockedMarketView(ctx.Date, ctx.Period, ctx.Recommendations)
	if len(ctx.Recommendations) == 0 {
		ctx.BacktestStatus = "无推荐股票"
	}
	if recentReplacementStatus != "" {
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, recentReplacementStatus)
	}
	ctx.EmptyReason = aStockEveningRecommendationEmptyReason(ctx)
	if persist && shouldPersistAStockRecommendationSelectionsForDate(ctx.Date, ctx.IgnoreRecent, ctx.IgnoreLimitUp, ctx.IgnoreFundFlow, ctx.TodayMarketFilterEnabled, true) && (len(ctx.Recommendations) > 0 || rebuildRecommendations) {
		if err := s.saveAStockRecommendationSelections(ctx); err != nil && ctx.LoadMessage == "" {
			ctx.LoadMessage = "A股已选股票保存失败：" + err.Error()
		}
	}
	if persist {
		if err := s.saveAStockRecommendationSnapshot(ctx); err != nil && ctx.LoadMessage == "" {
			ctx.LoadMessage = "A股推荐保存失败：" + err.Error()
		}
	}
	return ctx
}

func buildAStockEveningRecommendationsFromEligible(recommendations []aStockRecommendation, maxRecommendations int) []aStockRecommendation {
	if len(recommendations) == 0 {
		return recommendations
	}
	if maxRecommendations > 0 && len(recommendations) > maxRecommendations {
		recommendations = recommendations[:maxRecommendations]
	}
	return rerankAStockRecommendations(recommendations)
}

func (s *Server) applyAStockEveningPersistedRecommendationsWithCache(ctx *aStockContext, cache *aStockRequestCache) bool {
	if ctx == nil || !isAStockOfficialSelectionContextForDate(ctx.Date, ctx.IgnoreRecent, ctx.IgnoreLimitUp, ctx.IgnoreFundFlow, ctx.TodayMarketFilterEnabled) {
		return false
	}
	if result, ok := s.loadAStockRecommendationSelectionsWithCache(ctx.Date, ctx.Period, cache); ok && len(result.Items) > 0 {
		ctx.Recommendations, _ = s.repairAStockPersistedRecommendationsForPeriodWithCache(ctx.Date, ctx.Period, aStockRecommendationSelectionsToRecommendations(result.Items, ctx.Period), cache)
		ctx.GeneratedRecommendationCount = len(ctx.Recommendations)
		return len(ctx.Recommendations) > 0
	}
	snapshot, ok := s.loadAStockRecommendationSnapshotForContextWithCache(*ctx, cache)
	if !ok {
		return false
	}
	var recommendations []aStockRecommendation
	if err := json.Unmarshal([]byte(normalizeAStockSnapshotJSONArray(snapshot.RecommendationsJSON)), &recommendations); err != nil || len(recommendations) == 0 {
		return false
	}
	ctx.Recommendations, _ = s.repairAStockPersistedRecommendationsForPeriodWithCache(ctx.Date, ctx.Period, recommendations, cache)
	applyAStockSnapshotMetadata(ctx, snapshot)
	if ctx.GeneratedRecommendationCount <= 0 {
		ctx.GeneratedRecommendationCount = len(ctx.Recommendations)
	}
	ctx.EmptyReason = snapshot.EmptyReason
	return len(ctx.Recommendations) > 0
}

func aStockEveningRecommendationEmptyReason(ctx aStockContext) string {
	if len(ctx.Recommendations) > 0 {
		return ""
	}
	status := strings.TrimSpace(ctx.MarketCandidateStatus)
	switch {
	case status == "evening_snapshot_unconfigured":
		return "暂无推荐股票：晚间快照接口未配置，无法执行晚间量价筛选。"
	case strings.HasPrefix(status, "evening_snapshot_status_") || strings.HasPrefix(status, "evening_snapshot_failed"):
		return "暂无推荐股票：晚间快照读取失败，" + status
	case ctx.GeneratedRecommendationCount == 0:
		return "暂无推荐股票：" + aStockEveningEmptyReason
	case ctx.RecentFiltered > 0:
		return fmt.Sprintf("暂无推荐股票：晚间量价筛选命中 %d 只，但%s过滤 %d 只。", ctx.GeneratedRecommendationCount, aStockRecentLookbackLabel(), ctx.RecentFiltered)
	default:
		return fmt.Sprintf("暂无推荐股票：晚间量价筛选命中 %d 只，但未形成可回测推荐。状态：%s", ctx.GeneratedRecommendationCount, nonEmpty(status, ctx.BacktestStatus))
	}
}

func isAStockOfficialSelectionContext(ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) bool {
	return !ignoreRecent && !ignoreLimitUp && !ignoreFundFlow && !filterTodayMarket
}

func isAStockOfficialSelectionContextForDate(strategyDate string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) bool {
	return !ignoreRecent && !ignoreLimitUp && ignoreFundFlow == aStockDefaultIgnoreFundFlow(strategyDate) && !filterTodayMarket
}

func shouldPersistAStockRecommendationSelections(ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, forceRecommendationRefresh bool) bool {
	return forceRecommendationRefresh && isAStockOfficialSelectionContext(ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
}

func shouldPersistAStockRecommendationSelectionsForDate(strategyDate string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, forceRecommendationRefresh bool) bool {
	return forceRecommendationRefresh && isAStockOfficialSelectionContextForDate(strategyDate, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
}

func (s *Server) loadRecentAStockRecommendationCodesForPeriodWithCache(strategyDate string, period string, lookbackDays int, cache *aStockRequestCache) map[string]struct{} {
	result := s.loadRecentAStockRecommendationCodesWithCache(strategyDate, lookbackDays, cache)
	switch normalizeAStockPeriod(period).Key {
	case "afternoon":
		if len(result) == 0 {
			result = make(map[string]struct{})
		}
		for code := range s.loadPersistedAStockRecommendationCodesFromAllSourcesWithCache(strategyDate, "morning", false, cache) {
			result[code] = struct{}{}
		}
	case "evening":
		if len(result) == 0 {
			result = make(map[string]struct{})
		}
		for _, sameDayPeriod := range []string{"morning", "afternoon"} {
			for code := range s.loadPersistedAStockRecommendationCodesFromAllSourcesWithCache(strategyDate, sameDayPeriod, false, cache) {
				result[code] = struct{}{}
			}
		}
	}
	return result
}

func formatAStockPublishTime(value time.Time) string {
	return value.In(aStockLocation()).Format("2006-01-02 15:04:05")
}

func (s *Server) applyAStockRecommendationSelections(ctx *aStockContext) bool {
	return s.applyAStockRecommendationSelectionsWithCache(ctx, nil)
}

func (s *Server) applyAStockRecommendationSelectionsWithCache(ctx *aStockContext, cache *aStockRequestCache) bool {
	if ctx == nil || !isAStockOfficialSelectionContextForDate(ctx.Date, ctx.IgnoreRecent, ctx.IgnoreLimitUp, ctx.IgnoreFundFlow, ctx.TodayMarketFilterEnabled) {
		return false
	}
	result, ok := s.loadAStockRecommendationSelectionsWithCache(ctx.Date, ctx.Period, cache)
	if !ok || len(result.Items) == 0 {
		return false
	}
	ctx.Recommendations, _ = s.repairAStockPersistedRecommendationsForPeriodWithCache(ctx.Date, ctx.Period, aStockRecommendationSelectionsToRecommendations(result.Items, ctx.Period), cache)
	if exDividendSkipped, dailyLimitSkipped := s.applyAStockRecommendationOutputFiltersWithCache(ctx, cache); exDividendSkipped > 0 || dailyLimitSkipped > 0 {
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, formatAStockExDividendFilterStatus(exDividendSkipped))
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, formatAStockDailyRecommendationLimitStatus(dailyLimitSkipped))
	}
	if len(ctx.Recommendations) == 0 {
		return false
	}
	ctx.GeneratedRecommendationCount = len(ctx.Recommendations)
	return true
}

func (s *Server) hasAStockRecommendationSelectionsWithCache(strategyDate string, period string, cache *aStockRequestCache) bool {
	result, ok := s.loadAStockRecommendationSelectionsWithCache(strategyDate, period, cache)
	return ok && len(result.Items) > 0
}

func (s *Server) applyAStockRecommendationSnapshotRecommendations(ctx *aStockContext) bool {
	return s.applyAStockRecommendationSnapshotRecommendationsWithCache(ctx, nil)
}

func (s *Server) applyAStockRecommendationSnapshotRecommendationsWithCache(ctx *aStockContext, cache *aStockRequestCache) bool {
	if ctx == nil || strings.TrimSpace(s.cfg.ContentURL) == "" || !isAStockOfficialSelectionContextForDate(ctx.Date, ctx.IgnoreRecent, ctx.IgnoreLimitUp, ctx.IgnoreFundFlow, ctx.TodayMarketFilterEnabled) {
		return false
	}
	snapshot, ok := s.loadAStockRecommendationSnapshotForContextWithCache(*ctx, cache)
	if !ok {
		return false
	}
	if ctx.Period == "afternoon" && snapshot.LimitUpFilterEnabled != ctx.LimitUpFilterEnabled {
		return false
	}
	if snapshot.TodayMarketFilterEnabled != ctx.TodayMarketFilterEnabled {
		return false
	}
	if snapshot.FundFlowFilterEnabled != ctx.FundFlowFilterEnabled {
		return false
	}
	if !ctx.TodayMarketFilterEnabled && strings.Contains(snapshot.BacktestStatus, "过滤无当日行情") {
		return false
	}
	var recommendations []aStockRecommendation
	if err := json.Unmarshal([]byte(normalizeAStockSnapshotJSONArray(snapshot.RecommendationsJSON)), &recommendations); err != nil || len(recommendations) == 0 {
		return false
	}
	ctx.Recommendations, _ = s.repairAStockPersistedRecommendationsForPeriodWithCache(ctx.Date, ctx.Period, recommendations, cache)
	if exDividendSkipped, dailyLimitSkipped := s.applyAStockRecommendationOutputFiltersWithCache(ctx, cache); exDividendSkipped > 0 || dailyLimitSkipped > 0 {
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, formatAStockExDividendFilterStatus(exDividendSkipped))
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, formatAStockDailyRecommendationLimitStatus(dailyLimitSkipped))
	}
	if len(ctx.Recommendations) == 0 {
		return false
	}
	applyAStockSnapshotMetadata(ctx, snapshot)
	if ctx.GeneratedRecommendationCount <= 0 {
		ctx.GeneratedRecommendationCount = len(ctx.Recommendations)
	}
	ctx.EmptyReason = snapshot.EmptyReason
	return true
}

func (s *Server) applyAStockRecommendationSnapshot(ctx *aStockContext) bool {
	return s.applyAStockRecommendationSnapshotWithCache(ctx, nil)
}

func (s *Server) applyAStockRecommendationSnapshotWithCache(ctx *aStockContext, cache *aStockRequestCache) bool {
	return s.applyAStockRecommendationSnapshotWithFreshnessCache(ctx, cache, true)
}

func (s *Server) applyAStockRecommendationSnapshotReadOnlyWithCache(ctx *aStockContext, cache *aStockRequestCache) bool {
	return s.applyAStockRecommendationSnapshotWithFreshnessCache(ctx, cache, false)
}

func (s *Server) applyAStockRecommendationSnapshotWithFreshnessCache(ctx *aStockContext, cache *aStockRequestCache, requireFreshBacktest bool) bool {
	if ctx == nil || strings.TrimSpace(s.cfg.ContentURL) == "" {
		return false
	}
	snapshot, ok := s.loadAStockRecommendationSnapshotForContextWithCache(*ctx, cache)
	if !ok {
		return false
	}
	if ctx.Period == "afternoon" && snapshot.LimitUpFilterEnabled != ctx.LimitUpFilterEnabled {
		return false
	}
	if snapshot.TodayMarketFilterEnabled != ctx.TodayMarketFilterEnabled {
		return false
	}
	if snapshot.FundFlowFilterEnabled != ctx.FundFlowFilterEnabled {
		return false
	}
	if !ctx.TodayMarketFilterEnabled && strings.Contains(snapshot.BacktestStatus, "过滤无当日行情") {
		return false
	}
	var recommendations []aStockRecommendation
	recommendationsJSON := normalizeAStockSnapshotJSONArray(snapshot.RecommendationsJSON)
	if err := json.Unmarshal([]byte(recommendationsJSON), &recommendations); err != nil {
		return false
	}
	var backtests []aStockBacktestRow
	backtestsJSON := normalizeAStockSnapshotJSONArray(snapshot.BacktestsJSON)
	if err := json.Unmarshal([]byte(backtestsJSON), &backtests); err != nil {
		return false
	}
	if strings.TrimSpace(recommendationsJSON) == "[]" && strings.TrimSpace(backtestsJSON) == "[]" && len(recommendations) == 0 && len(backtests) == 0 {
		ctx.Recommendations = nil
		ctx.Backtests = nil
		ctx.BacktestStatus = nonEmpty(snapshot.BacktestStatus, "无推荐股票")
		applyAStockSnapshotMetadata(ctx, snapshot)
		ctx.EmptyReason = nonEmpty(snapshot.EmptyReason, aStockRecommendationEmptyReason(*ctx))
		return true
	}
	if requireFreshBacktest {
		if !isFreshAStockBacktestSnapshot(ctx.Period, backtests) {
			return false
		}
		if ctx.Period == "afternoon" && !isFreshAStockLimitUpReplacementSnapshot(snapshot, recommendations) {
			return false
		}
	}
	ctx.Recommendations, _ = s.repairAStockPersistedRecommendationsForPeriodWithCache(ctx.Date, ctx.Period, recommendations, cache)
	exDividendFiltered, dailyLimitFiltered := s.applyAStockRecommendationOutputFiltersWithCache(ctx, cache)
	if len(ctx.Recommendations) == 0 {
		return false
	}
	ctx.Backtests = filterAStockBacktestsForRecommendations(backtests, ctx.Recommendations)
	ctx.BacktestStatus = nonEmpty(snapshot.BacktestStatus, "已读取推荐快照")
	s.enrichAStockCurrentSnapshotMarket(ctx)
	if exDividendFiltered > 0 {
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, formatAStockExDividendFilterStatus(exDividendFiltered))
	}
	if dailyLimitFiltered > 0 {
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, formatAStockDailyRecommendationLimitStatus(dailyLimitFiltered))
	}
	applyAStockSnapshotMetadata(ctx, snapshot)
	ctx.EmptyReason = snapshot.EmptyReason
	if ctx.EmptyReason == "" {
		ctx.EmptyReason = aStockRecommendationEmptyReason(*ctx)
	}
	return true
}

func (s *Server) enrichAStockCurrentSnapshotMarket(ctx *aStockContext) {
	if ctx == nil || normalizeAStockStrategyDate(ctx.Date) != aStockTodayDate() || len(ctx.Recommendations) == 0 {
		return
	}
	rows := s.enrichAStockBacktestsWithRealtimeQuotes(ctx.Date, ctx.Period, ctx.Backtests)
	if len(rows) > 0 {
		ctx.Backtests = rows
	}
	byCode := aStockBacktestRowsByCode(ctx.Backtests)
	for i := range ctx.Recommendations {
		code := normalizeAStockCode(ctx.Recommendations[i].Code)
		row, ok := byCode[code]
		if !ok {
			continue
		}
		if !aStockBacktestValueMissing(row.CurrentPrice) {
			ctx.Recommendations[i].CurrentPrice = row.CurrentPrice
		}
		if !aStockBacktestValueMissing(row.CurrentMarketPct) {
			ctx.Recommendations[i].TodayPct = row.CurrentMarketPct
			ctx.Recommendations[i].TodayPctClass = nonEmpty(strings.TrimSpace(row.CurrentMarketPctClass), aStockPctClassFromText(row.CurrentMarketPct))
		}
	}
}

func (s *Server) loadAStockRecommendationSnapshot(strategyDate string, period string, ignoreRecent bool) (model.AStockRecommendationSnapshot, bool) {
	return s.loadAStockRecommendationSnapshotWithCache(strategyDate, period, ignoreRecent, nil)
}

func (s *Server) loadAStockRecommendationSnapshotWithCache(strategyDate string, period string, ignoreRecent bool, cache *aStockRequestCache) (model.AStockRecommendationSnapshot, bool) {
	return s.loadAStockRecommendationSnapshotQueryWithCache(strategyDate, period, ignoreRecent, false, false, false, false, false, false, cache)
}

func (s *Server) loadAStockRecommendationSnapshotForContextWithCache(ctx aStockContext, cache *aStockRequestCache) (model.AStockRecommendationSnapshot, bool) {
	return s.loadAStockRecommendationSnapshotExactWithCache(ctx.Date, ctx.Period, ctx.IgnoreRecent, ctx.LimitUpFilterEnabled, ctx.TodayMarketFilterEnabled, ctx.FundFlowFilterEnabled, cache)
}

func (s *Server) loadAStockRecommendationSnapshotExactWithCache(strategyDate string, period string, ignoreRecent bool, limitUpFilterEnabled bool, todayMarketFilterEnabled bool, fundFlowFilterEnabled bool, cache *aStockRequestCache) (model.AStockRecommendationSnapshot, bool) {
	return s.loadAStockRecommendationSnapshotQueryWithCache(strategyDate, period, ignoreRecent, true, limitUpFilterEnabled, true, todayMarketFilterEnabled, true, fundFlowFilterEnabled, cache)
}

func (s *Server) loadAStockRecommendationSnapshotQueryWithCache(strategyDate string, period string, ignoreRecent bool, hasLimitUpFilterEnabled bool, limitUpFilterEnabled bool, hasTodayMarketFilterEnabled bool, todayMarketFilterEnabled bool, hasFundFlowFilterEnabled bool, fundFlowFilterEnabled bool, cache *aStockRequestCache) (model.AStockRecommendationSnapshot, bool) {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return model.AStockRecommendationSnapshot{}, false
	}
	date := normalizeAStockStrategyDate(strategyDate)
	normalizedPeriod := normalizeAStockPeriod(period).Key
	cacheKey := strings.Join([]string{
		date,
		normalizedPeriod,
		fmt.Sprint(ignoreRecent),
		fmt.Sprint(hasLimitUpFilterEnabled),
		fmt.Sprint(limitUpFilterEnabled),
		fmt.Sprint(hasTodayMarketFilterEnabled),
		fmt.Sprint(todayMarketFilterEnabled),
		fmt.Sprint(hasFundFlowFilterEnabled),
		fmt.Sprint(fundFlowFilterEnabled),
	}, "|")
	if cache != nil {
		if entry, ok := cache.snapshots[cacheKey]; ok {
			return entry.snapshot, entry.found
		}
	}
	query := "/api/v1/a-stock/recommendations?date=" + url.QueryEscape(date) + "&period=" + url.QueryEscape(normalizedPeriod)
	if ignoreRecent {
		query += "&ignore_recent=1"
	}
	if hasLimitUpFilterEnabled {
		query += "&limit_up_filter_enabled=" + aStockBoolQueryValue(limitUpFilterEnabled)
	}
	if hasTodayMarketFilterEnabled {
		query += "&today_market_filter_enabled=" + aStockBoolQueryValue(todayMarketFilterEnabled)
	}
	if hasFundFlowFilterEnabled {
		query += "&fund_flow_filter_enabled=" + aStockBoolQueryValue(fundFlowFilterEnabled)
	}
	snapshot := model.AStockRecommendationSnapshot{}
	found := true
	if err := s.getJSON(s.cfg.ContentURL+query, &snapshot); err != nil || !snapshot.Found {
		snapshot = model.AStockRecommendationSnapshot{}
		found = false
	}
	if cache != nil {
		cache.snapshots[cacheKey] = aStockRecommendationSnapshotCacheEntry{snapshot: snapshot, found: found}
	}
	return snapshot, found
}

func aStockBoolQueryValue(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func (s *Server) loadAStockRecommendationPerformance(strategyDate string, period string, strategy string) (model.AStockRecommendationPerformanceSummary, bool) {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return model.AStockRecommendationPerformanceSummary{}, false
	}
	date := normalizeAStockStrategyDate(strategyDate)
	if date == "" {
		date = aStockTodayDate()
	}
	endDate := date
	startDate := date
	if parsed, err := time.Parse("2006-01-02", date); err == nil {
		startDate = parsed.AddDate(0, 0, -90).Format("2006-01-02")
	}
	query := url.Values{}
	query.Set("start", startDate)
	query.Set("end", endDate)
	query.Set("period", nonEmpty(strings.TrimSpace(period), "all"))
	query.Set("strategy", nonEmpty(strings.TrimSpace(strategy), "official"))
	var summary model.AStockRecommendationPerformanceSummary
	if err := s.getJSON(strings.TrimRight(s.cfg.ContentURL, "/")+"/api/v1/a-stock/recommendation-performance?"+query.Encode(), &summary); err != nil {
		return model.AStockRecommendationPerformanceSummary{}, false
	}
	return summary, true
}

func (s *Server) loadAStockRecommendationSelections(strategyDate string, period string) (model.AStockRecommendationSelectionListResult, bool) {
	return s.loadAStockRecommendationSelectionsWithCache(strategyDate, period, nil)
}

func (s *Server) loadAStockRecommendationSelectionsWithCache(strategyDate string, period string, cache *aStockRequestCache) (model.AStockRecommendationSelectionListResult, bool) {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return model.AStockRecommendationSelectionListResult{}, false
	}
	date := normalizeAStockStrategyDate(strategyDate)
	normalizedPeriod := normalizeAStockPeriod(period).Key
	cacheKey := date + "|" + normalizedPeriod
	if cache != nil {
		if entry, ok := cache.selections[cacheKey]; ok {
			return entry.result, entry.found
		}
	}
	query := "/api/v1/a-stock/recommendation-selections?date=" + url.QueryEscape(date) + "&period=" + url.QueryEscape(normalizedPeriod)
	result := model.AStockRecommendationSelectionListResult{}
	found := true
	if err := s.getJSON(s.cfg.ContentURL+query, &result); err != nil || !result.Found || len(result.Items) == 0 {
		result = model.AStockRecommendationSelectionListResult{}
		found = false
	}
	if cache != nil {
		cache.selections[cacheKey] = aStockRecommendationSelectionCacheEntry{result: result, found: found}
	}
	return result, found
}

func (s *Server) applyAStockHotspotRecommendationDatesWithCache(hotspots []aStockHotspot, strategyDate string, period string, cache *aStockRequestCache) []aStockHotspot {
	codes := aStockHotspotTopStockCodes(hotspots)
	if len(codes) == 0 {
		return hotspots
	}
	latestDates := s.loadAStockRecommendationLatestDatesWithCache(strategyDate, period, codes, cache)
	if len(latestDates) == 0 {
		return hotspots
	}
	result := make([]aStockHotspot, len(hotspots))
	copy(result, hotspots)
	for i := range result {
		if len(result[i].TopStocks) == 0 {
			continue
		}
		result[i].TopStocks = append([]aStockHotspotStock(nil), result[i].TopStocks...)
		for j := range result[i].TopStocks {
			code := normalizeAStockCode(result[i].TopStocks[j].Code)
			if recommendationDate := latestDates[code]; recommendationDate != "" {
				result[i].TopStocks[j].RecommendationDate = recommendationDate
			}
		}
	}
	return result
}

func (s *Server) loadAStockRecommendationLatestDatesWithCache(strategyDate string, period string, codes []string, cache *aStockRequestCache) map[string]string {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return nil
	}
	normalizedCodes := normalizeAStockCodeList(codes)
	if len(normalizedCodes) == 0 {
		return nil
	}
	date := normalizeAStockStrategyDate(strategyDate)
	normalizedPeriod := normalizeAStockPeriod(period).Key
	cacheKey := aStockRecommendationLatestDateCacheKey(date, normalizedPeriod, normalizedCodes)
	if cache != nil {
		if cached, ok := cache.latestRecommendationDates[cacheKey]; ok {
			return cached
		}
	}
	query := "/api/v1/a-stock/recommendation-latest-dates?date=" + url.QueryEscape(date) + "&period=" + url.QueryEscape(normalizedPeriod) + "&codes=" + url.QueryEscape(strings.Join(normalizedCodes, ","))
	result := model.AStockRecommendationLatestDateListResult{}
	latestDates := make(map[string]string)
	if err := s.getJSON(strings.TrimRight(s.cfg.ContentURL, "/")+query, &result); err == nil {
		for _, item := range result.Items {
			code := normalizeAStockCode(item.Code)
			latestDate := strings.TrimSpace(item.LatestDate)
			if code != "" && latestDate != "" {
				latestDates[code] = latestDate
			}
		}
	}
	if cache != nil {
		cache.latestRecommendationDates[cacheKey] = latestDates
	}
	return latestDates
}

func (s *Server) loadAStockCodeNamesWithCache(codes []string, cache *aStockRequestCache) map[string]string {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return nil
	}
	normalizedCodes := normalizeAStockCodeList(codes)
	if len(normalizedCodes) == 0 {
		return nil
	}
	cacheKey := aStockCodeNameCacheKey(normalizedCodes)
	if cache != nil {
		if cached, ok := cache.codeNames[cacheKey]; ok {
			return cached
		}
	}
	query := "/api/v1/a-stock/code-names?codes=" + url.QueryEscape(strings.Join(normalizedCodes, ","))
	result := model.AStockCodeNameListResult{}
	names := make(map[string]string)
	if err := s.getJSON(strings.TrimRight(s.cfg.ContentURL, "/")+query, &result); err == nil {
		for _, item := range result.Items {
			code := normalizeAStockCode(item.Code)
			name := astockcode.DisplayName(code, item.Name)
			if astockcode.IsShanghaiShenzhen(code) && hasResolvedAStockRecommendationName(code, name) {
				names[code] = name
			}
		}
	}
	if cache != nil {
		cache.codeNames[cacheKey] = names
	}
	return names
}

func aStockCodeNameCacheKey(codes []string) string {
	keyCodes := append([]string(nil), codes...)
	sort.Strings(keyCodes)
	return strings.Join(keyCodes, ",")
}

func aStockHotspotTopStockCodes(hotspots []aStockHotspot) []string {
	codes := make([]string, 0)
	seen := make(map[string]struct{})
	for _, hotspot := range hotspots {
		for _, stock := range hotspot.TopStocks {
			code := normalizeAStockCode(stock.Code)
			if code == "" {
				continue
			}
			if _, exists := seen[code]; exists {
				continue
			}
			seen[code] = struct{}{}
			codes = append(codes, code)
		}
	}
	return codes
}

func normalizeAStockCodeList(codes []string) []string {
	normalized := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, raw := range codes {
		code := normalizeAStockCode(raw)
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		normalized = append(normalized, code)
	}
	return normalized
}

func aStockRecommendationLatestDateCacheKey(strategyDate string, period string, codes []string) string {
	keyCodes := append([]string(nil), codes...)
	sort.Strings(keyCodes)
	return strings.TrimSpace(strategyDate) + "|" + strings.TrimSpace(period) + "|" + strings.Join(keyCodes, ",")
}

func (s *Server) saveAStockRecommendationSelections(ctx aStockContext) error {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return nil
	}
	recommendations, _ := s.repairAStockRecommendationsForPersistence(ctx.Date, ctx.Period, ctx.Recommendations)
	selectionSet := model.AStockRecommendationSelectionSet{
		StrategyDate: ctx.Date,
		Period:       ctx.Period,
		Items:        aStockRecommendationsToSelectionItems(recommendations, ctx.Date, ctx.Period),
	}
	resp, err := s.client.R().
		SetBody(selectionSet).
		Post(strings.TrimRight(s.cfg.ContentURL, "/") + "/api/v1/internal/a-stock/recommendation-selections")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf(resp.Status())
	}
	return nil
}

func (s *Server) saveAStockRecommendationSnapshot(ctx aStockContext) error {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return nil
	}
	snapshot, err := s.buildAStockRecommendationSnapshot(ctx)
	if err != nil {
		return err
	}
	resp, err := s.client.R().
		SetBody(snapshot).
		Post(strings.TrimRight(s.cfg.ContentURL, "/") + "/api/v1/internal/a-stock/recommendations")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf(resp.Status())
	}
	return nil
}

func (s *Server) buildAStockRecommendationSnapshot(ctx aStockContext) (model.AStockRecommendationSnapshot, error) {
	recommendations, _ := s.repairAStockRecommendationsForPersistence(ctx.Date, ctx.Period, ctx.Recommendations)
	recommendationsJSON, err := json.Marshal(recommendations)
	if err != nil {
		return model.AStockRecommendationSnapshot{}, err
	}
	filteredRecommendationsJSON, err := json.Marshal(normalizeAStockFilteredRecommendations(ctx.FilteredRecommendations))
	if err != nil {
		return model.AStockRecommendationSnapshot{}, err
	}
	backtestsJSON, err := json.Marshal(filterAStockBacktestsForRecommendations(ctx.Backtests, recommendations))
	if err != nil {
		return model.AStockRecommendationSnapshot{}, err
	}
	newsSummaryJSON, err := buildAStockSnapshotNewsSummaryJSON(ctx)
	if err != nil {
		return model.AStockRecommendationSnapshot{}, err
	}
	snapshot := model.AStockRecommendationSnapshot{
		Found:                       true,
		StrategyDate:                ctx.Date,
		Period:                      ctx.Period,
		IgnoreRecent:                ctx.IgnoreRecent,
		RecommendationsJSON:         normalizeAStockSnapshotJSONArray(string(recommendationsJSON)),
		FilteredRecommendationsJSON: normalizeAStockSnapshotJSONArray(string(filteredRecommendationsJSON)),
		BacktestsJSON:               normalizeAStockSnapshotJSONArray(string(backtestsJSON)),
		NewsSummaryJSON:             newsSummaryJSON,
		BacktestStatus:              ctx.BacktestStatus,
		GeneratedCount:              ctx.GeneratedRecommendationCount,
		RecentFiltered:              ctx.RecentFiltered,
		SameDayMorningFiltered:      ctx.SameDayMorningFiltered,
		LimitUpFilterEnabled:        ctx.LimitUpFilterEnabled,
		LimitUpFiltered:             ctx.LimitUpFiltered,
		TodayMarketFilterEnabled:    ctx.TodayMarketFilterEnabled,
		NoTodayMarketCount:          ctx.NoTodayMarketCount,
		FundFlowFilterEnabled:       ctx.FundFlowFilterEnabled,
		FundFlowFiltered:            ctx.FundFlowFiltered,
		FundFlowMissingCount:        ctx.FundFlowMissingCount,
		MarketCandidateStatus:       ctx.MarketCandidateStatus,
		MarketCandidateCount:        ctx.MarketCandidateCount,
		AuctionAmountLabel:          ctx.AuctionAmountLabel,
		EmptyReason:                 ctx.EmptyReason,
	}
	return snapshot, nil
}

func (s *Server) saveAStockT1ShadowRecommendationSnapshot(ctx aStockContext) error {
	if strings.TrimSpace(s.cfg.ContentURL) == "" || strings.TrimSpace(ctx.Date) == "" || strings.TrimSpace(ctx.Period) == "" {
		return nil
	}
	cache := newAStockRequestCache()
	shadowCtx := s.buildAStockT1ShadowSnapshotContext(ctx, cache, true)
	return s.saveAStockShadowRecommendationSnapshot(aStockT1ShadowStrategyKey, shadowCtx)
}

func (s *Server) saveAStockAuctionStrengthShadowRecommendationSnapshot(ctx aStockContext) error {
	if strings.TrimSpace(s.cfg.ContentURL) == "" || strings.TrimSpace(ctx.Date) == "" || strings.TrimSpace(ctx.Period) == "" {
		return nil
	}
	cache := newAStockRequestCache()
	shadowCtx := s.buildAStockAuctionStrengthSnapshotContext(ctx, cache, true)
	return s.saveAStockShadowRecommendationSnapshot(aStockAuctionStrengthStrategyKey, shadowCtx)
}

func (s *Server) saveAStockShadowRecommendationSnapshot(strategyKey string, shadowCtx aStockContext) error {
	snapshot, err := s.buildAStockRecommendationSnapshot(shadowCtx)
	if err != nil {
		return err
	}
	payload := model.AStockRecommendationShadowSnapshot{
		StrategyKey:                  strings.TrimSpace(strategyKey),
		AStockRecommendationSnapshot: snapshot,
	}
	resp, err := s.client.R().
		SetBody(payload).
		Post(strings.TrimRight(s.cfg.ContentURL, "/") + "/api/v1/internal/a-stock/recommendation-shadow-snapshots")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf(resp.Status())
	}
	return nil
}

func (s *Server) buildAStockT1ShadowSnapshotContext(ctx aStockContext, cache *aStockRequestCache, applyAfternoonDailyLimit bool) aStockContext {
	if cache == nil {
		cache = newAStockRequestCache()
	}
	marketCandidates, candidateStatus, auctionResult := s.loadAStockMarketCandidatesWithStatusWithCache(ctx.Date, cache)
	hotspots := aStockRecommendationHotspotSlice(ctx.Hotspots)
	if len(hotspots) == 0 && len(ctx.Articles) > 0 {
		hotspots = buildAStockHotspots(ctx.Articles)
	}
	sectorGate := s.loadAStockHotspotSectorGateWithCache(hotspots, cache)
	recommendations := buildAStockT1ShadowRecommendationsWithSectorGate(hotspots, marketCandidates, sectorGate)
	recommendations = withAStockRecommendationEntryTimes(recommendations, ctx.Period, "")
	recommendations = s.applyAStockHoldingSummariesWithCache(recommendations, cache)
	var fundFlowFiltered int
	var fundFlowMissing int
	fundFlowEnabled := ctx.FundFlowFilterEnabled && !ctx.IgnoreFundFlow
	if fundFlowEnabled {
		recommendations, fundFlowFiltered, fundFlowMissing = s.applyAStockT1ShadowFundFlowWithCache(ctx.Date, recommendations, cache)
	}
	recommendations, backtests, status, limitUpFiltered, noTodayMarketCount := s.loadAStockMarketView(ctx.Date, ctx.Period, recommendations, true, true, 0)
	var riskFiltered int
	recommendations, backtests, riskFiltered = filterAStockT1ShadowMarketRiskRecommendations(ctx.Period, recommendations, backtests)
	recommendations = limitAStockRecommendationsByHotspot(recommendations, aStockT1ShadowRecommendationLimit, aStockT1ShadowStocksPerHotspot)
	generatedRecommendationCount := len(recommendations)
	dailyLimitFiltered := 0
	if applyAfternoonDailyLimit && ctx.Period == "afternoon" && len(recommendations) > 0 {
		morningRecommendations := s.buildSameDayMorningAStockT1ShadowRecommendationsWithCache(ctx, cache)
		recommendations, dailyLimitFiltered = limitAStockRecommendationsByCount(recommendations, remainingAStockDailyRecommendationLimit(countAStockDailyLimitRecommendations(morningRecommendations)))
	}
	backtests = filterAStockBacktestsForRecommendations(backtests, recommendations)
	status = appendAStockBacktestStatus(status, formatAStockT1ShadowStatus(fundFlowFiltered, fundFlowMissing, riskFiltered))
	status = appendAStockBacktestStatus(status, formatAStockDailyRecommendationLimitStatus(dailyLimitFiltered))
	shadowCtx := ctx
	shadowCtx.Recommendations = recommendations
	shadowCtx.Backtests = backtests
	shadowCtx.BacktestStatus = status
	shadowCtx.GeneratedRecommendationCount = generatedRecommendationCount
	shadowCtx.SameDayMorningFiltered = dailyLimitFiltered
	shadowCtx.LimitUpFilterEnabled = true
	shadowCtx.LimitUpFiltered = limitUpFiltered
	shadowCtx.TodayMarketFilterEnabled = true
	shadowCtx.NoTodayMarketCount = noTodayMarketCount
	shadowCtx.FundFlowFilterEnabled = fundFlowEnabled
	shadowCtx.FundFlowFiltered = fundFlowFiltered
	shadowCtx.FundFlowMissingCount = fundFlowMissing
	shadowCtx.MarketCandidateStatus = candidateStatus
	shadowCtx.MarketCandidateCount = len(aStockRecommendationCandidatesForLimit(hotspots, marketCandidates, aStockRecommendationLimit, aStockStocksPerHotspot))
	if auctionLabel := normalizeAStockAuctionSummaryLabel(formatAStockAuctionSummaryAmount(auctionResult)); auctionLabel != "" {
		shadowCtx.AuctionAmountLabel = auctionLabel
	}
	shadowCtx.EmptyReason = aStockRecommendationEmptyReason(shadowCtx)
	return shadowCtx
}

func (s *Server) buildSameDayMorningAStockT1ShadowRecommendationsWithCache(ctx aStockContext, cache *aStockRequestCache) []aStockRecommendation {
	if ctx.Period != "afternoon" || strings.TrimSpace(ctx.Date) == "" {
		return nil
	}
	morningCtx := newAStockBaseContext(ctx.Date, "morning", 1, ctx.IgnoreRecent, ctx.IgnoreLimitUp, ctx.IgnoreFundFlow, ctx.TodayMarketFilterEnabled, aStockRecommendationPhaseFinal)
	if err := s.populateAStockContextArticleStatsWithCache(&morningCtx, 1, cache); err != nil {
		return nil
	}
	return s.buildAStockT1ShadowSnapshotContext(morningCtx, cache, false).Recommendations
}

func (s *Server) buildAStockAuctionStrengthSnapshotContext(ctx aStockContext, cache *aStockRequestCache, applyAfternoonDailyLimit bool) aStockContext {
	if cache == nil {
		cache = newAStockRequestCache()
	}
	settings := aStockAlgorithmSettingsForRecommendationPeriod(ctx.Period, s.loadAStockAlgorithmSettingsWithCache(cache))
	marketCandidates, candidateStatus, auctionResult := s.loadAStockAuctionStrengthMarketCandidatesWithStatusWithCache(ctx.Date, cache)
	hotspots := aStockRecommendationHotspotSliceWithSettings(ctx.Hotspots, settings)
	if len(hotspots) == 0 && len(ctx.Articles) > 0 {
		hotspots = buildAStockHotspotsWithSettings(ctx.Articles, settings)
	}
	sectorGate := s.loadAStockHotspotSectorGateWithCache(hotspots, cache)
	candidates := s.buildAStockPriorityCandidatePoolWithSettings(ctx.Date, hotspots, marketCandidates, sectorGate, cache, settings)
	recommendations := buildAStockAuctionStrengthRecommendationsWithSectorGateWithSettings(hotspots, candidates, sectorGate, settings)
	recommendations = withAStockRecommendationEntryTimes(recommendations, ctx.Period, "")
	recommendations = s.applyAStockHoldingSummariesWithSettings(recommendations, cache, settings)
	var fundFlowFiltered int
	var fundFlowMissing int
	fundFlowEnabled := ctx.FundFlowFilterEnabled && !ctx.IgnoreFundFlow
	if fundFlowEnabled {
		recommendations, fundFlowFiltered, fundFlowMissing = s.applyAStockShadowFundFlowWithCache(ctx.Date, recommendations, cache, settings)
	}
	recommendations, backtests, status, limitUpFiltered, noTodayMarketCount := s.loadAStockMarketView(ctx.Date, ctx.Period, recommendations, true, true, 0)
	var riskFiltered int
	recommendations, backtests, riskFiltered = filterAStockAuctionStrengthMarketRiskRecommendations(ctx.Period, recommendations, backtests, settings)
	recommendations = limitAStockRecommendationsByHotspot(recommendations, aStockAuctionStrengthRecommendationLimit, aStockAuctionStrengthStocksPerHotspot)
	generatedRecommendationCount := len(recommendations)
	dailyLimitFiltered := 0
	if applyAfternoonDailyLimit && ctx.Period == "afternoon" && len(recommendations) > 0 {
		morningRecommendations := s.buildSameDayMorningAStockAuctionStrengthRecommendationsWithCache(ctx, cache)
		recommendations, dailyLimitFiltered = limitAStockRecommendationsByCount(recommendations, remainingAStockDailyRecommendationLimit(countAStockDailyLimitRecommendations(morningRecommendations)))
	}
	backtests = filterAStockBacktestsForRecommendations(backtests, recommendations)
	status = appendAStockBacktestStatus(status, formatAStockAuctionStrengthStatus(fundFlowFiltered, fundFlowMissing, riskFiltered))
	status = appendAStockBacktestStatus(status, formatAStockDailyRecommendationLimitStatus(dailyLimitFiltered))
	shadowCtx := ctx
	shadowCtx.Recommendations = recommendations
	shadowCtx.Backtests = backtests
	shadowCtx.BacktestStatus = status
	shadowCtx.GeneratedRecommendationCount = generatedRecommendationCount
	shadowCtx.SameDayMorningFiltered = dailyLimitFiltered
	shadowCtx.LimitUpFilterEnabled = true
	shadowCtx.LimitUpFiltered = limitUpFiltered
	shadowCtx.TodayMarketFilterEnabled = true
	shadowCtx.NoTodayMarketCount = noTodayMarketCount
	shadowCtx.FundFlowFilterEnabled = fundFlowEnabled
	shadowCtx.FundFlowFiltered = fundFlowFiltered
	shadowCtx.FundFlowMissingCount = fundFlowMissing
	shadowCtx.MarketCandidateStatus = candidateStatus
	shadowCtx.MarketCandidateCount = len(candidates)
	if auctionLabel := normalizeAStockAuctionSummaryLabel(formatAStockAuctionSummaryAmount(auctionResult)); auctionLabel != "" {
		shadowCtx.AuctionAmountLabel = auctionLabel
	}
	shadowCtx.EmptyReason = aStockRecommendationEmptyReason(shadowCtx)
	return shadowCtx
}

func (s *Server) buildSameDayMorningAStockAuctionStrengthRecommendationsWithCache(ctx aStockContext, cache *aStockRequestCache) []aStockRecommendation {
	if ctx.Period != "afternoon" || strings.TrimSpace(ctx.Date) == "" {
		return nil
	}
	morningCtx := newAStockBaseContext(ctx.Date, "morning", 1, ctx.IgnoreRecent, ctx.IgnoreLimitUp, ctx.IgnoreFundFlow, ctx.TodayMarketFilterEnabled, aStockRecommendationPhaseFinal)
	if err := s.populateAStockContextArticleStatsWithCache(&morningCtx, 1, cache); err != nil {
		return nil
	}
	return s.buildAStockAuctionStrengthSnapshotContext(morningCtx, cache, false).Recommendations
}

func (s *Server) loadAStockAuctionStrengthMarketCandidatesWithStatusWithCache(strategyDate string, cache *aStockRequestCache) ([]aStockMarketCandidate, string, model.AStockAuctionListResult) {
	date := normalizeAStockStrategyDate(strategyDate)
	result, ok := s.loadAStockMarketCandidateResultForCaptureSlotWithCache(date, "0929", cache)
	status := "date_auction"
	if !ok && date != "" {
		result, ok = s.loadAStockMarketCandidateResultForCaptureSlotWithCache("", "0929", cache)
		status = "latest_auction_fallback"
	}
	if !ok {
		return nil, "no_auction_candidates", result
	}
	candidates := aStockMarketCandidatesFromAuctionResult(result)
	if len(candidates) == 0 {
		return nil, "no_auction_candidates", result
	}
	slotDate := nonEmpty(normalizeAStockStrategyDate(result.Date), date)
	slotResults := map[string]model.AStockAuctionListResult{"0929": result}
	for _, slot := range []string{"0920", "0925"} {
		if slotResult, slotOK := s.loadAStockMarketCandidateResultForCaptureSlotWithCache(slotDate, slot, cache); slotOK {
			slotResults[slot] = slotResult
		}
	}
	return enrichAStockAuctionStrengthCandidatesWithSlotResults(candidates, slotResults), status, result
}

func enrichAStockAuctionStrengthCandidatesWithSlotResults(candidates []aStockMarketCandidate, slotResults map[string]model.AStockAuctionListResult) []aStockMarketCandidate {
	if len(candidates) == 0 || len(slotResults) == 0 {
		return candidates
	}
	bySlot := make(map[string]map[string]model.AStockAuctionAmount, len(slotResults))
	for slot, result := range slotResults {
		normalizedSlot := normalizeAStockAuctionStrengthCaptureSlot(slot)
		if normalizedSlot == "" {
			normalizedSlot = normalizeAStockAuctionStrengthCaptureSlot(result.CaptureSlot)
		}
		if normalizedSlot == "" {
			continue
		}
		bySlot[normalizedSlot] = aStockAuctionStrengthItemsByCode(result)
	}
	enriched := append([]aStockMarketCandidate(nil), candidates...)
	for i := range enriched {
		code := normalizeAStockCode(enriched[i].Code)
		for slot, items := range bySlot {
			item, ok := items[code]
			if !ok {
				continue
			}
			assignAStockAuctionStrengthSlot(&enriched[i], slot, item.AuctionAmount, item.AuctionVolume)
		}
		if enriched[i].AuctionAmount0929 <= 0 {
			enriched[i].AuctionAmount0929 = enriched[i].AuctionAmount
		}
		if enriched[i].AuctionVolume0929 <= 0 {
			enriched[i].AuctionVolume0929 = enriched[i].AuctionVolume
		}
	}
	return enriched
}

func aStockAuctionStrengthItemsByCode(result model.AStockAuctionListResult) map[string]model.AStockAuctionAmount {
	items := make(map[string]model.AStockAuctionAmount, len(result.Items))
	for _, item := range result.Items {
		code := normalizeAStockCode(item.Code)
		if code == "" {
			continue
		}
		if existing, ok := items[code]; !ok || item.AuctionAmount > existing.AuctionAmount {
			items[code] = item
		}
	}
	return items
}

func assignAStockAuctionStrengthSlot(candidate *aStockMarketCandidate, captureSlot string, amount float64, volume float64) {
	if candidate == nil {
		return
	}
	switch normalizeAStockAuctionStrengthCaptureSlot(captureSlot) {
	case "0920":
		candidate.AuctionAmount0920 = maxFloat(candidate.AuctionAmount0920, amount)
		candidate.AuctionVolume0920 = maxFloat(candidate.AuctionVolume0920, volume)
	case "0925":
		candidate.AuctionAmount0925 = maxFloat(candidate.AuctionAmount0925, amount)
		candidate.AuctionVolume0925 = maxFloat(candidate.AuctionVolume0925, volume)
	case "0929":
		candidate.AuctionAmount0929 = maxFloat(candidate.AuctionAmount0929, amount)
		candidate.AuctionVolume0929 = maxFloat(candidate.AuctionVolume0929, volume)
	}
}

func normalizeAStockAuctionStrengthCaptureSlot(value string) string {
	switch strings.TrimSpace(value) {
	case "0920":
		return "0920"
	case "0925":
		return "0925"
	case "0929", "0930":
		return "0929"
	default:
		return ""
	}
}

func buildAStockAuctionStrengthRecommendationsWithSectorGateWithSettings(hotspots []aStockHotspot, candidates []aStockMarketCandidate, sectorGate *aStockHotspotSectorGate, settings model.AStockRecommendationAlgorithmSettings) []aStockRecommendation {
	settings = model.NormalizeAStockRecommendationAlgorithmSettings(settings)
	hotspots = aStockRecommendationHotspotSliceWithSettings(hotspots, settings)
	if len(hotspots) == 0 || len(candidates) == 0 {
		return nil
	}
	byCode := make(map[string]aStockAuctionStrengthScoredRecommendation)
	for _, hotspot := range hotspots {
		for _, stock := range scoreAStockMarketCandidatesWithSettings(hotspot, candidates, sectorGate, settings) {
			strengthScore, strengthDetail := aStockAuctionStrengthSignalScore(stock)
			if !isAStockAuctionStrengthEligibleCandidate(stock) {
				continue
			}
			scoreBreakdown := buildAStockAuctionStrengthScoreBreakdownWithSettings(hotspot, stock, strengthScore, strengthDetail, settings)
			marketScore := aStockRecommendationFactorScoreTotalWithSettings(scoreBreakdown, settings)
			rec := aStockRecommendation{
				Hotspot:        hotspot.Name,
				Code:           stock.Code,
				Name:           stock.Name,
				HotspotScore:   hotspot.Score,
				MarketScore:    marketScore,
				Reason:         formatAStockAuctionStrengthReason(hotspot, stock, strengthDetail, marketScore),
				ScoreBreakdown: scoreBreakdown,
			}
			scored := aStockAuctionStrengthScoredRecommendation{
				Recommendation:  rec,
				StrengthScore:   strengthScore,
				StrongEvidence:  stock.StrongEvidence,
				AuctionAmount:   aStockAuctionStrengthFinalAmount(stock),
				AuctionVolume:   aStockAuctionStrengthFinalVolume(stock),
				CandidateSource: aStockPriorityCandidateSourceRank(stock),
			}
			if existing, ok := byCode[stock.Code]; !ok || lessAStockAuctionStrengthScoredRecommendation(existing, scored) {
				byCode[stock.Code] = scored
			}
		}
	}
	scored := make([]aStockAuctionStrengthScoredRecommendation, 0, len(byCode))
	for _, item := range byCode {
		scored = append(scored, item)
	}
	sort.SliceStable(scored, func(i, j int) bool {
		return lessAStockAuctionStrengthScoredRecommendation(scored[j], scored[i])
	})
	recommendations := make([]aStockRecommendation, 0, len(scored))
	for _, item := range scored {
		recommendations = append(recommendations, item.Recommendation)
	}
	return rerankAStockRecommendations(limitAStockRecommendationsByHotspot(recommendations, aStockReplacementPoolLimit, aStockReplacementPerHotspot))
}

type aStockAuctionStrengthScoredRecommendation struct {
	Recommendation  aStockRecommendation
	StrengthScore   int
	StrongEvidence  int
	AuctionAmount   float64
	AuctionVolume   float64
	CandidateSource int
}

func lessAStockAuctionStrengthScoredRecommendation(left aStockAuctionStrengthScoredRecommendation, right aStockAuctionStrengthScoredRecommendation) bool {
	if left.Recommendation.MarketScore != right.Recommendation.MarketScore {
		return left.Recommendation.MarketScore < right.Recommendation.MarketScore
	}
	if left.StrengthScore != right.StrengthScore {
		return left.StrengthScore < right.StrengthScore
	}
	if left.StrongEvidence != right.StrongEvidence {
		return left.StrongEvidence < right.StrongEvidence
	}
	if left.CandidateSource != right.CandidateSource {
		return left.CandidateSource > right.CandidateSource
	}
	if left.AuctionAmount != right.AuctionAmount {
		return left.AuctionAmount < right.AuctionAmount
	}
	if left.AuctionVolume != right.AuctionVolume {
		return left.AuctionVolume < right.AuctionVolume
	}
	return left.Recommendation.Code > right.Recommendation.Code
}

func isAStockAuctionStrengthEligibleCandidate(candidate aStockMarketCandidate) bool {
	if candidate.BadEvidence > 0 || candidate.Rank <= 0 || aStockAuctionStrengthFinalAmount(candidate) <= 0 {
		return false
	}
	if aStockAuctionStrengthLastStageFaded(candidate) {
		return false
	}
	return hasAStockCandidateSource(candidate, aStockCandidateSourceNews) ||
		hasAStockCandidateSource(candidate, aStockCandidateSourceSector) ||
		hasAStockCandidateSource(candidate, aStockCandidateSourceFund) ||
		len(candidate.Keywords) > 0 ||
		candidate.StrongEvidence > 0
}

func buildAStockAuctionStrengthScoreBreakdownWithSettings(hotspot aStockHotspot, stock aStockMarketCandidate, strengthScore int, strengthDetail string, settings model.AStockRecommendationAlgorithmSettings) []aStockRecommendationScoreComponent {
	components := buildAStockRecommendationScoreBreakdownWithSettings(hotspot, stock, settings)
	if strengthScore != 0 || strengthDetail != "" {
		components = append(components, newAStockScoreComponentWithFactor(aStockScoreFactorAuction, "竞价强度", nonEmpty(strengthDetail, "最终集合竞价确认"), strengthScore, strengthScore))
	}
	if stock.PreSectorScore != 0 {
		components = append(components, newAStockScoreComponentWithFactor(aStockScoreFactorSector, "板块资金预选", nonEmpty(stock.PreSectorDetail, "热点板块资金预评分"), stock.PreSectorScore, stock.PreSectorScore))
	}
	if stock.PreFundScore != 0 {
		components = append(components, newAStockScoreComponentWithFactor(aStockScoreFactorFund, "个股资金预选", nonEmpty(stock.PreFundDetail, "个股资金流入池预评分"), stock.PreFundScore, stock.PreFundScore))
	}
	return components
}

func aStockAuctionStrengthSignalScore(candidate aStockMarketCandidate) (int, string) {
	finalAmount := aStockAuctionStrengthFinalAmount(candidate)
	if finalAmount <= 0 {
		return 0, ""
	}
	score := 0
	parts := make([]string, 0, 4)
	amount0920 := candidate.AuctionAmount0920
	amount0925 := candidate.AuctionAmount0925
	amount0929 := finalAmount
	if amount0920 > 0 && amount0925 > 0 {
		switch {
		case amount0925 >= amount0920*1.05:
			score += 25
			parts = append(parts, "09:20到09:25增强")
		case amount0925 < amount0920*0.80:
			score -= 35
			parts = append(parts, "09:20到09:25回落")
		}
	}
	if amount0925 > 0 {
		switch {
		case amount0929 >= amount0925*1.05:
			score += 35
			parts = append(parts, "09:25到09:29增强")
		case amount0929 < amount0925*0.80:
			score -= 70
			parts = append(parts, "09:25到09:29回落")
		}
	}
	if amount0920 > 0 && amount0925 > 0 && amount0929 >= amount0925 && amount0925 >= amount0920 {
		score += 40
		parts = append(parts, "三段递增")
	}
	if len(parts) == 0 {
		parts = append(parts, "最终竞价额"+formatAStockAuctionMoney(finalAmount))
	}
	detail := fmt.Sprintf("%s；09:20 %s，09:25 %s，09:29 %s", strings.Join(parts, "，"), formatAStockAuctionMoney(amount0920), formatAStockAuctionMoney(amount0925), formatAStockAuctionMoney(amount0929))
	return clampAStockAuctionStrengthScore(score), detail
}

func clampAStockAuctionStrengthScore(score int) int {
	if score < -80 {
		return -80
	}
	if score > 120 {
		return 120
	}
	return score
}

func aStockAuctionStrengthLastStageFaded(candidate aStockMarketCandidate) bool {
	finalAmount := aStockAuctionStrengthFinalAmount(candidate)
	return candidate.AuctionAmount0925 > 0 && finalAmount > 0 && finalAmount < candidate.AuctionAmount0925*aStockAuctionStrengthFadeRatio
}

func aStockAuctionStrengthFinalAmount(candidate aStockMarketCandidate) float64 {
	if candidate.AuctionAmount0929 > 0 {
		return candidate.AuctionAmount0929
	}
	return candidate.AuctionAmount
}

func aStockAuctionStrengthFinalVolume(candidate aStockMarketCandidate) float64 {
	if candidate.AuctionVolume0929 > 0 {
		return candidate.AuctionVolume0929
	}
	return candidate.AuctionVolume
}

func formatAStockAuctionStrengthReason(hotspot aStockHotspot, candidate aStockMarketCandidate, strengthDetail string, marketScore int) string {
	reason := fmt.Sprintf(
		"集合竞价强承接影子策略：命中 %s，证据新闻 %d 条，热点分 %d；竞价排名 %d，成交额 %s，综合分 %d",
		strings.Join(hotspot.Keywords, "、"),
		hotspot.Evidence,
		hotspot.Score,
		candidate.Rank,
		formatAStockAuctionMoney(aStockAuctionStrengthFinalAmount(candidate)),
		marketScore,
	)
	if strengthDetail != "" {
		reason = appendAStockReason(reason, strengthDetail)
	}
	if candidate.StrongEvidence > 0 {
		reason = appendAStockReason(reason, fmt.Sprintf("强新闻证据 %d 条", candidate.StrongEvidence))
	}
	if len(candidate.Keywords) > 0 {
		reason = appendAStockReason(reason, "股票/热点关键词 "+strings.Join(candidate.Keywords, "、"))
	}
	if sources := formatAStockCandidateSources(candidate); sources != "" {
		reason = appendAStockReason(reason, "候选来源 "+sources)
	}
	if candidate.PreSectorDetail != "" {
		reason = appendAStockReason(reason, candidate.PreSectorDetail)
	}
	if candidate.PreFundDetail != "" {
		reason = appendAStockReason(reason, candidate.PreFundDetail)
	}
	return reason
}

func (s *Server) applyAStockShadowFundFlowWithCache(strategyDate string, recommendations []aStockRecommendation, cache *aStockRequestCache, settings model.AStockRecommendationAlgorithmSettings) ([]aStockRecommendation, int, int) {
	if len(recommendations) == 0 {
		return recommendations, 0, 0
	}
	settings = model.NormalizeAStockRecommendationAlgorithmSettings(settings)
	sectorTopStockResonance := s.loadAStockSectorTopStockResonanceWithCache(strategyDate, recommendations, cache)
	filtered := make([]aStockRecommendation, 0, len(recommendations))
	filteredCount := 0
	missingCount := 0
	for _, rec := range recommendations {
		assessment := s.assessAStockRecommendationFundFlow5DWithSettings(strategyDate, rec.Code, cache, settings)
		if !assessment.Missing && assessment.ScoreDelta > 0 {
			assessment = s.applyAStockHotspotSectorFundFlowCap(strategyDate, rec.Hotspot, assessment, cache)
		}
		if assessment.Missing {
			missingCount++
			rec = applyAStockFundFlow5DAssessmentToRecommendationWithSettings(rec, assessment, true, settings)
			rec = s.applyAStockSectorFundFlowTrendScoreWithCache(strategyDate, rec, cache)
			rec = applyAStockSectorTopStockResonanceToRecommendationWithSettings(rec, sectorTopStockResonance[normalizeAStockCode(rec.Code)], settings)
			filtered = append(filtered, rec)
			continue
		}
		if isAStockFundFlowHardFiltered(assessment) {
			filteredCount++
			continue
		}
		rec = applyAStockFundFlow5DAssessmentToRecommendationWithSettings(rec, assessment, true, settings)
		rec = s.applyAStockSectorFundFlowTrendScoreWithCache(strategyDate, rec, cache)
		rec = applyAStockSectorTopStockResonanceToRecommendationWithSettings(rec, sectorTopStockResonance[normalizeAStockCode(rec.Code)], settings)
		filtered = append(filtered, rec)
	}
	return sortAStockRecommendationsByScore(filtered), filteredCount, missingCount
}

func filterAStockAuctionStrengthMarketRiskRecommendations(period string, recommendations []aStockRecommendation, backtests []aStockBacktestRow, settings model.AStockRecommendationAlgorithmSettings) ([]aStockRecommendation, []aStockBacktestRow, int) {
	if len(recommendations) == 0 {
		return recommendations, backtests, 0
	}
	normalizedPeriod := normalizeAStockPeriod(period).Key
	backtestsByCode := aStockBacktestRowsByCode(backtests)
	filtered := make([]aStockRecommendation, 0, len(recommendations))
	filteredCount := 0
	for _, rec := range recommendations {
		row := backtestsByCode[normalizeAStockCode(rec.Code)]
		t0Return, hasT0 := parseAStockPctText(row.T0Return)
		highOpenScore := aStockRecommendationScoreByLabel(rec, "当日高开")
		lowOpenScore := aStockRecommendationScoreByLabel(rec, "当日低开")
		switch {
		case isAStockT1ShadowDrawdownBlocked(rec):
			filteredCount++
			continue
		case normalizedPeriod == "morning" && highOpenScore >= settings.Auction.HighOpenScore5 && hasT0 && t0Return <= 0:
			filteredCount++
			continue
		case normalizedPeriod == "morning" && highOpenScore >= settings.Auction.HighOpenStrongScore:
			filteredCount++
			continue
		case normalizedPeriod == "morning" && lowOpenScore < 0 && hasT0 && t0Return <= 0:
			filteredCount++
			continue
		}
		filtered = append(filtered, rec)
	}
	filtered = sortAStockRecommendationsByScore(filtered)
	return filtered, filterAStockBacktestsForRecommendations(backtests, filtered), filteredCount
}

func aStockRecommendationScoreByLabel(rec aStockRecommendation, label string) int {
	for _, component := range aStockRecommendationScoreBreakdown(rec) {
		if strings.TrimSpace(component.Label) == label {
			return component.Score
		}
	}
	return 0
}

func formatAStockAuctionStrengthStatus(fundFlowFiltered int, fundFlowMissing int, riskFiltered int) string {
	parts := make([]string, 0, 3)
	if fundFlowFiltered > 0 {
		parts = append(parts, fmt.Sprintf("强承接影子策略过滤资金净流出 %d 只", fundFlowFiltered))
	}
	if riskFiltered > 0 {
		parts = append(parts, fmt.Sprintf("强承接影子策略过滤高开无承接/回撤 %d 只", riskFiltered))
	}
	if fundFlowMissing > 0 {
		parts = append(parts, fmt.Sprintf("强承接影子策略资金缺失 %d 只", fundFlowMissing))
	}
	return strings.Join(parts, "，")
}

func buildAStockT1ShadowRecommendationsWithSectorGate(hotspots []aStockHotspot, marketCandidates []aStockMarketCandidate, sectorGate *aStockHotspotSectorGate) []aStockRecommendation {
	if len(hotspots) > aStockHotspotLimit {
		hotspots = hotspots[:aStockHotspotLimit]
	}
	candidates := aStockRecommendationCandidatesForLimit(hotspots, marketCandidates, aStockReplacementPoolLimit, aStockReplacementPerHotspot)
	if len(hotspots) == 0 || len(candidates) == 0 {
		return nil
	}
	byCode := make(map[string]aStockT1ShadowScoredRecommendation)
	for _, hotspot := range hotspots {
		for _, stock := range scoreAStockMarketCandidatesWithSectorGate(hotspot, candidates, sectorGate) {
			strongEvidence := aStockT1ShadowStrongEvidence(stock)
			if !isAStockT1ShadowEligibleCandidate(stock, strongEvidence) {
				continue
			}
			marketScore := aStockT1ShadowMarketScore(hotspot, stock, strongEvidence)
			rec := aStockRecommendation{
				Hotspot:      hotspot.Name,
				Code:         stock.Code,
				Name:         stock.Name,
				HotspotScore: hotspot.Score,
				MarketScore:  marketScore,
				Reason:       formatAStockT1ShadowReason(hotspot, stock, strongEvidence, marketScore),
			}
			scored := aStockT1ShadowScoredRecommendation{
				Recommendation: rec,
				StrongEvidence: strongEvidence,
				AuctionAmount:  stock.AuctionAmount,
				AuctionVolume:  stock.AuctionVolume,
			}
			if existing, ok := byCode[stock.Code]; !ok || lessAStockT1ShadowScoredRecommendation(existing, scored) {
				byCode[stock.Code] = scored
			}
		}
	}
	scored := make([]aStockT1ShadowScoredRecommendation, 0, len(byCode))
	for _, item := range byCode {
		scored = append(scored, item)
	}
	sort.SliceStable(scored, func(i, j int) bool {
		return lessAStockT1ShadowScoredRecommendation(scored[j], scored[i])
	})
	recommendations := make([]aStockRecommendation, 0, len(scored))
	for _, item := range scored {
		recommendations = append(recommendations, item.Recommendation)
	}
	return rerankAStockRecommendations(limitAStockRecommendationsByHotspot(recommendations, aStockReplacementPoolLimit, aStockReplacementPerHotspot))
}

type aStockT1ShadowScoredRecommendation struct {
	Recommendation aStockRecommendation
	StrongEvidence int
	AuctionAmount  float64
	AuctionVolume  float64
}

func lessAStockT1ShadowScoredRecommendation(left aStockT1ShadowScoredRecommendation, right aStockT1ShadowScoredRecommendation) bool {
	if left.Recommendation.MarketScore != right.Recommendation.MarketScore {
		return left.Recommendation.MarketScore < right.Recommendation.MarketScore
	}
	if left.StrongEvidence != right.StrongEvidence {
		return left.StrongEvidence < right.StrongEvidence
	}
	if left.AuctionAmount != right.AuctionAmount {
		return left.AuctionAmount < right.AuctionAmount
	}
	if left.AuctionVolume != right.AuctionVolume {
		return left.AuctionVolume < right.AuctionVolume
	}
	return left.Recommendation.Code > right.Recommendation.Code
}

func aStockT1ShadowStrongEvidence(candidate aStockMarketCandidate) int {
	strongEvidence := candidate.Evidence - candidate.WeakEvidence - candidate.BadEvidence
	if strongEvidence < 0 {
		return 0
	}
	return strongEvidence
}

func isAStockT1ShadowEligibleCandidate(candidate aStockMarketCandidate, strongEvidence int) bool {
	if candidate.BadEvidence > 0 || strongEvidence <= 0 {
		return false
	}
	if candidate.Fallback {
		return true
	}
	return len(candidate.Keywords) > 0
}

func aStockT1ShadowMarketScore(hotspot aStockHotspot, candidate aStockMarketCandidate, strongEvidence int) int {
	score := hotspot.Score + strongEvidence*80 + aStockMarketRankScore(candidate.Rank)*2 + len(candidate.Keywords)*25
	if candidate.Fallback {
		score += 30
	}
	return score
}

func formatAStockT1ShadowReason(hotspot aStockHotspot, candidate aStockMarketCandidate, strongEvidence int, marketScore int) string {
	source := "集合竞价候选"
	if candidate.Fallback {
		source = "实时新闻明确提及股票"
	}
	reason := fmt.Sprintf("T+1影子策略：%s，强新闻证据 %d 条，热点分 %d，T+1分 %d", source, strongEvidence, hotspot.Score, marketScore)
	if len(candidate.Keywords) > 0 {
		reason = appendAStockReason(reason, "股票/热点关键词 "+strings.Join(candidate.Keywords, "、"))
	}
	return reason
}

func (s *Server) applyAStockT1ShadowFundFlowWithCache(strategyDate string, recommendations []aStockRecommendation, cache *aStockRequestCache) ([]aStockRecommendation, int, int) {
	if len(recommendations) == 0 {
		return recommendations, 0, 0
	}
	sectorTopStockResonance := s.loadAStockSectorTopStockResonanceWithCache(strategyDate, recommendations, cache)
	settings := s.loadAStockAlgorithmSettingsWithCache(cache)
	filtered := make([]aStockRecommendation, 0, len(recommendations))
	filteredCount := 0
	missingCount := 0
	for _, rec := range recommendations {
		assessment := s.assessAStockRecommendationFundFlow5DWithCache(strategyDate, rec.Code, cache)
		if !assessment.Missing && assessment.ScoreDelta > 0 {
			assessment = s.applyAStockHotspotSectorFundFlowCap(strategyDate, rec.Hotspot, assessment, cache)
		}
		if assessment.Missing {
			missingCount++
			rec = applyAStockFundFlow5DAssessmentToRecommendationWithSettings(rec, assessment, true, settings)
			rec = s.applyAStockSectorFundFlowTrendScoreWithCache(strategyDate, rec, cache)
			rec = applyAStockSectorTopStockResonanceToRecommendationWithSettings(rec, sectorTopStockResonance[normalizeAStockCode(rec.Code)], settings)
			filtered = append(filtered, rec)
			continue
		}
		if isAStockFundFlowHardFiltered(assessment) {
			filteredCount++
			continue
		}
		rec = applyAStockFundFlow5DAssessmentToRecommendationWithSettings(rec, assessment, true, settings)
		rec = s.applyAStockSectorFundFlowTrendScoreWithCache(strategyDate, rec, cache)
		rec = applyAStockSectorTopStockResonanceToRecommendationWithSettings(rec, sectorTopStockResonance[normalizeAStockCode(rec.Code)], settings)
		filtered = append(filtered, rec)
	}
	return sortAStockRecommendationsByScore(filtered), filteredCount, missingCount
}

func filterAStockT1ShadowMarketRiskRecommendations(period string, recommendations []aStockRecommendation, backtests []aStockBacktestRow) ([]aStockRecommendation, []aStockBacktestRow, int) {
	if len(recommendations) == 0 {
		return recommendations, backtests, 0
	}
	normalizedPeriod := normalizeAStockPeriod(period).Key
	filtered := make([]aStockRecommendation, 0, len(recommendations))
	filteredCount := 0
	for _, rec := range recommendations {
		if isAStockT1ShadowDrawdownBlocked(rec) || (normalizedPeriod == "morning" && isAStockT1ShadowLowOpenBlocked(rec)) {
			filteredCount++
			continue
		}
		filtered = append(filtered, rec)
	}
	filtered = sortAStockRecommendationsByScore(filtered)
	return filtered, filterAStockBacktestsForRecommendations(backtests, filtered), filteredCount
}

func isAStockT1ShadowDrawdownBlocked(rec aStockRecommendation) bool {
	for _, value := range []string{rec.Change30, rec.Change60} {
		pct, ok := parseAStockPctText(value)
		if ok && pct <= aStockT1ShadowDrawdownThreshold {
			return true
		}
	}
	return false
}

func isAStockT1ShadowLowOpenBlocked(rec aStockRecommendation) bool {
	return strings.Contains(rec.Reason, "开盘低开")
}

func parseAStockPctText(value string) (float64, bool) {
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

func limitAStockRecommendationsByHotspot(recommendations []aStockRecommendation, maxRecommendations int, maxPerHotspot int) []aStockRecommendation {
	if maxRecommendations <= 0 {
		maxRecommendations = len(recommendations)
	}
	if maxPerHotspot <= 0 {
		maxPerHotspot = maxRecommendations
	}
	limited := make([]aStockRecommendation, 0, minInt(len(recommendations), maxRecommendations))
	hotspotCounts := make(map[string]int)
	seen := make(map[string]struct{})
	for _, rec := range recommendations {
		code := normalizeAStockCode(rec.Code)
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		hotspot := normalizeAStockRecommendationHotspot(rec.Hotspot)
		if hotspotCounts[hotspot] >= maxPerHotspot {
			continue
		}
		rec.Code = code
		limited = append(limited, rec)
		seen[code] = struct{}{}
		hotspotCounts[hotspot]++
		if len(limited) >= maxRecommendations {
			break
		}
	}
	return rerankAStockRecommendations(limited)
}

func formatAStockT1ShadowStatus(fundFlowFiltered int, fundFlowMissing int, riskFiltered int) string {
	parts := make([]string, 0, 3)
	if fundFlowFiltered > 0 {
		parts = append(parts, fmt.Sprintf("T+1影子策略过滤资金净流出 %d 只", fundFlowFiltered))
	}
	if riskFiltered > 0 {
		parts = append(parts, fmt.Sprintf("T+1影子策略过滤回撤/低开 %d 只", riskFiltered))
	}
	if fundFlowMissing > 0 {
		parts = append(parts, fmt.Sprintf("T+1影子策略资金缺失 %d 只", fundFlowMissing))
	}
	return strings.Join(parts, "，")
}

func formatAStockDailyRecommendationLimitStatus(filtered int) string {
	if filtered <= 0 {
		return ""
	}
	return fmt.Sprintf("日内推荐上限过滤 %d 只", filtered)
}

func (s *Server) restoreAStockBacktestsFromSnapshot(ctx *aStockContext) {
	s.restoreAStockBacktestsFromSnapshotWithCache(ctx, nil)
}

func (s *Server) restoreAStockBacktestsFromSnapshotWithCache(ctx *aStockContext, cache *aStockRequestCache) {
	if ctx == nil || strings.TrimSpace(s.cfg.ContentURL) == "" || len(ctx.Backtests) == 0 || !aStockBacktestsNeedRestore(ctx.Backtests, ctx.BacktestStatus) {
		return
	}
	snapshot, ok := s.loadAStockRecommendationSnapshotForContextWithCache(*ctx, cache)
	if !ok {
		return
	}
	var previous []aStockBacktestRow
	if err := json.Unmarshal([]byte(normalizeAStockSnapshotJSONArray(snapshot.BacktestsJSON)), &previous); err != nil || len(previous) == 0 {
		return
	}
	currentFilled := aStockBacktestFilledDataCount(ctx.Backtests)
	previousFilled := aStockBacktestFilledDataCount(previous)
	merged, changed := mergeAStockBacktestRowsFromSnapshot(ctx.Backtests, previous)
	if !changed {
		return
	}
	ctx.Backtests = merged
	mergedFilled := aStockBacktestFilledDataCount(ctx.Backtests)
	if mergedFilled > currentFilled && (aStockBacktestStatusNeedsRestore(ctx.BacktestStatus) || currentFilled < previousFilled) {
		ctx.BacktestStatus = nonEmpty(snapshot.BacktestStatus, ctx.BacktestStatus)
	}
}

func mergeAStockBacktestRowsFromSnapshot(current []aStockBacktestRow, previous []aStockBacktestRow) ([]aStockBacktestRow, bool) {
	if len(current) == 0 || len(previous) == 0 {
		return current, false
	}
	previousByCode := make(map[string]aStockBacktestRow, len(previous))
	for _, row := range previous {
		code := aStockBacktestRowCode(row)
		if code == "" {
			continue
		}
		previousByCode[code] = row
	}
	merged := make([]aStockBacktestRow, len(current))
	changed := false
	for i, row := range current {
		code := aStockBacktestRowCode(row)
		previousRow, ok := previousByCode[code]
		if !ok {
			merged[i] = row
			continue
		}
		mergedRow, rowChanged := mergeAStockBacktestRowFromSnapshot(row, previousRow)
		merged[i] = mergedRow
		changed = changed || rowChanged
	}
	return merged, changed
}

func mergeAStockBacktestRowFromSnapshot(current aStockBacktestRow, previous aStockBacktestRow) (aStockBacktestRow, bool) {
	originalFilled := aStockBacktestRowFilledDataCount(current)
	previousFilled := aStockBacktestRowFilledDataCount(previous)
	changed := false
	if aStockBacktestValueMissing(current.EntryOpen) && !aStockBacktestValueMissing(previous.EntryOpen) {
		current.EntryOpen = previous.EntryOpen
		changed = true
	}
	if aStockBacktestValueMissing(current.AfternoonOpen) && !aStockBacktestValueMissing(previous.AfternoonOpen) {
		current.AfternoonOpen = previous.AfternoonOpen
		changed = true
	}
	if aStockBacktestValueMissing(current.T0Return) && !aStockBacktestValueMissing(previous.T0Return) {
		current.T0Return = previous.T0Return
		current.T0ReturnClass = previous.T0ReturnClass
		changed = true
	}
	if aStockBacktestValueMissing(current.T0Close) && !aStockBacktestValueMissing(previous.T0Close) {
		current.T0Close = previous.T0Close
		changed = true
	}
	if aStockBacktestValueMissing(current.CurrentPrice) && !aStockBacktestValueMissing(previous.CurrentPrice) {
		current.CurrentPrice = previous.CurrentPrice
		changed = true
	}
	if aStockBacktestValueMissing(current.CurrentReturn) && !aStockBacktestValueMissing(previous.CurrentReturn) {
		current.CurrentReturn = previous.CurrentReturn
		current.CurrentReturnClass = previous.CurrentReturnClass
		changed = true
	} else if strings.TrimSpace(current.CurrentReturnClass) == "" && strings.TrimSpace(previous.CurrentReturnClass) != "" {
		current.CurrentReturnClass = previous.CurrentReturnClass
		changed = true
	}
	if aStockBacktestValueMissing(current.CurrentMarketPct) && !aStockBacktestValueMissing(previous.CurrentMarketPct) {
		current.CurrentMarketPct = previous.CurrentMarketPct
		current.CurrentMarketPctClass = previous.CurrentMarketPctClass
		changed = true
	} else if strings.TrimSpace(current.CurrentMarketPctClass) == "" && strings.TrimSpace(previous.CurrentMarketPctClass) != "" {
		current.CurrentMarketPctClass = previous.CurrentMarketPctClass
		changed = true
	}
	if aStockBacktestValueMissing(current.BestReturn) && !aStockBacktestValueMissing(previous.BestReturn) {
		current.BestReturn = previous.BestReturn
		current.BestReturnClass = previous.BestReturnClass
		changed = true
	}
	if len(previous.Days) > len(current.Days) {
		extra := make([]aStockBacktestCell, len(previous.Days)-len(current.Days))
		current.Days = append(current.Days, extra...)
		changed = true
	}
	for i := range current.Days {
		if i >= len(previous.Days) {
			break
		}
		if aStockBacktestValueMissing(current.Days[i].Close) && !aStockBacktestValueMissing(previous.Days[i].Close) {
			current.Days[i].Close = previous.Days[i].Close
			changed = true
		}
		if aStockBacktestValueMissing(current.Days[i].Return) && !aStockBacktestValueMissing(previous.Days[i].Return) {
			current.Days[i].Return = previous.Days[i].Return
			current.Days[i].ReturnClass = previous.Days[i].ReturnClass
			changed = true
		} else if strings.TrimSpace(current.Days[i].ReturnClass) == "" && strings.TrimSpace(previous.Days[i].ReturnClass) != "" {
			current.Days[i].ReturnClass = previous.Days[i].ReturnClass
			changed = true
		}
		if aStockBacktestValueMissing(current.Days[i].MarketPct) && !aStockBacktestValueMissing(previous.Days[i].MarketPct) {
			current.Days[i].MarketPct = previous.Days[i].MarketPct
			current.Days[i].MarketPctClass = previous.Days[i].MarketPctClass
			changed = true
		} else if strings.TrimSpace(current.Days[i].MarketPctClass) == "" && strings.TrimSpace(previous.Days[i].MarketPctClass) != "" {
			current.Days[i].MarketPctClass = previous.Days[i].MarketPctClass
			changed = true
		}
	}
	if previousFilled > originalFilled && aStockBacktestStatusNeedsRestore(current.Status) && strings.TrimSpace(previous.Status) != "" {
		current.Status = previous.Status
		changed = true
	}
	return current, changed
}

func aStockBacktestFilledDataCount(rows []aStockBacktestRow) int {
	total := 0
	for _, row := range rows {
		total += aStockBacktestRowFilledDataCount(row)
	}
	return total
}

func aStockBacktestRowFilledDataCount(row aStockBacktestRow) int {
	total := 0
	for _, value := range []string{row.EntryOpen, row.AfternoonOpen, row.T0Return, row.T0Close, row.CurrentPrice, row.CurrentReturn, row.CurrentMarketPct, row.BestReturn} {
		if !aStockBacktestValueMissing(value) {
			total++
		}
	}
	for _, day := range row.Days {
		if !aStockBacktestValueMissing(day.Close) {
			total++
		}
		if !aStockBacktestValueMissing(day.Return) {
			total++
		}
		if !aStockBacktestValueMissing(day.MarketPct) {
			total++
		}
	}
	return total
}

func aStockBacktestStatusNeedsRestore(status string) bool {
	status = strings.TrimSpace(status)
	if status == "" {
		return true
	}
	return strings.Contains(status, "等待下午开盘价") ||
		strings.Contains(status, "等待当日开盘价") ||
		strings.Contains(status, "无行情") ||
		strings.Contains(status, "行情读取失败") ||
		strings.Contains(status, "缺少当日行情")
}

func aStockBacktestsNeedRestore(rows []aStockBacktestRow, status string) bool {
	if aStockBacktestStatusNeedsRestore(status) {
		return true
	}
	for _, row := range rows {
		if aStockBacktestStatusNeedsRestore(row.Status) {
			return true
		}
	}
	return false
}

func aStockBacktestValueMissing(value string) bool {
	value = strings.TrimSpace(value)
	return value == "" || value == "--"
}

func aStockBacktestRowCode(row aStockBacktestRow) string {
	fields := strings.Fields(strings.TrimSpace(row.Stock))
	if len(fields) == 0 {
		return ""
	}
	return normalizeAStockCode(fields[0])
}

type aStockPreopenPopupWindow struct {
	Period      string
	KeyPrefix   string
	Title       string
	WindowLabel string
	StartHour   int
	StartMinute int
	EndHour     int
	EndMinute   int
}

func (s *Server) buildAStockPreopenPopup(userID int64, strategyDate string) aStockPopupPayload {
	date := normalizeAStockStrategyDate(strategyDate)
	if date == "" {
		date = aStockTodayDate()
	}
	window, active := aStockPreopenPopupWindowForNow(aStockNow().In(aStockLocation()))
	payload := aStockPopupPayload{
		Key:          aStockPreopenPopupKey(window, date),
		Title:        window.Title,
		StrategyDate: date,
		Period:       window.Period,
	}
	if userID <= 0 || date != aStockTodayDate() || !active {
		return payload
	}
	state, ok, err := s.getPopupState(userID, payload.Key)
	if err == nil && ok && state.Dismissed {
		return payload
	}
	recommendations, updatedAt, ok := s.loadAStockPopupRecommendations(date, window.Period)
	if !ok || len(recommendations) == 0 {
		return payload
	}
	items := make([]aStockPopupRecommendation, 0, len(recommendations))
	for _, rec := range recommendations {
		items = append(items, aStockPopupRecommendation{
			Rank:    rec.Rank,
			Code:    rec.Code,
			Name:    rec.Name,
			Hotspot: rec.Hotspot,
			Reason:  truncateAStockText(rec.Reason, 96),
		})
	}
	payload.Show = true
	payload.UpdatedAt = formatShanghaiTime(updatedAt)
	payload.Meta = fmt.Sprintf("%s %s 盘前窗口已生成 %d 只推荐股票", date, window.WindowLabel, len(items))
	if payload.UpdatedAt != "--" {
		payload.Meta += "，更新时间 " + payload.UpdatedAt
	}
	payload.Recommendations = items
	return payload
}

func (s *Server) loadAStockPopupRecommendations(strategyDate string, period string) ([]aStockRecommendation, time.Time, bool) {
	normalizedPeriod := normalizeAStockPeriod(period).Key
	if result, ok := s.loadAStockRecommendationSelections(strategyDate, normalizedPeriod); ok && len(result.Items) > 0 {
		recommendations, _ := s.repairAStockPersistedRecommendationsForPeriodWithCache(strategyDate, normalizedPeriod, aStockRecommendationSelectionsToRecommendations(result.Items, normalizedPeriod), nil)
		if len(recommendations) == 0 {
			return nil, time.Time{}, false
		}
		return recommendations, result.UpdatedAt, true
	}
	snapshot, ok := s.loadAStockRecommendationSnapshot(strategyDate, normalizedPeriod, false)
	if !ok {
		return nil, time.Time{}, false
	}
	var recommendations []aStockRecommendation
	if err := json.Unmarshal([]byte(normalizeAStockSnapshotJSONArray(snapshot.RecommendationsJSON)), &recommendations); err != nil || len(recommendations) == 0 {
		return nil, time.Time{}, false
	}
	recommendations, _ = s.repairAStockPersistedRecommendationsForPeriodWithCache(strategyDate, normalizedPeriod, recommendations, nil)
	if len(recommendations) == 0 {
		return nil, time.Time{}, false
	}
	return recommendations, snapshot.UpdatedAt, true
}

func aStockPreopenPopupWindows() []aStockPreopenPopupWindow {
	return []aStockPreopenPopupWindow{
		{
			Period:      "morning",
			KeyPrefix:   "a-stock-morning-preopen-recommendation-0927-",
			Title:       "09:27 上午盘前推荐股票",
			WindowLabel: "08:00-09:26:59",
			StartHour:   9,
			StartMinute: 27,
			EndHour:     9,
			EndMinute:   31,
		},
		{
			Period:      "morning",
			KeyPrefix:   "a-stock-morning-preopen-recommendation-0931-",
			Title:       "09:31 上午盘前推荐股票",
			WindowLabel: "08:00-09:26:59",
			StartHour:   9,
			StartMinute: 31,
			EndHour:     9,
			EndMinute:   35,
		},
		{
			Period:      "afternoon",
			KeyPrefix:   "a-stock-afternoon-preopen-recommendation-1257-",
			Title:       "12:57 下午盘前推荐股票",
			WindowLabel: "09:30-12:56:59",
			StartHour:   12,
			StartMinute: 57,
			EndHour:     13,
			EndMinute:   1,
		},
		{
			Period:      "afternoon",
			KeyPrefix:   "a-stock-afternoon-preopen-recommendation-1301-",
			Title:       "13:01 下午盘前推荐股票",
			WindowLabel: "09:30-12:56:59",
			StartHour:   13,
			StartMinute: 1,
			EndHour:     13,
			EndMinute:   5,
		},
	}
}

func aStockPreopenPopupWindowForNow(now time.Time) (aStockPreopenPopupWindow, bool) {
	windows := aStockPreopenPopupWindows()
	for _, window := range windows {
		end := aStockPopupWindowTime(now, window.EndHour, window.EndMinute)
		if now.Before(end) {
			return window, aStockPreopenPopupWindowActive(now, window)
		}
	}
	window := windows[len(windows)-1]
	return window, false
}

func aStockPreopenPopupWindowActive(now time.Time, window aStockPreopenPopupWindow) bool {
	start := aStockPopupWindowTime(now, window.StartHour, window.StartMinute)
	end := aStockPopupWindowTime(now, window.EndHour, window.EndMinute)
	return !now.Before(start) && now.Before(end)
}

func aStockPopupWindowTime(now time.Time, hour int, minute int) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
}

func aStockPreopenPopupKey(window aStockPreopenPopupWindow, strategyDate string) string {
	if strings.TrimSpace(window.KeyPrefix) == "" {
		return ""
	}
	return window.KeyPrefix + normalizeAStockStrategyDate(strategyDate)
}

func aStockActionRequiresTradingDay(action string) bool {
	switch strings.TrimSpace(action) {
	case "crawl", "backfill_window_news", "backfill_morning_stock", "generate_morning_stock", "generate_afternoon_stock", "generate_evening_stock", "generate_ignore_recent_stock", "generate", "refresh_current_backtest", "recalculate":
		return true
	default:
		return false
	}
}

func (s *Server) aStockRecommendationBlockedMessage(strategyDate string) (bool, string) {
	blocked, message, _ := s.aStockRecommendationBlockedStatus(strategyDate)
	return blocked, message
}

func (s *Server) aStockRecommendationBlockedStatus(strategyDate string) (bool, string, string) {
	status, err := s.loadAStockTradingDayStatus(strategyDate)
	return aStockBlockedStatusFromTradingDay(status, err)
}

func (s *Server) aStockRecommendationBlockedStatusWithCache(strategyDate string, cache *aStockRequestCache) (bool, string, string) {
	status, err := s.loadAStockTradingDayStatusWithCache(strategyDate, cache)
	return aStockBlockedStatusFromTradingDay(status, err)
}

func aStockBlockedStatusFromTradingDay(status aStockTradingDayStatus, err error) (bool, string, string) {
	if err != nil {
		return true, "交易日历不可用，不生成股票推荐：" + err.Error(), "calendar_unavailable"
	}
	if status.IsTradingDay {
		return false, "", status.Reason
	}
	message := strings.TrimSpace(status.Message)
	if message == "" {
		message = "该日 A 股休市，不生成股票推荐。"
	}
	return true, message, status.Reason
}

func (s *Server) loadAStockTradingDayStatusWithCache(strategyDate string, cache *aStockRequestCache) (aStockTradingDayStatus, error) {
	if cache == nil {
		return s.loadAStockTradingDayStatus(strategyDate)
	}
	date := normalizeAStockStrategyDate(strategyDate)
	if entry, ok := cache.tradingDay[date]; ok {
		return entry.status, entry.err
	}
	status, err := s.loadAStockTradingDayStatus(strategyDate)
	cache.tradingDay[date] = aStockTradingDayCacheEntry{status: status, err: err}
	return status, err
}

func (s *Server) loadAStockTradingDayStatus(strategyDate string) (aStockTradingDayStatus, error) {
	if strings.TrimSpace(s.cfg.SchedulerURL) == "" {
		return aStockTradingDayStatus{
			Date:         normalizeAStockStrategyDate(strategyDate),
			IsTradingDay: true,
			Source:       "scheduler_not_configured",
			Reason:       "calendar_check_disabled",
			Message:      "trading calendar check disabled because scheduler URL is not configured",
		}, nil
	}
	date := normalizeAStockStrategyDate(strategyDate)
	query := "/api/v1/scheduler/a-stock/trading-day"
	if date != "" {
		query += "?date=" + url.QueryEscape(date)
	}
	var status aStockTradingDayStatus
	if err := s.getJSON(s.cfg.SchedulerURL+query, &status); err != nil {
		return localAStockTradingDayStatus(date), nil
	}
	if strings.TrimSpace(status.Date) == "" {
		status.Date = date
	}
	if strings.TrimSpace(status.Reason) == "" && !status.IsTradingDay {
		status.Reason = "market_closed"
	}
	return status, nil
}

func (s *Server) loadAStockSourceRunsWithCache(cache *aStockRequestCache) []aStockSourceRun {
	if cache == nil {
		return s.loadAStockSourceRuns()
	}
	if cache.sourceRuns != nil {
		return cache.sourceRuns
	}
	cache.sourceRuns = s.loadAStockSourceRuns()
	return cache.sourceRuns
}

func (s *Server) loadAStockSourceRuns() []aStockSourceRun {
	if strings.TrimSpace(s.cfg.CrawlerURL) == "" {
		return aStockUnavailableSourceRuns("抓取状态接口未配置")
	}
	var runs []model.CrawlRun
	if err := s.getJSON(s.cfg.CrawlerURL+"/api/v1/admin/tasks/crawl/runs?limit=50", &runs); err != nil {
		return aStockUnavailableSourceRuns(err.Error())
	}
	latest := make(map[string]aStockSourceRun)
	for _, run := range runs {
		sourceType := strings.TrimSpace(run.SourceType)
		if sourceType == "" {
			continue
		}
		if _, ok := latest[sourceType]; ok {
			continue
		}
		latest[sourceType] = aStockSourceRun{
			SourceType:    sourceType,
			Status:        run.Status,
			FetchedCount:  run.FetchedCount,
			InsertedCount: run.InsertedCount,
			UpdatedCount:  run.UpdatedCount,
			ErrorText:     run.ErrorText,
			StartedAt:     run.StartedAt,
		}
	}
	out := make([]aStockSourceRun, 0, len(aStockCrawlSources()))
	for _, sourceType := range aStockCrawlSources() {
		if latest[sourceType].SourceType == "" {
			if run, ok := s.loadLatestAStockSourceRun(sourceType); ok {
				latest[sourceType] = run
			}
		}
		out = append(out, latest[sourceType])
	}
	return out
}

func (s *Server) loadLatestAStockSourceRun(sourceType string) (aStockSourceRun, bool) {
	var runs []model.CrawlRun
	query := "/api/v1/admin/tasks/crawl/runs?limit=1&source_type=" + url.QueryEscape(sourceType)
	if err := s.getJSON(s.cfg.CrawlerURL+query, &runs); err != nil || len(runs) == 0 {
		return aStockSourceRun{}, false
	}
	run := runs[0]
	if strings.TrimSpace(run.SourceType) == "" {
		run.SourceType = sourceType
	}
	return aStockSourceRun{
		SourceType:    strings.TrimSpace(run.SourceType),
		Status:        run.Status,
		FetchedCount:  run.FetchedCount,
		InsertedCount: run.InsertedCount,
		UpdatedCount:  run.UpdatedCount,
		ErrorText:     run.ErrorText,
		StartedAt:     run.StartedAt,
	}, true
}

func aStockUnavailableSourceRuns(message string) []aStockSourceRun {
	out := make([]aStockSourceRun, 0, len(aStockCrawlSources()))
	for _, sourceType := range aStockCrawlSources() {
		out = append(out, aStockSourceRun{
			SourceType: sourceType,
			Status:     "unavailable",
			ErrorText:  message,
		})
	}
	return out
}

func formatAStockCrawlRunStatus(run aStockSourceRun) string {
	if strings.TrimSpace(run.SourceType) == "" {
		return "--"
	}
	status := strings.TrimSpace(run.Status)
	if status == "" {
		status = "--"
	}
	if !run.StartedAt.IsZero() {
		return status + " " + run.StartedAt.In(aStockLocation()).Format("15:04")
	}
	return status
}

func formatAStockCrawlRunCounts(run aStockSourceRun) string {
	if strings.TrimSpace(run.SourceType) == "" {
		return "--"
	}
	return fmt.Sprintf("%d/%d/%d", run.FetchedCount, run.InsertedCount, run.UpdatedCount)
}

func formatAStockCrawlRunNote(run aStockSourceRun) string {
	if strings.TrimSpace(run.SourceType) == "" {
		return "暂无抓取记录"
	}
	if strings.TrimSpace(run.ErrorText) != "" {
		return truncateAStockText(run.ErrorText, 80)
	}
	if run.FetchedCount == 0 {
		return "源站返回 0 条或窗口过滤后为 0"
	}
	if run.InsertedCount == 0 && run.UpdatedCount == 0 {
		return "抓到内容但没有新增/更新"
	}
	return ""
}

func truncateAStockText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 || len([]rune(text)) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit]) + "..."
}

func localAStockTradingDayStatus(strategyDate string) aStockTradingDayStatus {
	date := normalizeAStockStrategyDate(strategyDate)
	if strings.TrimSpace(date) == "" {
		date = aStockTodayDate()
	}
	isTradingDay := isLocalAStockTradingDay(date)
	reason := "trading_day"
	message := "A-share market is open."
	if !isTradingDay {
		reason = "market_closed"
		message = "该日 A 股休市，不生成股票推荐。"
	}
	return aStockTradingDayStatus{
		Date:               date,
		IsTradingDay:       isTradingDay,
		LatestTradingDay:   localAStockAdjacentTradingDay(date, 0),
		PreviousTradingDay: localAStockAdjacentTradingDay(date, -1),
		NextTradingDay:     localAStockAdjacentTradingDay(date, 1),
		Source:             "portal_local_calendar_fallback",
		Reason:             reason,
		Message:            message,
	}
}

func isLocalAStockTradingDay(date string) bool {
	return astockcalendar.IsTradingDay(date)
}

func localAStockAdjacentTradingDay(date string, direction int) string {
	return astockcalendar.AdjacentTradingDay(date, direction)
}

func aStockRecommendationEmptyReason(ctx aStockContext) string {
	if len(ctx.Recommendations) > 0 {
		return ""
	}
	newsTotal := ctx.RecommendationNewsTotal
	if newsTotal == 0 {
		newsTotal = len(ctx.Articles)
	}
	windowLabel := strings.TrimSpace(ctx.RecommendationWindowLabel)
	if windowLabel == "" {
		windowLabel = ctx.WindowLabel
	}
	if ctx.TradingDayBlocked {
		message := strings.TrimSpace(ctx.TradingDayMessage)
		if message == "" {
			message = "该日 A 股休市，不生成股票推荐。"
		}
		return "暂无推荐股票：" + message
	}
	if newsTotal == 0 {
		return fmt.Sprintf("暂无推荐股票：%s %s 没有新闻，请先抓取或补抓财经信息。", ctx.PeriodLabel, windowLabel)
	}
	if len(ctx.Hotspots) == 0 {
		return fmt.Sprintf("暂无推荐股票：%s %s 有 %d 条新闻，但未命中 A股热点关键词。", ctx.PeriodLabel, windowLabel, newsTotal)
	}
	if ctx.MarketCandidateStatus == "no_auction_candidates" || ctx.MarketCandidateStatus == "content_unconfigured" {
		return fmt.Sprintf("暂无推荐股票：%s %s 有新闻和热点，但没有集合竞价候选数据。请点击“补录集合竞价”后重新生成推荐。", ctx.PeriodLabel, windowLabel)
	}
	if ctx.NegativeNoEvidenceFiltered > 0 && len(ctx.Recommendations) == 0 {
		return fmt.Sprintf("暂无推荐股票：%s %s 已生成候选，但负面新闻且无个股有效证据过滤 %d 只。", ctx.PeriodLabel, windowLabel, ctx.NegativeNoEvidenceFiltered)
	}
	if ctx.GeneratedRecommendationCount == 0 {
		if ctx.MarketCandidateStatus == "latest_auction_fallback" {
			return fmt.Sprintf("暂无推荐股票：%s %s 有新闻和热点；策略日没有集合竞价，已使用最新集合竞价字典，但新闻没有明确匹配到股票名称或代码。", ctx.PeriodLabel, windowLabel)
		}
		return fmt.Sprintf("暂无推荐股票：%s %s 有新闻和热点，也有 %d 条集合竞价候选，但新闻没有明确匹配到股票名称或代码。", ctx.PeriodLabel, windowLabel, ctx.MarketCandidateCount)
	}
	if ctx.RecentFiltered > 0 {
		recentStatus := formatAStockRecentReplenishmentStatus(ctx.RecentFiltered, ctx.RecentReplenished, ctx.RecentReplenishShortfall)
		if recentStatus == "" {
			recentStatus = fmt.Sprintf("%s推荐过滤 %d 只", aStockRecentLookbackStatusPrefix(), ctx.RecentFiltered)
		}
		return fmt.Sprintf("暂无推荐股票：%s %s 已生成候选，但%s。可关闭%s过滤后重新生成。", ctx.PeriodLabel, windowLabel, recentStatus, aStockRecentLookbackLabel())
	}
	if ctx.SameDayMorningFiltered > 0 {
		return fmt.Sprintf("暂无推荐股票：%s %s 已生成候选，但过滤上午同股票、热点或日内名额 %d 只。", ctx.PeriodLabel, windowLabel, ctx.SameDayMorningFiltered)
	}
	if ctx.ExDividendFiltered > 0 {
		return fmt.Sprintf("暂无推荐股票：%s %s 已生成候选，但除权除息过滤 %d 只。", ctx.PeriodLabel, windowLabel, ctx.ExDividendFiltered)
	}
	if ctx.LimitUpFiltered > 0 {
		return fmt.Sprintf("暂无推荐股票：%s %s 已生成候选，但涨停过滤 %d 只。", ctx.PeriodLabel, windowLabel, ctx.LimitUpFiltered)
	}
	if ctx.FundFlowFiltered > 0 {
		return fmt.Sprintf("暂无推荐股票：%s %s 已生成候选，但资金过滤 %d 只。可关闭资金过滤后重新生成。", ctx.PeriodLabel, windowLabel, ctx.FundFlowFiltered)
	}
	if ctx.TodayMarketFilterEnabled && (strings.Contains(ctx.BacktestStatus, "无当日行情") || strings.Contains(ctx.BacktestStatus, "过滤无当日行情")) {
		return fmt.Sprintf("暂无推荐股票：%s %s 已生成 %d 只候选，但没有当日行情或开盘价。请点击“同步行情”后重试。", ctx.PeriodLabel, windowLabel, ctx.GeneratedRecommendationCount)
	}
	if strings.Contains(ctx.BacktestStatus, "回撤过滤") || strings.Contains(ctx.BacktestStatus, "过滤回撤") {
		return fmt.Sprintf("暂无推荐股票：%s %s 已生成 %d 只候选，但被30/60天回撤过滤。", ctx.PeriodLabel, windowLabel, ctx.GeneratedRecommendationCount)
	}
	return fmt.Sprintf("暂无推荐股票：%s %s 已生成 %d 只候选，但未通过行情、回撤或回测过滤。状态：%s", ctx.PeriodLabel, windowLabel, ctx.GeneratedRecommendationCount, ctx.BacktestStatus)
}

func normalizeAStockBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func normalizeAStockIgnoreRecent(query url.Values) bool {
	if normalizeAStockBool(query.Get("filter_recent")) {
		return false
	}
	if _, ok := query["ignore_recent"]; ok {
		return normalizeAStockBool(query.Get("ignore_recent"))
	}
	return false
}

func normalizeAStockIgnoreLimitUp(query url.Values) bool {
	if normalizeAStockBool(query.Get("filter_limit_up")) {
		return false
	}
	if _, ok := query["ignore_limit_up"]; ok {
		return normalizeAStockBool(query.Get("ignore_limit_up"))
	}
	return false
}

func normalizeAStockIgnoreFundFlow(query url.Values) bool {
	ignoreFundFlow, _ := parseAStockFundFlowFilterValues(query)
	return ignoreFundFlow
}

func normalizeAStockIgnoreFundFlowFromRequest(r *http.Request, strategyDate string) (bool, bool) {
	if r == nil {
		return aStockDefaultIgnoreFundFlow(strategyDate), false
	}
	if ignoreFundFlow, explicit := parseAStockFundFlowFilterValues(r.URL.Query()); explicit {
		return ignoreFundFlow, true
	}
	if aStockDefaultIgnoreFundFlow(strategyDate) {
		return true, false
	}
	return aStockFundFlowFilterFromCookie(r)
}

func normalizeAStockIgnoreFundFlowFromForm(r *http.Request, strategyDate string) (bool, bool) {
	if r == nil {
		return aStockDefaultIgnoreFundFlow(strategyDate), false
	}
	if ignoreFundFlow, explicit := parseAStockFundFlowFilterValues(r.Form); explicit {
		return ignoreFundFlow, true
	}
	if aStockDefaultIgnoreFundFlow(strategyDate) {
		return true, false
	}
	return aStockFundFlowFilterFromCookie(r)
}

func parseAStockFundFlowFilterValues(values url.Values) (bool, bool) {
	if normalizeAStockBool(values.Get("filter_fund_flow")) {
		return false, true
	}
	if _, ok := values["ignore_fund_flow"]; ok {
		return normalizeAStockBool(values.Get("ignore_fund_flow")), true
	}
	return false, false
}

func aStockFundFlowFilterFromCookie(r *http.Request) (bool, bool) {
	cookie, err := r.Cookie(aStockFundFlowFilterCookieName)
	if err != nil {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(cookie.Value)) {
	case aStockFundFlowFilterCookieDisabled:
		return true, false
	case aStockFundFlowFilterCookieEnabled:
		return false, false
	default:
		return false, false
	}
}

func aStockDefaultIgnoreFundFlow(strategyDate string) bool {
	date := normalizeAStockStrategyDate(strategyDate)
	return date >= "2026-06-01" && date <= "2026-06-30"
}

func aStockFundFlowRenderExplicit(strategyDate string, ignoreFundFlow bool, requestExplicit bool) bool {
	return requestExplicit || ignoreFundFlow != aStockDefaultIgnoreFundFlow(strategyDate)
}

func setAStockFundFlowFilterQuery(query url.Values, strategyDate string, ignoreFundFlow bool, explicit bool) {
	if query == nil || !explicit || ignoreFundFlow == aStockDefaultIgnoreFundFlow(strategyDate) {
		return
	}
	if ignoreFundFlow {
		query.Set("ignore_fund_flow", "1")
		return
	}
	query.Set("filter_fund_flow", "1")
}

func setAStockFundFlowFilterCookie(w http.ResponseWriter, ignoreFundFlow bool, explicit bool) {
	if !explicit || w == nil {
		return
	}
	value := aStockFundFlowFilterCookieEnabled
	if ignoreFundFlow {
		value = aStockFundFlowFilterCookieDisabled
	}
	http.SetCookie(w, &http.Cookie{
		Name:     aStockFundFlowFilterCookieName,
		Value:    value,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
	})
}

func normalizeAStockFilterTodayMarket(query url.Values) bool {
	if normalizeAStockBool(query.Get("filter_today_market")) {
		return true
	}
	if normalizeAStockBool(query.Get("require_today_market")) {
		return true
	}
	return false
}

func paginateAStockNews(items []model.Item, page int, pageSize int) ([]model.Item, int, int) {
	if pageSize <= 0 {
		pageSize = aStockNewsPageSize
	}
	if page < 1 {
		page = 1
	}
	total := len(items)
	if total == 0 {
		return nil, 1, 0
	}
	totalPages := (total + pageSize - 1) / pageSize
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}
	return items[start:end], page, totalPages
}

func (s *Server) loadRecentAStockRecommendationCodes(strategyDate string, lookbackDays int) map[string]struct{} {
	return s.loadRecentAStockRecommendationCodesWithCache(strategyDate, lookbackDays, nil)
}

func (s *Server) loadRecentAStockRecommendationCodesWithCache(strategyDate string, lookbackDays int, cache *aStockRequestCache) map[string]struct{} {
	if lookbackDays <= 0 {
		return nil
	}
	dateKey := normalizeAStockStrategyDate(strategyDate)
	cacheKey := dateKey + "|" + fmt.Sprint(lookbackDays) + "|trading"
	if cache != nil {
		if cached, ok := cache.recentCodes[cacheKey]; ok {
			return cached
		}
	}
	dates := s.recentAStockTradingDatesWithCache(dateKey, lookbackDays, cache)
	result := make(map[string]struct{})
	for _, date := range dates {
		for _, period := range aStockPeriods() {
			for code := range s.loadPersistedAStockRecommendationCodesFromAllSourcesWithCache(date, period.Key, false, cache) {
				result[code] = struct{}{}
			}
		}
	}
	if cache != nil {
		cache.recentCodes[cacheKey] = result
	}
	return result
}

func (s *Server) recentAStockTradingDatesWithCache(strategyDate string, lookbackDays int, cache *aStockRequestCache) []string {
	day, err := time.ParseInLocation("2006-01-02", normalizeAStockStrategyDate(strategyDate), aStockLocation())
	if err != nil || lookbackDays <= 0 {
		return nil
	}
	dates := make([]string, 0, lookbackDays)
	maxScanDays := max(lookbackDays*3, lookbackDays+30)
	for offset := 1; len(dates) < lookbackDays && offset <= maxScanDays; offset++ {
		date := day.AddDate(0, 0, -offset).Format("2006-01-02")
		if s.isAStockRecentLookbackTradingDayWithCache(date, cache) {
			dates = append(dates, date)
		}
	}
	return dates
}

func (s *Server) isAStockRecentLookbackTradingDayWithCache(date string, cache *aStockRequestCache) bool {
	date = normalizeAStockStrategyDate(date)
	if date == "" {
		return false
	}
	if strings.TrimSpace(s.cfg.SchedulerURL) == "" {
		return isLocalAStockTradingDay(date)
	}
	status, err := s.loadAStockTradingDayStatusWithCache(date, cache)
	if err != nil {
		return isLocalAStockTradingDay(date)
	}
	if strings.TrimSpace(status.Date) == "" {
		status.Date = date
	}
	return status.IsTradingDay
}

func (s *Server) applyAStockAfternoonSameDayCaps(ctx *aStockContext, candidates []aStockMarketCandidate) {
	s.applyAStockAfternoonSameDayCapsWithCache(ctx, candidates, nil)
}

func (s *Server) applyAStockAfternoonSameDayCapsWithCache(ctx *aStockContext, candidates []aStockMarketCandidate, cache *aStockRequestCache) {
	remaining := s.applyAStockAfternoonSameDayDuplicateCapsWithCache(ctx, candidates, cache)
	if ctx == nil || ctx.Period != "afternoon" || len(ctx.Recommendations) == 0 {
		return
	}
	filtered, dailyLimitFiltered := limitAStockRecommendationsByCount(ctx.Recommendations, remaining)
	ctx.Recommendations = filtered
	ctx.SameDayMorningFiltered += dailyLimitFiltered
}

func (s *Server) applyAStockAfternoonSameDayDuplicateCapsWithCache(ctx *aStockContext, candidates []aStockMarketCandidate, cache *aStockRequestCache) int {
	if ctx == nil || ctx.Period != "afternoon" || len(ctx.Recommendations) == 0 {
		return s.loadAStockAlgorithmSettingsWithCache(cache).Auction.RecommendationLimit
	}
	settings := s.loadAStockAlgorithmSettingsWithCache(cache)
	morningRecommendations := s.loadSameDayMorningAStockRecommendationsWithCache(ctx.Date, candidates, cache)
	filtered := ctx.Recommendations
	sameCodeFiltered := 0
	hotspotQuotaFiltered := 0
	if len(morningRecommendations) > 0 {
		filtered, sameCodeFiltered = filterAStockRecommendationsByCodes(filtered, aStockRecommendationCodeSet(morningRecommendations))
		filtered, hotspotQuotaFiltered = filterAStockRecommendationsByMorningHotspotQuota(filtered, aStockRecommendationHotspotCounts(morningRecommendations), settings.Auction.StocksPerHotspot)
	}
	ctx.Recommendations = filtered
	ctx.SameDayMorningFiltered += sameCodeFiltered + hotspotQuotaFiltered
	return remainingAStockDailyRecommendationLimitWithSettings(countAStockDailyLimitRecommendations(morningRecommendations), settings)
}

func (s *Server) applyAStockAfternoonDailyLimitOnlyWithCache(ctx *aStockContext, cache *aStockRequestCache) int {
	if ctx == nil || ctx.Period != "afternoon" || len(ctx.Recommendations) == 0 {
		return 0
	}
	morningRecommendations := s.loadSameDayMorningAStockRecommendationsWithCache(ctx.Date, nil, cache)
	filtered, dailyLimitFiltered := limitAStockRecommendationsByCount(ctx.Recommendations, remainingAStockDailyRecommendationLimitWithSettings(countAStockDailyLimitRecommendations(morningRecommendations), s.loadAStockAlgorithmSettingsWithCache(cache)))
	ctx.Recommendations = filtered
	ctx.SameDayMorningFiltered += dailyLimitFiltered
	return dailyLimitFiltered
}

func (s *Server) loadSameDayMorningAStockRecommendations(strategyDate string, candidates []aStockMarketCandidate) []aStockRecommendation {
	return s.loadSameDayMorningAStockRecommendationsWithCache(strategyDate, candidates, nil)
}

func (s *Server) loadSameDayMorningAStockRecommendationsWithCache(strategyDate string, candidates []aStockMarketCandidate, cache *aStockRequestCache) []aStockRecommendation {
	if persisted := s.loadPersistedAStockRecommendationsWithCache(strategyDate, "morning", false, cache); len(persisted) > 0 {
		return persisted
	}
	start, end := aStockWindow(strategyDate, "morning")
	items, err := s.loadAStockWindowArticlesWithCache(start, end, cache)
	if err != nil || len(items) == 0 {
		return nil
	}
	settings, _ := astocknews.LoadSettings("")
	items = astocknews.FilterRecommendationItems(items, settings)
	return buildAStockSnapshotRecommendations(strategyDate, "morning", items, candidates)
}

func (s *Server) loadPersistedAStockRecommendationCodes(strategyDate string, period string, ignoreRecent bool) map[string]struct{} {
	return s.loadPersistedAStockRecommendationCodesWithCache(strategyDate, period, ignoreRecent, nil)
}

func (s *Server) loadPersistedAStockRecommendationCodesWithCache(strategyDate string, period string, ignoreRecent bool, cache *aStockRequestCache) map[string]struct{} {
	return aStockRecommendationCodeSet(s.loadPersistedAStockRecommendationsWithCache(strategyDate, period, ignoreRecent, cache))
}

func (s *Server) loadPersistedAStockRecommendationCodesFromAllSourcesWithCache(strategyDate string, period string, ignoreRecent bool, cache *aStockRequestCache) map[string]struct{} {
	codes := make(map[string]struct{})
	addCode := func(code string) {
		code = normalizeAStockCode(code)
		if astockcode.IsShanghaiShenzhen(code) {
			codes[code] = struct{}{}
		}
	}
	if !ignoreRecent {
		if result, ok := s.loadAStockRecommendationSelectionsWithCache(strategyDate, period, cache); ok {
			for _, item := range result.Items {
				addCode(item.Code)
			}
		}
	}
	if snapshot, ok := s.loadAStockRecommendationSnapshotWithCache(strategyDate, period, ignoreRecent, cache); ok {
		var recommendations []aStockRecommendation
		if err := json.Unmarshal([]byte(normalizeAStockSnapshotJSONArray(snapshot.RecommendationsJSON)), &recommendations); err == nil {
			for _, rec := range recommendations {
				addCode(rec.Code)
			}
		}
	}
	return codes
}

func (s *Server) loadPersistedAStockRecommendations(strategyDate string, period string, ignoreRecent bool) []aStockRecommendation {
	return s.loadPersistedAStockRecommendationsWithCache(strategyDate, period, ignoreRecent, nil)
}

func (s *Server) loadPersistedAStockRecommendationsWithCache(strategyDate string, period string, ignoreRecent bool, cache *aStockRequestCache) []aStockRecommendation {
	if !ignoreRecent {
		if result, ok := s.loadAStockRecommendationSelectionsWithCache(strategyDate, period, cache); ok && len(result.Items) > 0 {
			recommendations, _ := s.repairAStockPersistedRecommendationsForPeriodWithCache(strategyDate, period, aStockRecommendationSelectionsToRecommendations(result.Items, period), cache)
			if snapshot, ok := s.loadAStockRecommendationSnapshotWithCache(strategyDate, period, ignoreRecent, cache); ok {
				var snapshotRecommendations []aStockRecommendation
				if err := json.Unmarshal([]byte(normalizeAStockSnapshotJSONArray(snapshot.RecommendationsJSON)), &snapshotRecommendations); err == nil {
					snapshotRecommendations, _ = s.repairAStockPersistedRecommendationsForPeriodWithCache(strategyDate, period, snapshotRecommendations, cache)
					recommendations = appendAStockRecoveredSnapshotRecommendations(recommendations, snapshotRecommendations)
				}
			}
			return recommendations
		}
	}
	snapshot, ok := s.loadAStockRecommendationSnapshotWithCache(strategyDate, period, ignoreRecent, cache)
	if !ok {
		return nil
	}
	var recommendations []aStockRecommendation
	if err := json.Unmarshal([]byte(normalizeAStockSnapshotJSONArray(snapshot.RecommendationsJSON)), &recommendations); err != nil {
		return nil
	}
	recommendations, _ = s.repairAStockPersistedRecommendationsForPeriodWithCache(strategyDate, period, recommendations, cache)
	return recommendations
}

func appendAStockRecoveredSnapshotRecommendations(base []aStockRecommendation, snapshotRecommendations []aStockRecommendation) []aStockRecommendation {
	if len(snapshotRecommendations) == 0 {
		return rerankAStockRecommendations(base)
	}
	seen := aStockRecommendationCodeSet(base)
	merged := append([]aStockRecommendation(nil), base...)
	for _, rec := range snapshotRecommendations {
		if !isAStockRecoveredRecommendation(rec) {
			continue
		}
		code := normalizeAStockCode(rec.Code)
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		rec.Code = code
		merged = append(merged, rec)
		seen[code] = struct{}{}
	}
	return rerankAStockRecommendations(merged)
}

func (s *Server) applyAStockHoldingSummaries(recommendations []aStockRecommendation) []aStockRecommendation {
	return s.applyAStockHoldingSummariesWithCache(recommendations, nil)
}

func (s *Server) applyAStockHoldingSummariesWithCache(recommendations []aStockRecommendation, cache *aStockRequestCache) []aStockRecommendation {
	settings := s.loadAStockAlgorithmSettingsWithCache(cache)
	return s.applyAStockHoldingSummariesWithSettings(recommendations, cache, settings)
}

func (s *Server) applyAStockHoldingSummariesWithSettings(recommendations []aStockRecommendation, cache *aStockRequestCache, settings model.AStockRecommendationAlgorithmSettings) []aStockRecommendation {
	if len(recommendations) == 0 {
		return recommendations
	}
	settings = model.NormalizeAStockRecommendationAlgorithmSettings(settings)
	for i := range recommendations {
		recommendations[i].HoldingSummary = "--"
		recommendations[i].HoldingRatio = "--"
		code := normalizeAStockCode(recommendations[i].Code)
		if code == "" {
			continue
		}
		summary, err := s.loadAStockHoldingSummaryWithCache(code, cache)
		if err != nil {
			continue
		}
		if summary.HolderCount <= 0 {
			continue
		}
		bonus := aStockHoldingScore(summary)
		applyAStockScoreDeltaWithSettings(
			&recommendations[i],
			settings,
			aStockScoreFactorFund,
			"机构持仓",
			fmt.Sprintf("机构共持 %d 家，类型 %d 类", summary.HolderCount, summary.HolderTypeCount),
			bonus,
		)
		recommendations[i].HoldingSummary = fmt.Sprintf("%d家/%d类", summary.HolderCount, summary.HolderTypeCount)
		recommendations[i].HoldingRatio = formatAStockHoldingPct(summary.TotalFloatRatio)
		recommendations[i].Reason = fmt.Sprintf(
			"%s，机构共持 %d 家，类型 %d 类，合计流通占比 %.2f%%，持仓加分 %d",
			recommendations[i].Reason,
			summary.HolderCount,
			summary.HolderTypeCount,
			summary.TotalFloatRatio,
			bonus,
		)
	}
	return recommendations
}

func (s *Server) loadAStockHoldingSummaryWithCache(code string, cache *aStockRequestCache) (model.StockInstitutionHoldingSummary, error) {
	code = normalizeAStockCode(code)
	if code == "" {
		return model.StockInstitutionHoldingSummary{}, fmt.Errorf("empty stock code")
	}
	if cache != nil {
		if entry, ok := cache.holdingSummaries[code]; ok {
			return entry.summary, entry.err
		}
	}
	summary := model.StockInstitutionHoldingSummary{}
	query := "/api/v1/a-stock/holdings/summary?code=" + url.QueryEscape(code)
	err := s.getJSON(s.cfg.ContentURL+query, &summary)
	if cache != nil {
		cache.holdingSummaries[code] = aStockHoldingSummaryCacheEntry{summary: summary, err: err}
	}
	return summary, err
}

func (s *Server) applyAStockRecommendationFundFlow5DToContext(ctx *aStockContext, cache *aStockRequestCache) {
	if ctx == nil || len(ctx.Recommendations) == 0 {
		return
	}
	settings := aStockAlgorithmSettingsForRecommendationPeriod(ctx.Period, s.loadAStockAlgorithmSettingsWithCache(cache))
	ctx.Recommendations = s.applyAStockRecommendationFundFlow5DWithSettings(ctx.Date, ctx.Recommendations, cache, settings)
}

func (s *Server) applyAStockRecommendationFundFlow5DWithCache(strategyDate string, recommendations []aStockRecommendation, cache *aStockRequestCache) []aStockRecommendation {
	settings := s.loadAStockAlgorithmSettingsWithCache(cache)
	return s.applyAStockRecommendationFundFlow5DWithSettings(strategyDate, recommendations, cache, settings)
}

func (s *Server) applyAStockRecommendationFundFlow5DWithSettings(strategyDate string, recommendations []aStockRecommendation, cache *aStockRequestCache, settings model.AStockRecommendationAlgorithmSettings) []aStockRecommendation {
	if len(recommendations) == 0 {
		return recommendations
	}
	settings = model.NormalizeAStockRecommendationAlgorithmSettings(settings)
	for i := range recommendations {
		code := normalizeAStockCode(recommendations[i].Code)
		if code == "" {
			recommendations[i] = applyAStockFundFlow5DAssessmentToRecommendationWithSettings(recommendations[i], aStockFundFlow5DAssessment{Missing: true}, false, settings)
			continue
		}
		assessment := s.assessAStockRecommendationFundFlow5DWithSettings(strategyDate, code, cache, settings)
		recommendations[i] = applyAStockFundFlow5DAssessmentToRecommendationWithSettings(recommendations[i], assessment, false, settings)
	}
	return recommendations
}

type aStockFundFlow5DAssessment struct {
	Total            float64
	Total5D          float64
	Total10D         float64
	Recent2DTotal    float64
	NegativeDays     int
	NegativeDays5D   int
	LatestNetInflow  float64
	LatestChangePct  float64
	Recent2DInflow   bool
	Recent2DOutflow  bool
	TenDayRepair     bool
	SectorWeak       bool
	SectorNetOutflow bool
	ScoreDelta       int
	ScoreReasons     []string
	HardFiltered     bool
	HardFilterReason string
	Missing          bool
}

type aStockSectorFundFlowTrendAssessment struct {
	SectorType      string
	SectorName      string
	Total5D         float64
	Total10D        float64
	Recent2DTotal   float64
	LatestNetInflow float64
	LatestChangePct float64
	LatestRank      int
	InflowDays5D    int
	OutflowDays5D   int
	SignChanges5D   int
	ScoreDelta      int
	Status          string
	ScoreReasons    []string
	Missing         bool
}

type aStockSectorTopStockResonance struct {
	Code                     string
	Name                     string
	SectorCount              int
	PositiveSectorCount      int
	SectorNames              []string
	TotalSectorMainNetInflow float64
	SourceTypes              []string
	ScoreDelta               int
}

type aStockSectorTopStockCodeResolver struct {
	codeByName   map[string]string
	allowedCodes map[string]struct{}
}

type aStockFundFlowRecommendationFilterResult struct {
	Recommendations []aStockRecommendation
	Filtered        int
	FilteredStocks  []aStockRecommendation
	Missing         int
	Replenished     int
	Shortfall       bool
}

func (s *Server) assessAStockRecommendationFundFlow5DWithCache(strategyDate string, code string, cache *aStockRequestCache) aStockFundFlow5DAssessment {
	settings := s.loadAStockAlgorithmSettingsWithCache(cache)
	return s.assessAStockRecommendationFundFlow5DWithSettings(strategyDate, code, cache, settings)
}

func (s *Server) assessAStockRecommendationFundFlow5DWithSettings(strategyDate string, code string, cache *aStockRequestCache, settings model.AStockRecommendationAlgorithmSettings) aStockFundFlow5DAssessment {
	settings = model.NormalizeAStockRecommendationAlgorithmSettings(settings)
	code = normalizeAStockCode(code)
	if code == "" {
		return aStockFundFlow5DAssessment{Missing: true}
	}
	result, err := s.loadAStockStockFundFlowTrendWithCache(strategyDate, code, 10, cache)
	if err != nil || len(result.Items) == 0 {
		return aStockFundFlow5DAssessment{Missing: true}
	}
	items := append([]model.AStockStockFundFlow(nil), result.Items...)
	sort.SliceStable(items, func(i, j int) bool {
		return normalizeAStockStrategyDate(items[i].TradeDate) > normalizeAStockStrategyDate(items[j].TradeDate)
	})
	assessment := aStockFundFlow5DAssessment{}
	limit10 := min(len(items), 10)
	limit5 := min(len(items), 5)
	for i := 0; i < limit10; i++ {
		assessment.Total10D += items[i].MainNetInflow
	}
	for i := 0; i < limit5; i++ {
		assessment.Total5D += items[i].MainNetInflow
		if items[i].MainNetInflow < 0 {
			assessment.NegativeDays5D++
		}
	}
	if len(items) > 0 {
		assessment.LatestNetInflow = items[0].MainNetInflow
		assessment.LatestChangePct = items[0].ChangePct
	}
	if len(items) >= 2 {
		assessment.Recent2DTotal = items[0].MainNetInflow + items[1].MainNetInflow
		assessment.Recent2DInflow = items[0].MainNetInflow > 0 && items[1].MainNetInflow > 0
		assessment.Recent2DOutflow = items[0].MainNetInflow < 0 && items[1].MainNetInflow < 0
	}
	assessment.Total = assessment.Total5D
	assessment.NegativeDays = assessment.NegativeDays5D
	assessment = scoreAStockFundFlowAssessmentWithSettings(assessment, settings)
	return assessment
}

func (s *Server) applyAStockRecommendationFundFlowFilterWithCache(strategyDate string, base []aStockRecommendation, replacementPool []aStockRecommendation, recentCodes map[string]struct{}, target int, cache *aStockRequestCache, allowHardFilteredFallback bool) aStockFundFlowRecommendationFilterResult {
	settings := s.loadAStockAlgorithmSettingsWithCache(cache)
	return s.applyAStockRecommendationFundFlowFilterWithSettings(strategyDate, base, replacementPool, recentCodes, target, cache, allowHardFilteredFallback, settings)
}

func (s *Server) applyAStockRecommendationFundFlowFilterWithSettings(strategyDate string, base []aStockRecommendation, replacementPool []aStockRecommendation, recentCodes map[string]struct{}, target int, cache *aStockRequestCache, allowHardFilteredFallback bool, settings model.AStockRecommendationAlgorithmSettings) aStockFundFlowRecommendationFilterResult {
	if target <= 0 {
		target = len(base)
	}
	settings = model.NormalizeAStockRecommendationAlgorithmSettings(settings)
	result := aStockFundFlowRecommendationFilterResult{}
	if len(base) == 0 || target <= 0 {
		result.Recommendations = rerankAStockRecommendations(base)
		return result
	}
	resonancePool := append([]aStockRecommendation(nil), base...)
	resonancePool = append(resonancePool, replacementPool...)
	sectorTopStockResonance := s.loadAStockSectorTopStockResonanceWithSettings(strategyDate, resonancePool, cache, settings)
	seen := make(map[string]struct{}, len(base))
	assessments := make(map[string]aStockFundFlow5DAssessment)
	baseHotspotCounts := make(map[string]int)
	keptHotspotCounts := make(map[string]int)
	kept := make([]aStockRecommendation, 0, minInt(len(base), target))
	type hardFilteredFallback struct {
		rec           aStockRecommendation
		countFiltered bool
		replenished   bool
		used          bool
	}
	var hardFilteredFallbacks []hardFilteredFallback
	isRecent := func(code string) bool {
		if len(recentCodes) == 0 {
			return false
		}
		_, ok := recentCodes[normalizeAStockCode(code)]
		return ok
	}
	appendRecommendation := func(rec aStockRecommendation, countFiltered bool, replenished bool, allowHardFiltered bool) bool {
		if len(kept) >= target {
			return false
		}
		code := normalizeAStockCode(rec.Code)
		if code == "" {
			return false
		}
		rec.Code = code
		if _, exists := seen[code]; exists {
			return false
		}
		if isBlockedAStockRecommendationStock(rec.Code, rec.Name) || isRecent(code) {
			return false
		}
		assessment := s.assessAStockRecommendationFundFlow5DWithSettings(strategyDate, code, cache, settings)
		if !assessment.Missing && assessment.ScoreDelta > 0 {
			assessment = s.applyAStockHotspotSectorFundFlowCapWithSettings(strategyDate, rec.Hotspot, assessment, cache, settings)
		}
		if !assessment.Missing {
			assessments[code] = assessment
		}
		if !assessment.Missing && isAStockFundFlowHardFiltered(assessment) && !allowHardFiltered {
			if allowHardFilteredFallback {
				rec = applyAStockFundFlow5DAssessmentToRecommendationWithSettings(rec, assessment, true, settings)
				hardFilteredFallbacks = append(hardFilteredFallbacks, hardFilteredFallback{rec: rec, countFiltered: countFiltered, replenished: replenished})
				return false
			}
			if countFiltered {
				result.Filtered++
				result.FilteredStocks = append(result.FilteredStocks, rec)
			}
			return false
		}
		rec = applyAStockFundFlow5DAssessmentToRecommendationWithSettings(rec, assessment, true, settings)
		rec = s.applyAStockSectorFundFlowTrendScoreWithSettings(strategyDate, rec, cache, settings)
		rec = applyAStockSectorTopStockResonanceToRecommendationWithSettings(rec, sectorTopStockResonance[code], settings)
		seen[code] = struct{}{}
		kept = append(kept, rec)
		if assessment.Missing {
			result.Missing++
		}
		if replenished {
			result.Replenished++
		}
		if hotspot := normalizeAStockRecommendationHotspot(rec.Hotspot); hotspot != "" {
			keptHotspotCounts[hotspot]++
		}
		return true
	}
	for _, rec := range base {
		if hotspot := normalizeAStockRecommendationHotspot(rec.Hotspot); hotspot != "" {
			baseHotspotCounts[hotspot]++
		}
		appendRecommendation(rec, true, false, false)
	}
	if len(kept) < target && len(replacementPool) > 0 {
		hotspotDeficits := make(map[string]int, len(baseHotspotCounts))
		for hotspot, baseCount := range baseHotspotCounts {
			if deficit := baseCount - keptHotspotCounts[hotspot]; deficit > 0 {
				hotspotDeficits[hotspot] = deficit
			}
		}
		candidates := sortedAStockReplacementRecommendations(replacementPool)
		for _, rec := range candidates {
			hotspot := normalizeAStockRecommendationHotspot(rec.Hotspot)
			if hotspot == "" || hotspotDeficits[hotspot] <= 0 {
				continue
			}
			if appendRecommendation(rec, false, true, false) {
				hotspotDeficits[hotspot]--
			}
			if len(kept) >= target {
				break
			}
		}
		if len(kept) < target {
			for _, rec := range candidates {
				if appendRecommendation(rec, false, true, false) && len(kept) >= target {
					break
				}
			}
		}
	}
	if len(kept) < target && allowHardFilteredFallback && len(hardFilteredFallbacks) > 0 {
		for i := range hardFilteredFallbacks {
			if appendRecommendation(hardFilteredFallbacks[i].rec, false, hardFilteredFallbacks[i].replenished, true) {
				hardFilteredFallbacks[i].used = true
			}
			if len(kept) >= target {
				break
			}
		}
		for _, fallback := range hardFilteredFallbacks {
			if fallback.countFiltered && !fallback.used {
				result.Filtered++
				result.FilteredStocks = append(result.FilteredStocks, fallback.rec)
			}
		}
	}
	result.Shortfall = len(kept) < target
	kept = applyAStockFundFlowMedianPenaltyToRecommendationsWithSettings(kept, assessments, settings)
	result.Recommendations = sortAStockRecommendationsByScore(kept)
	return result
}

func applyAStockFundFlowMedianPenaltyToRecommendations(recommendations []aStockRecommendation, assessments map[string]aStockFundFlow5DAssessment) []aStockRecommendation {
	return applyAStockFundFlowMedianPenaltyToRecommendationsWithSettings(recommendations, assessments, defaultAStockAlgorithmSettings())
}

func applyAStockFundFlowMedianPenaltyToRecommendationsWithSettings(recommendations []aStockRecommendation, assessments map[string]aStockFundFlow5DAssessment, settings model.AStockRecommendationAlgorithmSettings) []aStockRecommendation {
	if len(recommendations) == 0 || len(assessments) == 0 {
		return recommendations
	}
	totalsByHotspot := make(map[string][]float64)
	for _, rec := range recommendations {
		hotspot := normalizeAStockRecommendationHotspot(rec.Hotspot)
		if hotspot == "" {
			continue
		}
		assessment, ok := assessments[normalizeAStockCode(rec.Code)]
		if !ok || assessment.Missing {
			continue
		}
		totalsByHotspot[hotspot] = append(totalsByHotspot[hotspot], aStockAssessmentFundFlow5DTotal(assessment))
	}
	medianByHotspot := make(map[string]float64, len(totalsByHotspot))
	for hotspot, totals := range totalsByHotspot {
		if len(totals) < 3 {
			continue
		}
		sort.Float64s(totals)
		mid := len(totals) / 2
		if len(totals)%2 == 0 {
			medianByHotspot[hotspot] = (totals[mid-1] + totals[mid]) / 2
		} else {
			medianByHotspot[hotspot] = totals[mid]
		}
	}
	if len(medianByHotspot) == 0 {
		return recommendations
	}
	result := append([]aStockRecommendation(nil), recommendations...)
	for i := range result {
		hotspot := normalizeAStockRecommendationHotspot(result[i].Hotspot)
		median, ok := medianByHotspot[hotspot]
		if !ok {
			continue
		}
		assessment, ok := assessments[normalizeAStockCode(result[i].Code)]
		if !ok || assessment.Missing || aStockAssessmentFundFlow5DTotal(assessment) >= median {
			continue
		}
		reason := fmt.Sprintf("5日资金低于同热点中位数 %s，资金强度减分 %d", formatSectorFundFlowMoney(median), settings.Fund.MedianPenalty)
		applyAStockScoreDeltaWithSettings(&result[i], settings, aStockScoreFactorFund, "资金强度", reason, -settings.Fund.MedianPenalty)
		result[i].Reason = appendAStockReason(result[i].Reason, reason)
	}
	return result
}

func aStockAssessmentFundFlow5DTotal(assessment aStockFundFlow5DAssessment) float64 {
	if assessment.Total5D != 0 {
		return assessment.Total5D
	}
	return assessment.Total
}

func applyAStockFundFlow5DAssessmentToRecommendation(rec aStockRecommendation, assessment aStockFundFlow5DAssessment, applyScore bool) aStockRecommendation {
	return applyAStockFundFlow5DAssessmentToRecommendationWithSettings(rec, assessment, applyScore, defaultAStockAlgorithmSettings())
}

func applyAStockFundFlow5DAssessmentToRecommendationWithSettings(rec aStockRecommendation, assessment aStockFundFlow5DAssessment, applyScore bool, settings model.AStockRecommendationAlgorithmSettings) aStockRecommendation {
	if assessment.Missing {
		rec.FundFlow5D = "--"
		rec.FundFlow5DClass = "astock-flat"
		return rec
	}
	total5 := aStockAssessmentFundFlow5DTotal(assessment)
	rec.FundFlow5D = formatSectorFundFlowMoney(total5)
	rec.FundFlow5DClass = aStockPctClass(total5)
	if !applyScore {
		return rec
	}
	scoreDelta := assessment.ScoreDelta
	if scoreDelta == 0 {
		return rec
	}
	reason := formatAStockFundFlowScoreReason(assessment)
	applyAStockScoreDeltaWithSettings(&rec, settings, aStockScoreFactorFund, "资金动向", reason, scoreDelta)
	rec.Reason = appendAStockReason(rec.Reason, reason)
	return rec
}

func aStockFundFlowScoreAdjustment(total float64) int {
	return scoreAStockFundFlowAssessment(aStockFundFlow5DAssessment{Total: total, Total5D: total}).ScoreDelta
}

func isAStockFundFlowHardFiltered(assessment aStockFundFlow5DAssessment) bool {
	return assessment.HardFiltered
}

func scoreAStockFundFlowAssessment(assessment aStockFundFlow5DAssessment) aStockFundFlow5DAssessment {
	return scoreAStockFundFlowAssessmentWithSettings(assessment, defaultAStockAlgorithmSettings())
}

func scoreAStockFundFlowAssessmentWithSettings(assessment aStockFundFlow5DAssessment, settings model.AStockRecommendationAlgorithmSettings) aStockFundFlow5DAssessment {
	if assessment.Missing {
		return assessment
	}
	total5 := assessment.Total5D
	if total5 == 0 && assessment.Total != 0 {
		total5 = assessment.Total
		assessment.Total5D = total5
	}
	assessment.Total = total5
	score := 0
	reasons := make([]string, 0, 8)
	switch {
	case total5 >= settings.Fund.ExtremeThreshold:
		score += settings.Fund.ExtremeScore
	case total5 >= settings.Fund.VeryStrongThreshold:
		score += settings.Fund.VeryStrongScore
	case total5 >= settings.Fund.StrongBonusThreshold:
		score += settings.Fund.StrongBonusScore
	case total5 >= settings.Fund.BonusThreshold:
		score += settings.Fund.BonusScore
	}
	switch {
	case assessment.NegativeDays5D >= 4:
		score -= settings.Fund.NegativeDays4Penalty
		reasons = append(reasons, fmt.Sprintf("近5日净流出%d天，资金连续性减分 %d", assessment.NegativeDays5D, settings.Fund.NegativeDays4Penalty))
	case assessment.NegativeDays5D == 3:
		score -= settings.Fund.NegativeDays3Penalty
		reasons = append(reasons, fmt.Sprintf("近5日净流出3天，资金连续性减分 %d", settings.Fund.NegativeDays3Penalty))
	case assessment.NegativeDays5D == 2:
		score -= settings.Fund.NegativeDays2Penalty
		reasons = append(reasons, fmt.Sprintf("近5日净流出2天，资金连续性减分 %d", settings.Fund.NegativeDays2Penalty))
	}
	if assessment.Total10D > 0 && total5 > 0 && total5/assessment.Total10D >= settings.Fund.TenDayAccelerationRatio {
		score += settings.Fund.TenDayAccelerationScore
		reasons = append(reasons, "10日资金加速流入")
	}
	if assessment.Total10D > 0 && total5 < 0 {
		score -= settings.Fund.TenDayRetreatPenalty
		reasons = append(reasons, fmt.Sprintf("10日净流入但5日转净流出，资金退潮减分 %d", settings.Fund.TenDayRetreatPenalty))
	}
	if assessment.Total10D < 0 && total5 < 0 {
		assessment.HardFiltered = true
		assessment.HardFilterReason = "10日净流出且5日仍净流出"
	}
	if assessment.Total10D < 0 && total5 > 0 {
		assessment.TenDayRepair = true
		reasons = append(reasons, fmt.Sprintf("10日净流出但5日转净流入，资金修复加分上限%d", settings.Fund.TenDayRepairCap))
	}
	if assessment.Recent2DInflow && assessment.Recent2DTotal > settings.Fund.RecentTurnThreshold {
		score += settings.Fund.Recent2DInflowScore
		reasons = append(reasons, fmt.Sprintf("最近2日连续净流入，资金拐点加分 %d", settings.Fund.Recent2DInflowScore))
	}
	if assessment.Recent2DOutflow {
		score -= settings.Fund.Recent2DOutflowPenalty
		reasons = append(reasons, fmt.Sprintf("最近2日连续净流出，资金拐点减分 %d", settings.Fund.Recent2DOutflowPenalty))
	}
	if assessment.LatestNetInflow < 0 && assessment.LatestChangePct < 0 {
		score -= settings.Fund.LatestOutflowDownPenalty
		reasons = append(reasons, fmt.Sprintf("当日净流出且股价下跌，资金风险减分 %d", settings.Fund.LatestOutflowDownPenalty))
	}
	score = clampAStockFundFlowScoreWithSettings(score, settings)
	if assessment.TenDayRepair && score > settings.Fund.TenDayRepairCap {
		score = settings.Fund.TenDayRepairCap
	}
	if assessment.SectorNetOutflow && score > settings.Fund.SectorNetOutflowCap {
		score = settings.Fund.SectorNetOutflowCap
		reasons = append(reasons, fmt.Sprintf("板块当日资金净流出，个股资金加分上限%d", settings.Fund.SectorNetOutflowCap))
	} else if assessment.SectorWeak && score > settings.Fund.SectorWeakCap {
		score = settings.Fund.SectorWeakCap
		reasons = append(reasons, fmt.Sprintf("板块当天走弱，个股资金加分上限%d", settings.Fund.SectorWeakCap))
	}
	assessment.ScoreDelta = clampAStockFundFlowScoreWithSettings(score, settings)
	assessment.ScoreReasons = reasons
	return assessment
}

func clampAStockFundFlowScore(score int) int {
	return clampAStockFundFlowScoreWithSettings(score, defaultAStockAlgorithmSettings())
}

func clampAStockFundFlowScoreWithSettings(score int, settings model.AStockRecommendationAlgorithmSettings) int {
	if score < settings.Fund.ScoreMin {
		return settings.Fund.ScoreMin
	}
	if score > settings.Fund.ScoreMax {
		return settings.Fund.ScoreMax
	}
	return score
}

func formatAStockFundFlowScoreReason(assessment aStockFundFlow5DAssessment) string {
	total := assessment.Total5D
	if total == 0 && assessment.Total != 0 {
		total = assessment.Total
	}
	scoreDelta := assessment.ScoreDelta
	direction := "净流入"
	if total < 0 {
		direction = "净流出"
	}
	action := "加分"
	points := scoreDelta
	if scoreDelta < 0 {
		action = "减分"
		points = -scoreDelta
	}
	parts := []string{fmt.Sprintf("5日主力资金%s %s", direction, formatSectorFundFlowMoney(total))}
	if assessment.Total10D != 0 {
		parts = append(parts, fmt.Sprintf("10日主力资金%s", formatSectorFundFlowMoney(assessment.Total10D)))
	}
	parts = append(parts, assessment.ScoreReasons...)
	parts = append(parts, fmt.Sprintf("资金%s %d", action, points))
	return strings.Join(parts, "，")
}

func formatAStockFilterCountWithStocks(label string, count int, unit string, stocks []aStockRecommendation) string {
	if count <= 0 {
		return ""
	}
	unit = strings.TrimSpace(unit)
	status := fmt.Sprintf("%s %d", strings.TrimSpace(label), count)
	if unit != "" {
		status += " " + unit
	}
	if details := formatAStockFilteredStockList(stocks); details != "" {
		status += details
	}
	return status
}

func formatAStockFilteredStockList(stocks []aStockRecommendation) string {
	if len(stocks) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(stocks))
	labels := make([]string, 0, len(stocks))
	for _, stock := range stocks {
		code := normalizeAStockCode(stock.Code)
		name := strings.TrimSpace(stock.Name)
		if code == "" && name == "" {
			continue
		}
		key := code + "\x00" + name
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		switch {
		case code == "":
			labels = append(labels, name)
		case name == "" || name == code:
			labels = append(labels, code)
		default:
			labels = append(labels, code+" "+name)
		}
	}
	if len(labels) == 0 {
		return ""
	}
	return "（" + strings.Join(labels, "、") + "）"
}

func formatAStockFundFlowFilterStatus(filtered int, replenished int, missing int, shortfall bool) string {
	return formatAStockFundFlowFilterStatusWithStocks(filtered, nil, replenished, missing, shortfall)
}

func formatAStockFundFlowFilterStatusWithStocks(filtered int, filteredStocks []aStockRecommendation, replenished int, missing int, shortfall bool) string {
	if filtered <= 0 && missing <= 0 {
		return ""
	}
	parts := make([]string, 0, 4)
	if filtered > 0 {
		parts = append(parts, formatAStockFilterCountWithStocks("资金过滤", filtered, "只", filteredStocks))
	}
	if replenished > 0 {
		parts = append(parts, fmt.Sprintf("递补 %d 只", replenished))
	}
	if missing > 0 {
		parts = append(parts, fmt.Sprintf("资金数据缺失 %d 只", missing))
	}
	if shortfall {
		parts = append(parts, "候选不足未补满")
	}
	return strings.Join(parts, "，")
}

func (s *Server) loadAStockStockFundFlowTrendWithCache(endDate string, code string, days int, cache *aStockRequestCache) (model.AStockStockFundFlowTrendResult, error) {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return model.AStockStockFundFlowTrendResult{}, fmt.Errorf("content service url is empty")
	}
	endDate = normalizeAStockStrategyDate(endDate)
	code = normalizeAStockCode(code)
	if code == "" {
		return model.AStockStockFundFlowTrendResult{}, fmt.Errorf("empty stock code")
	}
	if days <= 0 {
		days = 5
	}
	cacheKey := strings.Join([]string{endDate, code, fmt.Sprint(days)}, "|")
	if cache != nil {
		if entry, ok := cache.stockFundFlowTrends[cacheKey]; ok {
			return entry.result, entry.err
		}
	}
	query := url.Values{}
	query.Set("end_date", endDate)
	query.Set("code", code)
	query.Set("indicator", "今日")
	query.Set("days", fmt.Sprint(days))
	result := model.AStockStockFundFlowTrendResult{}
	err := s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/stock-fund-flow-trend?"+query.Encode(), &result)
	if cache != nil {
		cache.stockFundFlowTrends[cacheKey] = aStockStockFundFlowTrendCacheEntry{result: result, err: err}
	}
	return result, err
}

func (s *Server) loadAStockStockFundFlowListWithCache(strategyDate string, indicator string, pageSize int, cache *aStockRequestCache) (model.AStockStockFundFlowListResult, error) {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return model.AStockStockFundFlowListResult{}, fmt.Errorf("content service url is empty")
	}
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	indicator = strings.TrimSpace(indicator)
	if indicator == "" {
		indicator = "5日"
	}
	if pageSize <= 0 {
		pageSize = 300
	}
	cacheKey := strings.Join([]string{strategyDate, indicator, fmt.Sprint(pageSize)}, "|")
	if cache != nil {
		if cache.stockFundFlows == nil {
			cache.stockFundFlows = make(map[string]aStockStockFundFlowListCacheEntry)
		}
		if entry, ok := cache.stockFundFlows[cacheKey]; ok {
			return entry.result, entry.err
		}
	}
	query := url.Values{}
	query.Set("date", strategyDate)
	query.Set("indicator", indicator)
	query.Set("page", "1")
	query.Set("page_size", fmt.Sprint(pageSize))
	result := model.AStockStockFundFlowListResult{}
	err := s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/stock-fund-flows?"+query.Encode(), &result)
	if cache != nil {
		cache.stockFundFlows[cacheKey] = aStockStockFundFlowListCacheEntry{result: result, err: err}
	}
	return result, err
}

func (s *Server) applyAStockHotspotSectorFundFlowCap(strategyDate string, hotspot string, assessment aStockFundFlow5DAssessment, cache *aStockRequestCache) aStockFundFlow5DAssessment {
	settings := s.loadAStockAlgorithmSettingsWithCache(cache)
	return s.applyAStockHotspotSectorFundFlowCapWithSettings(strategyDate, hotspot, assessment, cache, settings)
}

func (s *Server) applyAStockHotspotSectorFundFlowCapWithSettings(strategyDate string, hotspot string, assessment aStockFundFlow5DAssessment, cache *aStockRequestCache, settings model.AStockRecommendationAlgorithmSettings) aStockFundFlow5DAssessment {
	if assessment.Missing || strings.TrimSpace(hotspot) == "" {
		return assessment
	}
	settings = model.NormalizeAStockRecommendationAlgorithmSettings(settings)
	status := s.assessAStockHotspotSectorFundFlowWithCache(strategyDate, hotspot, cache)
	if !status.found {
		return assessment
	}
	assessment.SectorWeak = status.weak
	assessment.SectorNetOutflow = status.netOutflow
	return scoreAStockFundFlowAssessmentWithSettings(assessment, settings)
}

type aStockHotspotSectorFundFlowStatus struct {
	found      bool
	weak       bool
	netOutflow bool
}

func (s *Server) assessAStockHotspotSectorFundFlowWithCache(strategyDate string, hotspot string, cache *aStockRequestCache) aStockHotspotSectorFundFlowStatus {
	aliases := aStockHotspotSectorAliases(hotspot)
	if len(aliases) == 0 || strings.TrimSpace(s.cfg.ContentURL) == "" {
		return aStockHotspotSectorFundFlowStatus{}
	}
	status := aStockHotspotSectorFundFlowStatus{}
	for _, alias := range aliases {
		result, err := s.loadAStockSectorFundFlowsWithCache(strategyDate, alias, cache)
		if err != nil || len(result.Items) == 0 {
			continue
		}
		for _, item := range result.Items {
			if strings.TrimSpace(item.Name) != strings.TrimSpace(alias.SectorName) {
				continue
			}
			status.found = true
			if item.ChangePct < 0 {
				status.weak = true
			}
			if item.MainNetInflow < 0 {
				status.netOutflow = true
			}
			break
		}
	}
	return status
}

func (s *Server) loadAStockSectorFundFlowsWithCache(strategyDate string, alias aStockHotspotSectorAlias, cache *aStockRequestCache) (model.AStockSectorFundFlowListResult, error) {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return model.AStockSectorFundFlowListResult{}, fmt.Errorf("content service url is empty")
	}
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	sectorType := normalizeSectorFundFlowSectorType(alias.SectorType)
	sectorName := strings.TrimSpace(alias.SectorName)
	if sectorName == "" {
		return model.AStockSectorFundFlowListResult{}, fmt.Errorf("empty sector name")
	}
	cacheKey := strings.Join([]string{strategyDate, sectorType, sectorName}, "|")
	if cache != nil {
		if cache.sectorFundFlows == nil {
			cache.sectorFundFlows = make(map[string]aStockSectorFundFlowCacheEntry)
		}
		if entry, ok := cache.sectorFundFlows[cacheKey]; ok {
			return entry.result, entry.err
		}
	}
	query := url.Values{}
	query.Set("date", strategyDate)
	query.Set("sector_type", sectorType)
	query.Set("indicator", "今日")
	query.Set("keyword", sectorName)
	query.Set("page", "1")
	query.Set("page_size", "20")
	result := model.AStockSectorFundFlowListResult{}
	err := s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/sector-fund-flows?"+query.Encode(), &result)
	if cache != nil {
		cache.sectorFundFlows[cacheKey] = aStockSectorFundFlowCacheEntry{result: result, err: err}
	}
	return result, err
}

func (s *Server) applyAStockSectorFundFlowTrendScoreWithCache(strategyDate string, rec aStockRecommendation, cache *aStockRequestCache) aStockRecommendation {
	settings := s.loadAStockAlgorithmSettingsWithCache(cache)
	return s.applyAStockSectorFundFlowTrendScoreWithSettings(strategyDate, rec, cache, settings)
}

func (s *Server) applyAStockSectorFundFlowTrendScoreWithSettings(strategyDate string, rec aStockRecommendation, cache *aStockRequestCache, settings model.AStockRecommendationAlgorithmSettings) aStockRecommendation {
	if strings.TrimSpace(rec.Hotspot) == "" || aStockRecommendationHasScoreLabel(rec, "板块资金趋势") {
		return rec
	}
	settings = model.NormalizeAStockRecommendationAlgorithmSettings(settings)
	assessment := s.assessAStockRecommendationSectorFundFlowTrendWithSettings(strategyDate, rec.Hotspot, cache, settings)
	return applyAStockSectorFundFlowTrendAssessmentToRecommendationWithSettings(rec, assessment, settings)
}

func (s *Server) assessAStockRecommendationSectorFundFlowTrendWithCache(strategyDate string, hotspot string, cache *aStockRequestCache) aStockSectorFundFlowTrendAssessment {
	settings := s.loadAStockAlgorithmSettingsWithCache(cache)
	return s.assessAStockRecommendationSectorFundFlowTrendWithSettings(strategyDate, hotspot, cache, settings)
}

func (s *Server) assessAStockRecommendationSectorFundFlowTrendWithSettings(strategyDate string, hotspot string, cache *aStockRequestCache, settings model.AStockRecommendationAlgorithmSettings) aStockSectorFundFlowTrendAssessment {
	aliases := aStockHotspotSectorAliases(hotspot)
	if len(aliases) == 0 || strings.TrimSpace(s.cfg.ContentURL) == "" {
		return aStockSectorFundFlowTrendAssessment{Missing: true}
	}
	settings = model.NormalizeAStockRecommendationAlgorithmSettings(settings)
	best := aStockSectorFundFlowTrendAssessment{Missing: true}
	for _, alias := range aliases {
		result, err := s.loadAStockSectorFundFlowTrendWithCache(strategyDate, alias, 10, cache)
		if err != nil || len(result.Items) == 0 {
			continue
		}
		assessment := scoreAStockSectorFundFlowTrendAssessmentWithSettings(alias, result.Items, settings)
		if betterAStockSectorFundFlowTrendAssessment(assessment, best) {
			best = assessment
		}
	}
	return best
}

func betterAStockSectorFundFlowTrendAssessment(candidate aStockSectorFundFlowTrendAssessment, current aStockSectorFundFlowTrendAssessment) bool {
	if candidate.Missing {
		return false
	}
	if current.Missing {
		return true
	}
	if candidate.ScoreDelta > 0 || current.ScoreDelta > 0 {
		if candidate.ScoreDelta != current.ScoreDelta {
			return candidate.ScoreDelta > current.ScoreDelta
		}
		return math.Abs(candidate.Total5D) > math.Abs(current.Total5D)
	}
	if candidate.ScoreDelta != current.ScoreDelta {
		return candidate.ScoreDelta < current.ScoreDelta
	}
	return math.Abs(candidate.Total5D) > math.Abs(current.Total5D)
}

func scoreAStockSectorFundFlowTrendAssessment(alias aStockHotspotSectorAlias, items []model.AStockSectorFundFlow) aStockSectorFundFlowTrendAssessment {
	return scoreAStockSectorFundFlowTrendAssessmentWithSettings(alias, items, defaultAStockAlgorithmSettings())
}

func scoreAStockSectorFundFlowTrendAssessmentWithSettings(alias aStockHotspotSectorAlias, items []model.AStockSectorFundFlow, settings model.AStockRecommendationAlgorithmSettings) aStockSectorFundFlowTrendAssessment {
	assessment := aStockSectorFundFlowTrendAssessment{
		SectorType: normalizeSectorFundFlowSectorType(alias.SectorType),
		SectorName: strings.TrimSpace(alias.SectorName),
	}
	if len(items) < settings.Sector.TrendMinDays {
		assessment.Missing = true
		return assessment
	}
	sorted := append([]model.AStockSectorFundFlow(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return normalizeAStockStrategyDate(sorted[i].TradeDate) > normalizeAStockStrategyDate(sorted[j].TradeDate)
	})
	if assessment.SectorName == "" {
		assessment.SectorName = strings.TrimSpace(sorted[0].Name)
	}
	limit10 := min(len(sorted), 10)
	limit5 := min(len(sorted), 5)
	gross5 := 0.0
	previousSign := 0
	for i := 0; i < limit10; i++ {
		assessment.Total10D += sorted[i].MainNetInflow
	}
	for i := 0; i < limit5; i++ {
		net := sorted[i].MainNetInflow
		assessment.Total5D += net
		gross5 += math.Abs(net)
		sign := 0
		switch {
		case net > 0:
			assessment.InflowDays5D++
			sign = 1
		case net < 0:
			assessment.OutflowDays5D++
			sign = -1
		}
		if sign != 0 {
			if previousSign != 0 && sign != previousSign {
				assessment.SignChanges5D++
			}
			previousSign = sign
		}
	}
	if len(sorted) > 0 {
		assessment.LatestNetInflow = sorted[0].MainNetInflow
		assessment.LatestChangePct = sorted[0].ChangePct
		assessment.LatestRank = sorted[0].Rank
	}
	if len(sorted) >= 2 {
		assessment.Recent2DTotal = sorted[0].MainNetInflow + sorted[1].MainNetInflow
	}

	score := 0
	reasons := make([]string, 0, 8)
	consistency := 0.0
	if gross5 > 0 {
		consistency = math.Abs(assessment.Total5D) / gross5
	}
	switch {
	case assessment.Total5D > 0 && assessment.InflowDays5D >= 4 && consistency >= 0.5:
		assessment.Status = "连续流入"
		score += settings.Sector.TrendContinuousInflowScore
		reasons = append(reasons, fmt.Sprintf("近5日净流入%d天", assessment.InflowDays5D))
	case assessment.Total5D > 0 && assessment.InflowDays5D >= 3:
		assessment.Status = "震荡偏流入"
		score += settings.Sector.TrendPartialInflowScore
		reasons = append(reasons, fmt.Sprintf("近5日净流入%d天", assessment.InflowDays5D))
	case assessment.Total5D > 0:
		assessment.Status = "弱流入"
		score += settings.Sector.TrendWeakInflowScore
	case assessment.Total5D < 0 && assessment.OutflowDays5D >= 4:
		assessment.Status = "连续流出"
		score -= settings.Sector.TrendContinuousOutflowPenalty
		reasons = append(reasons, fmt.Sprintf("近5日净流出%d天", assessment.OutflowDays5D))
	case assessment.Total5D < 0 && assessment.OutflowDays5D >= 3:
		assessment.Status = "震荡偏流出"
		score -= settings.Sector.TrendPartialOutflowPenalty
		reasons = append(reasons, fmt.Sprintf("近5日净流出%d天", assessment.OutflowDays5D))
	case assessment.Total5D < 0:
		assessment.Status = "弱流出"
		score -= settings.Sector.TrendWeakOutflowPenalty
	default:
		assessment.Status = "震荡"
	}
	if limit10 > limit5 && assessment.Total10D > 0 && assessment.Total5D > 0 && assessment.Total5D/assessment.Total10D >= 0.6 {
		score += settings.Sector.TrendTenDayAccelerationScore
		reasons = append(reasons, "10日资金加速流入")
	}
	if limit10 > limit5 && assessment.Total10D < 0 && assessment.Total5D < 0 {
		score -= settings.Sector.TrendTenDayOutflowPenalty
		reasons = append(reasons, "10日与5日均净流出")
	}
	if len(sorted) >= 2 {
		if sorted[0].MainNetInflow > 0 && sorted[1].MainNetInflow > 0 && assessment.Recent2DTotal > settings.Sector.TrendTurnThreshold {
			score += settings.Sector.TrendRecent2DInflowScore
			reasons = append(reasons, "最近2日连续净流入")
		} else if sorted[0].MainNetInflow < 0 && sorted[1].MainNetInflow < 0 {
			score -= settings.Sector.TrendRecent2DOutflowPenalty
			reasons = append(reasons, "最近2日连续净流出")
		}
	}
	switch {
	case assessment.LatestRank > 0 && assessment.LatestRank <= 10:
		score += settings.Sector.TrendRankTop10Score
		reasons = append(reasons, fmt.Sprintf("最新排名%d", assessment.LatestRank))
	case assessment.LatestRank > 0 && assessment.LatestRank <= 30:
		score += settings.Sector.TrendRankTop30Score
		reasons = append(reasons, fmt.Sprintf("最新排名%d", assessment.LatestRank))
	}
	if assessment.LatestNetInflow < 0 && assessment.LatestChangePct < 0 {
		score -= settings.Sector.TrendLatestOutflowDownPenalty
		reasons = append(reasons, "当日净流出且板块下跌")
	}
	if assessment.SignChanges5D >= 3 && consistency < 0.35 {
		score -= settings.Sector.TrendChoppyPenalty
		reasons = append(reasons, "资金方向震荡")
	}
	assessment.ScoreDelta = clampAStockSectorFundFlowTrendScoreWithSettings(score, settings)
	assessment.ScoreReasons = reasons
	return assessment
}

func clampAStockSectorFundFlowTrendScore(score int) int {
	return clampAStockSectorFundFlowTrendScoreWithSettings(score, defaultAStockAlgorithmSettings())
}

func clampAStockSectorFundFlowTrendScoreWithSettings(score int, settings model.AStockRecommendationAlgorithmSettings) int {
	if score < settings.Sector.TrendScoreMin {
		return settings.Sector.TrendScoreMin
	}
	if score > settings.Sector.TrendScoreMax {
		return settings.Sector.TrendScoreMax
	}
	return score
}

func applyAStockSectorFundFlowTrendAssessmentToRecommendation(rec aStockRecommendation, assessment aStockSectorFundFlowTrendAssessment) aStockRecommendation {
	return applyAStockSectorFundFlowTrendAssessmentToRecommendationWithSettings(rec, assessment, defaultAStockAlgorithmSettings())
}

func applyAStockSectorFundFlowTrendAssessmentToRecommendationWithSettings(rec aStockRecommendation, assessment aStockSectorFundFlowTrendAssessment, settings model.AStockRecommendationAlgorithmSettings) aStockRecommendation {
	if assessment.Missing || assessment.ScoreDelta == 0 {
		return rec
	}
	reason := formatAStockSectorFundFlowTrendReason(assessment)
	applyAStockScoreDeltaWithSettings(&rec, settings, aStockScoreFactorSector, "板块资金趋势", reason, assessment.ScoreDelta)
	rec.Reason = appendAStockReason(rec.Reason, reason)
	return rec
}

func formatAStockSectorFundFlowTrendReason(assessment aStockSectorFundFlowTrendAssessment) string {
	direction := "净流入"
	if assessment.Total5D < 0 {
		direction = "净流出"
	}
	action := "加分"
	points := assessment.ScoreDelta
	if assessment.ScoreDelta < 0 {
		action = "减分"
		points = -assessment.ScoreDelta
	}
	sectorName := nonEmptyText(assessment.SectorName, "板块")
	status := nonEmptyText(assessment.Status, "震荡")
	parts := []string{fmt.Sprintf("%s 5日主力资金%s %s，状态%s", sectorName, direction, formatSectorFundFlowMoney(assessment.Total5D), status)}
	if assessment.Total10D != 0 {
		parts = append(parts, fmt.Sprintf("10日主力资金%s", formatSectorFundFlowMoney(assessment.Total10D)))
	}
	parts = append(parts, assessment.ScoreReasons...)
	parts = append(parts, fmt.Sprintf("板块资金趋势%s %d", action, points))
	return strings.Join(parts, "，")
}

func (s *Server) loadAStockSectorFundFlowTrendWithCache(endDate string, alias aStockHotspotSectorAlias, days int, cache *aStockRequestCache) (model.AStockSectorFundFlowTrendResult, error) {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return model.AStockSectorFundFlowTrendResult{}, fmt.Errorf("content service url is empty")
	}
	endDate = normalizeAStockStrategyDate(endDate)
	sectorType := normalizeSectorFundFlowSectorType(alias.SectorType)
	sectorName := strings.TrimSpace(alias.SectorName)
	if sectorName == "" {
		return model.AStockSectorFundFlowTrendResult{}, fmt.Errorf("empty sector name")
	}
	if days <= 0 {
		days = 10
	}
	cacheKey := strings.Join([]string{endDate, sectorType, sectorName, fmt.Sprint(days)}, "|")
	if cache != nil {
		if cache.sectorFundFlowTrends == nil {
			cache.sectorFundFlowTrends = make(map[string]aStockSectorFundFlowTrendCacheEntry)
		}
		if entry, ok := cache.sectorFundFlowTrends[cacheKey]; ok {
			return entry.result, entry.err
		}
	}
	query := url.Values{}
	query.Set("end_date", endDate)
	query.Set("sector_type", sectorType)
	query.Set("sector_name", sectorName)
	query.Set("indicator", "今日")
	query.Set("days", fmt.Sprint(days))
	result := model.AStockSectorFundFlowTrendResult{}
	err := s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/sector-fund-flow-trend?"+query.Encode(), &result)
	if cache != nil {
		cache.sectorFundFlowTrends[cacheKey] = aStockSectorFundFlowTrendCacheEntry{result: result, err: err}
	}
	return result, err
}

func (s *Server) loadAStockSectorFundFlowListWithCache(strategyDate string, sectorType string, indicator string, pageSize int, cache *aStockRequestCache) (model.AStockSectorFundFlowListResult, error) {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return model.AStockSectorFundFlowListResult{}, fmt.Errorf("content service url is empty")
	}
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	sectorType = normalizeSectorFundFlowSectorType(sectorType)
	indicator = strings.TrimSpace(indicator)
	if indicator == "" {
		indicator = "今日"
	}
	if pageSize <= 0 {
		pageSize = 500
	}
	cacheKey := strings.Join([]string{"list", strategyDate, sectorType, indicator, fmt.Sprint(pageSize)}, "|")
	if cache != nil {
		if cache.sectorFundFlows == nil {
			cache.sectorFundFlows = make(map[string]aStockSectorFundFlowCacheEntry)
		}
		if entry, ok := cache.sectorFundFlows[cacheKey]; ok {
			return entry.result, entry.err
		}
	}
	query := url.Values{}
	query.Set("date", strategyDate)
	query.Set("sector_type", sectorType)
	query.Set("indicator", indicator)
	query.Set("page", "1")
	query.Set("page_size", fmt.Sprint(pageSize))
	result := model.AStockSectorFundFlowListResult{}
	err := s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/sector-fund-flows?"+query.Encode(), &result)
	if cache != nil {
		cache.sectorFundFlows[cacheKey] = aStockSectorFundFlowCacheEntry{result: result, err: err}
	}
	return result, err
}

func (s *Server) loadAStockSectorTopStockResonanceWithCache(strategyDate string, recommendations []aStockRecommendation, cache *aStockRequestCache) map[string]aStockSectorTopStockResonance {
	settings := s.loadAStockAlgorithmSettingsWithCache(cache)
	return s.loadAStockSectorTopStockResonanceWithSettings(strategyDate, recommendations, cache, settings)
}

func (s *Server) loadAStockSectorTopStockResonanceWithSettings(strategyDate string, recommendations []aStockRecommendation, cache *aStockRequestCache, settings model.AStockRecommendationAlgorithmSettings) map[string]aStockSectorTopStockResonance {
	if normalizeAStockStrategyDate(strategyDate) < aStockSectorTopStockResonanceEffectiveDate {
		return nil
	}
	settings = model.NormalizeAStockRecommendationAlgorithmSettings(settings)
	resolver := newAStockSectorTopStockCodeResolver(recommendations)
	if resolver.empty() || strings.TrimSpace(s.cfg.ContentURL) == "" {
		return nil
	}
	flows := make([]model.AStockSectorFundFlow, 0, 256)
	for _, sectorType := range []string{"行业资金流", "概念资金流"} {
		result, err := s.loadAStockSectorFundFlowListWithCache(strategyDate, sectorType, "今日", 1000, cache)
		if err != nil {
			continue
		}
		flows = append(flows, result.Items...)
	}
	return buildAStockSectorTopStockResonanceMapWithSettings(flows, resolver, settings)
}

func newAStockSectorTopStockCodeResolver(recommendations []aStockRecommendation) aStockSectorTopStockCodeResolver {
	resolver := aStockSectorTopStockCodeResolver{
		codeByName:   make(map[string]string),
		allowedCodes: make(map[string]struct{}),
	}
	for _, rec := range recommendations {
		code := normalizeAStockCode(rec.Code)
		name := astockcode.DisplayName(code, rec.Name)
		if !astockcode.IsShanghaiShenzhen(code) || !hasResolvedAStockRecommendationName(code, name) {
			continue
		}
		resolver.allowedCodes[code] = struct{}{}
		for _, candidateName := range []string{name, rec.Name} {
			key := normalizeAStockSectorTopStockName(candidateName)
			if key != "" {
				resolver.codeByName[key] = code
			}
		}
	}
	if len(resolver.allowedCodes) == 0 {
		return aStockSectorTopStockCodeResolver{}
	}
	return resolver
}

func (resolver aStockSectorTopStockCodeResolver) empty() bool {
	return len(resolver.allowedCodes) == 0
}

func (resolver aStockSectorTopStockCodeResolver) resolve(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || resolver.empty() {
		return ""
	}
	if code := normalizeAStockCode(raw); code != "" {
		if _, ok := resolver.allowedCodes[code]; ok {
			return code
		}
	}
	for _, token := range strings.Fields(raw) {
		if code := normalizeAStockCode(token); code != "" {
			if _, ok := resolver.allowedCodes[code]; ok {
				return code
			}
		}
	}
	key := normalizeAStockSectorTopStockName(raw)
	if code := resolver.codeByName[key]; code != "" {
		return code
	}
	return ""
}

func normalizeAStockSectorTopStockName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "XD")
	value = strings.TrimPrefix(value, "XR")
	value = strings.TrimPrefix(value, "DR")
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "\t", "")
	return value
}

func buildAStockSectorTopStockResonanceMap(flows []model.AStockSectorFundFlow, resolver aStockSectorTopStockCodeResolver) map[string]aStockSectorTopStockResonance {
	return buildAStockSectorTopStockResonanceMapWithSettings(flows, resolver, defaultAStockAlgorithmSettings())
}

func buildAStockSectorTopStockResonanceMapWithSettings(flows []model.AStockSectorFundFlow, resolver aStockSectorTopStockCodeResolver, settings model.AStockRecommendationAlgorithmSettings) map[string]aStockSectorTopStockResonance {
	if len(flows) == 0 || resolver.empty() {
		return nil
	}
	result := make(map[string]aStockSectorTopStockResonance)
	sectorSeenByCode := make(map[string]map[string]struct{})
	sourceSeenByCode := make(map[string]map[string]struct{})
	for _, flow := range flows {
		if flow.MainNetInflow <= 0 || flow.ChangePct < 0 {
			continue
		}
		code := resolver.resolve(flow.TopStock)
		if code == "" {
			continue
		}
		sectorName := strings.TrimSpace(flow.Name)
		if sectorName == "" {
			continue
		}
		sectorKey := normalizeSectorFundFlowSectorType(flow.SectorType) + "|" + sectorName
		sectorSeen := sectorSeenByCode[code]
		if sectorSeen == nil {
			sectorSeen = make(map[string]struct{})
			sectorSeenByCode[code] = sectorSeen
		}
		if _, exists := sectorSeen[sectorKey]; exists {
			continue
		}
		sectorSeen[sectorKey] = struct{}{}
		resonance := result[code]
		resonance.Code = code
		resonance.Name = normalizeAStockSectorTopStockName(flow.TopStock)
		resonance.SectorNames = append(resonance.SectorNames, sectorName)
		resonance.TotalSectorMainNetInflow += flow.MainNetInflow
		sourceSeen := sourceSeenByCode[code]
		if sourceSeen == nil {
			sourceSeen = make(map[string]struct{})
			sourceSeenByCode[code] = sourceSeen
		}
		for _, sourceType := range aStockSectorFundFlowSourceTypes(flow) {
			if _, exists := sourceSeen[sourceType]; exists {
				continue
			}
			sourceSeen[sourceType] = struct{}{}
			resonance.SourceTypes = append(resonance.SourceTypes, sourceType)
		}
		resonance.PositiveSectorCount = len(resonance.SectorNames)
		resonance.SectorCount = resonance.PositiveSectorCount
		resonance.ScoreDelta = scoreAStockSectorTopStockResonanceWithSettings(resonance, settings)
		result[code] = resonance
	}
	for code, resonance := range result {
		if resonance.ScoreDelta <= 0 {
			delete(result, code)
			continue
		}
		sort.Strings(resonance.SourceTypes)
		result[code] = resonance
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func aStockSectorFundFlowSourceTypes(flow model.AStockSectorFundFlow) []string {
	raw := strings.TrimSpace(flow.SourceTypes)
	if raw == "" {
		raw = strings.TrimSpace(flow.SourceType)
	}
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == '、' || r == '|' || r == ';' || r == '；'
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func scoreAStockSectorTopStockResonance(resonance aStockSectorTopStockResonance) int {
	return scoreAStockSectorTopStockResonanceWithSettings(resonance, defaultAStockAlgorithmSettings())
}

func scoreAStockSectorTopStockResonanceWithSettings(resonance aStockSectorTopStockResonance, settings model.AStockRecommendationAlgorithmSettings) int {
	count := resonance.PositiveSectorCount
	if count <= 0 {
		count = resonance.SectorCount
	}
	if count <= 0 {
		return 0
	}
	if count > 3 {
		count = 3
	}
	score := 0
	switch count {
	case 1:
		score = settings.Sector.TopStockOneSectorScore
	case 2:
		score = settings.Sector.TopStockTwoSectorScore
	default:
		score = settings.Sector.TopStockThreeSectorScore
	}
	if resonance.TotalSectorMainNetInflow > settings.Sector.TopStockLargeInflowThreshold {
		score += settings.Sector.TopStockLargeInflowScore
	}
	if score > settings.Sector.TopStockScoreCap {
		score = settings.Sector.TopStockScoreCap
	}
	return score
}

func applyAStockSectorTopStockResonanceToRecommendation(rec aStockRecommendation, resonance aStockSectorTopStockResonance) aStockRecommendation {
	return applyAStockSectorTopStockResonanceToRecommendationWithSettings(rec, resonance, defaultAStockAlgorithmSettings())
}

func applyAStockSectorTopStockResonanceToRecommendationWithSettings(rec aStockRecommendation, resonance aStockSectorTopStockResonance, settings model.AStockRecommendationAlgorithmSettings) aStockRecommendation {
	if resonance.ScoreDelta <= 0 {
		return rec
	}
	scoreDelta := resonance.ScoreDelta
	if change60, ok := parseAStockPctText(rec.Change60); ok && change60 > settings.Volatility.Overheat60ThresholdPct {
		return rec
	}
	if change30, ok := parseAStockPctText(rec.Change30); ok && change30 > settings.Volatility.Overheat30ThresholdPct && scoreDelta > settings.Sector.TopStockOverheat30Cap {
		scoreDelta = settings.Sector.TopStockOverheat30Cap
	}
	if scoreDelta <= 0 || aStockRecommendationHasScoreLabel(rec, "板块资金共振") {
		return rec
	}
	reason := formatAStockSectorTopStockResonanceReason(rec, resonance, scoreDelta)
	applyAStockScoreDeltaWithSettings(&rec, settings, aStockScoreFactorSector, "板块资金共振", reason, scoreDelta)
	rec.Reason = appendAStockReason(rec.Reason, reason)
	return rec
}

func aStockRecommendationHasScoreLabel(rec aStockRecommendation, label string) bool {
	for _, component := range aStockRecommendationScoreBreakdown(rec) {
		if component.Label == label {
			return true
		}
	}
	return false
}

func formatAStockSectorTopStockResonanceReason(rec aStockRecommendation, resonance aStockSectorTopStockResonance, scoreDelta int) string {
	name := astockcode.DisplayName(normalizeAStockCode(rec.Code), rec.Name)
	if !hasResolvedAStockRecommendationName(rec.Code, name) {
		name = nonEmpty(resonance.Name, rec.Code)
	}
	sectorNames := resonance.SectorNames
	if len(sectorNames) > 3 {
		sectorNames = sectorNames[:3]
	}
	detail := fmt.Sprintf("板块资金共振：%s为%s主力净流入代表股", name, strings.Join(sectorNames, "、"))
	if resonance.TotalSectorMainNetInflow > 0 {
		detail += "，板块合计主力净流入 " + formatSectorFundFlowMoney(resonance.TotalSectorMainNetInflow)
	}
	detail += fmt.Sprintf("，板块共振加分 %d", scoreDelta)
	return detail
}

func aStockHoldingScore(summary model.StockInstitutionHoldingSummary) int {
	score := summary.HolderCount*2 + summary.HolderTypeCount*3 + int(summary.TotalFloatRatio)
	if score < 0 {
		return 0
	}
	if score > 20 {
		return 20
	}
	return score
}

func (s *Server) loadAStockWindowArticles(start time.Time, end time.Time) ([]model.Item, error) {
	return s.loadAStockWindowArticlesWithCache(start, end, nil)
}

func (s *Server) loadAStockWindowArticlesWithCache(start time.Time, end time.Time, cache *aStockRequestCache) ([]model.Item, error) {
	cacheKey := "window|" + start.UTC().Format(time.RFC3339Nano) + "|" + end.UTC().Format(time.RFC3339Nano)
	if cache != nil {
		if entry, ok := cache.articles[cacheKey]; ok {
			return entry.items, entry.err
		}
	}
	filtered, err := s.loadAStockWindowArticlesByPublishTimeWithCache(start, end, cache)
	if err != nil {
		if cache != nil {
			cache.articles[cacheKey] = aStockArticlesCacheEntry{err: err}
		}
		return nil, err
	}
	if len(filtered) > 0 {
		if fallbackItems, fallbackErr := s.loadAStockWindowArticlesByCapturedAt(start, end); fallbackErr == nil && len(fallbackItems) > 0 {
			filtered = mergeAStockPublishWindowArticles(filtered, fallbackItems)
		}
		if cache != nil {
			cache.articles[cacheKey] = aStockArticlesCacheEntry{items: filtered}
		}
		return filtered, nil
	}
	items, err := s.loadAStockWindowArticlePages("captured_at", start, end)
	if err != nil {
		if cache != nil {
			cache.articles[cacheKey] = aStockArticlesCacheEntry{items: filtered}
		}
		return filtered, nil
	}
	if cache != nil {
		cache.articles[cacheKey] = aStockArticlesCacheEntry{items: items}
	}
	return items, nil
}

func (s *Server) loadAStockWindowArticlesByCapturedAt(start time.Time, end time.Time) ([]model.Item, error) {
	return s.loadAStockWindowArticlePages("captured_at", start, end)
}

func mergeAStockPublishWindowArticles(primary []model.Item, capturedFallback []model.Item) []model.Item {
	if len(primary) == 0 {
		return append([]model.Item(nil), capturedFallback...)
	}
	if len(capturedFallback) == 0 {
		return append([]model.Item(nil), primary...)
	}
	sourceHasPublish := make(map[string]struct{})
	seen := make(map[string]struct{}, len(primary)+len(capturedFallback))
	result := make([]model.Item, 0, len(primary)+len(capturedFallback))
	for _, item := range primary {
		sourceType := provider.CanonicalSourceType(item.SourceType)
		if sourceType != "" {
			sourceHasPublish[sourceType] = struct{}{}
		}
		seen[aStockArticleDedupeKey(item)] = struct{}{}
		result = append(result, item)
	}
	for _, item := range capturedFallback {
		if strings.TrimSpace(item.PublishTime) != "" {
			continue
		}
		sourceType := provider.CanonicalSourceType(item.SourceType)
		if _, ok := sourceHasPublish[sourceType]; ok {
			continue
		}
		key := aStockArticleDedupeKey(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].CapturedAt.Before(result[j].CapturedAt)
	})
	return result
}

func aStockArticleDedupeKey(item model.Item) string {
	if item.ID > 0 {
		return fmt.Sprintf("id:%d", item.ID)
	}
	for _, value := range []string{item.SourceKey, item.DetailURL} {
		value = strings.TrimSpace(value)
		if value != "" {
			return strings.TrimSpace(item.SourceType) + "|" + value
		}
	}
	return strings.TrimSpace(item.SourceType) + "|" + strings.TrimSpace(item.Title) + "|" + strings.TrimSpace(item.PublishTimeText)
}

func (s *Server) loadAStockWindowArticlesByPublishTime(start time.Time, end time.Time) ([]model.Item, error) {
	return s.loadAStockWindowArticlesByPublishTimeWithCache(start, end, nil)
}

func (s *Server) loadAStockWindowArticlesByPublishTimeWithCache(start time.Time, end time.Time, cache *aStockRequestCache) ([]model.Item, error) {
	cacheKey := "publish|" + start.UTC().Format(time.RFC3339Nano) + "|" + end.UTC().Format(time.RFC3339Nano)
	if cache != nil {
		if entry, ok := cache.articles[cacheKey]; ok {
			return entry.items, entry.err
		}
	}
	items, err := s.loadAStockWindowArticlePages("publish_time", start, end)
	if err != nil {
		if cache != nil {
			cache.articles[cacheKey] = aStockArticlesCacheEntry{err: err}
		}
		return nil, err
	}
	if cache != nil {
		cache.articles[cacheKey] = aStockArticlesCacheEntry{items: items}
	}
	return items, nil
}

func (s *Server) loadAStockWindowArticlePages(timeField string, start time.Time, end time.Time) ([]model.Item, error) {
	cacheKey := aStockArticlePagesCacheKey(timeField, start, end)
	if items, err, ok := s.loadCachedAStockArticlePages(cacheKey); ok {
		return items, err
	}
	all := make([]model.Item, 0, aStockArticleFetchPageSize)
	for page := 1; page <= aStockArticleFetchMaxPages; page++ {
		result := model.ItemListResult{}
		query := aStockWindowArticlesQuery(timeField, start, end, page, aStockArticleFetchPageSize)
		if err := s.getJSON(s.cfg.ContentURL+query, &result); err != nil {
			return nil, err
		}
		if len(result.Items) == 0 {
			break
		}
		all = append(all, result.Items...)
		if result.Total > 0 && len(all) >= result.Total {
			break
		}
		if len(result.Items) < aStockArticleFetchPageSize {
			break
		}
	}
	items := filterAStockNews(all)
	s.storeCachedAStockArticlePages(cacheKey, aStockWindowCacheDate(start), items)
	return items, nil
}

func aStockWindowArticlesQuery(timeField string, start time.Time, end time.Time, page int, pageSize int) string {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = aStockArticleFetchPageSize
	}
	timeField = strings.TrimSpace(timeField)
	if timeField == "" {
		timeField = "captured_at"
	}
	startText, endText := aStockWindowArticleQueryBounds(timeField, start, end)
	query := url.Values{}
	query.Set("page", fmt.Sprint(page))
	query.Set("page_size", fmt.Sprint(pageSize))
	query.Set("time_field", timeField)
	query.Set("start", startText)
	query.Set("end", endText)
	query.Set("lite", "1")
	return "/api/v1/articles?" + query.Encode()
}

func aStockWindowArticleQueryBounds(timeField string, start time.Time, end time.Time) (string, string) {
	switch strings.ToLower(strings.TrimSpace(timeField)) {
	case "publish_time", "published_at":
		return formatAStockPublishTime(start), formatAStockPublishTime(end)
	default:
		return start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339)
	}
}

func (s *Server) loadAStockMarketCandidates(strategyDate string) []aStockMarketCandidate {
	candidates, _, _ := s.loadAStockMarketCandidatesWithStatus(strategyDate)
	return candidates
}

func (s *Server) loadAStockMarketCandidatesWithStatus(strategyDate string) ([]aStockMarketCandidate, string, model.AStockAuctionListResult) {
	return s.loadAStockMarketCandidatesWithStatusWithCache(strategyDate, nil)
}

func (s *Server) loadAStockMarketCandidatesWithStatusWithCache(strategyDate string, cache *aStockRequestCache) ([]aStockMarketCandidate, string, model.AStockAuctionListResult) {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return nil, "content_unconfigured", model.AStockAuctionListResult{}
	}
	date := normalizeAStockStrategyDate(strategyDate)
	result, ok := s.loadAStockMarketCandidateResultWithCache(date, cache)
	dateResult := result
	if ok {
		candidates := aStockMarketCandidatesFromAuctionResult(result)
		if len(candidates) > 0 {
			return candidates, "date_auction", result
		}
		return nil, "no_auction_candidates", result
	}
	if date != "" {
		result, ok = s.loadAStockMarketCandidateResultWithCache("", cache)
		if ok {
			return aStockMarketCandidatesFromAuctionResult(result), "latest_auction_fallback", dateResult
		}
	}
	return nil, "no_auction_candidates", dateResult
}

func formatAStockAuctionSummaryAmount(result model.AStockAuctionListResult) string {
	if result.TotalAmount > 0 {
		return formatAStockAuctionMoney(result.TotalAmount)
	}
	total := 0.0
	for _, item := range result.Items {
		total += item.AuctionAmount
	}
	return formatAStockAuctionMoney(total)
}

func (s *Server) loadAStockMarketCandidateResult(strategyDate string) (model.AStockAuctionListResult, bool) {
	return s.loadAStockMarketCandidateResultWithCache(strategyDate, nil)
}

func (s *Server) loadAStockMarketCandidateResultWithCache(strategyDate string, cache *aStockRequestCache) (model.AStockAuctionListResult, bool) {
	return s.loadAStockMarketCandidateResultForCaptureSlotWithCache(strategyDate, "", cache)
}

func (s *Server) loadAStockMarketCandidateResultForCaptureSlotWithCache(strategyDate string, captureSlot string, cache *aStockRequestCache) (model.AStockAuctionListResult, bool) {
	cacheKey := aStockAuctionCandidateCacheKeyWithSlot(strategyDate, captureSlot)
	if cache != nil && cache.auctionResults != nil {
		if cached, ok := cache.auctionResults[cacheKey]; ok {
			return cached.result, cached.found
		}
		if result, found, ok := s.loadCachedAStockAuctionCandidateResult(cacheKey); ok {
			cache.auctionResults[cacheKey] = aStockAuctionResultCacheEntry{result: result, found: found}
			return result, found
		}
		result, found := s.fetchAStockMarketCandidateResultForCaptureSlot(strategyDate, captureSlot)
		s.storeCachedAStockAuctionCandidateResult(cacheKey, result, found)
		cache.auctionResults[cacheKey] = aStockAuctionResultCacheEntry{result: result, found: found}
		return result, found
	}
	if result, found, ok := s.loadCachedAStockAuctionCandidateResult(cacheKey); ok {
		return result, found
	}
	result, found := s.fetchAStockMarketCandidateResultForCaptureSlot(strategyDate, captureSlot)
	s.storeCachedAStockAuctionCandidateResult(cacheKey, result, found)
	return result, found
}

func aStockAuctionCandidateCacheKey(strategyDate string) string {
	return aStockAuctionCandidateCacheKeyWithSlot(strategyDate, "")
}

func aStockAuctionCandidateCacheKeyWithSlot(strategyDate string, captureSlot string) string {
	slot := normalizeAStockAuctionStrengthCaptureSlot(captureSlot)
	if strings.TrimSpace(strategyDate) == "" {
		if slot != "" {
			return "__latest__|" + slot
		}
		return "__latest__"
	}
	key := normalizeAStockStrategyDate(strategyDate)
	if slot != "" {
		key += "|" + slot
	}
	return key
}

func (s *Server) loadCachedAStockAuctionCandidateResult(cacheKey string) (model.AStockAuctionListResult, bool, bool) {
	if strings.TrimSpace(cacheKey) == "" || s == nil {
		return model.AStockAuctionListResult{}, false, false
	}
	s.aStockCacheMu.Lock()
	defer s.aStockCacheMu.Unlock()
	if len(s.aStockAuctions) == 0 {
		return model.AStockAuctionListResult{}, false, false
	}
	entry, ok := s.aStockAuctions[cacheKey]
	if !ok {
		return model.AStockAuctionListResult{}, false, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(s.aStockAuctions, cacheKey)
		return model.AStockAuctionListResult{}, false, false
	}
	return entry.result, entry.found, true
}

func (s *Server) storeCachedAStockAuctionCandidateResult(cacheKey string, result model.AStockAuctionListResult, found bool) {
	if strings.TrimSpace(cacheKey) == "" || s == nil || !found {
		return
	}
	s.aStockCacheMu.Lock()
	defer s.aStockCacheMu.Unlock()
	if s.aStockAuctions == nil {
		s.aStockAuctions = make(map[string]aStockServerAuctionCacheEntry)
	}
	s.aStockAuctions[cacheKey] = aStockServerAuctionCacheEntry{
		result:    result,
		found:     found,
		expiresAt: time.Now().Add(aStockAuctionCandidateCacheTTL),
	}
}

func (s *Server) clearAStockAuctionCandidateCache(strategyDates ...string) {
	if s == nil {
		return
	}
	s.aStockCacheMu.Lock()
	defer s.aStockCacheMu.Unlock()
	if len(strategyDates) == 0 {
		s.aStockAuctions = make(map[string]aStockServerAuctionCacheEntry)
		return
	}
	for _, date := range strategyDates {
		delete(s.aStockAuctions, aStockAuctionCandidateCacheKey(date))
		for _, slot := range []string{"0920", "0925", "0929"} {
			delete(s.aStockAuctions, aStockAuctionCandidateCacheKeyWithSlot(date, slot))
		}
	}
	delete(s.aStockAuctions, "__latest__")
	for _, slot := range []string{"0920", "0925", "0929"} {
		delete(s.aStockAuctions, aStockAuctionCandidateCacheKeyWithSlot("", slot))
	}
}

func (s *Server) fetchAStockMarketCandidateResult(strategyDate string) (model.AStockAuctionListResult, bool) {
	return s.fetchAStockMarketCandidateResultForCaptureSlot(strategyDate, "")
}

func (s *Server) fetchAStockMarketCandidateResultForCaptureSlot(strategyDate string, captureSlot string) (model.AStockAuctionListResult, bool) {
	var result model.AStockAuctionListResult
	limit := s.loadAStockAlgorithmSettings().Auction.MarketCandidateLimit
	if limit <= 0 {
		limit = aStockMarketCandidateLimit
	}
	query := "/api/v1/a-stock/auction?page=1&page_size=" + fmt.Sprint(limit)
	if strings.TrimSpace(strategyDate) != "" {
		query += "&date=" + url.QueryEscape(normalizeAStockStrategyDate(strategyDate))
	}
	if slot := normalizeAStockAuctionStrengthCaptureSlot(captureSlot); slot != "" {
		query += "&capture_slot=" + url.QueryEscape(slot)
	}
	if err := s.getJSON(s.cfg.ContentURL+query, &result); err != nil {
		return model.AStockAuctionListResult{}, false
	}
	if result.TotalAmount <= 0 && len(result.Items) == 0 {
		return result, false
	}
	return result, true
}

func (s *Server) loadAStockAuctionAmountLabelWithCache(strategyDate string, cache *aStockRequestCache) string {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return ""
	}
	result, ok := s.loadAStockMarketCandidateResultWithCache(strategyDate, cache)
	if !ok {
		return ""
	}
	return normalizeAStockAuctionSummaryLabel(formatAStockAuctionSummaryAmount(result))
}

func normalizeAStockAuctionSummaryLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "--" {
		return ""
	}
	return value
}

func aStockMarketCandidatesFromAuctionResult(result model.AStockAuctionListResult) []aStockMarketCandidate {
	candidates := make([]aStockMarketCandidate, 0, len(result.Items))
	for i, item := range result.Items {
		code := normalizeAStockCode(item.Code)
		name := astockcode.DisplayName(code, item.Name)
		if !astockcode.IsShanghaiShenzhen(code) || !hasResolvedAStockRecommendationName(code, name) {
			continue
		}
		if item.AuctionAmount <= 0 && item.AuctionVolume <= 0 {
			continue
		}
		candidate := aStockMarketCandidate{
			Code:          code,
			Name:          name,
			TradeDate:     nonEmpty(item.TradeDate, result.Date),
			Rank:          i + 1,
			AuctionAmount: item.AuctionAmount,
			AuctionVolume: item.AuctionVolume,
		}
		assignAStockAuctionStrengthSlot(&candidate, nonEmpty(item.CaptureSlot, result.CaptureSlot), item.AuctionAmount, item.AuctionVolume)
		candidates = append(candidates, candidate)
	}
	return candidates
}

func (s *Server) loadAStockEveningCandidates(strategyDate string) ([]aStockEveningCandidate, string) {
	baseURL := strings.TrimRight(strings.TrimSpace(s.cfg.AStockAuctionURL), "/")
	if baseURL == "" {
		return nil, "evening_snapshot_unconfigured"
	}
	date := normalizeAStockStrategyDate(strategyDate)
	var payload aStockEveningSnapshotPayload
	resp, err := s.client.R().
		SetQueryParam("date", date).
		SetQueryParam("limit", "0").
		SetResult(&payload).
		Get(baseURL + "/api/a-stock/evening-snapshot")
	if err != nil {
		return nil, "evening_snapshot_failed: " + err.Error()
	}
	if !resp.IsSuccess() {
		message := strings.TrimSpace(payload.Message)
		if message == "" {
			message = strings.TrimSpace(payload.Warning)
		}
		if message != "" {
			return nil, fmt.Sprintf("evening_snapshot_status_%d: %s", resp.StatusCode(), message)
		}
		return nil, fmt.Sprintf("evening_snapshot_status_%d", resp.StatusCode())
	}
	candidates := normalizeAStockEveningCandidates(payload.Items, nonEmpty(payload.Date, date))
	if len(candidates) == 0 {
		return nil, nonEmpty(strings.TrimSpace(payload.Warning), strings.TrimSpace(payload.Message), "evening_snapshot_empty")
	}
	if warning := strings.TrimSpace(payload.Warning); warning != "" {
		return candidates, "evening_snapshot_with_warning: " + warning
	}
	return candidates, "evening_snapshot"
}

func normalizeAStockEveningCandidates(items []aStockEveningCandidate, fallbackDate string) []aStockEveningCandidate {
	out := make([]aStockEveningCandidate, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		code := normalizeAStockCode(item.Code)
		name := astockcode.DisplayName(code, item.Name)
		if !astockcode.IsShanghaiShenzhen(code) || !hasResolvedAStockRecommendationName(code, name) || isBlockedAStockRecommendationStock(code, name) {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		item.Code = code
		item.Name = name
		item.TradeDate = nonEmpty(normalizeAStockMarketDate(item.TradeDate), normalizeAStockStrategyDate(fallbackDate))
		out = append(out, item)
	}
	return out
}

func buildAStockEveningRecommendations(strategyDate string, candidates []aStockEveningCandidate, maxRecommendations int) []aStockRecommendation {
	eligible := eligibleAStockEveningRecommendations(strategyDate, candidates)
	if maxRecommendations > 0 && len(eligible) > maxRecommendations {
		eligible = eligible[:maxRecommendations]
	}
	return rerankAStockRecommendations(eligible)
}

func eligibleAStockEveningRecommendations(strategyDate string, candidates []aStockEveningCandidate) []aStockRecommendation {
	recommendations := make([]aStockRecommendation, 0, len(candidates))
	for _, candidate := range candidates {
		if !isEligibleAStockEveningCandidate(candidate) {
			continue
		}
		recommendations = append(recommendations, aStockRecommendation{
			Rank:        len(recommendations) + 1,
			Hotspot:     aStockEveningHotspot,
			Code:        normalizeAStockCode(candidate.Code),
			Name:        astockcode.DisplayName(candidate.Code, candidate.Name),
			MarketScore: aStockEveningMarketScore(candidate),
			Reason:      formatAStockEveningRecommendationReason(candidate),
			EntryTime:   aStockDefaultRecommendationEntryTime("evening"),
		})
	}
	sort.SliceStable(recommendations, func(i, j int) bool {
		left := aStockEveningCandidateForRecommendation(recommendations[i], candidates)
		right := aStockEveningCandidateForRecommendation(recommendations[j], candidates)
		if left.Amount != right.Amount {
			return left.Amount > right.Amount
		}
		if left.Speed != right.Speed {
			return left.Speed > right.Speed
		}
		if left.ChangePct != right.ChangePct {
			return left.ChangePct > right.ChangePct
		}
		return recommendations[i].Code < recommendations[j].Code
	})
	return rerankAStockRecommendations(recommendations)
}

func isEligibleAStockEveningCandidate(candidate aStockEveningCandidate) bool {
	code := normalizeAStockCode(candidate.Code)
	name := astockcode.DisplayName(code, candidate.Name)
	if !astockcode.IsShanghaiShenzhen(code) || !hasResolvedAStockRecommendationName(code, name) || isBlockedAStockRecommendationStock(code, name) {
		return false
	}
	return candidate.VolumeRatio > 3 && candidate.VolumeRatio < 10 &&
		candidate.ChangePct > 3 && candidate.ChangePct < 8 &&
		candidate.Price > 4 &&
		candidate.TurnoverPct > 2 && candidate.TurnoverPct < 15 &&
		candidate.Amount > 200000000 && candidate.Amount < 2000000000 &&
		candidate.Speed > 0
}

func aStockEveningMarketScore(candidate aStockEveningCandidate) int {
	score := int(candidate.Amount / 1000000)
	if score > 1000 {
		return 1000
	}
	if score < 0 {
		return 0
	}
	return score
}

func aStockEveningCandidateForRecommendation(rec aStockRecommendation, candidates []aStockEveningCandidate) aStockEveningCandidate {
	code := normalizeAStockCode(rec.Code)
	for _, candidate := range candidates {
		if normalizeAStockCode(candidate.Code) == code {
			return candidate
		}
	}
	return aStockEveningCandidate{}
}

func formatAStockEveningRecommendationReason(candidate aStockEveningCandidate) string {
	return fmt.Sprintf(
		"量比 %.2f，涨跌幅 %s，价格 %s元，换手率 %s，成交额 %s，涨速 %s，后一交易日09:30开盘入场",
		candidate.VolumeRatio,
		formatAStockPct(candidate.ChangePct),
		formatAStockPrice(candidate.Price),
		formatAStockPct(candidate.TurnoverPct),
		formatAStockAuctionMoney(candidate.Amount),
		formatAStockPct(candidate.Speed),
	)
}

func filterRecentAStockRecommendations(recommendations []aStockRecommendation, recentCodes map[string]struct{}) ([]aStockRecommendation, int) {
	result := filterRecentAStockRecommendationsWithReplenishment(recommendations, nil, recentCodes, len(recommendations))
	return result.Recommendations, result.Filtered
}

type aStockRecentRecommendationFilterResult struct {
	Recommendations []aStockRecommendation
	Filtered        int
	FilteredStocks  []aStockRecommendation
	Replenished     int
	Shortfall       bool
}

func filterRecentAStockRecommendationsWithReplenishment(base []aStockRecommendation, replacementPool []aStockRecommendation, recentCodes map[string]struct{}, target int) aStockRecentRecommendationFilterResult {
	if target <= 0 {
		target = len(base)
	}
	if len(base) == 0 || len(recentCodes) == 0 {
		return aStockRecentRecommendationFilterResult{Recommendations: rerankAStockRecommendations(base)}
	}
	if len(replacementPool) == 0 {
		replacementPool = base
	}
	result := aStockRecentRecommendationFilterResult{}
	filteredCodes := make(map[string]struct{})
	recordRecent := func(rec aStockRecommendation) {
		code := normalizeAStockCode(rec.Code)
		code = normalizeAStockCode(code)
		if code == "" {
			return
		}
		if _, exists := filteredCodes[code]; exists {
			return
		}
		filteredCodes[code] = struct{}{}
		result.Filtered++
		rec.Code = code
		result.FilteredStocks = append(result.FilteredStocks, rec)
	}
	isRecent := func(code string) bool {
		_, ok := recentCodes[normalizeAStockCode(code)]
		return ok
	}

	baseHotspotCounts := make(map[string]int)
	keptHotspotCounts := make(map[string]int)
	seen := make(map[string]struct{}, len(base))
	kept := make([]aStockRecommendation, 0, minInt(len(base), target))
	for _, rec := range base {
		code := normalizeAStockCode(rec.Code)
		if code == "" {
			continue
		}
		rec.Code = code
		hotspot := normalizeAStockRecommendationHotspot(rec.Hotspot)
		if hotspot != "" {
			baseHotspotCounts[hotspot]++
		}
		if isRecent(code) {
			recordRecent(rec)
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		kept = append(kept, rec)
		if hotspot != "" {
			keptHotspotCounts[hotspot]++
		}
		if len(kept) >= target {
			result.Recommendations = rerankAStockRecommendations(kept[:target])
			return result
		}
	}

	hotspotDeficits := make(map[string]int, len(baseHotspotCounts))
	for hotspot, baseCount := range baseHotspotCounts {
		if deficit := baseCount - keptHotspotCounts[hotspot]; deficit > 0 {
			hotspotDeficits[hotspot] = deficit
		}
	}
	candidates := sortedAStockReplacementRecommendations(replacementPool)
	appendCandidate := func(rec aStockRecommendation) bool {
		if len(kept) >= target {
			return false
		}
		code := normalizeAStockCode(rec.Code)
		if code == "" {
			return false
		}
		rec.Code = code
		if _, exists := seen[code]; exists {
			return false
		}
		if isBlockedAStockRecommendationStock(rec.Code, rec.Name) {
			return false
		}
		if isRecent(code) {
			recordRecent(rec)
			return false
		}
		seen[code] = struct{}{}
		kept = append(kept, rec)
		result.Replenished++
		return true
	}
	for _, rec := range candidates {
		hotspot := normalizeAStockRecommendationHotspot(rec.Hotspot)
		if hotspot == "" || hotspotDeficits[hotspot] <= 0 {
			continue
		}
		if appendCandidate(rec) {
			hotspotDeficits[hotspot]--
		}
		if len(kept) >= target {
			break
		}
	}
	if len(kept) < target {
		for _, rec := range candidates {
			if appendCandidate(rec) && len(kept) >= target {
				break
			}
		}
	}
	result.Shortfall = len(kept) < target
	result.Recommendations = rerankAStockRecommendations(kept)
	return result
}

func sortedAStockReplacementRecommendations(recommendations []aStockRecommendation) []aStockRecommendation {
	result := append([]aStockRecommendation(nil), recommendations...)
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].MarketScore == result[j].MarketScore {
			if result[i].Hotspot == result[j].Hotspot {
				return normalizeAStockCode(result[i].Code) < normalizeAStockCode(result[j].Code)
			}
			return result[i].Hotspot < result[j].Hotspot
		}
		return result[i].MarketScore > result[j].MarketScore
	})
	return result
}

func sortAStockRecommendationsByScore(recommendations []aStockRecommendation) []aStockRecommendation {
	result := append([]aStockRecommendation(nil), recommendations...)
	sort.SliceStable(result, func(i, j int) bool {
		leftScore := result[i].MarketScore
		if leftScore == 0 {
			leftScore = result[i].HotspotScore
		}
		rightScore := result[j].MarketScore
		if rightScore == 0 {
			rightScore = result[j].HotspotScore
		}
		if leftScore == rightScore {
			if result[i].Hotspot == result[j].Hotspot {
				return normalizeAStockCode(result[i].Code) < normalizeAStockCode(result[j].Code)
			}
			return result[i].Hotspot < result[j].Hotspot
		}
		return leftScore > rightScore
	})
	return rerankAStockRecommendations(result)
}

func filterAStockNegativeNoEvidenceRecommendations(recommendations []aStockRecommendation, counted map[string]struct{}) ([]aStockRecommendation, int) {
	if len(recommendations) == 0 {
		return recommendations, 0
	}
	filtered := make([]aStockRecommendation, 0, len(recommendations))
	skipped := 0
	for _, rec := range recommendations {
		if isAStockNegativeNoEvidenceRecommendation(rec) {
			code := normalizeAStockCode(rec.Code)
			if counted != nil && code != "" {
				if _, exists := counted[code]; exists {
					continue
				}
				counted[code] = struct{}{}
			}
			skipped++
			continue
		}
		filtered = append(filtered, rec)
	}
	return rerankAStockRecommendations(filtered), skipped
}

func isAStockNegativeNoEvidenceRecommendation(rec aStockRecommendation) bool {
	hasNegative := false
	hasEvidenceComponent := false
	strongEvidenceScore := 0
	for _, component := range aStockRecommendationScoreBreakdown(rec) {
		switch strings.TrimSpace(component.Label) {
		case "负面新闻":
			if component.Score < 0 || strings.Contains(component.Detail, "负面新闻") {
				hasNegative = true
			}
		case "个股证据":
			hasEvidenceComponent = true
			if component.Score > 0 {
				strongEvidenceScore += component.Score
			}
		}
	}
	if hasNegative && hasEvidenceComponent && strongEvidenceScore <= 0 {
		return true
	}
	reason := strings.TrimSpace(rec.Reason)
	return hasNegative && !hasEvidenceComponent && strings.Contains(reason, "个股证据 0 条")
}

func formatAStockNegativeNoEvidenceFilterStatus(filtered int) string {
	if filtered <= 0 {
		return ""
	}
	return fmt.Sprintf("过滤负面无个股证据股票 %d", filtered)
}

func formatAStockRecentReplenishmentStatus(filtered int, replenished int, shortfall bool) string {
	return formatAStockRecentReplenishmentStatusWithStocks(filtered, nil, replenished, shortfall)
}

func formatAStockRecentReplenishmentStatusWithStocks(filtered int, filteredStocks []aStockRecommendation, replenished int, shortfall bool) string {
	if filtered <= 0 {
		return ""
	}
	parts := []string{formatAStockFilterCountWithStocks(aStockRecentLookbackStatusPrefix()+"过滤", filtered, "只", filteredStocks)}
	if replenished > 0 {
		parts = append(parts, fmt.Sprintf("递补 %d 只", replenished))
	}
	if shortfall {
		parts = append(parts, "候选不足未补满")
	}
	return strings.Join(parts, "，")
}

func appendAStockBacktestStatus(status string, addition string) string {
	status = strings.TrimSpace(status)
	addition = strings.TrimSpace(addition)
	if addition == "" {
		return status
	}
	if status == "" {
		return addition
	}
	if strings.Contains(status, addition) {
		return status
	}
	return status + "，" + addition
}

func appendAStockReason(reason string, addition string) string {
	reason = strings.TrimSpace(reason)
	addition = strings.TrimSpace(addition)
	if addition == "" {
		return reason
	}
	if reason == "" {
		return addition
	}
	if strings.Contains(reason, addition) {
		return reason
	}
	return reason + "，" + addition
}

func appendAStockScoreComponent(rec *aStockRecommendation, label string, detail string, score int) {
	appendAStockScoreComponentWithUnit(rec, label, detail, score, score)
}

func appendAStockScoreComponentWithUnit(rec *aStockRecommendation, label string, detail string, score int, unitValue int) {
	appendAStockScoreComponentWithFactor(rec, "", label, detail, score, unitValue)
}

func appendAStockScoreComponentWithFactor(rec *aStockRecommendation, factor string, label string, detail string, score int, unitValue int) {
	if rec == nil {
		return
	}
	label = strings.TrimSpace(label)
	detail = strings.TrimSpace(detail)
	if label == "" {
		return
	}
	component := newAStockScoreComponent(label, detail, unitValue, score)
	if normalized := normalizeAStockScoreFactor(factor); normalized != "" {
		component.Factor = normalized
	}
	rec.ScoreBreakdown = append(rec.ScoreBreakdown, aStockRecommendationScoreComponent{
		Factor:    component.Factor,
		Label:     component.Label,
		Detail:    component.Detail,
		UnitValue: component.UnitValue,
		Score:     component.Score,
	})
}

func appendAStockScoreComponentIfNonZero(rec *aStockRecommendation, label string, detail string, score int) {
	if score == 0 {
		return
	}
	appendAStockScoreComponent(rec, label, detail, score)
}

func appendAStockScoreAdjustment(rec *aStockRecommendation, label string, detail string, scoreDelta int) {
	appendAStockScoreComponentIfNonZero(rec, label, detail, scoreDelta)
}

func applyAStockScoreDeltaWithSettings(rec *aStockRecommendation, settings model.AStockRecommendationAlgorithmSettings, factor string, label string, detail string, scoreDelta int) {
	if rec == nil || scoreDelta == 0 {
		return
	}
	canRecalculate := canRecalculateAStockRecommendationScore(rec.ScoreBreakdown)
	if !canRecalculate {
		baseScore := rec.MarketScore
		if baseScore == 0 {
			baseScore = rec.HotspotScore
		}
		rec.MarketScore = baseScore + scoreDelta
	}
	appendAStockScoreComponentWithFactor(rec, factor, label, detail, scoreDelta, scoreDelta)
	if canRecalculate {
		recalculateAStockRecommendationScoreWithSettings(rec, settings)
	}
}

func canRecalculateAStockRecommendationScore(components []aStockRecommendationScoreComponent) bool {
	if len(components) == 0 {
		return false
	}
	for _, component := range components {
		factor := normalizeAStockScoreFactor(component.Factor)
		if factor == "" && aStockScoreComponentFactor(component) != aStockScoreFactorHistory {
			return false
		}
		if factor != "" && !isAStockPrimaryScoreFactor(factor) && factor != aStockScoreFactorHistory {
			return false
		}
	}
	return true
}

func aStockRecommendationScoreTotal(rec aStockRecommendation, components []aStockRecommendationScoreComponent) int {
	if rec.MarketScore != 0 {
		return rec.MarketScore
	}
	return aStockRecommendationFactorScoreTotalWithSettings(components, defaultAStockAlgorithmSettings())
}

func aStockRecommendationScoreBreakdown(rec aStockRecommendation) []aStockRecommendationScoreComponent {
	if len(rec.ScoreBreakdown) > 0 {
		return rec.ScoreBreakdown
	}
	return parseAStockRecommendationScoreBreakdown(rec)
}

func newAStockScoreComponent(label string, detail string, unitValue int, score int) aStockRecommendationScoreComponent {
	component := aStockRecommendationScoreComponent{
		Label:     label,
		Detail:    detail,
		UnitValue: unitValue,
		Score:     score,
	}
	component.Factor = aStockScoreComponentFactor(component)
	return component
}

func newAStockScoreComponentWithFactor(factor string, label string, detail string, unitValue int, score int) aStockRecommendationScoreComponent {
	component := newAStockScoreComponent(label, detail, unitValue, score)
	if normalized := normalizeAStockScoreFactor(factor); normalized != "" {
		component.Factor = normalized
	}
	return component
}

func aStockRecommendationFactorScoreTotalWithSettings(components []aStockRecommendationScoreComponent, settings model.AStockRecommendationAlgorithmSettings) int {
	total := 0
	for _, category := range newAStockScoreCategorySummary().order {
		if !isAStockPrimaryScoreFactor(category.Key) {
			continue
		}
		total += aStockRecommendationFactorSubtotalWithSettings(components, category.Key, settings)
	}
	if total < 0 {
		return 0
	}
	if total > 1000 {
		return 1000
	}
	return total
}

func aStockRecommendationFactorSubtotalWithSettings(components []aStockRecommendationScoreComponent, factor string, settings model.AStockRecommendationAlgorithmSettings) int {
	raw := 0
	for _, component := range components {
		if aStockScoreComponentFactor(component) == factor {
			raw += component.Score
		}
	}
	return clampAStockScoreFactorSubtotal(raw, aStockScoreFactorCapWithSettings(factor, settings))
}

func recalculateAStockRecommendationScoreWithSettings(rec *aStockRecommendation, settings model.AStockRecommendationAlgorithmSettings) {
	if rec == nil || len(rec.ScoreBreakdown) == 0 {
		return
	}
	rec.MarketScore = aStockRecommendationFactorScoreTotalWithSettings(rec.ScoreBreakdown, settings)
}

var (
	aStockReasonHotspotScorePattern        = regexp.MustCompile(`命中\s*([^，；]+)，证据新闻\s*(\d+)\s*条，热度分\s*(-?\d+)`)
	aStockReasonSimpleHotspotScorePattern  = regexp.MustCompile(`热度分\s*(-?\d+)`)
	aStockReasonNegativePenaltyPattern     = regexp.MustCompile(`负面新闻\s*(\d+)\s*条，(?:板块减分|情绪扣分)\s*(\d+)`)
	aStockReasonMarketScorePattern         = regexp.MustCompile(`行情排名\s*(\d+)[^，；]*，成交额[^，；]*，个股证据\s*(\d+)\s*条，行情分\s*(-?\d+)`)
	aStockReasonMatchedScorePattern        = regexp.MustCompile(`个股证据\s*(\d+)\s*条，匹配分\s*(-?\d+)`)
	aStockReasonComprehensiveScorePattern  = regexp.MustCompile(`综合分\s*(-?\d+)`)
	aStockReasonStockKeywordPattern        = regexp.MustCompile(`股票名命中\s*([^，；]+)`)
	aStockReasonWeakPenaltyPattern         = regexp.MustCompile(`个股证据减分\s*(\d+)`)
	aStockReasonHoldingBonusPattern        = regexp.MustCompile(`持仓加分\s*(\d+)`)
	aStockReasonFundBonusPattern           = regexp.MustCompile(`资金加分\s*(\d+)`)
	aStockReasonFundPenaltyPattern         = regexp.MustCompile(`资金减分\s*(\d+)`)
	aStockReasonLowOpenPenaltyPattern      = regexp.MustCompile(`盘口减分\s*(\d+)`)
	aStockReasonFundStrengthPenaltyPattern = regexp.MustCompile(`资金强度减分\s*(\d+)`)
	aStockReasonSectorDrawdownPattern      = regexp.MustCompile(`板块回撤减分\s*(\d+)`)
	aStockReasonHighOpenBonusPattern       = regexp.MustCompile(`(?:([^，；]*高开[^，；]*)，)?高开加分\s*(\d+)`)
	aStockReasonLowOpenTierPenaltyPattern  = regexp.MustCompile(`(?:([^，；]*低开[^，；]*)，)?低开扣分\s*(\d+)`)
	aStockReasonMomentumBonusPattern       = regexp.MustCompile(`动能趋势加分\s*(\d+)(?:（([^）]+)）)?`)
)

func parseAStockRecommendationScoreBreakdown(rec aStockRecommendation) []aStockRecommendationScoreComponent {
	reason := strings.TrimSpace(rec.Reason)
	components := make([]aStockRecommendationScoreComponent, 0, 8)
	if reason == "" {
		if rec.MarketScore != 0 {
			components = append(components, newAStockScoreComponent("保存总分", "历史推荐记录", rec.MarketScore, rec.MarketScore))
		}
		return components
	}
	if matches := aStockReasonHotspotScorePattern.FindStringSubmatch(reason); len(matches) == 4 {
		keywordCount := countAStockDelimitedKeywords(matches[1])
		newsCount := atoiAStockScorePart(matches[2])
		hotspotScore := atoiAStockScorePart(matches[3])
		negativePenalty := 0
		if negativeMatches := aStockReasonNegativePenaltyPattern.FindStringSubmatch(reason); len(negativeMatches) == 3 {
			negativeCount := atoiAStockScorePart(negativeMatches[1])
			negativePenalty = atoiAStockScorePart(negativeMatches[2])
			components = append(components, newAStockScoreComponent("新闻热度", fmt.Sprintf("证据新闻 %d 条", newsCount), 10, newsCount*10))
			components = append(components, newAStockScoreComponent("热点关键词", fmt.Sprintf("命中关键词 %d 个", keywordCount), 3, keywordCount*3))
			components = append(components, newAStockScoreComponent("负面新闻", fmt.Sprintf("负面新闻 %d 条，板块减分 %d", negativeCount, negativePenalty), -negativePenalty, -negativePenalty))
		} else {
			components = append(components, newAStockScoreComponent("新闻热度", fmt.Sprintf("证据新闻 %d 条", newsCount), 10, newsCount*10))
			components = append(components, newAStockScoreComponent("热点关键词", fmt.Sprintf("命中关键词 %d 个", keywordCount), 3, keywordCount*3))
		}
		hotspotSum := 0
		for _, component := range components {
			hotspotSum += component.Score
		}
		if hotspotSum != hotspotScore {
			delta := hotspotScore - hotspotSum
			components = append(components, newAStockScoreComponent("热度修正", fmt.Sprintf("展示热度分 %d", hotspotScore), delta, delta))
		}
	} else if matches := aStockReasonSimpleHotspotScorePattern.FindStringSubmatch(reason); len(matches) == 2 {
		score := atoiAStockScorePart(matches[1])
		components = append(components, newAStockScoreComponent("热点热度", "历史理由热度分", score, score))
	} else if rec.HotspotScore != 0 {
		components = append(components, newAStockScoreComponent("热点热度", "已保存热度分", rec.HotspotScore, rec.HotspotScore))
	}
	if matches := aStockReasonMarketScorePattern.FindStringSubmatch(reason); len(matches) == 4 {
		rank := atoiAStockScorePart(matches[1])
		evidence := atoiAStockScorePart(matches[2])
		matchScore := atoiAStockScorePart(matches[3])
		components = appendAStockParsedMatchComponents(components, reason, rank, evidence, matchScore)
	} else if matches := aStockReasonMatchedScorePattern.FindStringSubmatch(reason); len(matches) == 3 {
		evidence := atoiAStockScorePart(matches[1])
		matchScore := atoiAStockScorePart(matches[2])
		components = appendAStockParsedMatchComponents(components, reason, 0, evidence, matchScore)
	}
	components = appendAStockParsedAdjustment(components, reason, aStockReasonHoldingBonusPattern, "机构持仓", "持仓加分", 1)
	components = appendAStockParsedAdjustment(components, reason, aStockReasonFundBonusPattern, "资金动向", "资金加分", 1)
	components = appendAStockParsedAdjustment(components, reason, aStockReasonFundPenaltyPattern, "资金动向", "资金减分", -1)
	components = appendAStockParsedAdjustment(components, reason, aStockReasonLowOpenPenaltyPattern, "开盘盘口", "盘口减分", -1)
	components = appendAStockParsedAdjustment(components, reason, aStockReasonFundStrengthPenaltyPattern, "资金强度", "资金强度减分", -1)
	components = appendAStockParsedAdjustment(components, reason, aStockReasonSectorDrawdownPattern, "板块回撤", "板块回撤减分", -1)
	components = appendAStockParsedHighOpenBonus(components, reason)
	components = appendAStockParsedLowOpenPenalty(components, reason)
	components = appendAStockParsedMomentumBonus(components, reason)
	return components
}

func appendAStockParsedMatchComponents(components []aStockRecommendationScoreComponent, reason string, rank int, evidence int, matchScore int) []aStockRecommendationScoreComponent {
	rankScore := aStockMarketRankScore(rank)
	if rank > 0 {
		components = append(components, newAStockScoreComponent("行情排名", fmt.Sprintf("排名 %d", rank), rankScore, rankScore))
	}
	weakPenalty := 0
	if matches := aStockReasonWeakPenaltyPattern.FindStringSubmatch(reason); len(matches) == 2 {
		weakPenalty = atoiAStockScorePart(matches[1])
		components = append(components, newAStockScoreComponent("弱证据", "融资融券弱新闻", -weakPenalty, -weakPenalty))
	}
	keywordScore := 0
	if matches := aStockReasonStockKeywordPattern.FindStringSubmatch(reason); len(matches) == 2 {
		keywordCount := countAStockDelimitedKeywords(matches[1])
		keywordScore = keywordCount * 12
		components = append(components, newAStockScoreComponent("股票名命中", fmt.Sprintf("命中关键词 %d 个", keywordCount), 12, keywordScore))
	}
	evidenceScore := evidence * 25
	evidenceUnitValue := 25
	if weakPenalty > 0 && evidence == 1 {
		evidenceScore = 0
		evidenceUnitValue = 0
	}
	components = append(components, newAStockScoreComponent("个股证据", fmt.Sprintf("个股证据 %d 条", evidence), evidenceUnitValue, evidenceScore))
	_ = matchScore
	return components
}

func appendAStockParsedAdjustment(components []aStockRecommendationScoreComponent, reason string, pattern *regexp.Regexp, label string, detailPrefix string, sign int) []aStockRecommendationScoreComponent {
	matches := pattern.FindAllStringSubmatch(reason, -1)
	for _, match := range matches {
		if len(match) != 2 {
			continue
		}
		value := atoiAStockScorePart(match[1])
		if value == 0 {
			continue
		}
		score := sign * value
		components = append(components, newAStockScoreComponent(label, fmt.Sprintf("%s %d", detailPrefix, value), score, score))
	}
	return components
}

func appendAStockParsedHighOpenBonus(components []aStockRecommendationScoreComponent, reason string) []aStockRecommendationScoreComponent {
	matches := aStockReasonHighOpenBonusPattern.FindAllStringSubmatch(reason, -1)
	for _, match := range matches {
		if len(match) != 3 {
			continue
		}
		value := atoiAStockScorePart(match[2])
		if value == 0 {
			continue
		}
		detail := strings.TrimSpace(match[1])
		if detail == "" {
			detail = fmt.Sprintf("高开加分 %d", value)
		}
		components = append(components, newAStockScoreComponent("当日高开", detail, value, value))
	}
	return components
}

func appendAStockParsedLowOpenPenalty(components []aStockRecommendationScoreComponent, reason string) []aStockRecommendationScoreComponent {
	matches := aStockReasonLowOpenTierPenaltyPattern.FindAllStringSubmatch(reason, -1)
	for _, match := range matches {
		if len(match) != 3 {
			continue
		}
		value := atoiAStockScorePart(match[2])
		if value == 0 {
			continue
		}
		detail := strings.TrimSpace(match[1])
		if detail == "" {
			detail = fmt.Sprintf("低开扣分 %d", value)
		}
		components = append(components, newAStockScoreComponent("当日低开", detail, -value, -value))
	}
	return components
}

func appendAStockParsedMomentumBonus(components []aStockRecommendationScoreComponent, reason string) []aStockRecommendationScoreComponent {
	matches := aStockReasonMomentumBonusPattern.FindAllStringSubmatch(reason, -1)
	for _, match := range matches {
		if len(match) != 3 {
			continue
		}
		value := atoiAStockScorePart(match[1])
		if value == 0 {
			continue
		}
		detail := strings.TrimSpace(match[2])
		if detail == "" {
			detail = fmt.Sprintf("动能趋势加分 %d", value)
		}
		components = append(components, newAStockScoreComponent("动能趋势", detail, value, value))
	}
	return components
}

func countAStockDelimitedKeywords(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return r == '、' || r == ',' || r == '，' || r == '/' || r == ' '
	})
	count := 0
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			count++
		}
	}
	return count
}

func atoiAStockScorePart(text string) int {
	value, _ := strconv.Atoi(strings.TrimSpace(text))
	return value
}

func applyAStockRecommendationScorePenalty(rec aStockRecommendation, penalty int, reason string) aStockRecommendation {
	if penalty <= 0 {
		return rec
	}
	baseScore := rec.MarketScore
	if baseScore == 0 {
		baseScore = rec.HotspotScore
	}
	rec.MarketScore = baseScore - penalty
	appendAStockScoreAdjustment(&rec, "扣分调整", reason, -penalty)
	rec.Reason = appendAStockReason(rec.Reason, reason)
	return rec
}

func capAStockOverheatedFundFlowScore(rec aStockRecommendation, change30 float64) aStockRecommendation {
	return capAStockOverheatedFundFlowScoreWithSettings(rec, change30, defaultAStockAlgorithmSettings())
}

func capAStockOverheatedFundFlowScoreWithSettings(rec aStockRecommendation, change30 float64, settings model.AStockRecommendationAlgorithmSettings) aStockRecommendation {
	fundBonus := positiveAStockFundFlowScore(rec)
	if fundBonus <= settings.Fund.Overheat30Cap {
		return rec
	}
	penalty := fundBonus - settings.Fund.Overheat30Cap
	reason := fmt.Sprintf("30日涨幅超过%s，资金加分上限%d，过热减分 %d", formatAStockPct(settings.Volatility.Overheat30ThresholdPct), settings.Fund.Overheat30Cap, penalty)
	applyAStockScoreDeltaWithSettings(&rec, settings, aStockScoreFactorFund, "资金强度", reason, -penalty)
	rec.Reason = appendAStockReason(rec.Reason, reason)
	return rec
}

func capAStockOverheatedSectorTopStockResonanceScore(rec aStockRecommendation, change30 float64) aStockRecommendation {
	return capAStockOverheatedSectorTopStockResonanceScoreWithSettings(rec, change30, defaultAStockAlgorithmSettings())
}

func capAStockOverheatedSectorTopStockResonanceScoreWithSettings(rec aStockRecommendation, change30 float64, settings model.AStockRecommendationAlgorithmSettings) aStockRecommendation {
	resonanceBonus := positiveAStockSectorTopStockResonanceScore(rec)
	if resonanceBonus <= settings.Sector.TopStockOverheat30Cap {
		return rec
	}
	penalty := resonanceBonus - settings.Sector.TopStockOverheat30Cap
	reason := fmt.Sprintf("30日涨幅超过%s，板块共振加分上限%d，过热减分 %d", formatAStockPct(settings.Volatility.Overheat30ThresholdPct), settings.Sector.TopStockOverheat30Cap, penalty)
	applyAStockScoreDeltaWithSettings(&rec, settings, aStockScoreFactorSector, "板块资金共振", reason, -penalty)
	rec.Reason = appendAStockReason(rec.Reason, reason)
	return rec
}

func applyAStockPreviousLimitUpPenalty(rec aStockRecommendation, prevPct float64) aStockRecommendation {
	return applyAStockPreviousLimitUpPenaltyWithSettings(rec, prevPct, defaultAStockAlgorithmSettings())
}

func applyAStockPreviousLimitUpPenaltyWithSettings(rec aStockRecommendation, prevPct float64, settings model.AStockRecommendationAlgorithmSettings) aStockRecommendation {
	detail := fmt.Sprintf("昨日涨停%s，风险扣分 %d", formatAStockPct(prevPct), settings.Volatility.PreviousLimitUpPenalty)
	applyAStockScoreDeltaWithSettings(&rec, settings, aStockScoreFactorVolatility, "昨日涨停", detail, -settings.Volatility.PreviousLimitUpPenalty)
	rec.Reason = appendAStockReason(rec.Reason, detail)
	return rec
}

func applyAStockPreviousHighPctPenalty(rec aStockRecommendation, prevPct float64) aStockRecommendation {
	return applyAStockPreviousHighPctPenaltyWithSettings(rec, prevPct, defaultAStockAlgorithmSettings())
}

func applyAStockPreviousHighPctPenaltyWithSettings(rec aStockRecommendation, prevPct float64, settings model.AStockRecommendationAlgorithmSettings) aStockRecommendation {
	detail := fmt.Sprintf("昨日涨幅%s，追高风险扣分 %d", formatAStockPct(prevPct), settings.Volatility.PreviousHighPctPenalty)
	applyAStockScoreDeltaWithSettings(&rec, settings, aStockScoreFactorVolatility, "昨日涨幅过高", detail, -settings.Volatility.PreviousHighPctPenalty)
	rec.Reason = appendAStockReason(rec.Reason, detail)
	return rec
}

func applyAStockHighOpenScore(rec aStockRecommendation, period string, entry aStockMarketBar, prev aStockMarketBar, hasPrev bool) aStockRecommendation {
	return applyAStockHighOpenScoreWithSettings(rec, period, entry, prev, hasPrev, defaultAStockAlgorithmSettings())
}

func applyAStockHighOpenScoreWithSettings(rec aStockRecommendation, period string, entry aStockMarketBar, prev aStockMarketBar, hasPrev bool, settings model.AStockRecommendationAlgorithmSettings) aStockRecommendation {
	entryPrice, entryLabel, basePrice, baseLabel, ok := aStockOpenScoreContext(period, entry, prev, hasPrev, rec)
	if !ok {
		return rec
	}
	openPct := (entryPrice/basePrice - 1) * 100
	score := aStockHighOpenScoreWithSettings(openPct, settings)
	if score <= 0 {
		return rec
	}
	detail := fmt.Sprintf("%s 较 %s 高开 %s", entryLabel, baseLabel, formatAStockPct(openPct))
	applyAStockScoreDeltaWithSettings(&rec, settings, aStockScoreFactorAuction, "当日高开", detail, score)
	rec.Reason = appendAStockReason(rec.Reason, fmt.Sprintf("%s，高开加分 %d", detail, score))
	return rec
}

func applyAStockLowOpenPenalty(rec aStockRecommendation, period string, entry aStockMarketBar, prev aStockMarketBar, hasPrev bool) aStockRecommendation {
	return applyAStockLowOpenPenaltyWithSettings(rec, period, entry, prev, hasPrev, defaultAStockAlgorithmSettings())
}

func applyAStockLowOpenPenaltyWithSettings(rec aStockRecommendation, period string, entry aStockMarketBar, prev aStockMarketBar, hasPrev bool, settings model.AStockRecommendationAlgorithmSettings) aStockRecommendation {
	entryPrice, entryLabel, basePrice, baseLabel, ok := aStockOpenScoreContext(period, entry, prev, hasPrev, rec)
	if !ok {
		return rec
	}
	openPct := (entryPrice/basePrice - 1) * 100
	penalty := aStockLowOpenPenaltyScoreWithSettings(openPct, settings)
	if penalty <= 0 {
		return rec
	}
	detail := fmt.Sprintf("%s 较 %s 低开 %s", entryLabel, baseLabel, formatAStockPct(openPct))
	applyAStockScoreDeltaWithSettings(&rec, settings, aStockScoreFactorAuction, "当日低开", detail, -penalty)
	rec.Reason = appendAStockReason(rec.Reason, fmt.Sprintf("%s，低开扣分 %d", detail, penalty))
	return rec
}

func aStockOpenScoreContext(period string, entry aStockMarketBar, prev aStockMarketBar, hasPrev bool, rec aStockRecommendation) (float64, string, float64, string, bool) {
	normalizedPeriod := normalizeAStockPeriod(period).Key
	entryPrice := aStockEntryPriceForRecommendation(entry, normalizedPeriod, rec)
	if entryPrice <= 0 {
		return 0, "", 0, "", false
	}
	entryLabel := aStockDefaultRecommendationEntryTime(normalizedPeriod)
	basePrice := 0.0
	baseLabel := ""
	if normalizedPeriod == "afternoon" {
		entryLabel = aStockRecommendationEffectiveEntryTime(rec, "afternoon")
		if entryLabel != "13:01" {
			return 0, "", 0, "", false
		}
		basePrice, baseLabel = aStockAfternoonHighOpenBasePrice(entry)
	} else if hasPrev && prev.Close > 0 {
		entryLabel = "09:30"
		basePrice = prev.Close
		baseLabel = "昨日收盘"
	}
	if basePrice <= 0 || baseLabel == "" {
		return 0, "", 0, "", false
	}
	return entryPrice, entryLabel, basePrice, baseLabel, true
}

func aStockAfternoonHighOpenBasePrice(entry aStockMarketBar) (float64, string) {
	if entry.SessionPrices == nil {
		return 0, ""
	}
	for _, session := range []string{"12:30", "11:30"} {
		if price := entry.SessionPrices[session]; price > 0 {
			return price, session
		}
	}
	return 0, ""
}

func aStockHighOpenScore(openPct float64) int {
	return aStockHighOpenScoreWithSettings(openPct, defaultAStockAlgorithmSettings())
}

func aStockHighOpenScoreWithSettings(openPct float64, settings model.AStockRecommendationAlgorithmSettings) int {
	switch {
	case openPct >= settings.Auction.HighOpenStrongThresholdPct:
		return settings.Auction.HighOpenStrongScore
	case openPct >= settings.Auction.HighOpenThreshold5Pct:
		return settings.Auction.HighOpenScore5
	case openPct >= settings.Auction.HighOpenThreshold4Pct:
		return settings.Auction.HighOpenScore4
	case openPct >= settings.Auction.HighOpenThreshold3Pct:
		return settings.Auction.HighOpenScore3
	case openPct >= settings.Auction.HighOpenThreshold2Pct:
		return settings.Auction.HighOpenScore2
	case openPct >= settings.Auction.HighOpenThreshold1Pct:
		return settings.Auction.HighOpenScore1
	default:
		return 0
	}
}

func aStockLowOpenPenaltyScore(openPct float64) int {
	return aStockLowOpenPenaltyScoreWithSettings(openPct, defaultAStockAlgorithmSettings())
}

func aStockLowOpenPenaltyScoreWithSettings(openPct float64, settings model.AStockRecommendationAlgorithmSettings) int {
	dropPct := -openPct
	switch {
	case dropPct >= settings.Auction.LowOpenStrongThresholdPct:
		return settings.Auction.LowOpenStrongPenalty
	case dropPct >= settings.Auction.LowOpenThreshold5Pct:
		return settings.Auction.LowOpenPenalty5
	case dropPct >= settings.Auction.LowOpenThreshold4Pct:
		return settings.Auction.LowOpenPenalty4
	case dropPct >= settings.Auction.LowOpenThreshold3Pct:
		return settings.Auction.LowOpenPenalty3
	case dropPct >= settings.Auction.LowOpenThreshold2Pct:
		return settings.Auction.LowOpenPenalty2
	case dropPct >= settings.Auction.LowOpenThreshold1Pct:
		return settings.Auction.LowOpenPenalty1
	default:
		return 0
	}
}

type aStockMomentumSignal struct {
	Name  string
	Score int
}

func applyAStockMomentumTrendScore(rec aStockRecommendation, bars []aStockMarketBar, strategyDate string) aStockRecommendation {
	return applyAStockMomentumTrendScoreWithSettings(rec, bars, strategyDate, defaultAStockAlgorithmSettings())
}

func applyAStockMomentumTrendScoreWithSettings(rec aStockRecommendation, bars []aStockMarketBar, strategyDate string, settings model.AStockRecommendationAlgorithmSettings) aStockRecommendation {
	score, signals := aStockMomentumTrendScoreWithSettings(bars, strategyDate, settings)
	if score <= 0 || len(signals) == 0 {
		return rec
	}
	details := make([]string, 0, len(signals)+1)
	names := make([]string, 0, len(signals))
	for _, signal := range signals {
		details = append(details, fmt.Sprintf("%s %d", signal.Name, signal.Score))
		names = append(names, signal.Name)
	}
	if rawScore := aStockMomentumSignalScore(signals); rawScore > score {
		details = append(details, fmt.Sprintf("上限 %d", score))
	}
	detail := strings.Join(details, "；")
	applyAStockScoreDeltaWithSettings(&rec, settings, aStockScoreFactorVolatility, "动能趋势", detail, score)
	rec.Reason = appendAStockReason(rec.Reason, fmt.Sprintf("动能趋势加分 %d（%s）", score, strings.Join(names, " + ")))
	return rec
}

func aStockMomentumTrendScore(bars []aStockMarketBar, strategyDate string) (int, []aStockMomentumSignal) {
	return aStockMomentumTrendScoreWithSettings(bars, strategyDate, defaultAStockAlgorithmSettings())
}

func aStockMomentumTrendScoreWithSettings(bars []aStockMarketBar, strategyDate string, settings model.AStockRecommendationAlgorithmSettings) (int, []aStockMomentumSignal) {
	usable := aStockMomentumUsableBars(bars, strategyDate)
	if len(usable) < settings.Volatility.MomentumMinBars || usable[len(usable)-1].Date != normalizeAStockStrategyDate(strategyDate) {
		return 0, nil
	}
	signals := make([]aStockMomentumSignal, 0, 5)
	if aStockMomentumADXStarted(usable) {
		signals = append(signals, aStockMomentumSignal{Name: "ADX转强", Score: settings.Volatility.MomentumADXScore})
	}
	if aStockMomentumBollingerBreakout(usable) {
		signals = append(signals, aStockMomentumSignal{Name: "布林突破", Score: settings.Volatility.MomentumBollingerScore})
	}
	if aStockMomentumMACDStrengthened(usable) {
		signals = append(signals, aStockMomentumSignal{Name: "MACD转强", Score: settings.Volatility.MomentumMACDScore})
	}
	if aStockMomentumMAConfirmed(usable) {
		signals = append(signals, aStockMomentumSignal{Name: "均线趋势", Score: settings.Volatility.MomentumMAScore})
	}
	if aStockMomentumVolumeConfirmed(usable) {
		signals = append(signals, aStockMomentumSignal{Name: "放量确认", Score: settings.Volatility.MomentumVolumeScore})
	}
	score := aStockMomentumSignalScore(signals)
	if score > settings.Volatility.MomentumMaxScore {
		score = settings.Volatility.MomentumMaxScore
	}
	return score, signals
}

func aStockMomentumSignalScore(signals []aStockMomentumSignal) int {
	score := 0
	for _, signal := range signals {
		score += signal.Score
	}
	return score
}

func aStockMomentumUsableBars(bars []aStockMarketBar, strategyDate string) []aStockMarketBar {
	date := normalizeAStockStrategyDate(strategyDate)
	usable := make([]aStockMarketBar, 0, len(bars))
	for _, bar := range bars {
		bar.Date = normalizeAStockMarketDate(bar.Date)
		if bar.Date == "" || bar.Date > date || bar.Close <= 0 {
			continue
		}
		if bar.Open <= 0 {
			bar.Open = bar.Close
		}
		if bar.High <= 0 {
			bar.High = maxFloat(bar.Open, bar.Close)
		} else {
			bar.High = maxFloat(bar.High, bar.Open, bar.Close)
		}
		if bar.Low <= 0 {
			bar.Low = minPositiveFloat(bar.Open, bar.Close)
		} else {
			bar.Low = minPositiveFloat(bar.Low, bar.Open, bar.Close)
		}
		if bar.Amount <= 0 && bar.Volume > 0 {
			bar.Amount = bar.Volume * bar.Close
		}
		usable = append(usable, bar)
	}
	sort.SliceStable(usable, func(i, j int) bool {
		return usable[i].Date < usable[j].Date
	})
	return usable
}

func aStockMomentumADXStarted(bars []aStockMarketBar) bool {
	adx, prevADX, plusDI, minusDI, ok := aStockADX(bars, 14)
	if !ok || plusDI <= minusDI {
		return false
	}
	return adx >= 20 || (prevADX < 20 && adx >= 18)
}

func aStockADX(bars []aStockMarketBar, period int) (float64, float64, float64, float64, bool) {
	if period <= 0 || len(bars) < period*2+1 {
		return 0, 0, 0, 0, false
	}
	trs := make([]float64, len(bars))
	plusDMs := make([]float64, len(bars))
	minusDMs := make([]float64, len(bars))
	for i := 1; i < len(bars); i++ {
		high := bars[i].High
		low := bars[i].Low
		prevHigh := bars[i-1].High
		prevLow := bars[i-1].Low
		prevClose := bars[i-1].Close
		trs[i] = maxFloat(high-low, math.Abs(high-prevClose), math.Abs(low-prevClose))
		upMove := high - prevHigh
		downMove := prevLow - low
		if upMove > downMove && upMove > 0 {
			plusDMs[i] = upMove
		}
		if downMove > upMove && downMove > 0 {
			minusDMs[i] = downMove
		}
	}
	smoothedTR := 0.0
	smoothedPlusDM := 0.0
	smoothedMinusDM := 0.0
	for i := 1; i <= period; i++ {
		smoothedTR += trs[i]
		smoothedPlusDM += plusDMs[i]
		smoothedMinusDM += minusDMs[i]
	}
	dxs := make([]float64, 0, len(bars)-period)
	plusDI, minusDI, dx := aStockDMIValues(smoothedTR, smoothedPlusDM, smoothedMinusDM)
	dxs = append(dxs, dx)
	adx := 0.0
	prevADX := 0.0
	for i := period + 1; i < len(bars); i++ {
		smoothedTR = smoothedTR - smoothedTR/float64(period) + trs[i]
		smoothedPlusDM = smoothedPlusDM - smoothedPlusDM/float64(period) + plusDMs[i]
		smoothedMinusDM = smoothedMinusDM - smoothedMinusDM/float64(period) + minusDMs[i]
		plusDI, minusDI, dx = aStockDMIValues(smoothedTR, smoothedPlusDM, smoothedMinusDM)
		dxs = append(dxs, dx)
		switch {
		case len(dxs) == period:
			adx = averageFloat64(dxs)
			prevADX = adx
		case len(dxs) > period:
			prevADX = adx
			adx = (adx*float64(period-1) + dx) / float64(period)
		}
	}
	if len(dxs) < period || smoothedTR <= 0 {
		return 0, 0, 0, 0, false
	}
	return adx, prevADX, plusDI, minusDI, true
}

func aStockDMIValues(smoothedTR float64, smoothedPlusDM float64, smoothedMinusDM float64) (float64, float64, float64) {
	if smoothedTR <= 0 {
		return 0, 0, 0
	}
	plusDI := 100 * smoothedPlusDM / smoothedTR
	minusDI := 100 * smoothedMinusDM / smoothedTR
	denominator := plusDI + minusDI
	if denominator <= 0 {
		return plusDI, minusDI, 0
	}
	return plusDI, minusDI, 100 * math.Abs(plusDI-minusDI) / denominator
}

func aStockMomentumBollingerBreakout(bars []aStockMarketBar) bool {
	if len(bars) < aStockMomentumMinBars {
		return false
	}
	n := len(bars)
	prevEnd := n - 1
	prevMA, prevStd, ok := aStockCloseMeanStd(bars, prevEnd-20, prevEnd)
	if !ok || prevMA <= 0 {
		return false
	}
	prevUpper := prevMA + 2*prevStd
	prevHigh := aStockHighestHigh(bars, prevEnd-20, prevEnd)
	lastClose := bars[n-1].Close
	if lastClose <= prevUpper && lastClose <= prevHigh {
		return false
	}
	bandwidths := make([]float64, 0, 60)
	startEnd := prevEnd - 59
	if startEnd < 20 {
		startEnd = 20
	}
	for end := startEnd; end <= prevEnd; end++ {
		ma, std, ok := aStockCloseMeanStd(bars, end-20, end)
		if !ok || ma <= 0 {
			continue
		}
		bandwidths = append(bandwidths, 4*std/ma)
	}
	if len(bandwidths) == 0 {
		return false
	}
	prevBandwidth := 4 * prevStd / prevMA
	minBandwidth := minFloat(bandwidths...)
	avgBandwidth := averageFloat64(bandwidths)
	return prevBandwidth <= minBandwidth*1.25 || prevBandwidth <= avgBandwidth*0.75
}

func aStockMomentumMACDStrengthened(bars []aStockMarketBar) bool {
	if len(bars) < aStockMomentumMinBars {
		return false
	}
	closes := aStockCloseValues(bars)
	dif, dea, ok := aStockMACDValues(closes, 12, 26, 9)
	if !ok || len(dif) < 2 || len(dea) < 2 {
		return false
	}
	last := len(dif) - 1
	prevHist := dif[last-1] - dea[last-1]
	hist := dif[last] - dea[last]
	return dif[last] > dea[last] && (dif[last-1] <= dea[last-1] || (prevHist <= 0 && hist > 0))
}

func aStockMACDValues(values []float64, fast int, slow int, signal int) ([]float64, []float64, bool) {
	if len(values) < slow+signal || fast <= 0 || slow <= fast || signal <= 0 {
		return nil, nil, false
	}
	fastEMA := aStockEMAValues(values, fast)
	slowEMA := aStockEMAValues(values, slow)
	dif := make([]float64, len(values))
	for i := range values {
		dif[i] = fastEMA[i] - slowEMA[i]
	}
	dea := aStockEMAValues(dif, signal)
	return dif, dea, true
}

func aStockEMAValues(values []float64, period int) []float64 {
	result := make([]float64, len(values))
	if len(values) == 0 || period <= 0 {
		return result
	}
	alpha := 2.0 / float64(period+1)
	result[0] = values[0]
	for i := 1; i < len(values); i++ {
		result[i] = alpha*values[i] + (1-alpha)*result[i-1]
	}
	return result
}

func aStockMomentumMAConfirmed(bars []aStockMarketBar) bool {
	if len(bars) < aStockMomentumMinBars {
		return false
	}
	n := len(bars)
	ma5, ok5 := aStockCloseAverage(bars, n-5, n)
	ma10, ok10 := aStockCloseAverage(bars, n-10, n)
	ma20, ok20 := aStockCloseAverage(bars, n-20, n)
	prevMA20, okPrev := aStockCloseAverage(bars, n-21, n-1)
	if !ok5 || !ok10 || !ok20 || !okPrev || bars[n-1].Close <= ma20 {
		return false
	}
	return (ma5 > ma10 && ma10 > ma20) || ma20 > prevMA20
}

func aStockMomentumVolumeConfirmed(bars []aStockMarketBar) bool {
	if len(bars) < aStockMomentumMinBars {
		return false
	}
	n := len(bars)
	lastAmount := aStockBarAmount(bars[n-1])
	if lastAmount <= 0 {
		return false
	}
	total := 0.0
	count := 0
	for i := n - 21; i < n-1; i++ {
		amount := aStockBarAmount(bars[i])
		if amount <= 0 {
			continue
		}
		total += amount
		count++
	}
	if count == 0 {
		return false
	}
	return lastAmount > total/float64(count)*1.5
}

func aStockBarAmount(bar aStockMarketBar) float64 {
	if bar.Amount > 0 {
		return bar.Amount
	}
	if bar.Volume > 0 && bar.Close > 0 {
		return bar.Volume * bar.Close
	}
	return 0
}

func aStockCloseValues(bars []aStockMarketBar) []float64 {
	values := make([]float64, 0, len(bars))
	for _, bar := range bars {
		if bar.Close > 0 {
			values = append(values, bar.Close)
		}
	}
	return values
}

func aStockCloseAverage(bars []aStockMarketBar, start int, end int) (float64, bool) {
	if start < 0 || end > len(bars) || start >= end {
		return 0, false
	}
	total := 0.0
	for i := start; i < end; i++ {
		if bars[i].Close <= 0 {
			return 0, false
		}
		total += bars[i].Close
	}
	return total / float64(end-start), true
}

func aStockCloseMeanStd(bars []aStockMarketBar, start int, end int) (float64, float64, bool) {
	mean, ok := aStockCloseAverage(bars, start, end)
	if !ok {
		return 0, 0, false
	}
	variance := 0.0
	for i := start; i < end; i++ {
		diff := bars[i].Close - mean
		variance += diff * diff
	}
	return mean, math.Sqrt(variance / float64(end-start)), true
}

func aStockHighestHigh(bars []aStockMarketBar, start int, end int) float64 {
	highest := 0.0
	for i := start; i < end && i < len(bars); i++ {
		if i < 0 {
			continue
		}
		value := bars[i].High
		if value <= 0 {
			value = bars[i].Close
		}
		if value > highest {
			highest = value
		}
	}
	return highest
}

func positiveAStockFundFlowScore(rec aStockRecommendation) int {
	score := 0
	for _, component := range aStockRecommendationScoreBreakdown(rec) {
		if component.Label != "资金动向" || component.Score <= 0 {
			continue
		}
		score += component.Score
	}
	return score
}

func positiveAStockSectorTopStockResonanceScore(rec aStockRecommendation) int {
	score := 0
	for _, component := range aStockRecommendationScoreBreakdown(rec) {
		if component.Label != "板块资金共振" || component.Score <= 0 {
			continue
		}
		score += component.Score
	}
	return score
}

func rerankAStockRecommendations(recommendations []aStockRecommendation) []aStockRecommendation {
	for i := range recommendations {
		recommendations[i].Rank = i + 1
	}
	return recommendations
}

func isFreshAStockLimitUpReplacementSnapshot(snapshot model.AStockRecommendationSnapshot, recommendations []aStockRecommendation) bool {
	if !snapshot.LimitUpFilterEnabled || snapshot.LimitUpFiltered == 0 {
		return true
	}
	status := strings.TrimSpace(snapshot.BacktestStatus)
	if !strings.Contains(status, "过滤涨停股票") && !strings.Contains(status, "涨停过滤后无推荐股票") {
		return true
	}
	if strings.Contains(status, "已按热度递补") || strings.Contains(status, "候选不足未补满") {
		return true
	}
	return len(recommendations) >= snapshot.GeneratedCount
}

func isFreshAStockBacktestSnapshot(period string, backtests []aStockBacktestRow) bool {
	normalizedPeriod := normalizeAStockPeriod(period).Key
	for _, row := range backtests {
		if strings.TrimSpace(row.Status) != "" && aStockBacktestStatusNeedsRestore(row.Status) {
			return false
		}
		t0Return := strings.TrimSpace(row.T0Return)
		t0Close := strings.TrimSpace(row.T0Close)
		if t0Return != "" && t0Return != "--" && (t0Close == "" || t0Close == "--") {
			return false
		}
		if normalizedPeriod == "afternoon" {
			morningOpen := strings.TrimSpace(row.EntryOpen)
			afternoonOpen := strings.TrimSpace(row.AfternoonOpen)
			if morningOpen != "" && morningOpen != "--" && (afternoonOpen == "" || afternoonOpen == "--") {
				return false
			}
		}
		if normalizedPeriod == "morning" && aStockBacktestValueMissing(row.EntryOpen) {
			return false
		}
		if normalizedPeriod == "afternoon" && aStockBacktestValueMissing(row.AfternoonOpen) {
			return false
		}
	}
	return true
}

func (s *Server) loadAStockMarketView(strategyDate string, period string, recommendations []aStockRecommendation, filterLimitUp bool, filterTodayMarket bool, maxRecommendations int) ([]aStockRecommendation, []aStockBacktestRow, string, int, int) {
	result := s.loadAStockMarketViewDetailed(strategyDate, period, recommendations, filterLimitUp, filterTodayMarket, maxRecommendations)
	return result.Recommendations, result.Backtests, result.Status, result.LimitUpFiltered, result.NoTodayMarketCount
}

func (s *Server) loadAStockMarketViewDetailed(strategyDate string, period string, recommendations []aStockRecommendation, filterLimitUp bool, filterTodayMarket bool, maxRecommendations int) aStockMarketViewResult {
	recommendations = initializeAStockRecommendationMarket(recommendations)
	if len(recommendations) == 0 {
		return aStockMarketViewResult{Recommendations: recommendations, Status: "无推荐股票"}
	}
	endpoint := aStockMarketEndpoint()
	codes := make([]string, 0, len(recommendations))
	for _, rec := range recommendations {
		codes = append(codes, rec.Code)
	}
	bars, err := s.loadAStockMarketBars(strategyDate, codes, endpoint)
	if err != nil {
		if maxRecommendations > 0 && len(recommendations) > maxRecommendations {
			recommendations, _ = limitAStockRecommendationsByCount(recommendations, maxRecommendations)
		}
		return aStockMarketViewResult{
			Recommendations: recommendations,
			Backtests:       buildAStockBacktestRows(strategyDate, period, recommendations, nil),
			Status:          "行情读取失败",
		}
	}
	if normalizeAStockPeriod(period).Key == "afternoon" {
		s.enrichAStockMiddaySessionPrices(strategyDate, codes, bars)
	}
	s.enrichAStockRecommendationEntryPrices(strategyDate, period, recommendations, bars)
	settings := aStockAlgorithmSettingsForRecommendationPeriod(period, s.loadAStockAlgorithmSettings())
	return applyAStockMarketBarsDetailedWithSettings(strategyDate, period, recommendations, bars, filterLimitUp, filterTodayMarket, maxRecommendations, settings)
}

func (s *Server) recoverAStockFilteredRecommendationsAfterClose(ctx *aStockContext, cache *aStockRequestCache) string {
	if ctx == nil || len(ctx.FilteredRecommendations) == 0 || !isAStockRecommendationAfterClose(ctx.Date) {
		return ""
	}
	candidates := recoverableAStockFilteredRecommendations(ctx.FilteredRecommendations)
	if len(candidates) == 0 {
		return ""
	}
	codes := make([]string, 0, len(candidates))
	seenCandidates := make(map[string]struct{})
	for _, item := range candidates {
		code := normalizeAStockCode(item.Recommendation.Code)
		if code == "" {
			continue
		}
		if _, exists := seenCandidates[code]; exists {
			continue
		}
		seenCandidates[code] = struct{}{}
		codes = append(codes, code)
	}
	if len(codes) == 0 {
		return ""
	}
	bars, err := s.loadAStockMarketBars(ctx.Date, codes, aStockMarketEndpoint())
	if err != nil {
		return ""
	}
	byCode := groupAStockMarketBars(bars)
	seen := aStockRecommendationCodeSet(ctx.Recommendations)
	recovered := make([]aStockRecommendation, 0)
	for _, item := range candidates {
		rec := item.Recommendation
		code := normalizeAStockCode(rec.Code)
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		codeBars := byCode[code]
		entry, ok := sameDayAStockBar(codeBars, ctx.Date)
		if !ok {
			continue
		}
		prev, hasPrev := previousAStockBar(codeBars, ctx.Date)
		if !aStockLimitUpOpenedForTrading(code, rec.Name, entry, prev, hasPrev) {
			continue
		}
		rec.Code = code
		if entry.Close > 0 {
			rec.CurrentPrice = formatAStockPrice(entry.Close)
			rec.TodayPct = formatAStockPct(entry.Pct)
			rec.TodayPctClass = aStockPctClass(entry.Pct)
		}
		if hasPrev {
			rec.PrevClose = formatAStockPrice(prev.Close)
			rec.PrevPct = formatAStockPct(prev.Pct)
			rec.PrevPctClass = aStockPctClass(prev.Pct)
		}
		rec.Recovered = true
		rec.RecoveryReason = "收盘确认开板恢复，不占每日5只限制"
		rec.Reason = appendAStockReason(rec.Reason, rec.RecoveryReason)
		rec.Rank = len(ctx.Recommendations) + len(recovered) + 1
		recovered = append(recovered, rec)
		seen[code] = struct{}{}
	}
	if len(recovered) == 0 {
		return ""
	}
	ctx.Recommendations = rerankAStockRecommendations(append(ctx.Recommendations, recovered...))
	ctx.RecoveredCount = countAStockRecoveredRecommendations(ctx.Recommendations)
	return fmt.Sprintf("%s，不占每日5只限制", formatAStockFilterCountWithStocks("收盘开板恢复股票", len(recovered), "", recovered))
}

func recoverableAStockFilteredRecommendations(filtered []aStockFilteredRecommendation) []aStockFilteredRecommendation {
	if len(filtered) == 0 {
		return nil
	}
	recoverable := make([]aStockFilteredRecommendation, 0, len(filtered))
	seen := make(map[string]struct{})
	for _, item := range filtered {
		reason := strings.TrimSpace(item.Reason)
		if reason != aStockFilteredReasonTodayHighPct && reason != aStockFilteredReasonLimitUp {
			continue
		}
		code := normalizeAStockCode(item.Recommendation.Code)
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		item.Recommendation.Code = code
		recoverable = append(recoverable, item)
	}
	return recoverable
}

func isAStockRecommendationAfterClose(strategyDate string) bool {
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	if strategyDate == "" {
		return false
	}
	today := aStockTodayDate()
	if strategyDate < today {
		return true
	}
	if strategyDate > today {
		return false
	}
	now := aStockNow().In(aStockLocation())
	return now.Hour() > 15 || (now.Hour() == 15 && now.Minute() >= 0)
}

func (s *Server) loadAStockLockedMarketView(strategyDate string, period string, recommendations []aStockRecommendation) ([]aStockRecommendation, []aStockBacktestRow, string, int) {
	recommendations = initializeAStockRecommendationMarket(recommendations)
	if len(recommendations) == 0 {
		return recommendations, nil, "无推荐股票", 0
	}
	endpoint := aStockMarketEndpoint()
	codes := make([]string, 0, len(recommendations))
	for _, rec := range recommendations {
		codes = append(codes, rec.Code)
	}
	bars, err := s.loadAStockMarketBars(strategyDate, codes, endpoint)
	if err != nil {
		return recommendations, buildAStockBacktestRows(strategyDate, period, recommendations, nil), "已锁定推荐股票，行情读取失败", 0
	}
	s.enrichAStockRecommendationEntryPrices(strategyDate, period, recommendations, bars)
	return applyAStockLockedMarketBars(strategyDate, period, recommendations, bars)
}

func (s *Server) loadAStockMarketBars(strategyDate string, codes []string, endpoint string) ([]aStockMarketBar, error) {
	if endpoint != "" {
		resp, err := s.client.R().
			SetQueryParam("date", strategyDate).
			SetQueryParam("codes", strings.Join(codes, ",")).
			Get(endpoint)
		if err == nil && resp.IsSuccess() {
			bars, decodeErr := decodeAStockMarketBars(resp.Body())
			if decodeErr == nil && len(bars) > 0 {
				if shouldSupplementAStockMarketBars(strategyDate, codes, bars) {
					if fallbackBars, fallbackErr := s.loadDefaultAStockBars(strategyDate, codes); fallbackErr == nil && len(fallbackBars) > 0 {
						bars = mergeAStockMarketBars(bars, fallbackBars)
					}
				}
				s.enrichAStockSessionPrices(strategyDate, codes, bars)
				return bars, nil
			}
		}
		if bars, fallbackErr := s.loadDefaultAStockBarsWithSessionPrices(strategyDate, codes); fallbackErr == nil && len(bars) > 0 {
			return bars, nil
		}
		if bars := s.realtimeAStockMarketBars(strategyDate, codes); len(bars) > 0 {
			return bars, nil
		}
		if err != nil {
			return nil, err
		}
		if !resp.IsSuccess() {
			return nil, fmt.Errorf("market endpoint status %d", resp.StatusCode())
		}
		return nil, fmt.Errorf("market endpoint returned no bars")
	}
	return s.loadDefaultAStockBarsWithSessionPrices(strategyDate, codes)
}

func (s *Server) loadDefaultAStockBarsWithSessionPrices(strategyDate string, codes []string) ([]aStockMarketBar, error) {
	bars, err := s.loadDefaultAStockBars(strategyDate, codes)
	if err != nil || len(bars) == 0 {
		if quoteBars := s.realtimeAStockMarketBars(strategyDate, codes); len(quoteBars) > 0 {
			return quoteBars, nil
		}
		return bars, err
	}
	s.enrichAStockSessionPrices(strategyDate, codes, bars)
	bars = s.supplementAStockMarketBarsWithRealtimeQuotes(strategyDate, codes, bars)
	return bars, nil
}

func (s *Server) supplementAStockMarketBarsWithRealtimeQuotes(strategyDate string, codes []string, bars []aStockMarketBar) []aStockMarketBar {
	quoteDate := aStockRealtimeQuoteDateForStrategyDate(strategyDate)
	if quoteDate == "" {
		return bars
	}
	quoteBars := s.realtimeAStockMarketBarsForDate(quoteDate, codes)
	if len(quoteBars) == 0 {
		return bars
	}
	return mergeAStockRealtimeMarketBars(bars, quoteBars)
}

func (s *Server) realtimeAStockMarketBars(strategyDate string, codes []string) []aStockMarketBar {
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	return s.realtimeAStockMarketBarsForDate(strategyDate, codes)
}

func (s *Server) realtimeAStockMarketBarsForDate(barDate string, codes []string) []aStockMarketBar {
	barDate = normalizeAStockStrategyDate(barDate)
	if barDate == "" || barDate != aStockTodayDate() || len(codes) == 0 {
		return nil
	}
	quotes := s.loadAStockRealtimeQuotes(codes)
	if len(quotes) == 0 {
		return nil
	}
	bars := make([]aStockMarketBar, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, rawCode := range codes {
		code := normalizeAStockCode(rawCode)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		quote, ok := quotes[code]
		if !ok || quote.Price <= 0 {
			continue
		}
		bar := aStockMarketBar{
			Code:  code,
			Date:  barDate,
			Open:  quote.Open,
			Close: quote.Price,
			Pct:   quote.Pct,
		}
		if quote.Open > 0 {
			bar.EntryPrice = quote.Open
		}
		bars = append(bars, bar)
	}
	return bars
}

func aStockRealtimeQuoteDateForStrategyDate(strategyDate string) string {
	return aStockRealtimeQuoteDateForStrategyDateWithMaxAge(strategyDate, 10*24*time.Hour)
}

func aStockRealtimeQuoteDateForBacktestCurrentReturn(strategyDate string) string {
	return aStockRealtimeQuoteDateForStrategyDateWithMaxAge(strategyDate, 0)
}

func aStockRealtimeQuoteDateForStrategyDateWithMaxAge(strategyDate string, maxAge time.Duration) string {
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	today := aStockTodayDate()
	if strategyDate == "" || today == "" {
		return ""
	}
	strategyDay, err := time.ParseInLocation("2006-01-02", strategyDate, aStockLocation())
	if err != nil {
		return ""
	}
	todayDay, err := time.ParseInLocation("2006-01-02", today, aStockLocation())
	if err != nil || todayDay.Before(strategyDay) {
		return ""
	}
	if maxAge > 0 && todayDay.Sub(strategyDay) > maxAge {
		return ""
	}
	return today
}

func shouldSupplementAStockMarketBars(strategyDate string, codes []string, bars []aStockMarketBar) bool {
	if len(codes) == 0 || len(bars) == 0 {
		return false
	}
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	grouped := groupAStockMarketBars(bars)
	for _, code := range codes {
		code = normalizeAStockCode(code)
		codeBars := grouped[code]
		if len(codeBars) == 0 {
			return true
		}
		hasStrategyDate := false
		for _, bar := range codeBars {
			if bar.Date == strategyDate {
				hasStrategyDate = true
				break
			}
		}
		if !hasStrategyDate {
			return true
		}
	}
	return false
}

func mergeAStockMarketBars(primary []aStockMarketBar, fallback []aStockMarketBar) []aStockMarketBar {
	if len(primary) == 0 {
		return append([]aStockMarketBar(nil), fallback...)
	}
	if len(fallback) == 0 {
		return primary
	}
	merged := append([]aStockMarketBar(nil), primary...)
	indexByKey := make(map[string]int, len(merged))
	for i, bar := range merged {
		key := normalizeAStockCode(bar.Code) + "|" + normalizeAStockMarketDate(bar.Date)
		if key != "|" {
			indexByKey[key] = i
		}
	}
	for _, bar := range fallback {
		key := normalizeAStockCode(bar.Code) + "|" + normalizeAStockMarketDate(bar.Date)
		if key == "|" {
			continue
		}
		if idx, ok := indexByKey[key]; ok {
			merged[idx] = mergeAStockMarketBar(merged[idx], bar)
			continue
		}
		indexByKey[key] = len(merged)
		merged = append(merged, bar)
	}
	return merged
}

func mergeAStockRealtimeMarketBars(primary []aStockMarketBar, realtime []aStockMarketBar) []aStockMarketBar {
	if len(primary) == 0 {
		return append([]aStockMarketBar(nil), realtime...)
	}
	if len(realtime) == 0 {
		return primary
	}
	merged := append([]aStockMarketBar(nil), primary...)
	indexByKey := make(map[string]int, len(merged))
	for i, bar := range merged {
		key := normalizeAStockCode(bar.Code) + "|" + normalizeAStockMarketDate(bar.Date)
		if key != "|" {
			indexByKey[key] = i
		}
	}
	for _, bar := range realtime {
		key := normalizeAStockCode(bar.Code) + "|" + normalizeAStockMarketDate(bar.Date)
		if key == "|" {
			continue
		}
		if idx, ok := indexByKey[key]; ok {
			merged[idx] = mergeAStockRealtimeMarketBar(merged[idx], bar)
			continue
		}
		indexByKey[key] = len(merged)
		merged = append(merged, bar)
	}
	return merged
}

func mergeAStockRealtimeMarketBar(primary aStockMarketBar, realtime aStockMarketBar) aStockMarketBar {
	merged := mergeAStockMarketBar(primary, realtime)
	if realtime.Open > 0 {
		merged.Open = realtime.Open
		if merged.EntryPrice <= 0 {
			merged.EntryPrice = realtime.Open
		}
	}
	if realtime.Close > 0 {
		merged.Close = realtime.Close
	}
	merged.Pct = realtime.Pct
	return merged
}

func mergeAStockMarketBar(primary aStockMarketBar, fallback aStockMarketBar) aStockMarketBar {
	if primary.Code == "" {
		primary.Code = fallback.Code
	}
	if primary.Date == "" {
		primary.Date = fallback.Date
	}
	if primary.Open <= 0 && fallback.Open > 0 {
		primary.Open = fallback.Open
	}
	if primary.Close <= 0 && fallback.Close > 0 {
		primary.Close = fallback.Close
	}
	if primary.Pct == 0 && fallback.Pct != 0 {
		primary.Pct = fallback.Pct
	}
	if primary.EntryPrice <= 0 && fallback.EntryPrice > 0 {
		primary.EntryPrice = fallback.EntryPrice
	}
	if primary.AfternoonEntryPrice <= 0 && fallback.AfternoonEntryPrice > 0 {
		primary.AfternoonEntryPrice = fallback.AfternoonEntryPrice
	}
	for entryTime, price := range fallback.SessionPrices {
		if price <= 0 {
			continue
		}
		if primary.SessionPrices == nil {
			primary.SessionPrices = make(map[string]float64)
		}
		if primary.SessionPrices[entryTime] <= 0 {
			primary.SessionPrices[entryTime] = price
		}
	}
	return primary
}

func (s *Server) enrichAStockSessionPrices(strategyDate string, codes []string, bars []aStockMarketBar) {
	if len(bars) == 0 {
		return
	}
	need0930 := false
	need1300 := false
	for _, bar := range bars {
		if bar.Date != strategyDate {
			continue
		}
		if bar.EntryPrice <= 0 {
			need0930 = true
		}
		if bar.AfternoonEntryPrice <= 0 {
			need1300 = true
		}
		if need0930 && need1300 {
			break
		}
	}
	if need0930 {
		entryPrices := s.loadEastmoneyAStock0930Prices(strategyDate, codes)
		for i := range bars {
			if bars[i].Date != strategyDate || bars[i].EntryPrice > 0 {
				continue
			}
			if price := entryPrices[normalizeAStockCode(bars[i].Code)]; price > 0 {
				bars[i].EntryPrice = price
			}
		}
	}
	if need1300 {
		afternoonEntryPrices := s.loadEastmoneyAStock1300Prices(strategyDate, codes)
		for i := range bars {
			if bars[i].Date != strategyDate || bars[i].AfternoonEntryPrice > 0 {
				continue
			}
			if price := afternoonEntryPrices[normalizeAStockCode(bars[i].Code)]; price > 0 {
				bars[i].AfternoonEntryPrice = price
			}
		}
	}
}

func (s *Server) enrichAStockMiddaySessionPrices(strategyDate string, codes []string, bars []aStockMarketBar) {
	if len(bars) == 0 || len(codes) == 0 {
		return
	}
	missingCodes := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for i := range bars {
		if bars[i].Date != strategyDate {
			continue
		}
		code := normalizeAStockCode(bars[i].Code)
		if code == "" {
			continue
		}
		if bars[i].SessionPrices != nil && (bars[i].SessionPrices["12:30"] > 0 || bars[i].SessionPrices["11:30"] > 0) {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		missingCodes = append(missingCodes, code)
	}
	if len(missingCodes) == 0 {
		return
	}
	fillSessionPrices := func(session string, prices map[string]float64) {
		for i := range bars {
			if bars[i].Date != strategyDate {
				continue
			}
			code := normalizeAStockCode(bars[i].Code)
			if code == "" || prices[code] <= 0 {
				continue
			}
			if bars[i].SessionPrices == nil {
				bars[i].SessionPrices = make(map[string]float64)
			}
			if bars[i].SessionPrices[session] <= 0 {
				bars[i].SessionPrices[session] = prices[code]
			}
		}
	}
	price1230 := s.loadAStockSessionPrices(strategyDate, missingCodes, "12:30")
	fillSessionPrices("12:30", price1230)
	fallbackCodes := missingCodes[:0]
	for _, code := range missingCodes {
		if price1230[code] <= 0 {
			fallbackCodes = append(fallbackCodes, code)
		}
	}
	if len(fallbackCodes) == 0 {
		return
	}
	fillSessionPrices("11:30", s.loadAStockSessionPrices(strategyDate, fallbackCodes, "11:30"))
}

func (s *Server) enrichAStockRecommendationEntryPrices(strategyDate string, period string, recommendations []aStockRecommendation, bars []aStockMarketBar) {
	if len(recommendations) == 0 || len(bars) == 0 {
		return
	}
	normalizedPeriod := normalizeAStockPeriod(period).Key
	codesByTime := make(map[string][]string)
	seen := make(map[string]map[string]struct{})
	for _, rec := range recommendations {
		code := normalizeAStockCode(rec.Code)
		entryTime := aStockRecommendationEffectiveEntryTime(rec, normalizedPeriod)
		if code == "" || entryTime == "" || isDefaultAStockBarEntryTime(normalizedPeriod, entryTime) {
			continue
		}
		if seen[entryTime] == nil {
			seen[entryTime] = make(map[string]struct{})
		}
		if _, exists := seen[entryTime][code]; exists {
			continue
		}
		seen[entryTime][code] = struct{}{}
		codesByTime[entryTime] = append(codesByTime[entryTime], code)
	}
	if len(codesByTime) == 0 {
		return
	}
	for entryTime, codes := range codesByTime {
		prices := s.loadAStockSessionPrices(strategyDate, codes, entryTime)
		if len(prices) == 0 {
			continue
		}
		for i := range bars {
			if bars[i].Date != strategyDate {
				continue
			}
			code := normalizeAStockCode(bars[i].Code)
			price := prices[code]
			if price <= 0 {
				continue
			}
			if bars[i].SessionPrices == nil {
				bars[i].SessionPrices = make(map[string]float64)
			}
			bars[i].SessionPrices[entryTime] = price
		}
	}
}

func isDefaultAStockBarEntryTime(period string, entryTime string) bool {
	entryTime = normalizeAStockEntryTimeMinute(entryTime)
	switch normalizeAStockPeriod(period).Key {
	case "afternoon":
		return entryTime == "13:01"
	default:
		return entryTime == "09:30"
	}
}

func aStockDefaultRecommendationEntryTime(period string) string {
	if normalizeAStockPeriod(period).Key == "afternoon" {
		return "13:01"
	}
	return "09:30"
}

func aStockRecommendationEffectiveEntryTime(rec aStockRecommendation, period string) string {
	if entryTime := normalizeAStockGeneratedEntryTime(period, rec.EntryTime); entryTime != "" {
		return entryTime
	}
	if entryTime := aStockRecommendationEntryTimeFromReason(rec.Reason, period); entryTime != "" {
		return entryTime
	}
	return aStockDefaultRecommendationEntryTime(period)
}

func aStockRecommendationEntryTimeFromReason(reason string, period string) string {
	raw := aStockRecommendationRawEntryTimeFromReason(reason)
	if raw == "" {
		return ""
	}
	return normalizeAStockGeneratedEntryTime(period, raw)
}

func aStockRecommendationRawEntryTimeFromReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ""
	}
	idx := strings.LastIndex(reason, "生成点")
	if idx < 0 {
		return ""
	}
	tail := reason[idx+len("生成点"):]
	for i := 0; i+4 < len(tail); i++ {
		if !isAStockASCIIDigit(tail[i]) {
			continue
		}
		for j := i + 1; j < len(tail) && j <= i+2; j++ {
			if j+2 >= len(tail) || tail[j] != ':' {
				continue
			}
			candidate := tail[i : j+3]
			if entryTime := normalizeAStockEntryTimeMinute(candidate); entryTime != "" {
				return entryTime
			}
		}
	}
	return ""
}

func normalizeAStockGeneratedEntryTime(period string, raw string) string {
	entryTime := normalizeAStockEntryTimeMinute(raw)
	if entryTime == "" {
		return ""
	}
	if normalizeAStockPeriod(period).Key != "afternoon" {
		return "09:30"
	}
	switch entryTime {
	case "12:57", "13:00":
		return "13:01"
	}
	if entryTime < "09:30" {
		return "09:30"
	}
	return entryTime
}

func normalizeAStockEntryTimeMinute(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "：", ":"))
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, ":")
	if len(parts) < 2 {
		return ""
	}
	hour, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return ""
	}
	minutePart := strings.TrimSpace(parts[1])
	if len(minutePart) > 2 {
		minutePart = minutePart[:2]
	}
	minute, err := strconv.Atoi(minutePart)
	if err != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return ""
	}
	return fmt.Sprintf("%02d:%02d", hour, minute)
}

func isAStockASCIIDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

func (s *Server) loadDefaultAStockBars(strategyDate string, codes []string) ([]aStockMarketBar, error) {
	if bars, err := s.loadEastmoneyAStockBars(strategyDate, codes); err == nil && len(bars) > 0 {
		return bars, nil
	}
	if bars, err := s.loadYahooAStockBars(strategyDate, codes); err == nil && len(bars) > 0 {
		return bars, nil
	}
	return nil, fmt.Errorf("no market bars")
}

func (s *Server) loadEastmoneyAStockBars(strategyDate string, codes []string) ([]aStockMarketBar, error) {
	endDate := aStockMarketEndDate(strategyDate)
	var wg sync.WaitGroup
	barCh := make(chan []aStockMarketBar, len(codes))
	for _, code := range codes {
		code := code
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
			defer cancel()
			resp, err := s.client.R().
				SetContext(ctx).
				SetQueryParam("secid", eastmoneyAStockSecID(code)).
				SetQueryParam("klt", "101").
				SetQueryParam("fqt", "1").
				SetQueryParam("end", endDate).
				SetQueryParam("lmt", "100").
				SetQueryParam("fields1", "f1,f2,f3,f4,f5,f6").
				SetQueryParam("fields2", "f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61").
				Get(aStockEastmoneyKlineURL)
			if err != nil || !resp.IsSuccess() {
				return
			}
			bars, err := decodeAStockMarketBars(resp.Body())
			if err != nil {
				return
			}
			for i := range bars {
				if bars[i].Code == "" {
					bars[i].Code = code
				}
			}
			barCh <- bars
		}()
	}
	wg.Wait()
	close(barCh)
	bars := make([]aStockMarketBar, 0)
	for chunk := range barCh {
		bars = append(bars, chunk...)
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("no eastmoney market bars")
	}
	if entryPrices := s.loadEastmoneyAStock0930Prices(strategyDate, codes); len(entryPrices) > 0 {
		for i := range bars {
			if bars[i].Date != strategyDate {
				continue
			}
			if price := entryPrices[normalizeAStockCode(bars[i].Code)]; price > 0 {
				bars[i].EntryPrice = price
			}
		}
	}
	if afternoonEntryPrices := s.loadEastmoneyAStock1300Prices(strategyDate, codes); len(afternoonEntryPrices) > 0 {
		for i := range bars {
			if bars[i].Date != strategyDate {
				continue
			}
			if price := afternoonEntryPrices[normalizeAStockCode(bars[i].Code)]; price > 0 {
				bars[i].AfternoonEntryPrice = price
			}
		}
	}
	return bars, nil
}

func (s *Server) loadEastmoneyAStock0930Prices(strategyDate string, codes []string) map[string]float64 {
	return s.loadAStockSessionPrices(strategyDate, codes, "09:30")
}

func (s *Server) loadEastmoneyAStock1300Prices(strategyDate string, codes []string) map[string]float64 {
	return s.loadAStockSessionPrices(strategyDate, codes, "13:01")
}

func (s *Server) loadAStockSessionPrices(strategyDate string, codes []string, endTime string) map[string]float64 {
	endTime = normalizeAStockEntryTimeMinute(endTime)
	if endTime == "" {
		return nil
	}
	prices := s.loadEastmoneyAStockSessionPrices(strategyDate, codes, endTime)
	missingCodes := make([]string, 0, len(codes))
	for _, code := range codes {
		code = normalizeAStockCode(code)
		if code == "" {
			continue
		}
		if prices[code] <= 0 {
			missingCodes = append(missingCodes, code)
		}
	}
	if len(missingCodes) == 0 {
		return prices
	}
	for code, price := range s.loadTencentAStockSessionPrices(strategyDate, missingCodes, endTime) {
		if price > 0 && prices[code] <= 0 {
			prices[code] = price
		}
	}
	missingCodes = missingCodes[:0]
	for _, code := range codes {
		code = normalizeAStockCode(code)
		if code == "" {
			continue
		}
		if prices[code] <= 0 {
			missingCodes = append(missingCodes, code)
		}
	}
	if len(missingCodes) == 0 {
		return prices
	}
	for code, price := range s.loadSinaAStockSessionPrices(strategyDate, missingCodes, endTime) {
		if price > 0 && prices[code] <= 0 {
			prices[code] = price
		}
	}
	return prices
}

func (s *Server) loadEastmoneyAStockSessionPrices(strategyDate string, codes []string, endTime string) map[string]float64 {
	endDate := strings.ReplaceAll(strategyDate, "-", "")
	var wg sync.WaitGroup
	type result struct {
		code  string
		price float64
	}
	priceCh := make(chan result, len(codes))
	for _, code := range codes {
		code := normalizeAStockCode(code)
		if code == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
			defer cancel()
			resp, err := s.client.R().
				SetContext(ctx).
				SetQueryParam("secid", eastmoneyAStockSecID(code)).
				SetQueryParam("klt", "1").
				SetQueryParam("fqt", "1").
				SetQueryParam("end", endDate+" "+endTime).
				SetQueryParam("lmt", "300").
				SetQueryParam("fields1", "f1,f2,f3,f4,f5,f6").
				SetQueryParam("fields2", "f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61").
				Get(aStockEastmoneyKlineURL)
			if err != nil || !resp.IsSuccess() {
				return
			}
			price, ok := decodeEastmoneyAStockSessionPrice(resp.Body(), strategyDate, endTime)
			if ok {
				priceCh <- result{code: code, price: price}
			}
		}()
	}
	wg.Wait()
	close(priceCh)
	prices := make(map[string]float64)
	for item := range priceCh {
		if item.code != "" && item.price > 0 {
			prices[item.code] = item.price
		}
	}
	return prices
}

func (s *Server) loadTencentAStockSessionPrices(strategyDate string, codes []string, session string) map[string]float64 {
	baseURL := strings.TrimSpace(aStockTencentMinuteURL)
	if baseURL == "" {
		return nil
	}
	var wg sync.WaitGroup
	type result struct {
		code  string
		price float64
	}
	priceCh := make(chan result, len(codes))
	for _, code := range codes {
		code := normalizeAStockCode(code)
		if code == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
			defer cancel()
			resp, err := s.client.R().
				SetContext(ctx).
				SetHeader("User-Agent", nonEmpty(strings.TrimSpace(s.cfg.UserAgent), "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36")).
				SetQueryParam("code", tencentAStockSymbol(code)).
				Get(baseURL)
			if err != nil || !resp.IsSuccess() {
				return
			}
			price, ok := decodeTencentAStockSessionPrice(resp.Body(), strategyDate, session)
			if ok {
				priceCh <- result{code: code, price: price}
			}
		}()
	}
	wg.Wait()
	close(priceCh)
	prices := make(map[string]float64)
	for item := range priceCh {
		if item.code != "" && item.price > 0 {
			prices[item.code] = item.price
		}
	}
	return prices
}

func (s *Server) loadSinaAStockSessionPrices(strategyDate string, codes []string, session string) map[string]float64 {
	baseURL := strings.TrimSpace(aStockSinaMinuteURL)
	if baseURL == "" {
		return nil
	}
	var wg sync.WaitGroup
	type result struct {
		code  string
		price float64
	}
	priceCh := make(chan result, len(codes))
	for _, code := range codes {
		code := normalizeAStockCode(code)
		if code == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 3500*time.Millisecond)
			defer cancel()
			resp, err := s.client.R().
				SetContext(ctx).
				SetHeader("User-Agent", nonEmpty(strings.TrimSpace(s.cfg.UserAgent), "Mozilla/5.0")).
				SetQueryParam("symbol", sinaAStockSymbol(code)).
				SetQueryParam("scale", "1").
				SetQueryParam("ma", "no").
				SetQueryParam("datalen", "1970").
				Get(baseURL)
			if err != nil || !resp.IsSuccess() {
				return
			}
			price, ok := decodeSinaAStockSessionPrice(resp.Body(), strategyDate, session)
			if ok {
				priceCh <- result{code: code, price: price}
			}
		}()
	}
	wg.Wait()
	close(priceCh)
	prices := make(map[string]float64)
	for item := range priceCh {
		if item.code != "" && item.price > 0 {
			prices[item.code] = item.price
		}
	}
	return prices
}

type aStockRealtimeQuote struct {
	Code   string
	Price  float64
	Open   float64
	Pct    float64
	Source string
	Cached bool
}

type aStockRealtimeQuoteCacheEntry struct {
	Quote     aStockRealtimeQuote
	FetchedAt time.Time
}

func (s *Server) loadAStockRealtimeQuotes(codes []string) map[string]aStockRealtimeQuote {
	uniqueCodes := uniqueAStockRealtimeQuoteCodes(codes)
	if len(uniqueCodes) == 0 {
		return nil
	}
	result := make(map[string]aStockRealtimeQuote, len(uniqueCodes))
	missing := s.loadCachedAStockRealtimeQuotes(uniqueCodes, result)
	if len(missing) > 0 {
		tdxQuotes := s.loadTongdaxinAStockRealtimeQuotes(missing)
		s.storeAStockRealtimeQuoteCache(tdxQuotes)
		for code, quote := range tdxQuotes {
			result[code] = quote
		}
		missing = missingAStockRealtimeQuoteCodes(missing, result)
	}
	if len(missing) > 0 {
		eastmoneyQuotes := s.loadEastmoneyAStockRealtimeQuotes(missing)
		s.storeAStockRealtimeQuoteCache(eastmoneyQuotes)
		for code, quote := range eastmoneyQuotes {
			result[code] = quote
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func uniqueAStockRealtimeQuoteCodes(codes []string) []string {
	seen := make(map[string]struct{}, len(codes))
	uniqueCodes := make([]string, 0, len(codes))
	for _, rawCode := range codes {
		code := normalizeAStockCode(rawCode)
		if !astockcode.IsShanghaiShenzhen(code) {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		uniqueCodes = append(uniqueCodes, code)
	}
	return uniqueCodes
}

func missingAStockRealtimeQuoteCodes(codes []string, quotes map[string]aStockRealtimeQuote) []string {
	missing := make([]string, 0, len(codes))
	for _, code := range codes {
		if quote, ok := quotes[code]; ok && quote.Price > 0 {
			continue
		}
		missing = append(missing, code)
	}
	return missing
}

func (s *Server) loadCachedAStockRealtimeQuotes(codes []string, result map[string]aStockRealtimeQuote) []string {
	now := aStockNow()
	missing := make([]string, 0, len(codes))
	s.aStockCacheMu.Lock()
	defer s.aStockCacheMu.Unlock()
	if s.aStockQuotes == nil {
		s.aStockQuotes = make(map[string]aStockRealtimeQuoteCacheEntry)
	}
	for _, code := range codes {
		entry, ok := s.aStockQuotes[code]
		if ok && entry.Quote.Price > 0 && now.Sub(entry.FetchedAt) <= aStockRealtimeQuoteCacheTTL {
			quote := entry.Quote
			quote.Cached = true
			result[code] = quote
			continue
		}
		missing = append(missing, code)
	}
	return missing
}

func (s *Server) storeAStockRealtimeQuoteCache(quotes map[string]aStockRealtimeQuote) {
	if len(quotes) == 0 {
		return
	}
	now := aStockNow()
	s.aStockCacheMu.Lock()
	defer s.aStockCacheMu.Unlock()
	if s.aStockQuotes == nil {
		s.aStockQuotes = make(map[string]aStockRealtimeQuoteCacheEntry)
	}
	for code, quote := range quotes {
		code = normalizeAStockCode(code)
		if code == "" || quote.Price <= 0 {
			continue
		}
		quote.Code = code
		quote.Cached = false
		s.aStockQuotes[code] = aStockRealtimeQuoteCacheEntry{Quote: quote, FetchedAt: now}
	}
}

func (s *Server) loadTongdaxinAStockRealtimeQuotes(codes []string) map[string]aStockRealtimeQuote {
	baseURL := strings.TrimRight(strings.TrimSpace(s.cfg.AStockQuoteURL), "/")
	if baseURL == "" || len(codes) == 0 {
		return nil
	}
	uniqueCodes := uniqueAStockRealtimeQuoteCodes(codes)
	if len(uniqueCodes) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3500*time.Millisecond)
	defer cancel()
	resp, err := s.client.R().
		SetContext(ctx).
		SetHeader("User-Agent", nonEmpty(strings.TrimSpace(s.cfg.UserAgent), "Mozilla/5.0")).
		SetQueryParam("codes", strings.Join(uniqueCodes, ",")).
		Get(baseURL + "/api/a-stock/tdx-quote")
	if err != nil || !resp.IsSuccess() {
		return nil
	}
	return decodeTongdaxinAStockRealtimeQuotes(resp.Body(), uniqueCodes)
}

func decodeTongdaxinAStockRealtimeQuotes(body []byte, expectedCodes []string) map[string]aStockRealtimeQuote {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil
	}
	expected := make(map[string]struct{}, len(expectedCodes))
	for _, code := range expectedCodes {
		code = normalizeAStockCode(code)
		if code != "" {
			expected[code] = struct{}{}
		}
	}
	items := collectTongdaxinAStockRealtimeQuoteItems(payload)
	quotes := make(map[string]aStockRealtimeQuote, len(items))
	for _, item := range items {
		quote, ok := mapToTongdaxinAStockRealtimeQuote(item)
		if !ok {
			continue
		}
		if len(expected) > 0 {
			if _, ok := expected[quote.Code]; !ok {
				continue
			}
		}
		quotes[quote.Code] = quote
	}
	if len(quotes) == 0 {
		return nil
	}
	return quotes
}

func collectTongdaxinAStockRealtimeQuoteItems(value any) []map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		if data, ok := typed["data"]; ok {
			if items := collectTongdaxinAStockRealtimeQuoteItems(data); len(items) > 0 {
				return items
			}
		}
		if items, ok := typed["items"]; ok {
			return collectTongdaxinAStockRealtimeQuoteItems(items)
		}
		if _, ok := typed["price"]; ok {
			return []map[string]any{typed}
		}
		if _, ok := typed["current_price"]; ok {
			return []map[string]any{typed}
		}
		if _, ok := typed["latest_price"]; ok {
			return []map[string]any{typed}
		}
	case []any:
		items := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]any); ok {
				items = append(items, mapped)
			}
		}
		return items
	}
	return nil
}

func mapToTongdaxinAStockRealtimeQuote(item map[string]any) (aStockRealtimeQuote, bool) {
	code := normalizeAStockCode(firstString(item, "code", "stock_code", "symbol"))
	if code == "" {
		return aStockRealtimeQuote{}, false
	}
	price, ok := firstFloat(item, "price", "current_price", "latest_price", "close")
	if !ok || price <= 0 {
		return aStockRealtimeQuote{}, false
	}
	open, _ := firstFloat(item, "open", "open_price")
	pct, _ := firstFloat(item, "pct", "change_pct", "pct_chg", "market_pct")
	return aStockRealtimeQuote{Code: code, Price: price, Open: open, Pct: pct, Source: "tdx"}, true
}

func (s *Server) loadEastmoneyAStockRealtimeQuotes(codes []string) map[string]aStockRealtimeQuote {
	baseURL := strings.TrimSpace(aStockEastmoneyQuoteURL)
	if baseURL == "" || len(codes) == 0 {
		return nil
	}
	uniqueCodes := uniqueAStockRealtimeQuoteCodes(codes)
	if len(uniqueCodes) == 0 {
		return nil
	}
	var wg sync.WaitGroup
	quoteCh := make(chan aStockRealtimeQuote, len(uniqueCodes))
	for _, code := range uniqueCodes {
		code := code
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
			defer cancel()
			resp, err := s.client.R().
				SetContext(ctx).
				SetHeader("User-Agent", nonEmpty(strings.TrimSpace(s.cfg.UserAgent), "Mozilla/5.0")).
				SetQueryParam("secid", eastmoneyAStockSecID(code)).
				SetQueryParam("fields", "f43,f46,f57,f58,f60,f170").
				Get(baseURL)
			if err != nil || !resp.IsSuccess() {
				return
			}
			quote, ok := decodeEastmoneyAStockRealtimeQuote(resp.Body(), code)
			if ok {
				quoteCh <- quote
			}
		}()
	}
	wg.Wait()
	close(quoteCh)
	quotes := make(map[string]aStockRealtimeQuote)
	for quote := range quoteCh {
		if quote.Code != "" && quote.Price > 0 {
			quote.Source = "eastmoney"
			quotes[quote.Code] = quote
		}
	}
	return quotes
}

func (s *Server) loadYahooAStockBars(strategyDate string, codes []string) ([]aStockMarketBar, error) {
	day, err := time.ParseInLocation("2006-01-02", strategyDate, aStockLocation())
	if err != nil {
		return nil, err
	}
	period1 := day.AddDate(0, 0, -90).Unix()
	period2 := day.AddDate(0, 0, 15).Unix()
	baseURL := strings.TrimRight(aStockYahooChartURL, "/")
	var wg sync.WaitGroup
	barCh := make(chan []aStockMarketBar, len(codes))
	for _, code := range codes {
		code := normalizeAStockCode(code)
		if code == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 3500*time.Millisecond)
			defer cancel()
			resp, err := s.client.R().
				SetContext(ctx).
				SetHeader("User-Agent", "Mozilla/5.0").
				SetQueryParam("period1", fmt.Sprint(period1)).
				SetQueryParam("period2", fmt.Sprint(period2)).
				SetQueryParam("interval", "1d").
				SetQueryParam("events", "history").
				SetQueryParam("includeAdjustedClose", "true").
				Get(baseURL + "/" + url.PathEscape(yahooAStockSymbol(code)))
			if err != nil || !resp.IsSuccess() {
				return
			}
			bars, err := decodeYahooAStockBars(resp.Body(), code)
			if err != nil || len(bars) == 0 {
				return
			}
			barCh <- bars
		}()
	}
	wg.Wait()
	close(barCh)
	bars := make([]aStockMarketBar, 0)
	for chunk := range barCh {
		bars = append(bars, chunk...)
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("no yahoo market bars")
	}
	return bars, nil
}

func initializeAStockRecommendationMarket(recommendations []aStockRecommendation) []aStockRecommendation {
	for i := range recommendations {
		recommendations[i].PrevClose = "--"
		recommendations[i].PrevPct = "--"
		recommendations[i].PrevPctClass = "astock-flat"
		recommendations[i].Change30 = "--"
		recommendations[i].Change30Class = "astock-flat"
		recommendations[i].Change60 = "--"
		recommendations[i].Change60Class = "astock-flat"
		recommendations[i].CurrentPrice = "--"
		recommendations[i].TodayPct = "--"
		recommendations[i].TodayPctClass = "astock-flat"
		if strings.TrimSpace(recommendations[i].FundFlow5D) == "" {
			recommendations[i].FundFlow5D = "--"
		}
		if strings.TrimSpace(recommendations[i].FundFlow5DClass) == "" {
			recommendations[i].FundFlow5DClass = "astock-flat"
		}
		if strings.TrimSpace(recommendations[i].HoldingSummary) == "" {
			recommendations[i].HoldingSummary = "--"
		}
		if strings.TrimSpace(recommendations[i].HoldingRatio) == "" {
			recommendations[i].HoldingRatio = "--"
		}
	}
	return recommendations
}

func applyAStockMarketBars(strategyDate string, period string, recommendations []aStockRecommendation, bars []aStockMarketBar, filterLimitUp bool, filterTodayMarket bool, maxRecommendations int) ([]aStockRecommendation, []aStockBacktestRow, string, int, int) {
	return applyAStockMarketBarsWithSettings(strategyDate, period, recommendations, bars, filterLimitUp, filterTodayMarket, maxRecommendations, defaultAStockAlgorithmSettings())
}

func applyAStockMarketBarsWithSettings(strategyDate string, period string, recommendations []aStockRecommendation, bars []aStockMarketBar, filterLimitUp bool, filterTodayMarket bool, maxRecommendations int, settings model.AStockRecommendationAlgorithmSettings) ([]aStockRecommendation, []aStockBacktestRow, string, int, int) {
	result := applyAStockMarketBarsDetailedWithSettings(strategyDate, period, recommendations, bars, filterLimitUp, filterTodayMarket, maxRecommendations, settings)
	return result.Recommendations, result.Backtests, result.Status, result.LimitUpFiltered, result.NoTodayMarketCount
}

func applyAStockMarketBarsDetailedWithSettings(strategyDate string, period string, recommendations []aStockRecommendation, bars []aStockMarketBar, filterLimitUp bool, filterTodayMarket bool, maxRecommendations int, settings model.AStockRecommendationAlgorithmSettings) aStockMarketViewResult {
	byCode := groupAStockMarketBars(bars)
	normalizedPeriod := normalizeAStockPeriod(period).Key
	entryDate := aStockBacktestEntryDate(strategyDate, normalizedPeriod)
	withPrev := 0
	sectorPenalties := make(map[string]int)
	filteredCount := 0
	overheatFilteredCount := 0
	todayHighPctFilteredCount := 0
	limitUpFilteredCount := 0
	noEntryPriceCount := 0
	waitingEntryPriceCount := 0
	drawdownFilteredStocks := make([]aStockRecommendation, 0)
	overheatFilteredStocks := make([]aStockRecommendation, 0)
	todayHighPctFilteredStocks := make([]aStockRecommendation, 0)
	limitUpFilteredStocks := make([]aStockRecommendation, 0)
	noEntryPriceFilteredStocks := make([]aStockRecommendation, 0)
	filteredRecommendations := make([]aStockFilteredRecommendation, 0)
	filtered := make([]aStockRecommendation, 0, len(recommendations))
	for i := range recommendations {
		blockedByDrawdown := false
		blockedByOverheat := false
		codeBars := byCode[recommendations[i].Code]
		entry, ok := aStockEntryBar(codeBars, strategyDate, period, recommendations[i])
		if !ok {
			if normalizedPeriod == "afternoon" {
				if sameDay, hasSameDay := sameDayAStockBar(codeBars, strategyDate); hasSameDay {
					entry = sameDay
					waitingEntryPriceCount++
					ok = true
				}
			}
		}
		if !ok {
			noEntryPriceCount++
			if filterTodayMarket {
				noEntryPriceFilteredStocks = append(noEntryPriceFilteredStocks, recommendations[i])
				continue
			}
		}
		if ok && entry.Close > 0 {
			recommendations[i].CurrentPrice = formatAStockPrice(entry.Close)
			recommendations[i].TodayPct = formatAStockPct(entry.Pct)
			recommendations[i].TodayPctClass = aStockPctClass(entry.Pct)
		}
		prev, hasPrev := previousAStockBar(byCode[recommendations[i].Code], entryDate)
		if hasPrev {
			recommendations[i].PrevClose = formatAStockPrice(prev.Close)
			recommendations[i].PrevPct = formatAStockPct(prev.Pct)
			recommendations[i].PrevPctClass = aStockPctClass(prev.Pct)
			if isAStockLimitUpPct(recommendations[i].Code, recommendations[i].Name, prev.Pct) {
				recommendations[i] = applyAStockPreviousLimitUpPenaltyWithSettings(recommendations[i], prev.Pct, settings)
			} else if prev.Pct > settings.Volatility.PreviousHighPctThreshold {
				recommendations[i] = applyAStockPreviousHighPctPenaltyWithSettings(recommendations[i], prev.Pct, settings)
			}
			if change, ok := aStockLookbackChange(byCode[recommendations[i].Code], entryDate, 30, prev.Close); ok {
				recommendations[i].Change30 = formatAStockPct(change)
				recommendations[i].Change30Class = aStockPctClass(change)
				if change <= settings.Volatility.DrawdownFilterThreshold {
					blockedByDrawdown = true
				}
				if change > settings.Volatility.Overheat30ThresholdPct {
					recommendations[i] = capAStockOverheatedFundFlowScoreWithSettings(recommendations[i], change, settings)
					recommendations[i] = capAStockOverheatedSectorTopStockResonanceScoreWithSettings(recommendations[i], change, settings)
				}
			}
			if change, ok := aStockLookbackChange(byCode[recommendations[i].Code], entryDate, 60, prev.Close); ok {
				recommendations[i].Change60 = formatAStockPct(change)
				recommendations[i].Change60Class = aStockPctClass(change)
				if change <= settings.Volatility.DrawdownFilterThreshold {
					blockedByDrawdown = true
				}
				if change > settings.Volatility.Overheat60ThresholdPct {
					blockedByOverheat = true
				}
			}
			withPrev++
		}
		if ok && shouldFilterAStockTodayHighPct(recommendations[i], entry, prev, hasPrev, settings) {
			todayHighPctFilteredCount++
			todayHighPctFilteredStocks = append(todayHighPctFilteredStocks, recommendations[i])
			filteredRecommendations = append(filteredRecommendations, aStockFilteredRecommendation{
				Reason:         aStockFilteredReasonTodayHighPct,
				Recommendation: recommendations[i],
			})
			continue
		}
		if ok && filterLimitUp && shouldFilterAStockLimitUp(recommendations[i], entry, prev, hasPrev) {
			limitUpFilteredCount++
			limitUpFilteredStocks = append(limitUpFilteredStocks, recommendations[i])
			filteredRecommendations = append(filteredRecommendations, aStockFilteredRecommendation{
				Reason:         aStockFilteredReasonLimitUp,
				Recommendation: recommendations[i],
			})
			continue
		}
		if ok {
			recommendations[i] = applyAStockHighOpenScoreWithSettings(recommendations[i], normalizedPeriod, entry, prev, hasPrev, settings)
			recommendations[i] = applyAStockLowOpenPenaltyWithSettings(recommendations[i], normalizedPeriod, entry, prev, hasPrev, settings)
		}
		if blockedByOverheat {
			overheatFilteredCount++
			overheatFilteredStocks = append(overheatFilteredStocks, recommendations[i])
			continue
		}
		if blockedByDrawdown {
			sectorPenalties[recommendations[i].Hotspot] += settings.Sector.DrawdownPenalty
			filteredCount++
			drawdownFilteredStocks = append(drawdownFilteredStocks, recommendations[i])
			continue
		}
		recommendations[i] = applyAStockMomentumTrendScoreWithSettings(recommendations[i], codeBars, entryDate, settings)
		filtered = append(filtered, recommendations[i])
	}
	recommendations = filtered
	for i := range recommendations {
		penalty := sectorPenalties[recommendations[i].Hotspot]
		if penalty > 0 {
			detail := fmt.Sprintf("板块回撤减分 %d", penalty)
			applyAStockScoreDeltaWithSettings(&recommendations[i], settings, aStockScoreFactorSector, "板块回撤", detail, -penalty)
			recommendations[i].Reason = fmt.Sprintf("%s，%s，调整分 %d", recommendations[i].Reason, detail, recommendations[i].MarketScore)
		}
	}
	sort.SliceStable(recommendations, func(i, j int) bool {
		if recommendations[i].MarketScore == recommendations[j].MarketScore {
			if recommendations[i].Hotspot == recommendations[j].Hotspot {
				return recommendations[i].Code < recommendations[j].Code
			}
			return recommendations[i].Hotspot < recommendations[j].Hotspot
		}
		return recommendations[i].MarketScore > recommendations[j].MarketScore
	})
	for i := range recommendations {
		recommendations[i].Rank = i + 1
	}
	preLimitCount := len(recommendations)
	if maxRecommendations > 0 && len(recommendations) > maxRecommendations {
		recommendations = recommendations[:maxRecommendations]
	}
	for i := range recommendations {
		recommendations[i].Rank = i + 1
	}
	rows := buildAStockBacktestRows(strategyDate, period, recommendations, byCode)
	completed := 0
	for _, row := range rows {
		if row.Status == "已回测" || strings.HasPrefix(row.Status, "已回测") {
			completed++
		}
	}
	status := fmt.Sprintf("已回测 %d/%d", completed, len(rows))
	if filteredCount > 0 {
		status = fmt.Sprintf("%s，%s", status, formatAStockFilterCountWithStocks("过滤回撤股票", filteredCount, "", drawdownFilteredStocks))
	}
	if overheatFilteredCount > 0 {
		status = fmt.Sprintf("%s，%s", status, formatAStockFilterCountWithStocks("过滤60日过热股票", overheatFilteredCount, "", overheatFilteredStocks))
	}
	if todayHighPctFilteredCount > 0 {
		status = fmt.Sprintf("%s，%s", status, formatAStockFilterCountWithStocks("过滤今日涨幅过高股票", todayHighPctFilteredCount, "", todayHighPctFilteredStocks))
	}
	if limitUpFilteredCount > 0 {
		status = fmt.Sprintf("%s，%s", status, formatAStockFilterCountWithStocks("过滤涨停股票", limitUpFilteredCount, "", limitUpFilteredStocks))
	}
	if noEntryPriceCount > 0 {
		if filterTodayMarket {
			status = fmt.Sprintf("%s，%s", status, formatAStockFilterCountWithStocks("过滤无当日行情股票", noEntryPriceCount, "", noEntryPriceFilteredStocks))
		} else {
			status = fmt.Sprintf("%s，缺少当日行情股票 %d", status, noEntryPriceCount)
		}
	}
	if waitingEntryPriceCount > 0 {
		status = fmt.Sprintf("%s，等待下午开盘价股票 %d", status, waitingEntryPriceCount)
	}
	if len(recommendations) == 0 && limitUpFilteredCount > 0 {
		status = fmt.Sprintf("涨停过滤后无推荐股票，%s", formatAStockFilterCountWithStocks("过滤涨停股票", limitUpFilteredCount, "", limitUpFilteredStocks))
	}
	if len(recommendations) == 0 && filteredCount > 0 {
		status = fmt.Sprintf("回撤过滤后无推荐股票，%s", formatAStockFilterCountWithStocks("过滤回撤股票", filteredCount, "", drawdownFilteredStocks))
	}
	if len(recommendations) == 0 && overheatFilteredCount > 0 {
		status = fmt.Sprintf("60日过热过滤后无推荐股票，%s", formatAStockFilterCountWithStocks("过滤60日过热股票", overheatFilteredCount, "", overheatFilteredStocks))
	}
	if len(recommendations) == 0 && todayHighPctFilteredCount > 0 {
		status = fmt.Sprintf("今日涨幅过高过滤后无推荐股票，%s", formatAStockFilterCountWithStocks("过滤今日涨幅过高股票", todayHighPctFilteredCount, "", todayHighPctFilteredStocks))
	}
	if filterTodayMarket && len(recommendations) == 0 && noEntryPriceCount > 0 {
		status = fmt.Sprintf("无当日行情可推荐，%s", formatAStockFilterCountWithStocks("过滤无当日行情股票", noEntryPriceCount, "", noEntryPriceFilteredStocks))
		if filteredCount > 0 {
			status = fmt.Sprintf("%s，%s", status, formatAStockFilterCountWithStocks("过滤回撤股票", filteredCount, "", drawdownFilteredStocks))
		}
	}
	if filterLimitUp && limitUpFilteredCount > 0 && maxRecommendations > 0 {
		if preLimitCount >= maxRecommendations {
			status = fmt.Sprintf("%s，已按热度递补", status)
		} else {
			status = fmt.Sprintf("%s，候选不足未补满", status)
		}
	}
	if withPrev == 0 && completed == 0 && filteredCount == 0 && limitUpFilteredCount == 0 && todayHighPctFilteredCount == 0 && noEntryPriceCount == 0 {
		status = "无匹配行情"
	}
	return aStockMarketViewResult{
		Recommendations:         recommendations,
		Backtests:               rows,
		Status:                  status,
		LimitUpFiltered:         limitUpFilteredCount,
		NoTodayMarketCount:      noEntryPriceCount,
		FilteredRecommendations: normalizeAStockFilteredRecommendations(filteredRecommendations),
	}
}

func applyAStockLockedMarketBars(strategyDate string, period string, recommendations []aStockRecommendation, bars []aStockMarketBar) ([]aStockRecommendation, []aStockBacktestRow, string, int) {
	byCode := groupAStockMarketBars(bars)
	normalizedPeriod := normalizeAStockPeriod(period).Key
	entryDate := aStockBacktestEntryDate(strategyDate, normalizedPeriod)
	for i := range recommendations {
		codeBars := byCode[recommendations[i].Code]
		if entry, ok := aStockEntryBar(codeBars, strategyDate, period, recommendations[i]); ok {
			if entry.Close > 0 {
				recommendations[i].CurrentPrice = formatAStockPrice(entry.Close)
				recommendations[i].TodayPct = formatAStockPct(entry.Pct)
				recommendations[i].TodayPctClass = aStockPctClass(entry.Pct)
			}
		} else if normalizedPeriod == "afternoon" {
			if sameDay, ok := sameDayAStockBar(codeBars, strategyDate); ok && sameDay.Close > 0 {
				recommendations[i].CurrentPrice = formatAStockPrice(sameDay.Close)
				recommendations[i].TodayPct = formatAStockPct(sameDay.Pct)
				recommendations[i].TodayPctClass = aStockPctClass(sameDay.Pct)
			}
		}
		if prev, ok := previousAStockBar(codeBars, entryDate); ok {
			recommendations[i].PrevClose = formatAStockPrice(prev.Close)
			recommendations[i].PrevPct = formatAStockPct(prev.Pct)
			recommendations[i].PrevPctClass = aStockPctClass(prev.Pct)
			if change, ok := aStockLookbackChange(codeBars, entryDate, 30, prev.Close); ok {
				recommendations[i].Change30 = formatAStockPct(change)
				recommendations[i].Change30Class = aStockPctClass(change)
			}
			if change, ok := aStockLookbackChange(codeBars, entryDate, 60, prev.Close); ok {
				recommendations[i].Change60 = formatAStockPct(change)
				recommendations[i].Change60Class = aStockPctClass(change)
			}
		}
	}
	rows := buildAStockBacktestRows(strategyDate, period, recommendations, byCode)
	return recommendations, rows, formatAStockLockedBacktestStatus(normalizedPeriod, rows), 0
}

func (s *Server) enrichAStockBacktestsWithRealtimeQuotes(strategyDate string, period string, rows []aStockBacktestRow) []aStockBacktestRow {
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	entryDate := aStockBacktestEntryDate(strategyDate, period)
	quoteDate := aStockRealtimeQuoteDateForBacktestCurrentReturn(entryDate)
	if len(rows) == 0 || quoteDate == "" {
		return rows
	}
	realtimeOffset := aStockBacktestRealtimeTradingDayOffset(entryDate, quoteDate)
	if realtimeOffset < 0 {
		return rows
	}
	codes := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		code := aStockBacktestRowCode(row)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	quotes := s.loadAStockRealtimeQuotes(codes)
	if len(quotes) == 0 {
		return rows
	}
	normalizedPeriod := normalizeAStockPeriod(period).Key
	enriched := append([]aStockBacktestRow(nil), rows...)
	for i := range enriched {
		code := aStockBacktestRowCode(enriched[i])
		quote, ok := quotes[code]
		if !ok || quote.Price <= 0 {
			continue
		}
		if normalizedPeriod != "afternoon" && aStockBacktestValueMissing(enriched[i].EntryOpen) && quote.Open > 0 {
			enriched[i].EntryOpen = formatAStockPrice(quote.Open)
		}
		enriched[i].CurrentPrice = formatAStockPrice(quote.Price)
		enriched[i].CurrentMarketPct = formatAStockPct(quote.Pct)
		enriched[i].CurrentMarketPctClass = aStockPctClass(quote.Pct)
		entryPrice := aStockBacktestEntryPriceForReturn(normalizedPeriod, enriched[i])
		if entryPrice <= 0 {
			if strings.TrimSpace(enriched[i].CurrentReturn) == "" {
				enriched[i].CurrentReturn = "--"
			}
			if strings.TrimSpace(enriched[i].CurrentReturnClass) == "" {
				enriched[i].CurrentReturnClass = "astock-flat"
			}
			continue
		}
		currentReturn := (quote.Price/entryPrice - 1) * 100
		enriched[i].CurrentReturn = formatAStockPct(currentReturn)
		enriched[i].CurrentReturnClass = aStockPctClass(currentReturn)
		if realtimeOffset == 0 {
			enriched[i].T0Close = formatAStockPrice(quote.Price)
			enriched[i].T0Return = formatAStockPct(currentReturn)
			enriched[i].T0ReturnClass = aStockPctClass(currentReturn)
		} else if realtimeOffset <= 5 {
			enriched[i].Days = ensureAStockBacktestCells(enriched[i].Days, 5)
			enriched[i].Days[realtimeOffset-1] = aStockBacktestCell{
				Close:          formatAStockPrice(quote.Price),
				Return:         formatAStockPct(currentReturn),
				ReturnClass:    aStockPctClass(currentReturn),
				MarketPct:      formatAStockPct(quote.Pct),
				MarketPctClass: aStockPctClass(quote.Pct),
			}
			enriched[i].BestReturn, enriched[i].BestReturnClass = aStockBestBacktestReturn(enriched[i])
		}
		if aStockBacktestStatusNeedsRestore(enriched[i].Status) && aStockBacktestRealtimeQuoteHasEntryOpen(normalizedPeriod, enriched[i]) {
			enriched[i].Status = "等待T+1行情"
		}
		if realtimeOffset > 0 && realtimeOffset <= 5 {
			enriched[i].Status = formatAStockBacktestStatusFromFilledDays(enriched[i].Days)
		}
	}
	return enriched
}

func aStockBacktestRealtimeTradingDayOffset(strategyDate string, quoteDate string) int {
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	quoteDate = normalizeAStockStrategyDate(quoteDate)
	if strategyDate == "" || quoteDate == "" || quoteDate < strategyDate {
		return -1
	}
	if quoteDate == strategyDate {
		return 0
	}
	start, err := time.ParseInLocation("2006-01-02", strategyDate, aStockLocation())
	if err != nil {
		return -1
	}
	end, err := time.ParseInLocation("2006-01-02", quoteDate, aStockLocation())
	if err != nil {
		return -1
	}
	offset := 0
	for day := start.AddDate(0, 0, 1); !day.After(end); day = day.AddDate(0, 0, 1) {
		if isLocalAStockTradingDay(day.Format("2006-01-02")) {
			offset++
		}
	}
	if offset == 0 {
		return -1
	}
	return offset
}

func ensureAStockBacktestCells(cells []aStockBacktestCell, size int) []aStockBacktestCell {
	if size <= 0 {
		return cells
	}
	if len(cells) >= size {
		return cells
	}
	result := append([]aStockBacktestCell(nil), cells...)
	for len(result) < size {
		result = append(result, defaultAStockBacktestCell())
	}
	return result
}

func aStockBestBacktestReturn(row aStockBacktestRow) (string, string) {
	bestSet := false
	best := 0.0
	for _, cell := range row.Days {
		if value, ok := aStockFloat(strings.TrimSpace(cell.Return)); ok {
			if !bestSet || value > best {
				bestSet = true
				best = value
			}
		}
	}
	if !bestSet {
		return "--", "astock-flat"
	}
	return formatAStockPct(best), aStockPctClass(best)
}

func formatAStockBacktestStatusFromFilledDays(days []aStockBacktestCell) string {
	filled := 0
	for i, cell := range days {
		if aStockBacktestValueMissing(cell.Close) && aStockBacktestValueMissing(cell.Return) {
			break
		}
		filled = i + 1
	}
	if filled <= 0 {
		return "等待T+1行情"
	}
	if filled < 5 {
		return fmt.Sprintf("已回测T+%d", filled)
	}
	return "已回测"
}

func aStockBacktestRealtimeQuoteHasEntryOpen(period string, row aStockBacktestRow) bool {
	if normalizeAStockPeriod(period).Key == "afternoon" {
		return !aStockBacktestValueMissing(row.AfternoonOpen)
	}
	return !aStockBacktestValueMissing(row.EntryOpen)
}

func formatAStockLockedBacktestStatus(period string, rows []aStockBacktestRow) string {
	normalizedPeriod := normalizeAStockPeriod(period).Key
	completed := 0
	waitingEntry := 0
	noData := 0
	for _, row := range rows {
		switch {
		case row.Status == "已回测" || strings.HasPrefix(row.Status, "已回测"):
			completed++
		case row.Status == "等待下午开盘价" || row.Status == "等待当日开盘价" || row.Status == "等待后一个交易日开盘价":
			waitingEntry++
		case row.Status == "无行情数据":
			noData++
		}
	}
	status := fmt.Sprintf("已锁定推荐股票，已回测 %d/%d", completed, len(rows))
	if waitingEntry > 0 {
		if normalizedPeriod == "afternoon" {
			status = fmt.Sprintf("%s，等待下午开盘价股票 %d", status, waitingEntry)
		} else if normalizedPeriod == "evening" {
			status = fmt.Sprintf("%s，等待后一个交易日开盘价股票 %d", status, waitingEntry)
		} else {
			status = fmt.Sprintf("%s，等待当日开盘价股票 %d", status, waitingEntry)
		}
	}
	if noData > 0 {
		status = fmt.Sprintf("%s，无行情数据股票 %d", status, noData)
	}
	if len(rows) == 0 {
		status = "已锁定推荐股票，无回测结果"
	}
	return status
}

func aStockBacktestEntryPriceForReturn(period string, row aStockBacktestRow) float64 {
	if normalizeAStockPeriod(period).Key == "afternoon" {
		if price := parseAStockPriceText(row.AfternoonOpen); price > 0 {
			return price
		}
		return parseAStockPriceText(row.EntryOpen)
	}
	if price := parseAStockPriceText(row.EntryOpen); price > 0 {
		return price
	}
	return parseAStockPriceText(row.AfternoonOpen)
}

func parseAStockPriceText(value string) float64 {
	price, ok := aStockFloat(strings.TrimSpace(value))
	if !ok || price <= 0 {
		return 0
	}
	return price
}

func isAStockLimitUpPct(code string, name string, pct float64) bool {
	return pct >= aStockLimitUpPctThreshold(code, name)
}

func shouldFilterAStockTodayHighPct(rec aStockRecommendation, entry aStockMarketBar, prev aStockMarketBar, hasPrev bool, settings model.AStockRecommendationAlgorithmSettings) bool {
	if entry.Pct <= settings.Volatility.TodayHighPctFilterThreshold {
		return false
	}
	return !aStockLimitUpOpenedForTrading(rec.Code, rec.Name, entry, prev, hasPrev)
}

func shouldFilterAStockLimitUp(rec aStockRecommendation, entry aStockMarketBar, prev aStockMarketBar, hasPrev bool) bool {
	if !isAStockLimitUpPct(rec.Code, rec.Name, entry.Pct) {
		return false
	}
	return !aStockLimitUpOpenedForTrading(rec.Code, rec.Name, entry, prev, hasPrev)
}

func aStockLimitUpOpenedForTrading(code string, name string, entry aStockMarketBar, prev aStockMarketBar, hasPrev bool) bool {
	if !hasPrev || prev.Close <= 0 {
		return false
	}
	threshold := aStockLimitUpPctThreshold(code, name)
	if entry.Pct > 0 && entry.Pct < threshold {
		return true
	}
	if entry.Open > 0 && aStockPriceChangePct(entry.Open, prev.Close) < threshold {
		return true
	}
	if entry.Low > 0 && aStockPriceChangePct(entry.Low, prev.Close) < threshold {
		return true
	}
	return false
}

func aStockPriceChangePct(price float64, prevClose float64) float64 {
	if price <= 0 || prevClose <= 0 {
		return 0
	}
	return (price - prevClose) / prevClose * 100
}

func aStockLimitUpPctThreshold(code string, name string) float64 {
	upperName := strings.ToUpper(strings.TrimSpace(name))
	if strings.Contains(upperName, "ST") {
		return 4.8
	}
	code = normalizeAStockCode(code)
	switch {
	case strings.HasPrefix(code, "300"), strings.HasPrefix(code, "301"), strings.HasPrefix(code, "688"):
		return 19.8
	case strings.HasPrefix(code, "8"), strings.HasPrefix(code, "4"), strings.HasPrefix(code, "92"):
		return 29.8
	default:
		return 9.8
	}
}

func groupAStockMarketBars(bars []aStockMarketBar) map[string][]aStockMarketBar {
	grouped := make(map[string][]aStockMarketBar)
	for _, bar := range bars {
		bar.Code = normalizeAStockCode(bar.Code)
		bar.Date = normalizeAStockMarketDate(bar.Date)
		if bar.Code == "" || bar.Date == "" {
			continue
		}
		grouped[bar.Code] = append(grouped[bar.Code], bar)
	}
	for code := range grouped {
		sort.SliceStable(grouped[code], func(i, j int) bool {
			return grouped[code][i].Date < grouped[code][j].Date
		})
	}
	return grouped
}

func previousAStockBar(bars []aStockMarketBar, strategyDate string) (aStockMarketBar, bool) {
	var found aStockMarketBar
	ok := false
	for _, bar := range bars {
		if bar.Date >= strategyDate {
			break
		}
		found = bar
		ok = true
	}
	return found, ok
}

func sameDayAStockBar(bars []aStockMarketBar, strategyDate string) (aStockMarketBar, bool) {
	for _, bar := range bars {
		if bar.Date == strategyDate && (bar.Open > 0 || bar.Close > 0 || bar.Pct != 0) {
			return bar, true
		}
	}
	return aStockMarketBar{}, false
}

func aStockLookbackChange(bars []aStockMarketBar, strategyDate string, days int, prevClose float64) (float64, bool) {
	if prevClose <= 0 {
		return 0, false
	}
	targetDate, err := aStockDateOffset(strategyDate, -days)
	if err != nil {
		return 0, false
	}
	bar, ok := latestAStockBarOnOrBefore(bars, targetDate)
	if !ok || bar.Close <= 0 {
		return 0, false
	}
	return (prevClose/bar.Close - 1) * 100, true
}

func latestAStockBarOnOrBefore(bars []aStockMarketBar, targetDate string) (aStockMarketBar, bool) {
	var found aStockMarketBar
	ok := false
	for _, bar := range bars {
		if bar.Date > targetDate {
			break
		}
		found = bar
		ok = true
	}
	return found, ok
}

func aStockEntryBar(bars []aStockMarketBar, strategyDate string, period string, rec aStockRecommendation) (aStockMarketBar, bool) {
	entryDate := aStockBacktestEntryDate(strategyDate, period)
	for _, bar := range bars {
		if bar.Date == entryDate && aStockEntryPriceForRecommendation(bar, period, rec) > 0 {
			return bar, true
		}
	}
	return aStockMarketBar{}, false
}

func aStockEntryPrice(bar aStockMarketBar, period string) float64 {
	return aStockEntryPriceForRecommendation(bar, period, aStockRecommendation{})
}

func aStockEntryPriceForRecommendation(bar aStockMarketBar, period string, rec aStockRecommendation) float64 {
	if normalizeAStockPeriod(period).Key == "afternoon" {
		entryTime := aStockRecommendationEffectiveEntryTime(rec, "afternoon")
		if entryTime != "" && !isDefaultAStockBarEntryTime("afternoon", entryTime) {
			if bar.SessionPrices != nil && bar.SessionPrices[entryTime] > 0 {
				return bar.SessionPrices[entryTime]
			}
			return 0
		}
		if bar.AfternoonEntryPrice > 0 {
			return bar.AfternoonEntryPrice
		}
		return 0
	}
	if bar.EntryPrice > 0 {
		return bar.EntryPrice
	}
	return bar.Open
}

func buildAStockBacktestRows(strategyDate string, period string, recommendations []aStockRecommendation, byCode map[string][]aStockMarketBar) []aStockBacktestRow {
	normalizedPeriod := normalizeAStockPeriod(period).Key
	rows := make([]aStockBacktestRow, 0, len(recommendations))
	for _, rec := range recommendations {
		row := aStockBacktestRow{
			Stock:                 rec.Code + " " + rec.Name,
			EntryOpen:             "--",
			AfternoonOpen:         "--",
			T0Return:              "--",
			T0Close:               "--",
			T0ReturnClass:         "astock-flat",
			CurrentPrice:          "--",
			CurrentReturn:         "--",
			CurrentReturnClass:    "astock-flat",
			CurrentMarketPct:      "--",
			CurrentMarketPctClass: "astock-flat",
			Days:                  make([]aStockBacktestCell, 5),
			BestReturn:            "--",
			BestReturnClass:       "astock-flat",
			Status:                "等待行情接口配置",
		}
		for i := range row.Days {
			row.Days[i] = defaultAStockBacktestCell()
		}
		bars := byCode[rec.Code]
		if len(bars) == 0 {
			if byCode != nil {
				row.Status = "无行情数据"
			}
			rows = append(rows, row)
			continue
		}
		entryIdx := aStockEntryBarIndex(bars, strategyDate, normalizedPeriod, rec)
		if entryIdx < 0 {
			if normalizedPeriod == "afternoon" {
				row.Status = "等待下午开盘价"
			} else if normalizedPeriod == "evening" {
				row.Status = "等待后一个交易日开盘价"
			} else {
				row.Status = "等待当日开盘价"
			}
			rows = append(rows, row)
			continue
		}
		entry := bars[entryIdx]
		entryPrice := aStockEntryPriceForRecommendation(entry, normalizedPeriod, rec)
		if normalizedPeriod == "afternoon" {
			row.AfternoonOpen = formatAStockPrice(entryPrice)
		} else {
			row.EntryOpen = formatAStockPrice(entryPrice)
		}
		t0Return := 0.0
		if entryPrice > 0 && entry.Close > 0 {
			t0Return = (entry.Close/entryPrice - 1) * 100
		}
		row.T0Return = formatAStockPct(t0Return)
		if entry.Close > 0 {
			row.T0Close = formatAStockPrice(entry.Close)
			setAStockBacktestCurrentMarket(&row, entry.Close, t0Return, entry.Pct)
		}
		row.T0ReturnClass = aStockPctClass(t0Return)
		bestSet := false
		bestReturn := 0.0
		filled := 0
		for day := 1; day <= 5; day++ {
			barIdx := entryIdx + day
			if barIdx >= len(bars) {
				break
			}
			bar := bars[barIdx]
			ret := (bar.Close/entryPrice - 1) * 100
			row.Days[day-1] = aStockBacktestCell{
				Close:          formatAStockPrice(bar.Close),
				Return:         formatAStockPct(ret),
				ReturnClass:    aStockPctClass(ret),
				MarketPct:      formatAStockPct(bar.Pct),
				MarketPctClass: aStockPctClass(bar.Pct),
			}
			setAStockBacktestCurrentMarket(&row, bar.Close, ret, bar.Pct)
			if !bestSet || ret > bestReturn {
				bestSet = true
				bestReturn = ret
			}
			filled++
		}
		switch {
		case filled == 0:
			row.Status = "等待T+1行情"
		case filled < 5:
			row.Status = fmt.Sprintf("已回测T+%d", filled)
		default:
			row.Status = "已回测"
		}
		if bestSet {
			row.BestReturn = formatAStockPct(bestReturn)
			row.BestReturnClass = aStockPctClass(bestReturn)
		}
		rows = append(rows, row)
	}
	return rows
}

func defaultAStockBacktestCell() aStockBacktestCell {
	return aStockBacktestCell{
		Close:          "--",
		Return:         "--",
		ReturnClass:    "astock-flat",
		MarketPct:      "--",
		MarketPctClass: "astock-flat",
	}
}

func setAStockBacktestCurrentMarket(row *aStockBacktestRow, closePrice float64, currentReturn float64, marketPct float64) {
	if row == nil || closePrice <= 0 {
		return
	}
	row.CurrentPrice = formatAStockPrice(closePrice)
	row.CurrentReturn = formatAStockPct(currentReturn)
	row.CurrentReturnClass = aStockPctClass(currentReturn)
	row.CurrentMarketPct = formatAStockPct(marketPct)
	row.CurrentMarketPctClass = aStockPctClass(marketPct)
}

func aStockEntryBarIndex(bars []aStockMarketBar, strategyDate string, period string, rec aStockRecommendation) int {
	entryDate := aStockBacktestEntryDate(strategyDate, period)
	for i, bar := range bars {
		if bar.Date == entryDate && aStockEntryPriceForRecommendation(bar, period, rec) > 0 {
			return i
		}
	}
	return -1
}

func decodeYahooAStockBars(body []byte, code string) ([]aStockMarketBar, error) {
	var payload struct {
		Chart struct {
			Result []struct {
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Open   []*float64 `json:"open"`
						High   []*float64 `json:"high"`
						Low    []*float64 `json:"low"`
						Close  []*float64 `json:"close"`
						Volume []*float64 `json:"volume"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error any `json:"error"`
		} `json:"chart"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if len(payload.Chart.Result) == 0 || len(payload.Chart.Result[0].Indicators.Quote) == 0 {
		return nil, fmt.Errorf("empty yahoo chart")
	}
	result := payload.Chart.Result[0]
	quote := result.Indicators.Quote[0]
	bars := make([]aStockMarketBar, 0, len(result.Timestamp))
	location := aStockLocation()
	code = normalizeAStockCode(code)
	for i, ts := range result.Timestamp {
		if i >= len(quote.Open) || i >= len(quote.Close) || quote.Open[i] == nil || quote.Close[i] == nil {
			continue
		}
		open := *quote.Open[i]
		closeValue := *quote.Close[i]
		if open <= 0 || closeValue <= 0 {
			continue
		}
		high := maxFloat(open, closeValue)
		if i < len(quote.High) && quote.High[i] != nil && *quote.High[i] > 0 {
			high = *quote.High[i]
		}
		low := minFloat(open, closeValue)
		if i < len(quote.Low) && quote.Low[i] != nil && *quote.Low[i] > 0 {
			low = *quote.Low[i]
		}
		volume := 0.0
		if i < len(quote.Volume) && quote.Volume[i] != nil && *quote.Volume[i] > 0 {
			volume = *quote.Volume[i]
		}
		bars = append(bars, aStockMarketBar{
			Code:   code,
			Date:   time.Unix(ts, 0).In(location).Format("2006-01-02"),
			Open:   open,
			High:   high,
			Low:    low,
			Close:  closeValue,
			Volume: volume,
			Amount: volume * closeValue,
		})
	}
	sort.SliceStable(bars, func(i, j int) bool {
		return bars[i].Date < bars[j].Date
	})
	for i := 1; i < len(bars); i++ {
		if bars[i-1].Close > 0 {
			bars[i].Pct = (bars[i].Close/bars[i-1].Close - 1) * 100
		}
	}
	return bars, nil
}

func decodeAStockMarketBars(body []byte) ([]aStockMarketBar, error) {
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	return collectAStockMarketBars(payload, nil), nil
}

func collectAStockMarketBars(value any, fields []string) []aStockMarketBar {
	switch typed := value.(type) {
	case map[string]any:
		if data, ok := typed["data"]; ok {
			return collectAStockMarketBars(data, fields)
		}
		if rawFields, ok := typed["fields"].([]any); ok {
			fields = make([]string, 0, len(rawFields))
			for _, field := range rawFields {
				fields = append(fields, strings.ToLower(strings.TrimSpace(fmt.Sprint(field))))
			}
		}
		if items, ok := typed["items"]; ok {
			return withAStockParentCode(collectAStockMarketBars(items, fields), typed)
		}
		if klines, ok := typed["klines"]; ok {
			return withAStockParentCode(collectAStockMarketBars(klines, fields), typed)
		}
		if bar, ok := mapToAStockMarketBar(typed); ok {
			return []aStockMarketBar{bar}
		}
	case []any:
		bars := make([]aStockMarketBar, 0, len(typed))
		for _, item := range typed {
			switch row := item.(type) {
			case map[string]any:
				if bar, ok := mapToAStockMarketBar(row); ok {
					bars = append(bars, bar)
				}
			case []any:
				if bar, ok := arrayToAStockMarketBar(row, fields); ok {
					bars = append(bars, bar)
				}
			case string:
				if bar, ok := eastmoneyKlineToAStockMarketBar(row); ok {
					bars = append(bars, bar)
				}
			}
		}
		return bars
	}
	return nil
}

func withAStockParentCode(bars []aStockMarketBar, parent map[string]any) []aStockMarketBar {
	parentCode := normalizeAStockCode(firstString(parent, "code", "stock_code", "symbol", "ts_code"))
	if parentCode == "" {
		return bars
	}
	for i := range bars {
		if bars[i].Code == "" {
			bars[i].Code = parentCode
		}
	}
	return bars
}

func mapToAStockMarketBar(row map[string]any) (aStockMarketBar, bool) {
	code := normalizeAStockCode(firstString(row, "code", "stock_code", "symbol", "ts_code"))
	date := normalizeAStockMarketDate(firstString(row, "date", "trade_date", "day"))
	open, _ := firstFloat(row, "open", "open_price")
	high, _ := firstFloat(row, "high", "high_price")
	low, _ := firstFloat(row, "low", "low_price")
	closeValue, ok := firstFloat(row, "close", "close_price", "pre_close")
	pct, _ := firstFloat(row, "pct", "pct_chg", "change_pct")
	volume, _ := firstFloat(row, "volume", "vol")
	amount, _ := firstFloat(row, "amount", "turnover", "turnover_amount")
	entryPrice, _ := firstFloat(row, "entry_price", "entry", "open0930", "open_0930", "price0930", "price_0930", "minute0930", "minute_0930")
	afternoonEntryPrice, _ := firstFloat(row, "afternoon_entry_price", "afternoon_entry", "afternoon_open", "afternoon_open_price", "open1300", "open_1300", "price1300", "price_1300", "minute1300", "minute_1300")
	if code == "" || date == "" || !ok {
		return aStockMarketBar{}, false
	}
	sessionPrices := make(map[string]float64)
	if price, ok := firstFloat(row, "price1230", "price_1230", "minute1230", "minute_1230", "midday_price", "midday"); ok && price > 0 {
		sessionPrices["12:30"] = price
	}
	if price, ok := firstFloat(row, "price1130", "price_1130", "minute1130", "minute_1130"); ok && price > 0 {
		sessionPrices["11:30"] = price
	}
	if len(sessionPrices) == 0 {
		sessionPrices = nil
	}
	return aStockMarketBar{Code: code, Date: date, Open: open, High: high, Low: low, Close: closeValue, Pct: pct, Volume: volume, Amount: amount, EntryPrice: entryPrice, AfternoonEntryPrice: afternoonEntryPrice, SessionPrices: sessionPrices}, true
}

func arrayToAStockMarketBar(row []any, fields []string) (aStockMarketBar, bool) {
	if len(fields) == 0 {
		return aStockMarketBar{}, false
	}
	values := make(map[string]any, len(fields))
	for i, field := range fields {
		if i < len(row) {
			values[field] = row[i]
		}
	}
	return mapToAStockMarketBar(values)
}

func eastmoneyKlineToAStockMarketBar(raw string) (aStockMarketBar, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) < 9 {
		return aStockMarketBar{}, false
	}
	open := parseAStockFloat(parts[1])
	closeValue := parseAStockFloat(parts[2])
	high := parseAStockFloat(parts[3])
	low := parseAStockFloat(parts[4])
	volume := parseAStockFloat(parts[5])
	amount := parseAStockFloat(parts[6])
	pct := parseAStockFloat(parts[8])
	return aStockMarketBar{Date: normalizeAStockMarketDate(parts[0]), Open: open, High: high, Low: low, Close: closeValue, Pct: pct, Volume: volume, Amount: amount}, true
}

func decodeEastmoneyAStock0930Price(body []byte, strategyDate string) (float64, bool) {
	return decodeEastmoneyAStockSessionPrice(body, strategyDate, "09:30")
}

func decodeEastmoneyAStock1300Price(body []byte, strategyDate string) (float64, bool) {
	return decodeEastmoneyAStockSessionPrice(body, strategyDate, "13:01")
}

func decodeEastmoneyAStockSessionPrice(body []byte, strategyDate string, session string) (float64, bool) {
	session = normalizeAStockEntryTimeMinute(session)
	if session == "" {
		return 0, false
	}
	klines := collectAStockKlineStringsFromJSON(body)
	for _, raw := range klines {
		price, ok := eastmoneySessionKlinePrice(raw, strategyDate, session)
		if ok {
			return price, true
		}
	}
	if session == "13:01" {
		for _, raw := range klines {
			price, ok := eastmoneySessionKlineOpen(raw, strategyDate, "13:00")
			if ok {
				return price, true
			}
		}
	}
	return 0, false
}

func decodeTencentAStockSessionPrice(body []byte, strategyDate string, session string) (float64, bool) {
	var payload any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return 0, false
	}
	dateKey := strings.ReplaceAll(normalizeAStockStrategyDate(strategyDate), "-", "")
	dateSeen, dateMatched := tencentAStockPayloadDateMatch(payload, dateKey)
	if dateSeen && !dateMatched {
		return 0, false
	}
	targetMinute := strings.ReplaceAll(strings.TrimSpace(session), ":", "")
	for _, raw := range collectTencentAStockMinuteRows(payload) {
		price, ok := tencentAStockMinuteRowPrice(raw, targetMinute)
		if ok {
			return price, true
		}
	}
	return 0, false
}

func decodeSinaAStockSessionPrice(body []byte, strategyDate string, session string) (float64, bool) {
	payload := extractSinaAStockJSONPBody(string(body))
	if payload == "" {
		return 0, false
	}
	var rows []map[string]any
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&rows); err != nil {
		return 0, false
	}
	targetPrefix := normalizeAStockStrategyDate(strategyDate) + " " + normalizeAStockSessionSecond(session)
	for _, row := range rows {
		if !strings.HasPrefix(strings.TrimSpace(fmt.Sprint(row["day"])), targetPrefix) {
			continue
		}
		if price, ok := aStockFloat(row["close"]); ok && price > 0 {
			return price, true
		}
		if price, ok := aStockFloat(row["open"]); ok && price > 0 {
			return price, true
		}
	}
	return 0, false
}

func decodeEastmoneyAStockRealtimeQuote(body []byte, expectedCode string) (aStockRealtimeQuote, bool) {
	var payload struct {
		Data map[string]any `json:"data"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil || len(payload.Data) == 0 {
		return aStockRealtimeQuote{}, false
	}
	code := normalizeAStockCode(firstString(payload.Data, "f57", "code", "stock_code", "symbol"))
	expectedCode = normalizeAStockCode(expectedCode)
	if code == "" {
		code = expectedCode
	}
	if expectedCode != "" && code != expectedCode {
		return aStockRealtimeQuote{}, false
	}
	rawPrice, ok := firstFloat(payload.Data, "f43", "price", "current_price", "latest_price")
	if !ok {
		return aStockRealtimeQuote{}, false
	}
	price := normalizeEastmoneyRealtimeScaledPrice(rawPrice)
	if price <= 0 {
		return aStockRealtimeQuote{}, false
	}
	rawOpen, _ := firstFloat(payload.Data, "f46", "open", "open_price")
	open := normalizeEastmoneyRealtimeScaledPrice(rawOpen)
	rawPct, _ := firstFloat(payload.Data, "f170", "pct", "pct_chg", "change_pct")
	pct := normalizeEastmoneyRealtimeScaledPct(rawPct)
	if rawPrevClose, ok := firstFloat(payload.Data, "f60", "prev_close", "pre_close", "yesterday_close"); ok {
		if prevClose := normalizeEastmoneyRealtimeScaledPrice(rawPrevClose); prevClose > 0 {
			pct = (price/prevClose - 1) * 100
		}
	}
	return aStockRealtimeQuote{Code: code, Price: price, Open: open, Pct: pct}, true
}

func normalizeEastmoneyRealtimeScaledPrice(value float64) float64 {
	if value <= 0 {
		return 0
	}
	if value >= 100 {
		return value / 100
	}
	return value
}

func normalizeEastmoneyRealtimeScaledPct(value float64) float64 {
	if value >= 100 || value <= -100 {
		return value / 100
	}
	return value
}

func extractSinaAStockJSONPBody(raw string) string {
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "=(")
	if start < 0 {
		return ""
	}
	start += len("=(")
	end := strings.LastIndex(raw, ");")
	if end < start {
		end = len(raw)
	}
	return strings.TrimSpace(raw[start:end])
}

func normalizeAStockSessionSecond(session string) string {
	session = strings.TrimSpace(session)
	if session == "" {
		return ""
	}
	if strings.Count(session, ":") == 1 {
		return session + ":00"
	}
	return session
}

func collectAStockKlineStringsFromJSON(body []byte) []string {
	var payload any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil
	}
	return collectAStockKlineStrings(payload)
}

func collectAStockKlineStrings(value any) []string {
	switch typed := value.(type) {
	case map[string]any:
		out := make([]string, 0)
		if klines, ok := typed["klines"]; ok {
			out = append(out, collectAStockKlineStrings(klines)...)
		}
		if data, ok := typed["data"]; ok {
			out = append(out, collectAStockKlineStrings(data)...)
		}
		if items, ok := typed["items"]; ok {
			out = append(out, collectAStockKlineStrings(items)...)
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			switch row := item.(type) {
			case string:
				out = append(out, row)
			case map[string]any, []any:
				out = append(out, collectAStockKlineStrings(row)...)
			}
		}
		return out
	default:
		return nil
	}
}

func eastmoneySessionKlinePrice(raw string, strategyDate string, session string) (float64, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) < 3 {
		return 0, false
	}
	timestamp := strings.TrimSpace(parts[0])
	if !strings.HasPrefix(timestamp, strategyDate+" ") || !strings.Contains(timestamp, session) {
		return 0, false
	}
	open := parseAStockFloat(parts[1])
	closeValue := parseAStockFloat(parts[2])
	if session == "13:00" && open > 0 {
		return open, true
	}
	if closeValue > 0 {
		return closeValue, true
	}
	if open > 0 {
		return open, true
	}
	return 0, false
}

func eastmoneySessionKlineOpen(raw string, strategyDate string, session string) (float64, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) < 2 {
		return 0, false
	}
	timestamp := strings.TrimSpace(parts[0])
	if !strings.HasPrefix(timestamp, strategyDate+" ") || !strings.Contains(timestamp, session) {
		return 0, false
	}
	open := parseAStockFloat(parts[1])
	if open > 0 {
		return open, true
	}
	return 0, false
}

func tencentAStockPayloadDateMatch(value any, dateKey string) (bool, bool) {
	switch typed := value.(type) {
	case map[string]any:
		seen := false
		matched := false
		for key, item := range typed {
			if strings.EqualFold(strings.TrimSpace(key), "date") {
				text := strings.TrimSpace(fmt.Sprint(item))
				if text != "" {
					seen = true
					if normalizeAStockMarketDate(text) == normalizeAStockMarketDate(dateKey) {
						matched = true
					}
				}
				continue
			}
			childSeen, childMatched := tencentAStockPayloadDateMatch(item, dateKey)
			seen = seen || childSeen
			matched = matched || childMatched
		}
		return seen, matched
	case []any:
		seen := false
		matched := false
		for _, item := range typed {
			childSeen, childMatched := tencentAStockPayloadDateMatch(item, dateKey)
			seen = seen || childSeen
			matched = matched || childMatched
		}
		return seen, matched
	default:
		return false, false
	}
}

func collectTencentAStockMinuteRows(value any) []string {
	switch typed := value.(type) {
	case map[string]any:
		out := make([]string, 0)
		for _, item := range typed {
			out = append(out, collectTencentAStockMinuteRows(item)...)
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			switch row := item.(type) {
			case string:
				fields := strings.Fields(strings.TrimSpace(row))
				if len(fields) >= 2 && len(fields[0]) == 4 {
					out = append(out, row)
				}
			default:
				out = append(out, collectTencentAStockMinuteRows(row)...)
			}
		}
		return out
	default:
		return nil
	}
}

func tencentAStockMinuteRowPrice(raw string, targetMinute string) (float64, bool) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) < 2 || strings.TrimSpace(fields[0]) != targetMinute {
		return 0, false
	}
	price := parseAStockFloat(fields[1])
	if price <= 0 {
		return 0, false
	}
	return price, true
}

func firstString(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := row[key]; ok {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func firstFloat(row map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		if value, ok := row[key]; ok {
			if number, ok := aStockFloat(value); ok {
				return number, true
			}
		}
	}
	return 0, false
}

func aStockFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		number, err := typed.Float64()
		return number, err == nil
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case string:
		text := strings.TrimSpace(strings.TrimSuffix(typed, "%"))
		if text == "" || text == "--" {
			return 0, false
		}
		var number float64
		if _, err := fmt.Sscanf(text, "%f", &number); err == nil {
			return number, true
		}
	}
	return 0, false
}

func maxFloat(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}
	maxValue := values[0]
	for _, value := range values[1:] {
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}

func minFloat(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}
	minValue := values[0]
	for _, value := range values[1:] {
		if value < minValue {
			minValue = value
		}
	}
	return minValue
}

func minPositiveFloat(values ...float64) float64 {
	minValue := 0.0
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if minValue == 0 || value < minValue {
			minValue = value
		}
	}
	return minValue
}

func averageFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func parseAStockFloat(raw string) float64 {
	number, _ := aStockFloat(strings.TrimSpace(raw))
	return number
}

func normalizeAStockCode(raw string) string {
	return astockcode.Normalize(raw)
}

func normalizeAStockMarketDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) >= 10 && raw[4] == '-' && raw[7] == '-' {
		return raw[:10]
	}
	if len(raw) == 8 {
		return raw[:4] + "-" + raw[4:6] + "-" + raw[6:8]
	}
	return raw
}

func formatAStockPrice(value float64) string {
	if value <= 0 {
		return "--"
	}
	return fmt.Sprintf("%.2f", value)
}

func formatAStockPct(value float64) string {
	return fmt.Sprintf("%+.2f%%", value)
}

func aStockPctClass(value float64) string {
	switch {
	case value > 0:
		return "astock-up"
	case value < 0:
		return "astock-down"
	default:
		return "astock-flat"
	}
}

func aStockPctClassFromText(value string) string {
	if pct, ok := parseAStockPctText(value); ok {
		return aStockPctClass(pct)
	}
	return "astock-flat"
}

func aStockMarketEndpoint() string {
	return strings.TrimSpace(os.Getenv("YUQING_ASTOCK_MARKET_URL"))
}

func aStockMarketConfigHint() string {
	if aStockMarketEndpoint() == "" {
		return "默认东方财富日 K，失败后使用 Yahoo Finance 日线；也可用 YUQING_ASTOCK_MARKET_URL 覆盖"
	}
	return "YUQING_ASTOCK_MARKET_URL，失败后回退东方财富日 K / Yahoo Finance 日线"
}

func aStockRealtimeQuoteConfigHint(strategyDate string) string {
	if aStockRealtimeQuoteDateForStrategyDate(strategyDate) == "" {
		return "现价：非今日策略日期不补"
	}
	return "现价：通达信 quote（1分钟缓存），失败后回退东方财富 quote"
}

func aStockMarketEndDate(strategyDate string) string {
	day, err := time.Parse("2006-01-02", strategyDate)
	if err != nil {
		return time.Now().Format("20060102")
	}
	return day.AddDate(0, 0, 14).Format("20060102")
}

func aStockDateOffset(strategyDate string, days int) (string, error) {
	day, err := time.Parse("2006-01-02", strategyDate)
	if err != nil {
		return "", err
	}
	return day.AddDate(0, 0, days).Format("2006-01-02"), nil
}

func aStockBacktestEntryDate(strategyDate string, period string) string {
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	if normalizeAStockPeriod(period).Key != "evening" {
		return strategyDate
	}
	if next := localAStockAdjacentTradingDay(strategyDate, 1); strings.TrimSpace(next) != "" {
		return next
	}
	return strategyDate
}

func recentAStockWeekdayDates(endDate string, count int) []string {
	if count <= 0 {
		return nil
	}
	day, err := time.ParseInLocation("2006-01-02", normalizeAStockStrategyDate(endDate), aStockLocation())
	if err != nil {
		return nil
	}
	dates := make([]string, 0, count)
	for len(dates) < count {
		if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday {
			dates = append(dates, day.Format("2006-01-02"))
		}
		day = day.AddDate(0, 0, -1)
	}
	for i, j := 0, len(dates)-1; i < j; i, j = i+1, j-1 {
		dates[i], dates[j] = dates[j], dates[i]
	}
	return dates
}

func eastmoneyAStockSecID(code string) string {
	code = normalizeAStockCode(code)
	market := "0"
	if strings.HasPrefix(code, "6") || strings.HasPrefix(code, "9") || strings.HasPrefix(code, "5") {
		market = "1"
	}
	return market + "." + code
}

func tencentAStockSymbol(code string) string {
	code = normalizeAStockCode(code)
	switch {
	case strings.HasPrefix(code, "6"), strings.HasPrefix(code, "9"), strings.HasPrefix(code, "5"):
		return "sh" + code
	case strings.HasPrefix(code, "4"), strings.HasPrefix(code, "8"):
		return "bj" + code
	default:
		return "sz" + code
	}
}

func sinaAStockSymbol(code string) string {
	return tencentAStockSymbol(code)
}

func yahooAStockSymbol(code string) string {
	code = normalizeAStockCode(code)
	if strings.HasPrefix(code, "6") || strings.HasPrefix(code, "9") || strings.HasPrefix(code, "5") {
		return code + ".SS"
	}
	return code + ".SZ"
}

func aStockLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("UTC+8", 8*60*60)
	}
	return location
}

func (s *Server) triggerAStockCrawl() string {
	sources := enabledAStockCrawlSources()
	if len(sources) == 0 {
		return "A股新闻抓取未触发：所有来源抓取开关均已关闭"
	}
	ok := 0
	failures := make([]string, 0)
	for _, sourceType := range sources {
		resp, err := s.client.R().
			SetQueryParam("source_type", sourceType).
			Post(s.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
		if err != nil || !resp.IsSuccess() {
			failures = append(failures, sourceType)
			continue
		}
		ok++
	}
	if len(failures) > 0 {
		return fmt.Sprintf("A股新闻抓取部分触发：成功 %d 个，失败 %s", ok, strings.Join(failures, "、"))
	}
	return "A股新闻抓取已触发：" + strings.Join(aStockNewsSourceLabels(sources), "、")
}

func (s *Server) triggerAStockWindowCrawl(strategyDate string, periodKey string) string {
	period := normalizeAStockPeriod(periodKey)
	start, end := aStockWindow(strategyDate, period.Key)
	sources := enabledAStockCrawlSources()
	if len(sources) == 0 {
		return fmt.Sprintf("%s 财经新闻补抓未触发：所有来源抓取开关均已关闭。", period.Label)
	}
	ok := 0
	failures := make([]string, 0)
	results := make([]aStockWindowCrawlResult, 0, len(sources))
	for _, sourceType := range sources {
		resp, err := s.client.R().
			SetQueryParam("source_type", sourceType).
			SetQueryParam("start", formatAStockPublishTime(start)).
			SetQueryParam("end", formatAStockPublishTime(end)).
			SetQueryParam("time_field", "publish_time").
			Post(s.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
		result := aStockWindowCrawlResult{SourceType: sourceType}
		if err != nil || !resp.IsSuccess() {
			failures = append(failures, sourceType)
			if err != nil {
				result.ErrorText = err.Error()
			} else if resp != nil {
				result.ErrorText = strings.TrimSpace(resp.String())
				if result.ErrorText == "" {
					result.ErrorText = resp.Status()
				}
			}
			results = append(results, result)
			continue
		}
		if summary, decoded := decodeAStockCrawlSummary(resp.Body()); decoded {
			result.Decoded = true
			result.Fetched = summary.FetchedCount
			result.Inserted = summary.InsertedCount
			result.Updated = summary.UpdatedCount
			result.ErrorText = summary.ErrorText
		}
		results = append(results, result)
		ok++
	}

	countText := ""
	windowCount := -1
	if count, err := s.countAStockWindowNews(start, end); err == nil {
		windowCount = count
		countText = fmt.Sprintf("当前窗口已有 %d 条财经新闻。", count)
	}
	windowText := fmt.Sprintf("%s %s %s", normalizeAStockStrategyDate(strategyDate), strings.TrimSuffix(period.Label, "推荐"), period.WindowLabel)
	statsText := formatAStockWindowCrawlStats(results)
	emptyExplain := ""
	if windowCount == 0 {
		emptyExplain = fmt.Sprintf("没有新闻：本次补抓没有写入 %s 窗口内带 publish_time 的可用新闻；历史补录依赖源站支持该日期窗口，或需要源数据提供准确发布时间。", period.WindowLabel)
	}
	parts := make([]string, 0, 4)
	if len(failures) > 0 {
		parts = append(parts, fmt.Sprintf("已补抓 %s：成功 %d 个来源，失败 %s。", windowText, ok, strings.Join(failures, "、")))
	} else {
		parts = append(parts, fmt.Sprintf("已补抓 %s：金十快讯、金十资讯、金十全站信息、东方财富快讯、东方财富全站、华尔街见闻、财联社、新浪财经。", windowText))
	}
	if statsText != "" {
		parts = append(parts, statsText)
	}
	if countText != "" {
		parts = append(parts, countText)
	}
	if emptyExplain != "" {
		parts = append(parts, emptyExplain)
	}
	return strings.Join(parts, "")
}

func aStockCrawlSources() []string {
	return astocknews.SourceTypes()
}

func enabledAStockCrawlSources() []string {
	settings, _ := astocknews.LoadSettings("")
	sources := make([]string, 0, len(astocknews.SourceTypes()))
	for _, sourceType := range astocknews.SourceTypes() {
		if astocknews.CrawlEnabled(settings, sourceType) {
			sources = append(sources, sourceType)
		}
	}
	return sources
}

func aStockNewsSourceLabels(sources []string) []string {
	labels := make([]string, 0, len(sources))
	for _, sourceType := range sources {
		labels = append(labels, aStockNewsSourceLabel(sourceType))
	}
	return labels
}

func aStockNewsSourceLabel(sourceType string) string {
	return astocknews.Label(sourceType)
}

type aStockWindowCrawlResult struct {
	SourceType string
	Fetched    int
	Inserted   int
	Updated    int
	ErrorText  string
	Decoded    bool
}

func decodeAStockCrawlSummary(body []byte) (model.CrawlSummary, bool) {
	if len(body) == 0 {
		return model.CrawlSummary{}, false
	}
	var envelope struct {
		Data model.CrawlSummary `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && (envelope.Data.SourceType != "" || envelope.Data.FetchedCount > 0 || envelope.Data.InsertedCount > 0 || envelope.Data.UpdatedCount > 0 || envelope.Data.RunID > 0 || envelope.Data.ErrorText != "") {
		return envelope.Data, true
	}
	var summary model.CrawlSummary
	if err := json.Unmarshal(body, &summary); err == nil && (summary.SourceType != "" || summary.FetchedCount > 0 || summary.InsertedCount > 0 || summary.UpdatedCount > 0 || summary.RunID > 0 || summary.ErrorText != "") {
		return summary, true
	}
	return model.CrawlSummary{}, false
}

func formatAStockWindowCrawlStats(results []aStockWindowCrawlResult) string {
	if len(results) == 0 {
		return ""
	}
	fetched := 0
	inserted := 0
	updated := 0
	hasDecoded := false
	for _, result := range results {
		fetched += result.Fetched
		inserted += result.Inserted
		updated += result.Updated
		if result.Decoded || result.Fetched > 0 || result.Inserted > 0 || result.Updated > 0 || result.ErrorText != "" {
			hasDecoded = true
		}
	}
	if !hasDecoded {
		return ""
	}
	return fmt.Sprintf("源站返回 %d 条，窗口内入库 %d 条、更新 %d 条。", fetched, inserted, updated)
}

func (s *Server) countAStockWindowNews(start time.Time, end time.Time) (int, error) {
	items, err := s.loadAStockWindowArticlesByPublishTime(start, end)
	if err != nil {
		return 0, err
	}
	return len(items), nil
}

func aStockWindow(strategyDate string, periodKey string) (time.Time, time.Time) {
	location := aStockLocation()
	day, err := time.ParseInLocation("2006-01-02", strategyDate, location)
	if err != nil {
		day = time.Now().In(location)
	}
	period := normalizeAStockPeriod(periodKey)
	startHour, startMinute, endHour, endMinute := 8, 0, 9, 30
	if period.Key == "afternoon" {
		startHour, startMinute, endHour, endMinute = 9, 30, 13, 0
	} else if period.Key == "evening" {
		startHour, startMinute, endHour, endMinute = 15, 0, 18, 30
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), startHour, startMinute, 0, 0, location)
	end := time.Date(day.Year(), day.Month(), day.Day(), endHour, endMinute, 59, 0, location)
	return start, end
}

func sameAStockWindow(startA time.Time, endA time.Time, startB time.Time, endB time.Time) bool {
	return startA.Equal(startB) && endA.Equal(endB)
}

func aStockWindowContains(outerStart time.Time, outerEnd time.Time, innerStart time.Time, innerEnd time.Time) bool {
	return !innerStart.Before(outerStart) && !innerEnd.After(outerEnd)
}

func aStockRecommendationPhaseWindow(strategyDate string, periodKey string, phase string) (time.Time, time.Time, string) {
	location := aStockLocation()
	day, err := time.ParseInLocation("2006-01-02", normalizeAStockStrategyDate(strategyDate), location)
	if err != nil {
		day = time.Now().In(location)
	}
	period := normalizeAStockPeriod(periodKey)
	normalizedPhase := normalizeAStockRecommendationPhase(phase)
	if period.Key == "evening" {
		return time.Date(day.Year(), day.Month(), day.Day(), 15, 0, 0, 0, location),
			time.Date(day.Year(), day.Month(), day.Day(), 18, 30, 59, 0, location),
			"15:00-18:30:59"
	}
	if period.Key == "afternoon" {
		if normalizedPhase == aStockRecommendationPhasePreopen {
			return time.Date(day.Year(), day.Month(), day.Day(), 9, 30, 0, 0, location),
				time.Date(day.Year(), day.Month(), day.Day(), 12, 56, 59, 0, location),
				"09:30-12:56:59"
		}
		start, end := aStockWindow(day.Format("2006-01-02"), period.Key)
		return start, end, "09:30-13:00:59"
	}
	if normalizedPhase == aStockRecommendationPhasePreopen {
		return time.Date(day.Year(), day.Month(), day.Day(), 8, 0, 0, 0, location),
			time.Date(day.Year(), day.Month(), day.Day(), 9, 26, 59, 0, location),
			"08:00-09:26:59"
	}
	return time.Date(day.Year(), day.Month(), day.Day(), 8, 0, 0, 0, location),
		time.Date(day.Year(), day.Month(), day.Day(), 9, 26, 59, 0, location),
		"08:00-09:26:59"
}

func normalizeAStockRecommendationPhase(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case aStockRecommendationPhasePreopen:
		return aStockRecommendationPhasePreopen
	default:
		return aStockRecommendationPhaseFinal
	}
}

func filterAStockNews(items []model.Item) []model.Item {
	filtered := make([]model.Item, 0, len(items))
	allowed := make(map[string]struct{}, len(aStockCrawlSources()))
	for _, sourceType := range aStockCrawlSources() {
		allowed[sourceType] = struct{}{}
	}
	for _, item := range items {
		if _, ok := allowed[provider.CanonicalSourceType(item.SourceType)]; !ok {
			continue
		}
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].CapturedAt.Before(filtered[j].CapturedAt)
	})
	return filtered
}

type aStockHotspotMatchItem struct {
	item model.Item
	text string
}

func buildAStockHotspots(items []model.Item) []aStockHotspot {
	return buildAStockHotspotsWithSettings(items, defaultAStockAlgorithmSettings())
}

func buildAStockHotspotsWithSettings(items []model.Item, settings model.AStockRecommendationAlgorithmSettings) []aStockHotspot {
	rules := aStockTopicRules()
	hotspots := make([]aStockHotspot, 0, len(rules))
	matchItems := make([]aStockHotspotMatchItem, 0, len(items))
	for _, item := range items {
		matchItems = append(matchItems, aStockHotspotMatchItem{
			item: item,
			text: strings.ToLower(item.Title + " " + item.Summary + " " + item.Content),
		})
	}
	for _, rule := range rules {
		keywordSet := make(map[string]struct{})
		matches := make([]model.Item, 0)
		lowerKeywords := make([]string, 0, len(rule.Keywords))
		for _, keyword := range rule.Keywords {
			lowerKeywords = append(lowerKeywords, strings.ToLower(keyword))
		}
		for _, matchItem := range matchItems {
			itemMatched := false
			for i, keyword := range rule.Keywords {
				if strings.Contains(matchItem.text, lowerKeywords[i]) {
					keywordSet[keyword] = struct{}{}
					itemMatched = true
				}
			}
			if itemMatched {
				matches = append(matches, matchItem.item)
			}
		}
		if len(matches) == 0 {
			continue
		}
		keywords := make([]string, 0, len(keywordSet))
		for keyword := range keywordSet {
			keywords = append(keywords, keyword)
		}
		sort.Strings(keywords)
		negativeNewsCount := countAStockNegativeNewsItems(matches)
		negativeNewsPenalty := negativeNewsCount * settings.Emotion.NegativeNewsPenalty
		score := len(matches)*settings.Emotion.NewsEvidenceScore + len(keywords)*settings.Emotion.KeywordScore - negativeNewsPenalty
		if score < settings.Emotion.HotspotMinScore {
			score = settings.Emotion.HotspotMinScore
		}
		hotspots = append(hotspots, aStockHotspot{
			Name:                rule.Name,
			Keywords:            keywords,
			Score:               score,
			Evidence:            len(matches),
			NegativeNewsCount:   negativeNewsCount,
			NegativeNewsPenalty: negativeNewsPenalty,
			MatchedItems:        matches,
		})
	}
	sort.SliceStable(hotspots, func(i, j int) bool {
		if hotspots[i].Score == hotspots[j].Score {
			return hotspots[i].Name < hotspots[j].Name
		}
		return hotspots[i].Score > hotspots[j].Score
	})
	if settings.Emotion.HotspotDisplayLimit > 0 && len(hotspots) > settings.Emotion.HotspotDisplayLimit {
		return hotspots[:settings.Emotion.HotspotDisplayLimit]
	}
	return hotspots
}

func countAStockNegativeNewsItems(items []model.Item) int {
	count := 0
	for _, item := range items {
		if isAStockNegativeNewsItem(item) {
			count++
		}
	}
	return count
}

func isAStockNegativeNewsItem(item model.Item) bool {
	text := strings.ToLower(strings.Join([]string{item.Title, item.Summary, item.Content}, " "))
	for _, keyword := range aStockNegativeNewsKeywords() {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func aStockNegativeNewsKeywords() []string {
	return []string{
		"震荡走弱",
		"走弱",
		"冲高回落",
		"回落",
		"下挫",
		"跳水",
		"跌超",
		"大跌",
		"跌停",
		"下跌",
		"走低",
		"领跌",
		"承压",
		"回调",
	}
}

func formatAStockHotspotNegativeNewsPenaltyReason(hotspot aStockHotspot) string {
	if hotspot.NegativeNewsCount <= 0 || hotspot.NegativeNewsPenalty <= 0 {
		return ""
	}
	return fmt.Sprintf("负面新闻 %d 条，情绪扣分 %d", hotspot.NegativeNewsCount, hotspot.NegativeNewsPenalty)
}

func buildAStockHotspotsWithTopStocks(hotspots []aStockHotspot, marketCandidates []aStockMarketCandidate, limit int) []aStockHotspot {
	return buildAStockHotspotsWithTopStocksWithSettings(hotspots, marketCandidates, limit, defaultAStockAlgorithmSettings())
}

func buildAStockHotspotsWithTopStocksWithSettings(hotspots []aStockHotspot, marketCandidates []aStockMarketCandidate, limit int, settings model.AStockRecommendationAlgorithmSettings) []aStockHotspot {
	if len(hotspots) == 0 {
		return hotspots
	}
	if limit <= 0 {
		limit = settings.Auction.HotspotTopStockLimit
	}
	candidates := aStockHotspotTopStockCandidatesWithSettings(hotspots, marketCandidates, settings)
	result := make([]aStockHotspot, len(hotspots))
	copy(result, hotspots)
	seen := make(map[string]struct{})
	for i := range result {
		result[i].TopStocks = buildAStockHotspotTopStocksWithSeenWithSettings(result[i], candidates.scored, candidates.fallback, limit, seen, settings)
	}
	return result
}

func buildAStockHotspotTopStocks(hotspot aStockHotspot, candidates []aStockMarketCandidate, limit int) []aStockHotspotStock {
	return buildAStockHotspotTopStocksWithSeen(hotspot, candidates, sortedAStockHotspotFallbackCandidates(candidates), limit, nil)
}

func buildAStockHotspotTopStocksWithSeen(hotspot aStockHotspot, scoredCandidates []aStockMarketCandidate, fallbackCandidates []aStockMarketCandidate, limit int, seen map[string]struct{}) []aStockHotspotStock {
	return buildAStockHotspotTopStocksWithSeenWithSettings(hotspot, scoredCandidates, fallbackCandidates, limit, seen, defaultAStockAlgorithmSettings())
}

func buildAStockHotspotTopStocksWithSeenWithSettings(hotspot aStockHotspot, scoredCandidates []aStockMarketCandidate, fallbackCandidates []aStockMarketCandidate, limit int, seen map[string]struct{}, settings model.AStockRecommendationAlgorithmSettings) []aStockHotspotStock {
	if limit <= 0 {
		limit = settings.Auction.HotspotTopStockLimit
	}
	scored := scoreAStockMarketCandidatesWithSettings(hotspot, scoredCandidates, nil, settings)
	stocks := make([]aStockHotspotStock, 0, limit)
	rowSeen := make(map[string]struct{})
	for _, stock := range scored {
		stocks = appendAStockHotspotTopStock(stocks, stock, hotspot.Score+stock.MatchedScore, limit, seen, rowSeen)
		if len(stocks) >= limit {
			break
		}
	}
	for _, stock := range fallbackCandidates {
		if len(stocks) >= limit {
			break
		}
		stocks = appendAStockHotspotTopStock(stocks, stock, hotspot.Score+aStockMarketRankScoreWithSettings(stock.Rank, settings), limit, seen, rowSeen)
	}
	return stocks
}

func appendAStockHotspotTopStock(stocks []aStockHotspotStock, stock aStockMarketCandidate, score int, limit int, seen map[string]struct{}, rowSeen map[string]struct{}) []aStockHotspotStock {
	if len(stocks) >= limit {
		return stocks
	}
	code := normalizeAStockCode(stock.Code)
	name := resolveAStockRecommendationName(code, stock.Name, nil)
	if !astockcode.IsShanghaiShenzhen(code) || !hasResolvedAStockRecommendationName(code, name) {
		return stocks
	}
	if isBlockedAStockRecommendationStock(code, name) {
		return stocks
	}
	if _, exists := rowSeen[code]; exists {
		return stocks
	}
	if seen != nil {
		if _, exists := seen[code]; exists {
			return stocks
		}
		seen[code] = struct{}{}
	}
	rowSeen[code] = struct{}{}
	return append(stocks, aStockHotspotStock{
		Rank:  len(stocks) + 1,
		Code:  code,
		Name:  name,
		Score: score,
	})
}

func aStockHotspotTopStockCandidates(hotspots []aStockHotspot, marketCandidates []aStockMarketCandidate) aStockHotspotTopStockCandidateSet {
	return aStockHotspotTopStockCandidatesWithSettings(hotspots, marketCandidates, defaultAStockAlgorithmSettings())
}

func aStockHotspotTopStockCandidatesWithSettings(hotspots []aStockHotspot, marketCandidates []aStockMarketCandidate, settings model.AStockRecommendationAlgorithmSettings) aStockHotspotTopStockCandidateSet {
	newsCandidates := newsDerivedAStockMarketCandidates(hotspots)
	fallbackCandidates := sortedAStockHotspotFallbackCandidates(marketCandidates)
	nameResolver := newAStockRecommendationNameResolver(marketCandidates)
	scoredCandidates := make([]aStockMarketCandidate, 0, len(newsCandidates)+minInt(settings.Auction.HotspotScoredCandidateLimit, len(fallbackCandidates)))
	seen := make(map[string]struct{}, len(newsCandidates)+minInt(settings.Auction.HotspotScoredCandidateLimit, len(fallbackCandidates)))
	for _, candidate := range newsCandidates {
		code := normalizeAStockCode(candidate.Code)
		if code != "" {
			candidate.Code = code
			candidate.Name = resolveAStockRecommendationName(code, candidate.Name, nameResolver)
			scoredCandidates = append(scoredCandidates, candidate)
			seen[code] = struct{}{}
		}
	}
	for _, candidate := range fallbackCandidates {
		if len(scoredCandidates) >= len(newsCandidates)+settings.Auction.HotspotScoredCandidateLimit {
			break
		}
		code := candidate.Code
		if _, exists := seen[code]; exists {
			continue
		}
		scoredCandidates = append(scoredCandidates, candidate)
		seen[code] = struct{}{}
	}
	return aStockHotspotTopStockCandidateSet{
		scored:   scoredCandidates,
		fallback: fallbackCandidates,
	}
}

func sortedAStockHotspotFallbackCandidates(candidates []aStockMarketCandidate) []aStockMarketCandidate {
	fallback := make([]aStockMarketCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		code := normalizeAStockCode(candidate.Code)
		name := astockcode.DisplayName(code, candidate.Name)
		if !astockcode.IsShanghaiShenzhen(code) || !hasResolvedAStockRecommendationName(code, name) {
			continue
		}
		candidate.Code = code
		candidate.Name = name
		fallback = append(fallback, candidate)
	}
	sort.SliceStable(fallback, func(i, j int) bool {
		if fallback[i].Rank != fallback[j].Rank {
			if fallback[i].Rank <= 0 {
				return false
			}
			if fallback[j].Rank <= 0 {
				return true
			}
			return fallback[i].Rank < fallback[j].Rank
		}
		if fallback[i].AuctionAmount != fallback[j].AuctionAmount {
			return fallback[i].AuctionAmount > fallback[j].AuctionAmount
		}
		if fallback[i].AuctionVolume != fallback[j].AuctionVolume {
			return fallback[i].AuctionVolume > fallback[j].AuctionVolume
		}
		return fallback[i].Code < fallback[j].Code
	})
	return fallback
}

func markAStockCandidateSource(candidate *aStockMarketCandidate, source string) {
	if candidate == nil {
		return
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return
	}
	for _, existing := range candidate.Sources {
		if existing == source {
			return
		}
	}
	candidate.Sources = append(candidate.Sources, source)
}

func hasAStockCandidateSource(candidate aStockMarketCandidate, source string) bool {
	source = strings.TrimSpace(source)
	if source == "" {
		return false
	}
	for _, existing := range candidate.Sources {
		if existing == source {
			return true
		}
	}
	return false
}

func hasAnyAStockCandidateSource(candidate aStockMarketCandidate) bool {
	for _, source := range candidate.Sources {
		if strings.TrimSpace(source) != "" {
			return true
		}
	}
	return false
}

func formatAStockCandidateSources(candidate aStockMarketCandidate) string {
	if len(candidate.Sources) == 0 {
		return ""
	}
	sourceSet := make(map[string]struct{}, len(candidate.Sources))
	for _, source := range candidate.Sources {
		source = strings.TrimSpace(source)
		if source != "" {
			sourceSet[source] = struct{}{}
		}
	}
	if len(sourceSet) == 0 {
		return ""
	}
	ordered := make([]string, 0, len(sourceSet))
	for _, source := range []string{
		aStockCandidateSourceNews,
		aStockCandidateSourceSector,
		aStockCandidateSourceFund,
		aStockCandidateSourceAuction,
	} {
		if _, ok := sourceSet[source]; ok {
			ordered = append(ordered, source)
			delete(sourceSet, source)
		}
	}
	extra := make([]string, 0, len(sourceSet))
	for source := range sourceSet {
		extra = append(extra, source)
	}
	sort.Strings(extra)
	ordered = append(ordered, extra...)
	return strings.Join(ordered, "、")
}

func mergeAStockPriorityCandidate(current aStockMarketCandidate, incoming aStockMarketCandidate) aStockMarketCandidate {
	incoming.Code = normalizeAStockCode(incoming.Code)
	if current.Code == "" {
		current.Code = incoming.Code
	}
	if shouldUseResolvedAStockRecommendationName(current.Code, current.Name, incoming.Name) {
		current.Name = incoming.Name
	}
	if current.TradeDate == "" {
		current.TradeDate = incoming.TradeDate
	}
	if incoming.Rank > 0 && (current.Rank <= 0 || incoming.Rank < current.Rank) {
		current.Rank = incoming.Rank
	}
	if incoming.AuctionAmount > current.AuctionAmount {
		current.AuctionAmount = incoming.AuctionAmount
	}
	if incoming.AuctionVolume > current.AuctionVolume {
		current.AuctionVolume = incoming.AuctionVolume
	}
	if incoming.AuctionAmount0920 > current.AuctionAmount0920 {
		current.AuctionAmount0920 = incoming.AuctionAmount0920
	}
	if incoming.AuctionAmount0925 > current.AuctionAmount0925 {
		current.AuctionAmount0925 = incoming.AuctionAmount0925
	}
	if incoming.AuctionAmount0929 > current.AuctionAmount0929 {
		current.AuctionAmount0929 = incoming.AuctionAmount0929
	}
	if incoming.AuctionVolume0920 > current.AuctionVolume0920 {
		current.AuctionVolume0920 = incoming.AuctionVolume0920
	}
	if incoming.AuctionVolume0925 > current.AuctionVolume0925 {
		current.AuctionVolume0925 = incoming.AuctionVolume0925
	}
	if incoming.AuctionVolume0929 > current.AuctionVolume0929 {
		current.AuctionVolume0929 = incoming.AuctionVolume0929
	}
	if incoming.MatchedScore > current.MatchedScore {
		current.MatchedScore = incoming.MatchedScore
	}
	if incoming.Evidence > current.Evidence {
		current.Evidence = incoming.Evidence
	}
	if incoming.StrongEvidence > current.StrongEvidence {
		current.StrongEvidence = incoming.StrongEvidence
	}
	if incoming.WeakEvidence > current.WeakEvidence {
		current.WeakEvidence = incoming.WeakEvidence
	}
	if incoming.WeakPenalty > current.WeakPenalty {
		current.WeakPenalty = incoming.WeakPenalty
	}
	if incoming.BadEvidence > current.BadEvidence {
		current.BadEvidence = incoming.BadEvidence
	}
	current.Keywords = mergeAStockStringSet(current.Keywords, incoming.Keywords)
	if incoming.Fallback {
		current.Fallback = true
	}
	current.Sources = mergeAStockStringSet(current.Sources, incoming.Sources)
	if incoming.PreFundScore != 0 && (current.PreFundScore == 0 || incoming.PreFundScore > current.PreFundScore) {
		current.PreFundScore = incoming.PreFundScore
		current.PreFundDetail = incoming.PreFundDetail
	}
	if incoming.PreSectorScore != 0 && (current.PreSectorScore == 0 || incoming.PreSectorScore > current.PreSectorScore) {
		current.PreSectorScore = incoming.PreSectorScore
		current.PreSectorDetail = incoming.PreSectorDetail
	}
	return current
}

func mergeAStockStringSet(left []string, right []string) []string {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(left)+len(right))
	merged := make([]string, 0, len(left)+len(right))
	for _, values := range [][]string{left, right} {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			merged = append(merged, value)
		}
	}
	return merged
}

func (s *Server) buildAStockPriorityCandidatePoolWithCache(strategyDate string, hotspots []aStockHotspot, marketCandidates []aStockMarketCandidate, sectorGate *aStockHotspotSectorGate, cache *aStockRequestCache) []aStockMarketCandidate {
	settings := s.loadAStockAlgorithmSettingsWithCache(cache)
	return s.buildAStockPriorityCandidatePoolWithSettings(strategyDate, hotspots, marketCandidates, sectorGate, cache, settings)
}

func (s *Server) aStockPriorityExternalSignalsEnabled() bool {
	return strings.TrimSpace(s.cfg.AStockAuctionURL) != ""
}

func (s *Server) buildAStockPriorityCandidatePoolWithSettings(strategyDate string, hotspots []aStockHotspot, marketCandidates []aStockMarketCandidate, sectorGate *aStockHotspotSectorGate, cache *aStockRequestCache, settings model.AStockRecommendationAlgorithmSettings) []aStockMarketCandidate {
	settings = model.NormalizeAStockRecommendationAlgorithmSettings(settings)
	hotspots = aStockRecommendationHotspotSliceWithSettings(hotspots, settings)
	if !s.aStockPriorityExternalSignalsEnabled() {
		return aStockRecommendationCandidatesForLimitWithSettings(hotspots, marketCandidates, settings.Auction.RecommendationLimit, settings.Auction.StocksPerHotspot, settings)
	}
	if sectorGate == nil {
		sectorGate = s.loadAStockHotspotSectorGateWithCache(hotspots, cache)
	}
	if len(hotspots) == 0 {
		return nil
	}
	nameResolver := newAStockRecommendationNameResolver(marketCandidates)
	auctionByCode := make(map[string]aStockMarketCandidate)
	for _, candidate := range sortedAStockHotspotFallbackCandidates(marketCandidates) {
		if _, exists := auctionByCode[candidate.Code]; !exists {
			auctionByCode[candidate.Code] = candidate
		}
	}
	candidatesByCode := make(map[string]aStockMarketCandidate)
	appendCandidate := func(candidate aStockMarketCandidate, sources ...string) {
		code := normalizeAStockCode(candidate.Code)
		name := resolveAStockRecommendationName(code, candidate.Name, nameResolver)
		if !astockcode.IsShanghaiShenzhen(code) || !hasResolvedAStockRecommendationName(code, name) {
			return
		}
		if isBlockedAStockRecommendationStock(code, name) {
			return
		}
		candidate.Code = code
		candidate.Name = name
		for _, source := range sources {
			markAStockCandidateSource(&candidate, source)
		}
		if current, exists := candidatesByCode[code]; exists {
			candidatesByCode[code] = mergeAStockPriorityCandidate(current, candidate)
			return
		}
		candidatesByCode[code] = candidate
	}

	newsCodes := make(map[string]struct{})
	for _, candidate := range newsDerivedAStockMarketCandidates(hotspots) {
		code := normalizeAStockCode(candidate.Code)
		if auction, ok := auctionByCode[code]; ok {
			candidate = mergeAStockPriorityCandidate(candidate, auction)
			markAStockCandidateSource(&candidate, aStockCandidateSourceAuction)
		}
		markAStockCandidateSource(&candidate, aStockCandidateSourceNews)
		appendCandidate(candidate)
		if code != "" {
			newsCodes[code] = struct{}{}
		}
	}

	sectorAssessments := make(map[string]aStockSectorFundFlowTrendAssessment)
	for _, hotspot := range hotspots {
		hotspotName := normalizeAStockRecommendationHotspot(hotspot.Name)
		if hotspotName == "" {
			continue
		}
		assessment := s.assessAStockRecommendationSectorFundFlowTrendWithSettings(strategyDate, hotspotName, cache, settings)
		sectorAssessments[hotspotName] = assessment
		if aStockSectorFundFlowTrendBlocksAuction(assessment) {
			continue
		}
		sectorCandidates := s.aStockHotspotSectorConstituentCandidatesWithCache(hotspot, sectorGate, auctionByCode, cache, settings)
		auctionConfirmations := 0
		for _, candidate := range sectorCandidates {
			if assessment.ScoreDelta != 0 {
				candidate.PreSectorScore = assessment.ScoreDelta
				candidate.PreSectorDetail = formatAStockSectorFundFlowTrendReason(assessment)
			}
			markAStockCandidateSource(&candidate, aStockCandidateSourceSector)
			if auction, ok := auctionByCode[normalizeAStockCode(candidate.Code)]; ok && auctionConfirmations < settings.Candidate.AuctionFallbackPerHotspot {
				candidate = mergeAStockPriorityCandidate(candidate, auction)
				markAStockCandidateSource(&candidate, aStockCandidateSourceAuction)
				auctionConfirmations++
			} else if !hasAStockCandidateSource(candidate, aStockCandidateSourceAuction) {
				candidate.Rank = 0
				candidate.AuctionAmount = 0
				candidate.AuctionVolume = 0
			}
			appendCandidate(candidate)
		}
	}

	s.appendAStockFundFlowPriorityCandidates(strategyDate, hotspots, candidatesByCode, auctionByCode, newsCodes, sectorGate, sectorAssessments, nameResolver, cache, settings)

	candidates := make([]aStockMarketCandidate, 0, len(candidatesByCode))
	for _, candidate := range candidatesByCode {
		if settings.Candidate.RequireHotspotLink && !aStockCandidateHasHotspotLink(hotspots, candidate, sectorGate) {
			continue
		}
		if hasAnyAStockCandidateSource(candidate) {
			candidates = append(candidates, candidate)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		leftRank := aStockPriorityCandidateSourceRank(candidates[i])
		rightRank := aStockPriorityCandidateSourceRank(candidates[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if candidates[i].PreFundScore != candidates[j].PreFundScore {
			return candidates[i].PreFundScore > candidates[j].PreFundScore
		}
		if candidates[i].PreSectorScore != candidates[j].PreSectorScore {
			return candidates[i].PreSectorScore > candidates[j].PreSectorScore
		}
		if candidates[i].Rank != candidates[j].Rank {
			if candidates[i].Rank <= 0 {
				return false
			}
			if candidates[j].Rank <= 0 {
				return true
			}
			return candidates[i].Rank < candidates[j].Rank
		}
		if candidates[i].AuctionAmount != candidates[j].AuctionAmount {
			return candidates[i].AuctionAmount > candidates[j].AuctionAmount
		}
		return candidates[i].Code < candidates[j].Code
	})
	return candidates
}

func aStockPriorityCandidateSourceRank(candidate aStockMarketCandidate) int {
	switch {
	case hasAStockCandidateSource(candidate, aStockCandidateSourceNews):
		return 0
	case hasAStockCandidateSource(candidate, aStockCandidateSourceFund):
		return 1
	case hasAStockCandidateSource(candidate, aStockCandidateSourceSector):
		return 2
	case hasAStockCandidateSource(candidate, aStockCandidateSourceAuction):
		return 3
	default:
		return 4
	}
}

func (s *Server) aStockHotspotSectorConstituentCandidatesWithCache(hotspot aStockHotspot, sectorGate *aStockHotspotSectorGate, auctionByCode map[string]aStockMarketCandidate, cache *aStockRequestCache, settings model.AStockRecommendationAlgorithmSettings) []aStockMarketCandidate {
	aliases := aStockHotspotSectorAliases(hotspot.Name)
	if len(aliases) == 0 {
		return nil
	}
	names := make(map[string]string)
	for _, alias := range aliases {
		for code, name := range s.loadAStockSectorConstituentCodeNamesWithCache(alias, cache) {
			if _, exists := names[code]; !exists {
				names[code] = name
			}
		}
	}
	if len(names) == 0 && sectorGate != nil {
		if codes := sectorGate.codesByHotspot[normalizeAStockRecommendationHotspot(hotspot.Name)]; len(codes) > 0 {
			for code := range codes {
				if auction, ok := auctionByCode[code]; ok {
					names[code] = auction.Name
				}
			}
		}
	}
	if len(names) == 0 {
		return nil
	}
	candidates := make([]aStockMarketCandidate, 0, len(names))
	for code, name := range names {
		candidate := aStockMarketCandidate{Code: code, Name: name}
		if auction, ok := auctionByCode[code]; ok {
			candidate = mergeAStockPriorityCandidate(candidate, auction)
		}
		candidates = append(candidates, candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Rank != candidates[j].Rank {
			if candidates[i].Rank <= 0 {
				return false
			}
			if candidates[j].Rank <= 0 {
				return true
			}
			return candidates[i].Rank < candidates[j].Rank
		}
		return candidates[i].Code < candidates[j].Code
	})
	if settings.Candidate.SectorCandidateLimitPerHotspot > 0 && len(candidates) > settings.Candidate.SectorCandidateLimitPerHotspot {
		candidates = candidates[:settings.Candidate.SectorCandidateLimitPerHotspot]
	}
	return candidates
}

func (s *Server) appendAStockFundFlowPriorityCandidates(strategyDate string, hotspots []aStockHotspot, candidatesByCode map[string]aStockMarketCandidate, auctionByCode map[string]aStockMarketCandidate, newsCodes map[string]struct{}, sectorGate *aStockHotspotSectorGate, sectorAssessments map[string]aStockSectorFundFlowTrendAssessment, nameResolver map[string]string, cache *aStockRequestCache, settings model.AStockRecommendationAlgorithmSettings) {
	if settings.Candidate.StockFundFlowCandidateLimit <= 0 {
		return
	}
	seen := make(map[string]struct{})
	addItem := func(item model.AStockStockFundFlow) {
		if item.MainNetInflow <= 0 {
			return
		}
		code := normalizeAStockCode(item.Code)
		name := resolveAStockRecommendationName(code, item.Name, nameResolver)
		if !astockcode.IsShanghaiShenzhen(code) || !hasResolvedAStockRecommendationName(code, name) {
			return
		}
		if _, exists := seen[code]; exists {
			return
		}
		mentioned := false
		if _, ok := newsCodes[code]; ok {
			mentioned = true
		}
		candidate := aStockMarketCandidate{
			Code:      code,
			Name:      name,
			TradeDate: item.TradeDate,
		}
		if !mentioned && !aStockCandidateMatchesAnyHotspotSector(hotspots, candidate, sectorGate) && !aStockCandidateMatchesAnyHotspot(hotspots, candidate) {
			return
		}
		assessment := s.assessAStockRecommendationFundFlow5DWithSettings(strategyDate, code, cache, settings)
		if !mentioned && isAStockFundFlowHardFiltered(assessment) {
			return
		}
		if !assessment.Missing {
			candidate.PreFundScore = assessment.ScoreDelta
			if assessment.ScoreDelta != 0 {
				candidate.PreFundDetail = formatAStockFundFlowScoreReason(assessment)
			}
		} else {
			candidate.PreFundScore = scoreAStockFundFlowListCandidateWithSettings(item, settings)
			if candidate.PreFundScore != 0 {
				candidate.PreFundDetail = fmt.Sprintf("历史资金流%s主力净流入%s，排名%d", strings.TrimSpace(item.Indicator), formatAStockAuctionMoney(item.MainNetInflow), item.Rank)
			}
		}
		for _, hotspot := range hotspots {
			if !aStockCandidateInHotspotSector(sectorGate, hotspot, candidate) {
				continue
			}
			if assessment, ok := sectorAssessments[normalizeAStockRecommendationHotspot(hotspot.Name)]; ok && assessment.ScoreDelta != 0 {
				candidate.PreSectorScore = assessment.ScoreDelta
				candidate.PreSectorDetail = formatAStockSectorFundFlowTrendReason(assessment)
			}
			break
		}
		if auction, ok := auctionByCode[code]; ok {
			candidate = mergeAStockPriorityCandidate(candidate, auction)
			markAStockCandidateSource(&candidate, aStockCandidateSourceAuction)
		}
		markAStockCandidateSource(&candidate, aStockCandidateSourceFund)
		if current, exists := candidatesByCode[code]; exists {
			candidatesByCode[code] = mergeAStockPriorityCandidate(current, candidate)
		} else {
			candidatesByCode[code] = candidate
		}
		seen[code] = struct{}{}
	}
	for _, indicator := range []string{"5日", "10日"} {
		result, err := s.loadAStockStockFundFlowListWithCache(strategyDate, indicator, settings.Candidate.StockFundFlowCandidateLimit, cache)
		if err != nil {
			continue
		}
		for _, item := range result.Items {
			if len(seen) >= settings.Candidate.StockFundFlowCandidateLimit {
				return
			}
			addItem(item)
		}
	}
}

func scoreAStockFundFlowListCandidateWithSettings(item model.AStockStockFundFlow, settings model.AStockRecommendationAlgorithmSettings) int {
	score := 0
	switch {
	case item.MainNetInflow >= settings.Fund.ExtremeThreshold:
		score += settings.Fund.ExtremeScore
	case item.MainNetInflow >= settings.Fund.VeryStrongThreshold:
		score += settings.Fund.VeryStrongScore
	case item.MainNetInflow >= settings.Fund.StrongBonusThreshold:
		score += settings.Fund.StrongBonusScore
	case item.MainNetInflow >= settings.Fund.BonusThreshold:
		score += settings.Fund.BonusScore
	}
	switch {
	case item.Rank > 0 && item.Rank <= 10:
		score += 20
	case item.Rank > 0 && item.Rank <= 30:
		score += 12
	case item.Rank > 0 && item.Rank <= 100:
		score += 6
	}
	if item.MainNetInflowPct >= 8 {
		score += 15
	} else if item.MainNetInflowPct >= 3 {
		score += 8
	}
	return clampAStockFundFlowScoreWithSettings(score, settings)
}

func aStockSectorFundFlowTrendBlocksAuction(assessment aStockSectorFundFlowTrendAssessment) bool {
	return !assessment.Missing && assessment.Total5D < 0 && assessment.Total10D < 0
}

func aStockCandidateHasHotspotLink(hotspots []aStockHotspot, candidate aStockMarketCandidate, sectorGate *aStockHotspotSectorGate) bool {
	if hasAStockCandidateSource(candidate, aStockCandidateSourceNews) ||
		hasAStockCandidateSource(candidate, aStockCandidateSourceSector) ||
		hasAStockCandidateSource(candidate, aStockCandidateSourceFund) {
		return true
	}
	return aStockCandidateMatchesAnyHotspotSector(hotspots, candidate, sectorGate) || aStockCandidateMatchesAnyHotspot(hotspots, candidate)
}

func aStockCandidateMatchesAnyHotspotSector(hotspots []aStockHotspot, candidate aStockMarketCandidate, sectorGate *aStockHotspotSectorGate) bool {
	for _, hotspot := range hotspots {
		if aStockCandidateInHotspotSector(sectorGate, hotspot, candidate) {
			return true
		}
	}
	return false
}

func aStockCandidateMatchesAnyHotspot(hotspots []aStockHotspot, candidate aStockMarketCandidate) bool {
	for _, hotspot := range hotspots {
		if aStockCandidateMatchesHotspot(hotspot, candidate) {
			return true
		}
	}
	return false
}

func aStockCandidateMatchesHotspot(hotspot aStockHotspot, candidate aStockMarketCandidate) bool {
	return len(aStockCandidateKeywordMatches(candidate.Name, hotspot.Keywords)) > 0 || len(intersectAStockKeywords(candidate.Keywords, hotspot.Keywords)) > 0
}

func aStockCandidateInHotspotSector(sectorGate *aStockHotspotSectorGate, hotspot aStockHotspot, candidate aStockMarketCandidate) bool {
	if sectorGate == nil {
		return false
	}
	hotspotName := normalizeAStockRecommendationHotspot(hotspot.Name)
	if _, gated := sectorGate.gatedHotspots[hotspotName]; !gated {
		return false
	}
	codes := sectorGate.codesByHotspot[hotspotName]
	if len(codes) == 0 {
		return len(aStockCandidateKeywordMatches(candidate.Name, hotspot.Keywords)) > 0 || len(intersectAStockKeywords(candidate.Keywords, hotspot.Keywords)) > 0
	}
	_, ok := codes[normalizeAStockCode(candidate.Code)]
	return ok
}

func buildAStockRecommendations(hotspots []aStockHotspot, candidates []aStockMarketCandidate) []aStockRecommendation {
	return buildAStockRecommendationsWithLimit(hotspots, candidates, aStockRecommendationLimit, aStockStocksPerHotspot)
}

func buildAStockRecommendationsWithLimit(hotspots []aStockHotspot, candidates []aStockMarketCandidate, maxRecommendations int, maxPerHotspot int) []aStockRecommendation {
	return buildAStockRecommendationsWithLimitAndSectorGate(hotspots, candidates, maxRecommendations, maxPerHotspot, nil)
}

func buildAStockRecommendationsWithLimitAndSectorGate(hotspots []aStockHotspot, candidates []aStockMarketCandidate, maxRecommendations int, maxPerHotspot int, sectorGate *aStockHotspotSectorGate) []aStockRecommendation {
	return buildAStockRecommendationsWithLimitAndSectorGateWithSettings(hotspots, candidates, maxRecommendations, maxPerHotspot, sectorGate, defaultAStockAlgorithmSettings())
}

func buildAStockRecommendationsWithLimitAndSectorGateWithSettings(hotspots []aStockHotspot, candidates []aStockMarketCandidate, maxRecommendations int, maxPerHotspot int, sectorGate *aStockHotspotSectorGate, settings model.AStockRecommendationAlgorithmSettings) []aStockRecommendation {
	if settings.Auction.HotspotLimit > 0 && len(hotspots) > settings.Auction.HotspotLimit {
		hotspots = hotspots[:settings.Auction.HotspotLimit]
	}
	if maxRecommendations <= 0 {
		maxRecommendations = settings.Auction.RecommendationLimit
	}
	if maxPerHotspot <= 0 {
		maxPerHotspot = settings.Auction.StocksPerHotspot
	}
	candidates = aStockRecommendationCandidatesForLimitWithSettings(hotspots, candidates, maxRecommendations, maxPerHotspot, settings)
	if len(candidates) == 0 {
		return nil
	}
	recommendations := make([]aStockRecommendation, 0)
	seen := make(map[string]struct{})
	for _, hotspot := range hotspots {
		stocks := scoreAStockMarketCandidatesWithSettings(hotspot, candidates, sectorGate, settings)
		picked := 0
		for _, stock := range stocks {
			if _, exists := seen[stock.Code]; exists {
				continue
			}
			seen[stock.Code] = struct{}{}
			scoreBreakdown := buildAStockRecommendationScoreBreakdownWithSettings(hotspot, stock, settings)
			marketScore := aStockRecommendationFactorScoreTotalWithSettings(scoreBreakdown, settings)
			reason := formatAStockFactorSummaryReason(scoreBreakdown, marketScore, settings)
			evidenceReason := ""
			if stock.Fallback {
				evidenceReason = fmt.Sprintf(
					"命中 %s，证据新闻 %d 条，热度分 %d；使用实时新闻明确提及股票，个股证据 %d 条，匹配分 %d，综合分 %d",
					strings.Join(hotspot.Keywords, "、"),
					hotspot.Evidence,
					hotspot.Score,
					stock.Evidence,
					stock.MatchedScore,
					marketScore,
				)
			} else {
				evidenceReason = fmt.Sprintf(
					"命中 %s，证据新闻 %d 条，热度分 %d；行情排名 %d，成交额 %s，个股证据 %d 条，行情分 %d，综合分 %d",
					strings.Join(hotspot.Keywords, "、"),
					hotspot.Evidence,
					hotspot.Score,
					stock.Rank,
					formatAStockAuctionMoney(stock.AuctionAmount),
					stock.Evidence,
					stock.MatchedScore,
					marketScore,
				)
			}
			reason = appendAStockReason(reason, evidenceReason)
			if sources := formatAStockCandidateSources(stock); sources != "" {
				reason = appendAStockReason(reason, "候选来源 "+sources)
			}
			if len(stock.Keywords) > 0 && !stock.Fallback {
				reason = fmt.Sprintf("%s，股票名命中 %s", reason, strings.Join(stock.Keywords, "、"))
			}
			if stock.WeakPenalty > 0 {
				reason = appendAStockReason(reason, fmt.Sprintf("融资融券弱新闻 %d 条，个股证据减分 %d", stock.WeakEvidence, stock.WeakPenalty))
			}
			reason = appendAStockReason(reason, formatAStockHotspotNegativeNewsPenaltyReason(hotspot))
			recommendations = append(recommendations, aStockRecommendation{
				Rank:           len(recommendations) + 1,
				Hotspot:        hotspot.Name,
				Code:           stock.Code,
				Name:           stock.Name,
				HotspotScore:   hotspot.Score,
				MarketScore:    marketScore,
				Reason:         reason,
				ScoreBreakdown: scoreBreakdown,
			})
			picked++
			if picked >= maxPerHotspot {
				break
			}
		}
	}
	return limitAStockRecommendationsByScoreWithSettings(recommendations, maxRecommendations, settings)
}

func formatAStockFactorSummaryReason(components []aStockRecommendationScoreComponent, total int, settings model.AStockRecommendationAlgorithmSettings) string {
	parts := make([]string, 0, 6)
	for _, category := range newAStockScoreCategorySummary().order {
		if !isAStockPrimaryScoreFactor(category.Key) {
			continue
		}
		cap := aStockScoreFactorCapWithSettings(category.Key, settings)
		score := aStockRecommendationFactorSubtotalWithSettings(components, category.Key, settings)
		parts = append(parts, fmt.Sprintf("%s %d/%d", category.Label, score, cap))
	}
	parts = append(parts, fmt.Sprintf("总分 %d/1000", total))
	return strings.Join(parts, "，")
}

func limitAStockRecommendationsByScore(recommendations []aStockRecommendation, maxRecommendations int) []aStockRecommendation {
	return limitAStockRecommendationsByScoreWithSettings(recommendations, maxRecommendations, defaultAStockAlgorithmSettings())
}

func limitAStockRecommendationsByScoreWithSettings(recommendations []aStockRecommendation, maxRecommendations int, settings model.AStockRecommendationAlgorithmSettings) []aStockRecommendation {
	if len(recommendations) == 0 {
		return recommendations
	}
	recommendations = sortAStockRecommendationsByScore(recommendations)
	softLimit := settings.Candidate.MaxStocksPerHotspotSoft
	if maxRecommendations > 0 && maxRecommendations <= settings.Auction.RecommendationLimit && softLimit > 0 && softLimit < maxRecommendations {
		selected := make([]aStockRecommendation, 0, minInt(maxRecommendations, len(recommendations)))
		counts := make(map[string]int)
		used := make([]bool, len(recommendations))
		for i, rec := range recommendations {
			hotspot := normalizeAStockRecommendationHotspot(rec.Hotspot)
			if hotspot != "" && counts[hotspot] >= softLimit {
				continue
			}
			selected = append(selected, rec)
			used[i] = true
			if hotspot != "" {
				counts[hotspot]++
			}
			if len(selected) >= maxRecommendations {
				return rerankAStockRecommendations(selected)
			}
		}
		for i, rec := range recommendations {
			if used[i] {
				continue
			}
			hotspot := normalizeAStockRecommendationHotspot(rec.Hotspot)
			if hotspot != "" && counts[hotspot] >= softLimit+1 {
				continue
			}
			selected = append(selected, rec)
			if hotspot != "" {
				counts[hotspot]++
			}
			if len(selected) >= maxRecommendations {
				return rerankAStockRecommendations(selected)
			}
		}
		return rerankAStockRecommendations(selected)
	}
	if maxRecommendations > 0 && len(recommendations) > maxRecommendations {
		recommendations = recommendations[:maxRecommendations]
	}
	return rerankAStockRecommendations(recommendations)
}

func buildAStockRecommendationScoreBreakdown(hotspot aStockHotspot, stock aStockMarketCandidate) []aStockRecommendationScoreComponent {
	return buildAStockRecommendationScoreBreakdownWithSettings(hotspot, stock, defaultAStockAlgorithmSettings())
}

func buildAStockRecommendationScoreBreakdownWithSettings(hotspot aStockHotspot, stock aStockMarketCandidate, settings model.AStockRecommendationAlgorithmSettings) []aStockRecommendationScoreComponent {
	settings = model.NormalizeAStockRecommendationAlgorithmSettings(settings)
	components := make([]aStockRecommendationScoreComponent, 0, 8)
	components = append(components, newAStockScoreComponentWithFactor(aStockScoreFactorEmotion, "新闻热度", fmt.Sprintf("证据新闻 %d 条", hotspot.Evidence), settings.Emotion.NewsEvidenceScore, hotspot.Evidence*settings.Emotion.NewsEvidenceScore))
	components = append(components, newAStockScoreComponentWithFactor(aStockScoreFactorEmotion, "热点关键词", fmt.Sprintf("命中关键词 %d 个", len(hotspot.Keywords)), settings.Emotion.KeywordScore, len(hotspot.Keywords)*settings.Emotion.KeywordScore))
	if hasAStockCandidateSource(stock, aStockCandidateSourceNews) {
		components = append(components, newAStockScoreComponentWithFactor(aStockScoreFactorEmotion, "候选来源", aStockCandidateSourceNews, settings.Emotion.NewsSourceScore, settings.Emotion.NewsSourceScore))
	}
	if hotspot.NegativeNewsPenalty > 0 {
		components = append(components, newAStockScoreComponentWithFactor(aStockScoreFactorEmotion, "负面新闻", fmt.Sprintf("负面新闻 %d 条，情绪扣分 %d", hotspot.NegativeNewsCount, hotspot.NegativeNewsPenalty), -hotspot.NegativeNewsPenalty, -hotspot.NegativeNewsPenalty))
	}
	rankScore := aStockMarketRankScoreWithSettings(stock.Rank, settings)
	components = append(components, newAStockScoreComponentWithFactor(aStockScoreFactorAuction, "行情排名", fmt.Sprintf("排名 %d", stock.Rank), rankScore, rankScore))
	components = append(components, newAStockScoreComponentWithFactor(aStockScoreFactorEmotion, "个股证据", fmt.Sprintf("有效证据 %d 条 / 总证据 %d 条", stock.StrongEvidence, stock.Evidence), settings.Emotion.StockEvidenceScore, stock.StrongEvidence*settings.Emotion.StockEvidenceScore))
	if len(stock.Keywords) > 0 {
		components = append(components, newAStockScoreComponentWithFactor(aStockScoreFactorEmotion, "股票名命中", fmt.Sprintf("命中关键词 %d 个", len(stock.Keywords)), settings.Emotion.StockNameKeywordScore, len(stock.Keywords)*settings.Emotion.StockNameKeywordScore))
	}
	if stock.WeakPenalty > 0 {
		components = append(components, newAStockScoreComponentWithFactor(aStockScoreFactorEmotion, "弱证据", fmt.Sprintf("融资融券弱新闻 %d 条", stock.WeakEvidence), -stock.WeakPenalty, -stock.WeakPenalty))
	}
	return components
}

func aStockRecommendationCandidatesForLimit(hotspots []aStockHotspot, marketCandidates []aStockMarketCandidate, maxRecommendations int, maxPerHotspot int) []aStockMarketCandidate {
	return aStockRecommendationCandidatesForLimitWithSettings(hotspots, marketCandidates, maxRecommendations, maxPerHotspot, defaultAStockAlgorithmSettings())
}

func aStockRecommendationCandidatesForLimitWithSettings(hotspots []aStockHotspot, marketCandidates []aStockMarketCandidate, maxRecommendations int, maxPerHotspot int, settings model.AStockRecommendationAlgorithmSettings) []aStockMarketCandidate {
	nameResolver := newAStockRecommendationNameResolver(marketCandidates)
	newsCandidates := newsDerivedAStockMarketCandidates(hotspots)
	sourceLinkedPool := false
	for _, candidate := range marketCandidates {
		if hasAnyAStockCandidateSource(candidate) {
			sourceLinkedPool = true
			break
		}
	}
	capacity := len(newsCandidates) + settings.Auction.HotspotScoredCandidateLimit
	if sourceLinkedPool {
		capacity = len(newsCandidates) + len(marketCandidates)
	}
	candidates := make([]aStockMarketCandidate, 0, capacity)
	seen := make(map[string]struct{}, capacity)
	appendCandidate := func(candidate aStockMarketCandidate) {
		code := normalizeAStockCode(candidate.Code)
		name := resolveAStockRecommendationName(code, candidate.Name, nameResolver)
		if !astockcode.IsShanghaiShenzhen(code) || !hasResolvedAStockRecommendationName(code, name) {
			return
		}
		if _, exists := seen[code]; exists {
			return
		}
		if isBlockedAStockRecommendationStock(code, name) {
			return
		}
		candidate.Code = code
		candidate.Name = name
		candidates = append(candidates, candidate)
		seen[code] = struct{}{}
	}
	if sourceLinkedPool {
		for _, candidate := range marketCandidates {
			if !hasAnyAStockCandidateSource(candidate) {
				continue
			}
			appendCandidate(candidate)
		}
		return candidates
	}
	for _, candidate := range newsCandidates {
		markAStockCandidateSource(&candidate, aStockCandidateSourceNews)
		appendCandidate(candidate)
	}
	for _, candidate := range sortedAStockHotspotFallbackCandidates(marketCandidates) {
		if len(candidates) >= len(newsCandidates)+settings.Auction.HotspotScoredCandidateLimit {
			break
		}
		if settings.Candidate.RequireHotspotLink && !aStockCandidateHasHotspotLink(hotspots, candidate, nil) {
			continue
		}
		appendCandidate(candidate)
	}
	return candidates
}

func newAStockRecommendationNameResolver(marketCandidates []aStockMarketCandidate) map[string]string {
	names := make(map[string]string)
	for _, candidate := range marketCandidates {
		addAStockRecommendationResolvedName(names, candidate.Code, candidate.Name)
	}
	return names
}

func addAStockRecommendationResolvedName(names map[string]string, code string, name string) {
	code = normalizeAStockCode(code)
	name = astockcode.DisplayName(code, name)
	if !astockcode.IsShanghaiShenzhen(code) || !hasResolvedAStockRecommendationName(code, name) {
		return
	}
	if _, exists := names[code]; exists {
		return
	}
	names[code] = name
}

func setAStockRecommendationResolvedName(names map[string]string, code string, name string) {
	code = normalizeAStockCode(code)
	name = astockcode.DisplayName(code, name)
	if !astockcode.IsShanghaiShenzhen(code) || !hasResolvedAStockRecommendationName(code, name) {
		return
	}
	names[code] = name
}

func resolveAStockRecommendationName(code string, name string, names map[string]string) string {
	code = normalizeAStockCode(code)
	name = astockcode.DisplayName(code, name)
	if names != nil {
		if resolved := astockcode.DisplayName(code, names[code]); hasResolvedAStockRecommendationName(code, resolved) {
			if shouldUseResolvedAStockRecommendationName(code, name, resolved) {
				return resolved
			}
		}
	}
	if hasResolvedAStockRecommendationName(code, name) {
		return name
	}
	return ""
}

func shouldUseResolvedAStockRecommendationName(code string, name string, resolved string) bool {
	name = astockcode.DisplayName(code, name)
	resolved = astockcode.DisplayName(code, resolved)
	if !hasResolvedAStockRecommendationName(code, resolved) {
		return false
	}
	if !hasResolvedAStockRecommendationName(code, name) {
		return true
	}
	if isInvalidAStockRecommendationName(name) {
		return true
	}
	if len([]rune(name)) <= 2 {
		return true
	}
	if name == resolved {
		return true
	}
	return strings.Contains(resolved, name) || strings.Contains(name, resolved)
}

func hasResolvedAStockRecommendationName(code string, name string) bool {
	name = astockcode.DisplayName(code, name)
	return astockcode.HasResolvedName(code, name) && !isInvalidAStockRecommendationName(name)
}

func isAStockRecommendationPlaceholderName(name string) bool {
	return astockcode.IsPlaceholderName(name)
}

func isInvalidAStockRecommendationName(name string) bool {
	return astockcode.IsInvalidRecommendationName(name)
}

func aStockRecommendationHotspotSlice(hotspots []aStockHotspot) []aStockHotspot {
	return aStockRecommendationHotspotSliceWithSettings(hotspots, defaultAStockAlgorithmSettings())
}

func aStockRecommendationHotspotSliceWithSettings(hotspots []aStockHotspot, settings model.AStockRecommendationAlgorithmSettings) []aStockHotspot {
	if settings.Auction.HotspotLimit > 0 && len(hotspots) > settings.Auction.HotspotLimit {
		return hotspots[:settings.Auction.HotspotLimit]
	}
	return hotspots
}

type aStockRecommendationSnapshot struct {
	Label string
	Start time.Time
	End   time.Time
}

func buildAStockSnapshotRecommendations(strategyDate string, periodKey string, articles []model.Item, candidates []aStockMarketCandidate) []aStockRecommendation {
	return buildAStockSnapshotRecommendationsWithPhaseAndLimit(strategyDate, periodKey, aStockRecommendationPhaseFinal, articles, candidates, aStockRecommendationLimit, aStockStocksPerHotspot)
}

func buildAStockSnapshotRecommendationsWithLimit(strategyDate string, periodKey string, articles []model.Item, candidates []aStockMarketCandidate, maxRecommendations int, maxPerHotspot int) []aStockRecommendation {
	return buildAStockSnapshotRecommendationsWithPhaseAndLimit(strategyDate, periodKey, aStockRecommendationPhaseFinal, articles, candidates, maxRecommendations, maxPerHotspot)
}

func buildAStockSnapshotRecommendationsWithPhase(strategyDate string, periodKey string, phase string, articles []model.Item, candidates []aStockMarketCandidate) []aStockRecommendation {
	return buildAStockSnapshotRecommendationsWithPhaseAndLimit(strategyDate, periodKey, phase, articles, candidates, aStockRecommendationLimit, aStockStocksPerHotspot)
}

func buildAStockSnapshotReplacementRecommendations(strategyDate string, periodKey string, phase string, articles []model.Item, candidates []aStockMarketCandidate) []aStockRecommendation {
	return buildAStockSnapshotRecommendationsWithPhaseAndLimit(strategyDate, periodKey, phase, articles, candidates, aStockReplacementPoolLimit, aStockReplacementPerHotspot)
}

func buildAStockSnapshotRecommendationsWithPhaseAndLimit(strategyDate string, periodKey string, phase string, articles []model.Item, candidates []aStockMarketCandidate, maxRecommendations int, maxPerHotspot int) []aStockRecommendation {
	return buildAStockSnapshotRecommendationsWithPhaseAndLimitAndSectorGate(strategyDate, periodKey, phase, articles, candidates, maxRecommendations, maxPerHotspot, nil)
}

func buildAStockSnapshotRecommendationsWithPhaseAndSectorGate(strategyDate string, periodKey string, phase string, articles []model.Item, candidates []aStockMarketCandidate, sectorGate *aStockHotspotSectorGate) []aStockRecommendation {
	return buildAStockSnapshotRecommendationsWithPhaseAndLimitAndSectorGate(strategyDate, periodKey, phase, articles, candidates, aStockRecommendationLimit, aStockStocksPerHotspot, sectorGate)
}

func buildAStockSnapshotReplacementRecommendationsWithSectorGate(strategyDate string, periodKey string, phase string, articles []model.Item, candidates []aStockMarketCandidate, sectorGate *aStockHotspotSectorGate) []aStockRecommendation {
	return buildAStockSnapshotRecommendationsWithPhaseAndLimitAndSectorGate(strategyDate, periodKey, phase, articles, candidates, aStockReplacementPoolLimit, aStockReplacementPerHotspot, sectorGate)
}

func buildAStockSnapshotRecommendationsWithPhaseAndLimitAndSectorGate(strategyDate string, periodKey string, phase string, articles []model.Item, candidates []aStockMarketCandidate, maxRecommendations int, maxPerHotspot int, sectorGate *aStockHotspotSectorGate) []aStockRecommendation {
	return buildAStockSnapshotRecommendationsWithPhaseAndLimitAndSectorGateWithSettings(strategyDate, periodKey, phase, articles, candidates, maxRecommendations, maxPerHotspot, sectorGate, defaultAStockAlgorithmSettings())
}

func buildAStockSnapshotReplacementRecommendationsWithSectorGateWithSettings(strategyDate string, periodKey string, phase string, articles []model.Item, candidates []aStockMarketCandidate, sectorGate *aStockHotspotSectorGate, settings model.AStockRecommendationAlgorithmSettings) []aStockRecommendation {
	return buildAStockSnapshotRecommendationsWithPhaseAndLimitAndSectorGateWithSettings(strategyDate, periodKey, phase, articles, candidates, settings.Auction.ReplacementPoolLimit, settings.Auction.ReplacementPerHotspot, sectorGate, settings)
}

func buildAStockSnapshotRecommendationsWithPhaseAndLimitAndSectorGateWithSettings(strategyDate string, periodKey string, phase string, articles []model.Item, candidates []aStockMarketCandidate, maxRecommendations int, maxPerHotspot int, sectorGate *aStockHotspotSectorGate, settings model.AStockRecommendationAlgorithmSettings) []aStockRecommendation {
	snapshots := aStockRecommendationSnapshots(strategyDate, periodKey, phase)
	if len(snapshots) == 0 {
		return withAStockRecommendationEntryTimes(buildAStockRecommendationsWithLimitAndSectorGateWithSettings(buildAStockHotspotsWithSettings(articles, settings), candidates, maxRecommendations, maxPerHotspot, sectorGate, settings), periodKey, "")
	}
	if maxRecommendations <= 0 {
		maxRecommendations = settings.Auction.RecommendationLimit
	}
	if maxPerHotspot <= 0 {
		maxPerHotspot = settings.Auction.StocksPerHotspot
	}
	combined := make([]aStockRecommendation, 0)
	seen := make(map[string]struct{})
	hotspotCounts := make(map[string]int)
	for _, snapshot := range snapshots {
		snapshotArticles := filterAStockArticlesByPublishWindow(articles, snapshot.Start, snapshot.End)
		if len(snapshotArticles) == 0 {
			continue
		}
		for _, rec := range buildAStockRecommendationsWithLimitAndSectorGateWithSettings(buildAStockHotspotsWithSettings(snapshotArticles, settings), candidates, maxRecommendations, maxPerHotspot, sectorGate, settings) {
			code := normalizeAStockCode(rec.Code)
			if code == "" {
				continue
			}
			if _, exists := seen[code]; exists {
				continue
			}
			hotspotKey := normalizeAStockRecommendationHotspot(rec.Hotspot)
			if hotspotKey != "" {
				if _, exists := hotspotCounts[hotspotKey]; !exists && len(hotspotCounts) >= settings.Auction.HotspotLimit {
					continue
				}
				if hotspotCounts[hotspotKey] >= maxPerHotspot {
					continue
				}
			}
			seen[code] = struct{}{}
			rec.Code = code
			rec.Rank = len(combined) + 1
			rec.EntryTime = aStockRecommendationEffectiveEntryTime(aStockRecommendation{EntryTime: snapshot.Label}, periodKey)
			if snapshot.Label != "" {
				rec.Reason = rec.Reason + "，生成点 " + snapshot.Label
			}
			combined = append(combined, rec)
			if hotspotKey != "" {
				hotspotCounts[hotspotKey]++
			}
			if maxRecommendations > 0 && len(combined) >= maxRecommendations {
				return combined
			}
		}
	}
	if len(combined) == 0 && len(articles) > 0 {
		return withAStockRecommendationEntryTimes(buildAStockRecommendationsWithLimitAndSectorGateWithSettings(buildAStockHotspotsWithSettings(articles, settings), candidates, maxRecommendations, maxPerHotspot, sectorGate, settings), periodKey, "")
	}
	return combined
}

func withAStockRecommendationEntryTimes(recommendations []aStockRecommendation, period string, entryTimeOverride string) []aStockRecommendation {
	entryTimeOverride = normalizeAStockGeneratedEntryTime(period, entryTimeOverride)
	for i := range recommendations {
		if entryTimeOverride != "" {
			recommendations[i].EntryTime = entryTimeOverride
			continue
		}
		recommendations[i].EntryTime = aStockRecommendationEffectiveEntryTime(recommendations[i], period)
	}
	return recommendations
}

func normalizeAStockRecommendationHotspot(value string) string {
	return strings.TrimSpace(value)
}

func newAStockHotspotSectorGate() *aStockHotspotSectorGate {
	return &aStockHotspotSectorGate{
		codesByHotspot: make(map[string]map[string]struct{}),
		gatedHotspots:  make(map[string]struct{}),
	}
}

func (g *aStockHotspotSectorGate) addHotspot(hotspot string) {
	if g == nil {
		return
	}
	hotspot = normalizeAStockRecommendationHotspot(hotspot)
	if hotspot == "" {
		return
	}
	if g.gatedHotspots == nil {
		g.gatedHotspots = make(map[string]struct{})
	}
	g.gatedHotspots[hotspot] = struct{}{}
}

func (g *aStockHotspotSectorGate) addCodes(hotspot string, codes map[string]struct{}) {
	if g == nil {
		return
	}
	hotspot = normalizeAStockRecommendationHotspot(hotspot)
	if hotspot == "" {
		return
	}
	g.addHotspot(hotspot)
	if len(codes) == 0 {
		return
	}
	if g.codesByHotspot == nil {
		g.codesByHotspot = make(map[string]map[string]struct{})
	}
	target := g.codesByHotspot[hotspot]
	if target == nil {
		target = make(map[string]struct{}, len(codes))
		g.codesByHotspot[hotspot] = target
	}
	for code := range codes {
		code = normalizeAStockCode(code)
		if astockcode.IsShanghaiShenzhen(code) {
			target[code] = struct{}{}
		}
	}
	if len(target) == 0 {
		delete(g.codesByHotspot, hotspot)
	}
}

func (g *aStockHotspotSectorGate) empty() bool {
	return g == nil || len(g.gatedHotspots) == 0
}

func (g *aStockHotspotSectorGate) Allows(hotspot aStockHotspot, candidate aStockMarketCandidate) bool {
	if g == nil || !candidate.Fallback || hasAStockCandidateSource(candidate, aStockCandidateSourceNews) {
		return true
	}
	hotspotName := normalizeAStockRecommendationHotspot(hotspot.Name)
	if _, gated := g.gatedHotspots[hotspotName]; !gated {
		return true
	}
	codes := g.codesByHotspot[hotspotName]
	if len(codes) == 0 {
		return len(aStockCandidateKeywordMatches(candidate.Name, hotspot.Keywords)) > 0
	}
	_, ok := codes[normalizeAStockCode(candidate.Code)]
	return ok
}

func aStockHotspotSectorAliases(hotspot string) []aStockHotspotSectorAlias {
	switch normalizeAStockRecommendationHotspot(hotspot) {
	case "人工智能":
		return []aStockHotspotSectorAlias{
			{SectorType: "概念资金流", SectorName: "人工智能"},
			{SectorType: "概念资金流", SectorName: "机器人概念"},
			{SectorType: "概念资金流", SectorName: "人形机器人"},
			{SectorType: "概念资金流", SectorName: "机器人执行器"},
			{SectorType: "行业资金流", SectorName: "机器人"},
		}
	case "半导体":
		return []aStockHotspotSectorAlias{
			{SectorType: "行业资金流", SectorName: "半导体"},
			{SectorType: "概念资金流", SectorName: "芯片概念"},
			{SectorType: "概念资金流", SectorName: "存储芯片"},
			{SectorType: "概念资金流", SectorName: "光刻机"},
			{SectorType: "概念资金流", SectorName: "先进封装"},
		}
	case "新能源":
		return []aStockHotspotSectorAlias{
			{SectorType: "行业资金流", SectorName: "光伏设备"},
			{SectorType: "行业资金流", SectorName: "电池"},
			{SectorType: "行业资金流", SectorName: "风电设备"},
			{SectorType: "行业资金流", SectorName: "能源金属"},
			{SectorType: "概念资金流", SectorName: "储能"},
			{SectorType: "概念资金流", SectorName: "锂电池"},
			{SectorType: "概念资金流", SectorName: "光伏概念"},
			{SectorType: "概念资金流", SectorName: "风能"},
		}
	case "低空经济":
		return []aStockHotspotSectorAlias{
			{SectorType: "概念资金流", SectorName: "低空经济"},
			{SectorType: "概念资金流", SectorName: "飞行汽车(eVTOL)"},
			{SectorType: "概念资金流", SectorName: "无人机"},
		}
	case "金融券商":
		return []aStockHotspotSectorAlias{
			{SectorType: "行业资金流", SectorName: "证券"},
			{SectorType: "行业资金流", SectorName: "证券Ⅱ"},
			{SectorType: "行业资金流", SectorName: "证券Ⅲ"},
			{SectorType: "行业资金流", SectorName: "银行"},
			{SectorType: "行业资金流", SectorName: "保险"},
			{SectorType: "概念资金流", SectorName: "券商概念"},
		}
	case "黄金有色":
		return []aStockHotspotSectorAlias{
			{SectorType: "行业资金流", SectorName: "黄金"},
			{SectorType: "行业资金流", SectorName: "有色金属"},
			{SectorType: "行业资金流", SectorName: "贵金属"},
			{SectorType: "行业资金流", SectorName: "稀土"},
			{SectorType: "概念资金流", SectorName: "黄金概念"},
			{SectorType: "概念资金流", SectorName: "稀土永磁"},
		}
	case "医药生物":
		return []aStockHotspotSectorAlias{
			{SectorType: "行业资金流", SectorName: "医药生物"},
			{SectorType: "行业资金流", SectorName: "医药商业"},
			{SectorType: "行业资金流", SectorName: "医药流通"},
			{SectorType: "行业资金流", SectorName: "化学制剂"},
			{SectorType: "行业资金流", SectorName: "中药"},
			{SectorType: "行业资金流", SectorName: "医疗器械"},
			{SectorType: "概念资金流", SectorName: "创新药"},
			{SectorType: "概念资金流", SectorName: "单抗概念"},
			{SectorType: "概念资金流", SectorName: "医药医疗风格"},
		}
	case "消费电子":
		return []aStockHotspotSectorAlias{
			{SectorType: "行业资金流", SectorName: "消费电子"},
			{SectorType: "概念资金流", SectorName: "苹果概念"},
			{SectorType: "概念资金流", SectorName: "华为概念"},
			{SectorType: "概念资金流", SectorName: "MR"},
			{SectorType: "概念资金流", SectorName: "AR"},
			{SectorType: "概念资金流", SectorName: "VR"},
		}
	case "房地产":
		return []aStockHotspotSectorAlias{
			{SectorType: "行业资金流", SectorName: "房地产"},
			{SectorType: "行业资金流", SectorName: "房地产开发"},
			{SectorType: "行业资金流", SectorName: "房地产服务"},
			{SectorType: "概念资金流", SectorName: "物业管理"},
			{SectorType: "概念资金流", SectorName: "租售同权"},
		}
	case "军工航天":
		return []aStockHotspotSectorAlias{
			{SectorType: "行业资金流", SectorName: "航天航空"},
			{SectorType: "行业资金流", SectorName: "航空装备"},
			{SectorType: "行业资金流", SectorName: "军工"},
			{SectorType: "概念资金流", SectorName: "军工"},
			{SectorType: "概念资金流", SectorName: "商业航天"},
			{SectorType: "概念资金流", SectorName: "卫星导航"},
		}
	default:
		return nil
	}
}

func (s *Server) loadAStockHotspotSectorGateWithCache(hotspots []aStockHotspot, cache *aStockRequestCache) *aStockHotspotSectorGate {
	if len(hotspots) == 0 || strings.TrimSpace(s.cfg.AStockAuctionURL) == "" {
		return nil
	}
	gate := newAStockHotspotSectorGate()
	for _, hotspot := range hotspots {
		aliases := aStockHotspotSectorAliases(hotspot.Name)
		if len(aliases) == 0 {
			continue
		}
		gate.addHotspot(hotspot.Name)
		for _, alias := range aliases {
			gate.addCodes(hotspot.Name, s.loadAStockSectorConstituentCodesWithCache(alias, cache))
		}
	}
	if gate.empty() {
		return nil
	}
	return gate
}

func (s *Server) loadAStockSectorConstituentCodesWithCache(alias aStockHotspotSectorAlias, cache *aStockRequestCache) map[string]struct{} {
	return s.loadAStockSectorConstituentEntryWithCache(alias, cache).codes
}

func (s *Server) loadAStockSectorConstituentCodeNamesWithCache(alias aStockHotspotSectorAlias, cache *aStockRequestCache) map[string]string {
	return s.loadAStockSectorConstituentEntryWithCache(alias, cache).names
}

func (s *Server) loadAStockSectorConstituentEntryWithCache(alias aStockHotspotSectorAlias, cache *aStockRequestCache) aStockSectorConstituentCodesCacheEntry {
	sectorType := normalizeSectorFundFlowSectorType(alias.SectorType)
	sectorName := strings.TrimSpace(alias.SectorName)
	if sectorName == "" {
		return aStockSectorConstituentCodesCacheEntry{}
	}
	key := sectorType + "|" + sectorName
	if cache != nil {
		if cache.sectorConstituents == nil {
			cache.sectorConstituents = make(map[string]aStockSectorConstituentCodesCacheEntry)
		}
		if entry, ok := cache.sectorConstituents[key]; ok && entry.found {
			return entry
		}
	}
	items := s.loadAStockCachedSectorConstituents(sectorType, sectorName)
	if len(items) == 0 {
		refreshed, _, err := s.refreshSectorFundFlowConstituents(model.AStockSectorFundFlowListResult{
			SectorType: sectorType,
			Indicator:  "今日",
		}, sectorName)
		if err != nil {
			items = nil
		} else {
			items = refreshed
		}
	}
	entry := aStockSectorConstituentCodesCacheEntry{
		codes: aStockSectorConstituentCodes(items),
		names: aStockSectorConstituentCodeNames(items),
		found: true,
	}
	if cache != nil {
		cache.sectorConstituents[key] = entry
	}
	return entry
}

func (s *Server) loadAStockCachedSectorConstituentCodes(sectorType string, sectorName string) map[string]struct{} {
	return aStockSectorConstituentCodes(s.loadAStockCachedSectorConstituents(sectorType, sectorName))
}

func (s *Server) loadAStockCachedSectorConstituents(sectorType string, sectorName string) []model.AStockSectorConstituent {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return nil
	}
	query := url.Values{}
	query.Set("sector_type", normalizeSectorFundFlowSectorType(sectorType))
	query.Set("sector_name", strings.TrimSpace(sectorName))
	query.Set("limit", fmt.Sprintf("%d", sectorFundFlowStockPageSize))
	var result model.AStockSectorConstituentListResult
	if err := s.getJSON(s.cfg.ContentURL+"/api/v1/a-stock/sector-constituents?"+query.Encode(), &result); err != nil {
		return nil
	}
	return result.Items
}

func aStockSectorConstituentCodes(items []model.AStockSectorConstituent) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	codes := make(map[string]struct{}, len(items))
	for _, item := range items {
		code := normalizeAStockCode(item.Code)
		if astockcode.IsShanghaiShenzhen(code) {
			codes[code] = struct{}{}
		}
	}
	if len(codes) == 0 {
		return nil
	}
	return codes
}

func aStockSectorConstituentCodeNames(items []model.AStockSectorConstituent) map[string]string {
	if len(items) == 0 {
		return nil
	}
	names := make(map[string]string, len(items))
	for _, item := range items {
		code := normalizeAStockCode(item.Code)
		name := astockcode.DisplayName(code, item.Name)
		if astockcode.IsShanghaiShenzhen(code) && hasResolvedAStockRecommendationName(code, name) {
			names[code] = name
		}
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

func aStockRecommendationSnapshots(strategyDate string, periodKey string, phase string) []aStockRecommendationSnapshot {
	location := aStockLocation()
	day, err := time.ParseInLocation("2006-01-02", normalizeAStockStrategyDate(strategyDate), location)
	if err != nil {
		return nil
	}
	period := normalizeAStockPeriod(periodKey)
	normalizedPhase := normalizeAStockRecommendationPhase(phase)
	if period.Key == "evening" {
		return []aStockRecommendationSnapshot{
			{Label: "18:30", Start: time.Date(day.Year(), day.Month(), day.Day(), 15, 0, 0, 0, location), End: time.Date(day.Year(), day.Month(), day.Day(), 18, 30, 59, 0, location)},
		}
	}
	if period.Key == "afternoon" {
		start := time.Date(day.Year(), day.Month(), day.Day(), 9, 30, 0, 0, location)
		if normalizedPhase == aStockRecommendationPhasePreopen {
			return []aStockRecommendationSnapshot{
				{Label: "12:57", Start: start, End: time.Date(day.Year(), day.Month(), day.Day(), 12, 56, 59, 0, location)},
			}
		}
		return []aStockRecommendationSnapshot{
			{Label: "13:00", Start: start, End: time.Date(day.Year(), day.Month(), day.Day(), 13, 0, 59, 0, location)},
		}
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), 8, 0, 0, 0, location)
	if normalizedPhase == aStockRecommendationPhasePreopen {
		return []aStockRecommendationSnapshot{
			{Label: "09:27", Start: start, End: time.Date(day.Year(), day.Month(), day.Day(), 9, 26, 59, 0, location)},
		}
	}
	return []aStockRecommendationSnapshot{
		{Label: "09:27", Start: start, End: time.Date(day.Year(), day.Month(), day.Day(), 9, 26, 59, 0, location)},
	}
}

func filterAStockArticlesByPublishWindow(items []model.Item, start time.Time, end time.Time) []model.Item {
	filtered := make([]model.Item, 0, len(items))
	for _, item := range items {
		publishedAt, ok := aStockItemPublishTime(item)
		if !ok {
			continue
		}
		if publishedAt.Before(start) || publishedAt.After(end) {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func aStockItemPublishTime(item model.Item) (time.Time, bool) {
	location := aStockLocation()
	for _, value := range []string{item.PublishTime, item.PublishTimeText} {
		value = strings.TrimSpace(value)
		if value == "" || strings.Contains(value, "前") || strings.Contains(value, "刚刚") {
			continue
		}
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02"} {
			if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
				return parsed.In(location), true
			}
		}
	}
	if !item.CapturedAt.IsZero() {
		return item.CapturedAt.In(location), true
	}
	return time.Time{}, false
}

func aStockRecommendationCodeSet(recommendations []aStockRecommendation) map[string]struct{} {
	if len(recommendations) == 0 {
		return nil
	}
	codes := make(map[string]struct{}, len(recommendations))
	for _, rec := range recommendations {
		if code := normalizeAStockCode(rec.Code); code != "" {
			codes[code] = struct{}{}
		}
	}
	return codes
}

func aStockRecommendationCodes(recommendations []aStockRecommendation) []string {
	if len(recommendations) == 0 {
		return nil
	}
	codes := make([]string, 0, len(recommendations))
	seen := make(map[string]struct{}, len(recommendations))
	for _, rec := range recommendations {
		code := normalizeAStockCode(rec.Code)
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	return codes
}

func mergeAStockLimitUpReplacementPool(base []aStockRecommendation, replacementPool []aStockRecommendation) []aStockRecommendation {
	if len(base) == 0 || len(replacementPool) <= len(base) {
		return base
	}
	merged := make([]aStockRecommendation, 0, len(replacementPool))
	merged = append(merged, base...)
	seen := aStockRecommendationCodeSet(base)
	lowScore := 0
	if len(base) > 0 {
		lowScore = base[0].MarketScore
		if lowScore == 0 {
			lowScore = base[0].HotspotScore
		}
		for _, rec := range base[1:] {
			score := rec.MarketScore
			if score == 0 {
				score = rec.HotspotScore
			}
			if score < lowScore {
				lowScore = score
			}
		}
	}
	for _, rec := range replacementPool {
		code := normalizeAStockCode(rec.Code)
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		rec.Code = code
		nextScore := lowScore - len(merged) - 1
		previousScore := rec.MarketScore
		if previousScore == 0 {
			previousScore = rec.HotspotScore
		}
		rec.MarketScore = nextScore
		appendAStockScoreAdjustment(&rec, "递补排序", fmt.Sprintf("递补候选调整到 %d 分", nextScore), nextScore-previousScore)
		rec.Reason = appendAStockReason(rec.Reason, "作为过滤递补候选")
		rec.Rank = len(merged) + 1
		merged = append(merged, rec)
		seen[code] = struct{}{}
	}
	return rerankAStockRecommendations(merged)
}

func aStockRecommendationHotspotCounts(recommendations []aStockRecommendation) map[string]int {
	if len(recommendations) == 0 {
		return nil
	}
	counts := make(map[string]int)
	for _, rec := range recommendations {
		hotspot := normalizeAStockRecommendationHotspot(rec.Hotspot)
		if hotspot != "" {
			counts[hotspot]++
		}
	}
	return counts
}

func filterAStockRecommendationsByMorningHotspotQuota(recommendations []aStockRecommendation, morningHotspotCounts map[string]int, dailyLimit int) ([]aStockRecommendation, int) {
	if len(recommendations) == 0 || len(morningHotspotCounts) == 0 || dailyLimit <= 0 {
		return rerankAStockRecommendations(recommendations), 0
	}
	filtered := make([]aStockRecommendation, 0, len(recommendations))
	afternoonCounts := make(map[string]int)
	skipped := 0
	for _, rec := range recommendations {
		hotspot := normalizeAStockRecommendationHotspot(rec.Hotspot)
		if hotspot != "" {
			remaining := dailyLimit - morningHotspotCounts[hotspot]
			if remaining <= 0 || afternoonCounts[hotspot] >= remaining {
				skipped++
				continue
			}
			afternoonCounts[hotspot]++
		}
		filtered = append(filtered, rec)
	}
	return rerankAStockRecommendations(filtered), skipped
}

func remainingAStockDailyRecommendationLimit(morningCount int) int {
	return remainingAStockDailyRecommendationLimitWithSettings(morningCount, defaultAStockAlgorithmSettings())
}

func remainingAStockDailyRecommendationLimitWithSettings(morningCount int, settings model.AStockRecommendationAlgorithmSettings) int {
	remaining := settings.Auction.RecommendationLimit - morningCount
	if remaining < 0 {
		return 0
	}
	return remaining
}

func isAStockRecoveredRecommendation(rec aStockRecommendation) bool {
	return rec.Recovered || strings.TrimSpace(rec.RecoveryReason) != "" || strings.Contains(rec.Reason, "收盘确认开板恢复")
}

func countAStockDailyLimitRecommendations(recommendations []aStockRecommendation) int {
	count := 0
	for _, rec := range recommendations {
		if !isAStockRecoveredRecommendation(rec) {
			count++
		}
	}
	return count
}

func countAStockRecoveredRecommendations(recommendations []aStockRecommendation) int {
	count := 0
	for _, rec := range recommendations {
		if isAStockRecoveredRecommendation(rec) {
			count++
		}
	}
	return count
}

func limitAStockRecommendationsByCount(recommendations []aStockRecommendation, maxRecommendations int) ([]aStockRecommendation, int) {
	if len(recommendations) == 0 {
		return recommendations, 0
	}
	if maxRecommendations < 0 {
		maxRecommendations = 0
	}
	filtered := make([]aStockRecommendation, 0, len(recommendations))
	regularCount := 0
	skipped := 0
	for _, rec := range recommendations {
		if isAStockRecoveredRecommendation(rec) {
			filtered = append(filtered, rec)
			continue
		}
		if regularCount >= maxRecommendations {
			skipped++
			continue
		}
		regularCount++
		filtered = append(filtered, rec)
	}
	return rerankAStockRecommendations(filtered), skipped
}

func aStockRecommendationSelectionEntryTime(item model.AStockRecommendationSelection, fallbackPeriod string) string {
	period := strings.TrimSpace(item.Period)
	if period == "" {
		period = strings.TrimSpace(fallbackPeriod)
	}
	if raw := strings.TrimSpace(item.EntryTime); raw != "" {
		if period == "" {
			return normalizeAStockEntryTimeMinute(raw)
		}
		return normalizeAStockGeneratedEntryTime(period, raw)
	}
	if !strings.Contains(item.Reason, "生成点") {
		return ""
	}
	if period == "" {
		return aStockRecommendationRawEntryTimeFromReason(item.Reason)
	}
	return aStockRecommendationEntryTimeFromReason(item.Reason, period)
}

func aStockRecommendationSelectionsToRecommendations(items []model.AStockRecommendationSelection, fallbackPeriod ...string) []aStockRecommendation {
	period := ""
	if len(fallbackPeriod) > 0 {
		period = strings.TrimSpace(fallbackPeriod[0])
	}
	recommendations := make([]aStockRecommendation, 0, len(items))
	for _, item := range items {
		code := normalizeAStockCode(item.Code)
		if code == "" {
			continue
		}
		recommendations = append(recommendations, aStockRecommendation{
			Rank:         item.Rank,
			Hotspot:      strings.TrimSpace(item.Hotspot),
			Code:         code,
			Name:         astockcode.DisplayName(code, item.Name),
			HotspotScore: item.HotspotScore,
			MarketScore:  item.MarketScore,
			Reason:       strings.TrimSpace(item.Reason),
			EntryTime:    aStockRecommendationSelectionEntryTime(item, period),
		})
	}
	sort.SliceStable(recommendations, func(i, j int) bool {
		if recommendations[i].Rank == recommendations[j].Rank {
			return recommendations[i].Code < recommendations[j].Code
		}
		return recommendations[i].Rank < recommendations[j].Rank
	})
	for i := range recommendations {
		if recommendations[i].Rank <= 0 {
			recommendations[i].Rank = i + 1
		}
	}
	return recommendations
}

func aStockRecommendationsToSelectionItems(recommendations []aStockRecommendation, strategyDate string, period string) []model.AStockRecommendationSelection {
	items := make([]model.AStockRecommendationSelection, 0, len(recommendations))
	for i, rec := range recommendations {
		code := normalizeAStockCode(rec.Code)
		if code == "" {
			continue
		}
		if isBlockedAStockRecommendationStock(code, rec.Name) {
			continue
		}
		rank := rec.Rank
		if rank <= 0 {
			rank = i + 1
		}
		items = append(items, model.AStockRecommendationSelection{
			StrategyDate: strategyDate,
			Period:       period,
			Rank:         rank,
			Hotspot:      strings.TrimSpace(rec.Hotspot),
			Code:         code,
			Name:         astockcode.DisplayName(code, rec.Name),
			HotspotScore: rec.HotspotScore,
			MarketScore:  rec.MarketScore,
			Reason:       strings.TrimSpace(rec.Reason),
			EntryTime:    aStockRecommendationEffectiveEntryTime(rec, period),
		})
	}
	return items
}

func aStockRecommendationNameMap(recommendations []aStockRecommendation) map[string]string {
	names := make(map[string]string, len(recommendations))
	for _, rec := range recommendations {
		code := normalizeAStockCode(rec.Code)
		if code == "" {
			continue
		}
		names[code] = astockcode.DisplayName(code, rec.Name)
	}
	return names
}

func countAStockRecommendationNameChanges(original map[string]string, recommendations []aStockRecommendation) int {
	changed := 0
	for _, rec := range recommendations {
		code := normalizeAStockCode(rec.Code)
		if code == "" {
			continue
		}
		if astockcode.DisplayName(code, rec.Name) != strings.TrimSpace(original[code]) {
			changed++
		}
	}
	return changed
}

func countAStockRecommendationTextRepairs(recommendations []aStockRecommendation) int {
	changed := 0
	for _, rec := range recommendations {
		if aStockRecommendationNeedsTextRepair(rec) {
			changed++
		}
	}
	return changed
}

func formatAStockExDividendFilterStatus(filtered int) string {
	if filtered <= 0 {
		return ""
	}
	return fmt.Sprintf("除权除息过滤股票 %d", filtered)
}

func (s *Server) applyAStockRecommendationOutputFiltersWithCache(ctx *aStockContext, cache *aStockRequestCache) (int, int) {
	exDividendFiltered := s.applyAStockExDividendFilterWithCache(ctx, cache)
	dailyLimitFiltered := applyAStockRecommendationOutputDailyLimitWithSettings(ctx, s.loadAStockAlgorithmSettingsWithCache(cache))
	return exDividendFiltered, dailyLimitFiltered
}

func applyAStockRecommendationOutputDailyLimit(ctx *aStockContext) int {
	return applyAStockRecommendationOutputDailyLimitWithSettings(ctx, defaultAStockAlgorithmSettings())
}

func applyAStockRecommendationOutputDailyLimitWithSettings(ctx *aStockContext, settings model.AStockRecommendationAlgorithmSettings) int {
	if ctx == nil || len(ctx.Recommendations) == 0 {
		return 0
	}
	filtered, skipped := limitAStockRecommendationsByCount(ctx.Recommendations, settings.Auction.RecommendationLimit)
	if skipped > 0 {
		ctx.Recommendations = filtered
		ctx.SameDayMorningFiltered += skipped
	}
	return skipped
}

func (s *Server) applyAStockExDividendFilterWithCache(ctx *aStockContext, cache *aStockRequestCache) int {
	if ctx == nil || len(ctx.Recommendations) == 0 {
		return 0
	}
	blockedCodes := aStockExDividendMarkedRecommendationCodes(ctx.Recommendations)
	if events, err := s.loadAStockDividendEventsWithCache(ctx.Date, aStockRecommendationCodes(ctx.Recommendations), cache); err == nil {
		for _, event := range events.Items {
			code := normalizeAStockCode(event.Code)
			if code == "" {
				continue
			}
			if blockedCodes == nil {
				blockedCodes = make(map[string]struct{})
			}
			blockedCodes[code] = struct{}{}
		}
	}
	filtered, skipped := filterAStockRecommendationsByCodes(ctx.Recommendations, blockedCodes)
	if skipped > 0 {
		ctx.Recommendations = filtered
		ctx.ExDividendFiltered += skipped
	}
	return skipped
}

func aStockExDividendMarkedRecommendationCodes(recommendations []aStockRecommendation) map[string]struct{} {
	if len(recommendations) == 0 {
		return nil
	}
	blockedCodes := make(map[string]struct{})
	for _, rec := range recommendations {
		code := normalizeAStockCode(rec.Code)
		if code == "" || !isAStockExDividendMarkedName(rec.Name) {
			continue
		}
		blockedCodes[code] = struct{}{}
	}
	if len(blockedCodes) == 0 {
		return nil
	}
	return blockedCodes
}

func isAStockExDividendMarkedName(name string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(name))
	return strings.HasPrefix(normalized, "XD") || strings.HasPrefix(normalized, "XR") || strings.HasPrefix(normalized, "DR")
}

func (s *Server) loadAStockDividendEventsWithCache(strategyDate string, codes []string, cache *aStockRequestCache) (aStockDividendEventResult, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(s.cfg.AStockAuctionURL), "/")
	if baseURL == "" {
		return aStockDividendEventResult{}, fmt.Errorf("a-stock auction url is empty")
	}
	date := normalizeAStockStrategyDate(strategyDate)
	normalizedCodes := normalizeAStockCodeList(codes)
	if len(normalizedCodes) == 0 {
		return aStockDividendEventResult{}, fmt.Errorf("empty stock codes")
	}
	windowDays := s.loadAStockAlgorithmSettingsWithCache(cache).Volatility.ExDividendWindowDays
	cacheKeyCodes := append([]string(nil), normalizedCodes...)
	sort.Strings(cacheKeyCodes)
	cacheKey := strings.Join([]string{date, fmt.Sprint(windowDays), strings.Join(cacheKeyCodes, ",")}, "|")
	if cache != nil {
		if entry, ok := cache.dividendEvents[cacheKey]; ok {
			return entry.result, entry.err
		}
	}
	query := url.Values{}
	query.Set("date", date)
	query.Set("codes", strings.Join(normalizedCodes, ","))
	query.Set("window_days", fmt.Sprint(windowDays))
	result := aStockDividendEventResult{}
	resp, err := s.client.R().SetResult(&result).Get(baseURL + "/api/a-stock/dividend-events?" + query.Encode())
	if err == nil && !resp.IsSuccess() {
		err = fmt.Errorf(resp.Status())
	}
	if cache != nil {
		cache.dividendEvents[cacheKey] = aStockDividendEventCacheEntry{result: result, err: err}
	}
	return result, err
}

func filterAStockRecommendationsByCodes(recommendations []aStockRecommendation, blockedCodes map[string]struct{}) ([]aStockRecommendation, int) {
	if len(recommendations) == 0 || len(blockedCodes) == 0 {
		return rerankAStockRecommendations(recommendations), 0
	}
	filtered := make([]aStockRecommendation, 0, len(recommendations))
	skipped := 0
	for _, rec := range recommendations {
		if _, blocked := blockedCodes[normalizeAStockCode(rec.Code)]; blocked {
			skipped++
			continue
		}
		filtered = append(filtered, rec)
	}
	return rerankAStockRecommendations(filtered), skipped
}

func filterBlockedAStockRecommendations(recommendations []aStockRecommendation) ([]aStockRecommendation, int) {
	if len(recommendations) == 0 {
		return recommendations, 0
	}
	filtered := make([]aStockRecommendation, 0, len(recommendations))
	skipped := 0
	for _, rec := range recommendations {
		if isBlockedAStockRecommendationStock(rec.Code, rec.Name) {
			skipped++
			continue
		}
		filtered = append(filtered, rec)
	}
	return rerankAStockRecommendations(filtered), skipped
}

func (s *Server) repairAStockPersistedRecommendationsWithCache(strategyDate string, recommendations []aStockRecommendation, cache *aStockRequestCache) ([]aStockRecommendation, int) {
	return s.repairAStockPersistedRecommendationsForPeriodWithCache(strategyDate, "", recommendations, cache)
}

func (s *Server) repairAStockPersistedRecommendationsForPeriodWithCache(strategyDate string, period string, recommendations []aStockRecommendation, cache *aStockRequestCache) ([]aStockRecommendation, int) {
	if len(recommendations) == 0 {
		return recommendations, 0
	}
	resolver := newAStockRecommendationNameResolver(nil)
	if needsAStockMarketNameResolver(recommendations, resolver) {
		resolver = s.loadAStockRecommendationNameResolverWithCache(strategyDate, recommendations, cache)
	}
	if needsAStockMarketNameResolver(recommendations, resolver) {
		s.addEastmoneyAStockNamesToResolver(resolver, recommendations)
	}
	filtered := make([]aStockRecommendation, 0, len(recommendations))
	skipped := 0
	for _, rec := range recommendations {
		code := normalizeAStockCode(rec.Code)
		name := resolveAStockRecommendationName(code, rec.Name, resolver)
		if !astockcode.IsShanghaiShenzhen(code) || !hasResolvedAStockRecommendationName(code, name) {
			skipped++
			continue
		}
		if isBlockedAStockRecommendationStock(code, name) {
			skipped++
			continue
		}
		rec.Code = code
		rec.Name = name
		if aStockRecommendationNeedsTextRepair(rec) {
			rec = s.repairAStockRecommendationTextWithCache(strategyDate, period, rec, recommendations, cache)
		}
		filtered = append(filtered, rec)
	}
	return rerankAStockRecommendations(filtered), skipped
}

func (s *Server) repairAStockRecommendationTextWithCache(strategyDate string, period string, rec aStockRecommendation, recommendations []aStockRecommendation, cache *aStockRequestCache) aStockRecommendation {
	baselineByCode := s.loadAStockRecommendationTextRepairBaselineWithCache(strategyDate, period, recommendations, cache)
	if baseline, ok := baselineByCode[normalizeAStockCode(rec.Code)]; ok {
		return mergeAStockRecommendationTextRepairBaseline(rec, baseline)
	}
	return sanitizeAStockGarbledRecommendationText(rec)
}

func (s *Server) loadAStockRecommendationTextRepairBaselineWithCache(strategyDate string, period string, recommendations []aStockRecommendation, cache *aStockRequestCache) map[string]aStockRecommendation {
	period = inferAStockRecommendationRepairPeriod(period, recommendations)
	if period == "" || strings.TrimSpace(s.cfg.ContentURL) == "" {
		return nil
	}
	start, end := aStockWindow(strategyDate, period)
	articles, err := s.loadAStockWindowArticlesWithCache(start, end, cache)
	if err != nil || len(articles) == 0 {
		return nil
	}
	settings, _ := astocknews.LoadSettings("")
	articles = astocknews.FilterRecommendationItems(articles, settings)
	if len(articles) == 0 {
		return nil
	}
	candidates, _, _ := s.loadAStockMarketCandidatesWithStatusWithCache(strategyDate, cache)
	if len(candidates) == 0 {
		return nil
	}
	hotspots := buildAStockHotspots(articles)
	if len(hotspots) == 0 {
		return nil
	}
	sectorGate := s.loadAStockHotspotSectorGateWithCache(hotspots, cache)
	rebuilt := buildAStockSnapshotRecommendationsWithPhaseAndLimitAndSectorGate(strategyDate, period, aStockRecommendationPhaseFinal, articles, candidates, aStockReplacementPoolLimit, aStockReplacementPerHotspot, sectorGate)
	if len(rebuilt) == 0 {
		rebuilt = buildAStockSnapshotReplacementRecommendations(strategyDate, period, aStockRecommendationPhaseFinal, articles, candidates)
	}
	baselineByCode := make(map[string]aStockRecommendation, len(rebuilt))
	for _, baseline := range rebuilt {
		code := normalizeAStockCode(baseline.Code)
		if code == "" {
			continue
		}
		if _, exists := baselineByCode[code]; !exists {
			baselineByCode[code] = baseline
		}
	}
	return baselineByCode
}

func inferAStockRecommendationRepairPeriod(period string, recommendations []aStockRecommendation) string {
	normalized := strings.TrimSpace(period)
	if normalized != "" {
		return normalizeAStockPeriod(normalized).Key
	}
	for _, rec := range recommendations {
		entry := aStockRecommendationEffectiveEntryTime(rec, "")
		if strings.HasPrefix(entry, "13:") {
			return "afternoon"
		}
	}
	for _, rec := range recommendations {
		entry := aStockRecommendationEffectiveEntryTime(rec, "")
		if strings.HasPrefix(entry, "09:") {
			return "morning"
		}
	}
	return ""
}

func mergeAStockRecommendationTextRepairBaseline(rec aStockRecommendation, baseline aStockRecommendation) aStockRecommendation {
	hotspotWasGarbled := isGarbledAStockText(rec.Hotspot)
	reasonWasGarbled := isGarbledAStockText(rec.Reason)
	breakdownWasGarbled := aStockScoreBreakdownHasGarbledText(rec.ScoreBreakdown)
	if hotspotWasGarbled || strings.TrimSpace(rec.Hotspot) == "" {
		rec.Hotspot = strings.TrimSpace(baseline.Hotspot)
	}
	if rec.HotspotScore == 0 || hotspotWasGarbled {
		rec.HotspotScore = baseline.HotspotScore
	}
	if reasonWasGarbled || strings.TrimSpace(rec.Reason) == "" {
		rec.Reason = strings.TrimSpace(baseline.Reason)
	}
	if len(rec.ScoreBreakdown) == 0 || breakdownWasGarbled || reasonWasGarbled || hotspotWasGarbled {
		rec.ScoreBreakdown = mergeAStockRecommendationScoreBreakdown(baseline.ScoreBreakdown, rec.ScoreBreakdown)
	}
	baseReason := strings.TrimSpace(baseline.Reason)
	for _, component := range rec.ScoreBreakdown {
		if isGarbledAStockText(component.Label) || isGarbledAStockText(component.Detail) {
			continue
		}
		if aStockScoreBreakdownContainsComponent(baseline.ScoreBreakdown, component) {
			continue
		}
		if strings.TrimSpace(component.Detail) != "" {
			rec.Reason = appendAStockReason(rec.Reason, component.Detail)
		}
	}
	if strings.TrimSpace(rec.Reason) == "" {
		rec.Reason = baseReason
	}
	return sanitizeAStockGarbledRecommendationText(rec)
}

func sanitizeAStockGarbledRecommendationText(rec aStockRecommendation) aStockRecommendation {
	if isGarbledAStockText(rec.Hotspot) {
		rec.Hotspot = ""
	}
	if isGarbledAStockText(rec.Reason) {
		rec.Reason = aStockCleanReasonFromScoreBreakdown(rec.ScoreBreakdown)
	}
	if len(rec.ScoreBreakdown) > 0 {
		clean := rec.ScoreBreakdown[:0]
		for _, component := range rec.ScoreBreakdown {
			if isGarbledAStockText(component.Label) || isGarbledAStockText(component.Detail) {
				continue
			}
			clean = append(clean, component)
		}
		rec.ScoreBreakdown = clean
	}
	return rec
}

func aStockCleanReasonFromScoreBreakdown(components []aStockRecommendationScoreComponent) string {
	reasons := make([]string, 0, len(components))
	seen := make(map[string]struct{}, len(components))
	for _, component := range components {
		detail := strings.TrimSpace(component.Detail)
		if detail == "" || isGarbledAStockText(detail) {
			continue
		}
		if _, exists := seen[detail]; exists {
			continue
		}
		seen[detail] = struct{}{}
		reasons = append(reasons, detail)
	}
	return strings.Join(reasons, "，")
}

func mergeAStockRecommendationScoreBreakdown(base []aStockRecommendationScoreComponent, existing []aStockRecommendationScoreComponent) []aStockRecommendationScoreComponent {
	merged := make([]aStockRecommendationScoreComponent, 0, len(base)+len(existing))
	seen := make(map[string]struct{}, len(base)+len(existing))
	appendClean := func(component aStockRecommendationScoreComponent) {
		component.Label = strings.TrimSpace(component.Label)
		component.Detail = strings.TrimSpace(component.Detail)
		if component.Label == "" || isGarbledAStockText(component.Label) || isGarbledAStockText(component.Detail) {
			return
		}
		key := aStockScoreComponentKey(component)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		merged = append(merged, component)
	}
	for _, component := range base {
		appendClean(component)
	}
	for _, component := range existing {
		appendClean(component)
	}
	return merged
}

func aStockScoreBreakdownContainsComponent(components []aStockRecommendationScoreComponent, target aStockRecommendationScoreComponent) bool {
	targetKey := aStockScoreComponentKey(target)
	for _, component := range components {
		if aStockScoreComponentKey(component) == targetKey {
			return true
		}
	}
	return false
}

func aStockScoreComponentKey(component aStockRecommendationScoreComponent) string {
	return strings.TrimSpace(component.Label) + "|" + strings.TrimSpace(component.Detail) + "|" + strconv.Itoa(component.Score)
}

func aStockRecommendationNeedsTextRepair(rec aStockRecommendation) bool {
	return isGarbledAStockText(rec.Hotspot) || isGarbledAStockText(rec.Reason) || aStockScoreBreakdownHasGarbledText(rec.ScoreBreakdown)
}

func aStockScoreBreakdownHasGarbledText(components []aStockRecommendationScoreComponent) bool {
	for _, component := range components {
		if isGarbledAStockText(component.Label) || isGarbledAStockText(component.Detail) {
			return true
		}
	}
	return false
}

func isGarbledAStockText(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if strings.Contains(text, "\uFFFD") {
		return true
	}
	return strings.Contains(text, "??")
}

func needsAStockMarketNameResolver(recommendations []aStockRecommendation, resolver map[string]string) bool {
	for _, rec := range recommendations {
		code := normalizeAStockCode(rec.Code)
		if !astockcode.IsShanghaiShenzhen(code) {
			continue
		}
		if resolver != nil {
			resolved := astockcode.DisplayName(code, resolver[code])
			if hasResolvedAStockRecommendationName(code, resolved) && shouldUseResolvedAStockRecommendationName(code, rec.Name, resolved) {
				continue
			}
		}
		if shouldResolveAStockRecommendationName(code, rec.Name) {
			return true
		}
	}
	return false
}

func shouldResolveAStockRecommendationName(code string, name string) bool {
	name = astockcode.DisplayName(code, name)
	if !hasResolvedAStockRecommendationName(code, name) {
		return true
	}
	if isInvalidAStockRecommendationName(name) {
		return true
	}
	return len([]rune(name)) <= 2
}

func (s *Server) addEastmoneyAStockNamesToResolver(resolver map[string]string, recommendations []aStockRecommendation) {
	if resolver == nil {
		return
	}
	for _, rec := range recommendations {
		code := normalizeAStockCode(rec.Code)
		if !astockcode.IsShanghaiShenzhen(code) {
			continue
		}
		if resolved := astockcode.DisplayName(code, resolver[code]); hasResolvedAStockRecommendationName(code, resolved) {
			continue
		}
		name := s.fetchEastmoneyAStockName(code)
		addAStockRecommendationResolvedName(resolver, code, name)
	}
}

func (s *Server) fetchEastmoneyAStockName(code string) string {
	code = normalizeAStockCode(code)
	if !astockcode.IsShanghaiShenzhen(code) {
		return ""
	}
	resp, err := s.client.R().
		SetQueryParam("secid", eastmoneyAStockSecID(code)).
		SetQueryParam("fields", "f57,f58,f107").
		Get(aStockEastmoneyQuoteURL)
	if err != nil || !resp.IsSuccess() {
		return ""
	}
	var payload struct {
		Data struct {
			Code string `json:"f57"`
			Name string `json:"f58"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body(), &payload); err != nil {
		return ""
	}
	if normalizeAStockCode(payload.Data.Code) != code {
		return ""
	}
	return astockcode.DisplayName(code, payload.Data.Name)
}

func (s *Server) loadAStockRecommendationNameResolverWithCache(strategyDate string, recommendations []aStockRecommendation, cache *aStockRequestCache) map[string]string {
	resolver := newAStockRecommendationNameResolver(nil)
	for code, name := range s.loadAStockCodeNamesWithCache(aStockRecommendationCodes(recommendations), cache) {
		setAStockRecommendationResolvedName(resolver, code, name)
	}
	if !needsAStockMarketNameResolver(recommendations, resolver) {
		return resolver
	}
	var marketCandidates []aStockMarketCandidate
	if strings.TrimSpace(s.cfg.ContentURL) != "" {
		marketCandidates, _, _ = s.loadAStockMarketCandidatesWithStatusWithCache(strategyDate, cache)
	}
	for _, candidate := range marketCandidates {
		addAStockRecommendationResolvedName(resolver, candidate.Code, candidate.Name)
	}
	return resolver
}

func (s *Server) repairAStockRecommendationsForPersistence(strategyDate string, period string, recommendations []aStockRecommendation) ([]aStockRecommendation, int) {
	return s.repairAStockPersistedRecommendationsForPeriodWithCache(strategyDate, period, recommendations, newAStockRequestCache())
}

func filterBlockedAStockBacktests(rows []aStockBacktestRow) []aStockBacktestRow {
	if len(rows) == 0 {
		return rows
	}
	filtered := make([]aStockBacktestRow, 0, len(rows))
	for _, row := range rows {
		if isBlockedAStockRecommendationStock(aStockBacktestRowCode(row), row.Stock) {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}

func filterAStockBacktestsForSnapshotRecommendations(rows []aStockBacktestRow, recommendations []aStockRecommendation) []aStockBacktestRow {
	if len(rows) == 0 {
		return rows
	}
	if len(recommendations) == 0 {
		return []aStockBacktestRow{}
	}
	codes := make(map[string]struct{}, len(recommendations))
	for _, rec := range recommendations {
		code := normalizeAStockCode(rec.Code)
		if astockcode.IsShanghaiShenzhen(code) {
			codes[code] = struct{}{}
		}
	}
	filtered := make([]aStockBacktestRow, 0, len(rows))
	for _, row := range rows {
		if _, ok := codes[aStockBacktestRowCode(row)]; ok {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func filterAStockBacktestsForRecommendations(rows []aStockBacktestRow, recommendations []aStockRecommendation) []aStockBacktestRow {
	if len(rows) == 0 {
		return rows
	}
	if len(recommendations) == 0 {
		return []aStockBacktestRow{}
	}
	namesByCode := make(map[string]string, len(recommendations))
	for _, rec := range recommendations {
		code := normalizeAStockCode(rec.Code)
		name := astockcode.DisplayName(code, rec.Name)
		if astockcode.IsShanghaiShenzhen(code) && hasResolvedAStockRecommendationName(code, name) {
			namesByCode[code] = name
		}
	}
	filtered := make([]aStockBacktestRow, 0, len(rows))
	for _, row := range rows {
		code := aStockBacktestRowCode(row)
		name, ok := namesByCode[code]
		if !ok {
			continue
		}
		row.Stock = code + " " + name
		filtered = append(filtered, row)
	}
	return filtered
}

func isBlockedAStockRecommendationCandidate(candidate aStockMarketCandidate) bool {
	return isBlockedAStockRecommendationStock(candidate.Code, candidate.Name)
}

func isBlockedAStockRecommendationStock(code string, name string) bool {
	code = normalizeAStockCode(code)
	name = astockcode.DisplayName(code, name)
	if !astockcode.IsShanghaiShenzhen(code) || !hasResolvedAStockRecommendationName(code, name) {
		return true
	}
	if code == "300059" {
		return true
	}
	return false
}

func newsDerivedAStockMarketCandidates(hotspots []aStockHotspot) []aStockMarketCandidate {
	type aggregate struct {
		Candidate aStockMarketCandidate
		Keywords  map[string]struct{}
		Evidence  int
	}
	aggregates := make(map[string]*aggregate)
	order := make([]string, 0)
	for _, hotspot := range hotspots {
		for _, item := range hotspot.MatchedItems {
			if isAStockNegativeNewsItem(item) {
				continue
			}
			for _, mention := range extractAStockMentionsFromItem(item) {
				code := normalizeAStockCode(mention.Code)
				if !isAStockCode(code) {
					continue
				}
				name := strings.TrimSpace(mention.Name)
				if name == "" {
					name = inferAStockNameFromNews(item)
				}
				if name == "" {
					name = code
				}
				entry, ok := aggregates[code]
				if !ok {
					entry = &aggregate{
						Candidate: aStockMarketCandidate{
							Code:     code,
							Name:     name,
							Rank:     len(order) + 1,
							Fallback: true,
						},
						Keywords: make(map[string]struct{}),
					}
					aggregates[code] = entry
					order = append(order, code)
				}
				if entry.Candidate.Name == entry.Candidate.Code && name != "" && name != code {
					entry.Candidate.Name = name
				}
				entry.Evidence++
				for _, keyword := range hotspot.Keywords {
					keyword = strings.TrimSpace(keyword)
					if keyword != "" {
						entry.Keywords[keyword] = struct{}{}
					}
				}
			}
		}
	}
	candidates := make([]aStockMarketCandidate, 0, len(order))
	for _, code := range order {
		entry := aggregates[code]
		keywords := make([]string, 0, len(entry.Keywords))
		for keyword := range entry.Keywords {
			keywords = append(keywords, keyword)
		}
		sort.Strings(keywords)
		entry.Candidate.Keywords = keywords
		entry.Candidate.Evidence = entry.Evidence
		candidates = append(candidates, entry.Candidate)
	}
	return candidates
}

type aStockMention struct {
	Code string
	Name string
}

func extractAStockMentionsFromItem(item model.Item) []aStockMention {
	mentions := make([]aStockMention, 0)
	seen := make(map[string]int)
	appendMention := func(code string, name string) {
		code = normalizeAStockCode(code)
		if !isAStockCode(code) {
			return
		}
		name = cleanAStockMentionName(name)
		if index, ok := seen[code]; ok {
			if mentions[index].Name == "" && name != "" {
				mentions[index].Name = name
			}
			return
		}
		seen[code] = len(mentions)
		mentions = append(mentions, aStockMention{Code: code, Name: name})
	}
	for _, code := range extractAStockCodes(item.TagFlags) {
		appendMention(code, "")
	}
	parseAStockMentionsFromJSON(item.RawPayload, appendMention)
	return mentions
}

func parseAStockMentionsFromJSON(raw string, appendMention func(code string, name string)) {
	raw = strings.TrimSpace(raw)
	if raw == "" || (!strings.HasPrefix(raw, "{") && !strings.HasPrefix(raw, "[")) {
		return
	}
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return
	}
	walkAStockMentionPayload(payload, "", appendMention)
}

func walkAStockMentionPayload(value any, parentKey string, appendMention func(code string, name string)) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok && isAStockListKey(parentKey) {
				appendMention(text, "")
				continue
			}
			walkAStockMentionPayload(item, parentKey, appendMention)
		}
	case map[string]any:
		code := firstAStockPayloadString(typed, "StockID", "stock_id", "stockCode", "stock_code", "code", "symbol", "secuCode", "SECURITY_CODE")
		name := firstAStockPayloadString(typed, "name", "stock_name", "stockName", "SECURITY_NAME_ABBR", "SECURITY_NAME", "股票名称")
		if code != "" {
			appendMention(code, name)
		}
		for key, child := range typed {
			walkAStockMentionPayload(child, key, appendMention)
		}
	}
}

func firstAStockPayloadString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			switch typed := value.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					return typed
				}
			case float64:
				if typed > 0 {
					return fmt.Sprintf("%.0f", typed)
				}
			}
		}
	}
	return ""
}

func isAStockListKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	return key == "stocklist" || key == "stock_list" || key == "stocks" || key == "stock"
}

func extractAStockCodes(raw string) []string {
	codes := make([]string, 0)
	seen := make(map[string]struct{})
	for _, token := range strings.FieldsFunc(raw, func(r rune) bool {
		return !(r >= '0' && r <= '9') && !(r >= 'A' && r <= 'Z') && !(r >= 'a' && r <= 'z')
	}) {
		code := normalizeAStockCode(token)
		if !isAStockCode(code) {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	return codes
}

func isAStockCode(code string) bool {
	return astockcode.IsShanghaiShenzhen(code)
}

func inferAStockNameFromNews(item model.Item) string {
	text := cleanAStockMentionName(item.Title)
	if text == "" {
		text = cleanAStockMentionName(item.Summary)
	}
	if text == "" {
		text = cleanAStockMentionName(item.Content)
	}
	return inferAStockNameFromText(text)
}

func inferAStockNameFromText(text string) string {
	text = strings.TrimSpace(strings.Trim(text, "【】[]()（）"))
	if text == "" {
		return ""
	}
	separators := []string{"：", ":", "丨", "|", " "}
	for _, sep := range separators {
		if idx := strings.Index(text, sep); idx > 0 {
			prefix := cleanAStockMentionName(text[:idx])
			if validAStockMentionName(prefix) {
				return prefix
			}
			break
		}
	}
	triggers := []string{"快速", "盘中", "异动", "涨停", "跌停", "涨超", "跌超", "大涨", "大跌", "拉升", "回调", "走强", "走弱", "封板", "冲高", "跳水"}
	best := ""
	bestIndex := len([]rune(text)) + 1
	for _, trigger := range triggers {
		if idx := strings.Index(text, trigger); idx > 0 && idx < bestIndex {
			best = cleanAStockMentionName(text[:idx])
			bestIndex = idx
		}
	}
	if idx := regexpAStockNewsDateIndex(text); idx > 0 && idx < bestIndex {
		best = cleanAStockMentionName(text[:idx])
	}
	if validAStockMentionName(best) {
		return best
	}
	return ""
}

func regexpAStockNewsDateIndex(text string) int {
	for i := 0; i+1 < len(text); i++ {
		if text[i] >= '0' && text[i] <= '9' {
			if idx := strings.Index(text[i:], "月"); idx > 0 {
				if dayIdx := strings.Index(text[i+idx:], "日"); dayIdx > 0 {
					return i
				}
			}
		}
	}
	return -1
}

func cleanAStockMentionName(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "【】[]()（）「」《》")
	return raw
}

func validAStockMentionName(name string) bool {
	runes := []rune(strings.TrimSpace(name))
	if len(runes) < 2 || len(runes) > 8 {
		return false
	}
	if isAStockRecommendationPlaceholderName(name) {
		return false
	}
	if isInvalidAStockRecommendationName(name) {
		return false
	}
	blocked := []string{"A股", "港股", "美股", "股票", "股指", "板块", "概念", "期货", "国债期货", "中证转债指数"}
	for _, word := range blocked {
		if name == word || strings.Contains(name, word) {
			return false
		}
	}
	return true
}

func scoreAStockMarketCandidates(hotspot aStockHotspot, candidates []aStockMarketCandidate) []aStockMarketCandidate {
	return scoreAStockMarketCandidatesWithSectorGate(hotspot, candidates, nil)
}

func scoreAStockMarketCandidatesWithSectorGate(hotspot aStockHotspot, candidates []aStockMarketCandidate, sectorGate *aStockHotspotSectorGate) []aStockMarketCandidate {
	return scoreAStockMarketCandidatesWithSettings(hotspot, candidates, sectorGate, defaultAStockAlgorithmSettings())
}

func scoreAStockMarketCandidatesWithSettings(hotspot aStockHotspot, candidates []aStockMarketCandidate, sectorGate *aStockHotspotSectorGate, settings model.AStockRecommendationAlgorithmSettings) []aStockMarketCandidate {
	scored := make([]aStockMarketCandidate, 0, len(candidates))
	evidenceIndex := newAStockStockEvidenceIndex(hotspot.MatchedItems)
	for _, candidate := range candidates {
		candidate.Code = normalizeAStockCode(candidate.Code)
		candidate.Name = astockcode.DisplayName(candidate.Code, candidate.Name)
		if !astockcode.IsShanghaiShenzhen(candidate.Code) || !hasResolvedAStockRecommendationName(candidate.Code, candidate.Name) {
			continue
		}
		if isBlockedAStockRecommendationCandidate(candidate) {
			continue
		}
		evidenceResult := evidenceIndex.Assess(candidate)
		evidence := evidenceResult.Count
		effectiveEvidence := evidenceResult.Strong
		weakPenalty := 0
		if evidence > 0 && effectiveEvidence == 0 && evidenceResult.Weak > 0 {
			weakPenalty = settings.Emotion.WeakEvidencePenalty
		}
		keywords := aStockCandidateKeywordMatches(candidate.Name, hotspot.Keywords)
		if candidate.Fallback && len(keywords) == 0 {
			keywords = intersectAStockKeywords(candidate.Keywords, hotspot.Keywords)
		}
		sourceLinked := hasAnyAStockCandidateSource(candidate)
		sourceMatchesHotspot := sourceLinked && (len(keywords) > 0 || aStockCandidateInHotspotSector(sectorGate, hotspot, candidate))
		if hasAStockCandidateSource(candidate, aStockCandidateSourceNews) && (evidence > 0 || len(keywords) > 0) {
			sourceMatchesHotspot = true
		}
		if effectiveEvidence == 0 && evidenceResult.Weak == 0 && len(keywords) == 0 && !sourceMatchesHotspot {
			continue
		}
		if !sectorGate.Allows(hotspot, candidate) {
			continue
		}
		candidate.Evidence = evidence
		candidate.StrongEvidence = effectiveEvidence
		candidate.WeakEvidence = evidenceResult.Weak
		candidate.WeakPenalty = weakPenalty
		candidate.BadEvidence = evidenceResult.Negative
		candidate.Keywords = keywords
		candidate.MatchedScore = aStockMarketRankScoreWithSettings(candidate.Rank, settings) + effectiveEvidence*settings.Emotion.StockEvidenceScore + len(keywords)*settings.Emotion.StockNameKeywordScore + candidate.PreFundScore + candidate.PreSectorScore - weakPenalty
		scored = append(scored, candidate)
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].MatchedScore == scored[j].MatchedScore {
			if scored[i].Evidence == scored[j].Evidence {
				if scored[i].AuctionAmount == scored[j].AuctionAmount {
					if scored[i].AuctionVolume == scored[j].AuctionVolume {
						return scored[i].Code < scored[j].Code
					}
					return scored[i].AuctionVolume > scored[j].AuctionVolume
				}
				return scored[i].AuctionAmount > scored[j].AuctionAmount
			}
			return scored[i].Evidence > scored[j].Evidence
		}
		return scored[i].MatchedScore > scored[j].MatchedScore
	})
	return scored
}

func intersectAStockKeywords(left []string, right []string) []string {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	rightSet := make(map[string]string, len(right))
	for _, keyword := range right {
		normalized := strings.ToLower(strings.TrimSpace(keyword))
		if normalized != "" {
			rightSet[normalized] = keyword
		}
	}
	matches := make([]string, 0)
	seen := make(map[string]struct{})
	for _, keyword := range left {
		normalized := strings.ToLower(strings.TrimSpace(keyword))
		if normalized == "" {
			continue
		}
		if value, ok := rightSet[normalized]; ok {
			if _, exists := seen[value]; !exists {
				seen[value] = struct{}{}
				matches = append(matches, value)
			}
		}
	}
	sort.Strings(matches)
	return matches
}

func aStockMarketRankScore(rank int) int {
	return aStockMarketRankScoreWithSettings(rank, defaultAStockAlgorithmSettings())
}

func aStockMarketRankScoreWithSettings(rank int, settings model.AStockRecommendationAlgorithmSettings) int {
	if rank <= 0 {
		return 0
	}
	score := (settings.Auction.MarketRankScoreBase - rank + 1) / settings.Auction.MarketRankScoreDivisor
	if score < 1 {
		return 1
	}
	return score
}

func aStockStockEvidenceCount(items []model.Item, candidate aStockMarketCandidate) int {
	return newAStockStockEvidenceIndex(items).Count(candidate)
}

type aStockStockEvidenceIndex struct {
	items []aStockStockEvidenceItem
}

type aStockStockEvidenceItem struct {
	codes map[string]struct{}
	text  string
	weak  bool
	bad   bool
}

type aStockStockEvidenceAssessment struct {
	Count    int
	Strong   int
	Weak     int
	Negative int
}

func newAStockStockEvidenceIndex(items []model.Item) aStockStockEvidenceIndex {
	if len(items) == 0 {
		return aStockStockEvidenceIndex{}
	}
	index := aStockStockEvidenceIndex{items: make([]aStockStockEvidenceItem, 0, len(items))}
	for _, item := range items {
		codes := aStockStockListCodeSet(item.TagFlags)
		if strings.Contains(item.RawPayload, "stockList") {
			codes = mergeAStockStockListCodeSet(codes, item.RawPayload)
		}
		index.items = append(index.items, aStockStockEvidenceItem{
			codes: codes,
			text:  strings.ToLower(item.Title + " " + item.Summary + " " + item.Content + " " + item.RawPayload),
			weak:  isAStockWeakFinancingEvidenceItem(item),
			bad:   isAStockNegativeNewsItem(item),
		})
	}
	return index
}

func (idx aStockStockEvidenceIndex) Count(candidate aStockMarketCandidate) int {
	return idx.Assess(candidate).Count
}

func (idx aStockStockEvidenceIndex) Assess(candidate aStockMarketCandidate) aStockStockEvidenceAssessment {
	if len(idx.items) == 0 {
		return aStockStockEvidenceAssessment{}
	}
	code := normalizeAStockCode(candidate.Code)
	name := strings.ToLower(strings.TrimSpace(candidate.Name))
	if code == "" && name == "" {
		return aStockStockEvidenceAssessment{}
	}
	assessment := aStockStockEvidenceAssessment{}
	for _, item := range idx.items {
		matched := false
		if code != "" {
			if _, ok := item.codes[code]; ok {
				matched = true
			}
		}
		if !matched && name != "" && strings.Contains(item.text, name) {
			matched = true
		}
		if !matched {
			continue
		}
		assessment.Count++
		if item.bad {
			assessment.Negative++
		} else if item.weak {
			assessment.Weak++
		} else {
			assessment.Strong++
		}
	}
	return assessment
}

func isAStockWeakFinancingEvidenceItem(item model.Item) bool {
	text := strings.ToLower(strings.Join([]string{item.Title, item.Summary, item.Content}, " "))
	for _, keyword := range aStockWeakFinancingEvidenceKeywords() {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func aStockWeakFinancingEvidenceKeywords() []string {
	return []string{
		"融资净买入",
		"融资净偿还",
		"融资余额",
		"融资买入",
		"融资偿还",
		"融资融券",
		"融券",
		"两融余额",
	}
}

func aStockStockListCodeSet(raw string) map[string]struct{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	codes := make(map[string]struct{})
	for _, token := range strings.FieldsFunc(raw, func(r rune) bool {
		return r < '0' || r > '9'
	}) {
		if len(token) >= 6 {
			if code := normalizeAStockCode(token[len(token)-6:]); code != "" {
				codes[code] = struct{}{}
			}
		}
	}
	if len(codes) == 0 {
		return nil
	}
	return codes
}

func mergeAStockStockListCodeSet(codes map[string]struct{}, raw string) map[string]struct{} {
	extra := aStockStockListCodeSet(raw)
	if len(extra) == 0 {
		return codes
	}
	if codes == nil {
		codes = make(map[string]struct{}, len(extra))
	}
	for code := range extra {
		codes[code] = struct{}{}
	}
	return codes
}

func aStockItemMentionsStock(item model.Item, candidate aStockMarketCandidate) bool {
	code := normalizeAStockCode(candidate.Code)
	name := strings.TrimSpace(candidate.Name)
	if code != "" && aStockStockListContainsCode(item.TagFlags, code) {
		return true
	}
	if code != "" && strings.Contains(item.RawPayload, "stockList") && aStockStockListContainsCode(item.RawPayload, code) {
		return true
	}
	text := strings.ToLower(item.Title + " " + item.Summary + " " + item.Content + " " + item.RawPayload)
	if name != "" && strings.Contains(text, strings.ToLower(name)) {
		return true
	}
	return false
}

func aStockStockListContainsCode(raw string, code string) bool {
	code = normalizeAStockCode(code)
	if code == "" || strings.TrimSpace(raw) == "" {
		return false
	}
	for _, token := range strings.FieldsFunc(raw, func(r rune) bool {
		return r < '0' || r > '9'
	}) {
		if len(token) >= 6 && token[len(token)-6:] == code {
			return true
		}
	}
	return false
}

func aStockCandidateKeywordMatches(name string, keywords []string) []string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil
	}
	matches := make([]string, 0)
	seen := make(map[string]struct{})
	for _, keyword := range keywords {
		normalized := strings.ToLower(strings.TrimSpace(keyword))
		if normalized == "" {
			continue
		}
		if strings.Contains(name, normalized) {
			if _, ok := seen[keyword]; !ok {
				seen[keyword] = struct{}{}
				matches = append(matches, keyword)
			}
		}
	}
	sort.Strings(matches)
	return matches
}

func aStockMatchedKeywords(item model.Item) []string {
	keywordSet := make(map[string]struct{})
	text := strings.ToLower(item.Title + " " + item.Summary + " " + item.Content)
	for _, rule := range aStockTopicRules() {
		for _, keyword := range rule.Keywords {
			if strings.Contains(text, strings.ToLower(keyword)) {
				keywordSet[keyword] = struct{}{}
			}
		}
	}
	keywords := make([]string, 0, len(keywordSet))
	for keyword := range keywordSet {
		keywords = append(keywords, keyword)
	}
	sort.Strings(keywords)
	return keywords
}

func aStockTopicRules() []aStockTopicRule {
	return []aStockTopicRule{
		{Name: "人工智能", Keywords: []string{"AI", "人工智能", "大模型", "算力", "AIGC", "机器人"}},
		{Name: "半导体", Keywords: []string{"半导体", "芯片", "光刻", "晶圆", "存储", "先进封装"}},
		{Name: "新能源", Keywords: []string{"新能源", "锂电", "储能", "光伏", "风电", "充电桩"}},
		{Name: "低空经济", Keywords: []string{"低空经济", "eVTOL", "无人机", "通航", "飞行汽车"}},
		{Name: "金融券商", Keywords: []string{"券商", "证券", "银行", "保险", "降准", "降息", "资本市场"}},
		{Name: "黄金有色", Keywords: []string{"黄金", "有色", "铜", "铝", "稀土", "贵金属"}},
		{Name: "医药生物", Keywords: []string{"医药", "创新药", "疫苗", "医疗器械", "CXO"}},
		{Name: "消费电子", Keywords: []string{"消费电子", "苹果", "华为", "手机", "MR", "AR", "VR"}},
		{Name: "房地产", Keywords: []string{"房地产", "地产", "房贷", "楼市", "保障房"}},
		{Name: "军工航天", Keywords: []string{"军工", "航天", "卫星", "商业航天", "航空发动机"}},
	}
}

func aStockTodayDate() string {
	return aStockNow().In(aStockLocation()).Format("2006-01-02")
}

func normalizeAStockStrategyDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			return parsed.Format("2006-01-02")
		}
		return raw
	}
	return aStockTodayDate()
}

func normalizeAStockNewsPage(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 1
	}
	var page int
	if _, err := fmt.Sscanf(raw, "%d", &page); err != nil || page < 1 {
		return 1
	}
	return page
}

func aStockPageHref(strategyDate string, period string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) string {
	return aStockPageHrefForPath("/a-stock", strategyDate, period, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
}

func aStockPageHrefForPath(targetPath string, strategyDate string, period string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) string {
	return aStockPageHrefForPathWithFundFlowExplicit(targetPath, strategyDate, period, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, false, filterTodayMarket)
}

func aStockPageHrefForPathWithFundFlowExplicit(targetPath string, strategyDate string, period string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, fundFlowExplicit bool, filterTodayMarket bool) string {
	href := aStockPagePath(targetPath) + "?date=" + url.QueryEscape(normalizeAStockStrategyDate(strategyDate)) + "&period=" + url.QueryEscape(normalizeAStockPeriod(period).Key)
	if newsPage > 1 {
		href += "&news_page=" + url.QueryEscape(fmt.Sprintf("%d", newsPage))
	}
	if ignoreRecent {
		href += "&ignore_recent=1"
	}
	if ignoreLimitUp {
		href += "&ignore_limit_up=1"
	}
	if fundFlowExplicit && ignoreFundFlow != aStockDefaultIgnoreFundFlow(strategyDate) && ignoreFundFlow {
		href += "&ignore_fund_flow=1"
	} else if fundFlowExplicit && ignoreFundFlow != aStockDefaultIgnoreFundFlow(strategyDate) {
		href += "&filter_fund_flow=1"
	}
	if filterTodayMarket {
		href += "&filter_today_market=1"
	}
	return href
}

func aStockFilterToggleHref(strategyDate string, period string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) string {
	return aStockFilterToggleHrefForPath("/a-stock", strategyDate, period, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, false, filterTodayMarket)
}

func aStockFilterToggleHrefForPath(targetPath string, strategyDate string, period string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, fundFlowExplicit bool, filterTodayMarket bool) string {
	return aStockPageHrefForPathWithFundFlowExplicit(targetPath, strategyDate, period, newsPage, !ignoreRecent, ignoreLimitUp, ignoreFundFlow, fundFlowExplicit, filterTodayMarket)
}

func aStockPagePath(targetPath string) string {
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" || !strings.HasPrefix(targetPath, "/") || strings.ContainsAny(targetPath, "?#") {
		return "/a-stock"
	}
	return targetPath
}

func aStockFilterToggleLabel(ignoreRecent bool) string {
	if ignoreRecent {
		return "启用" + aStockRecentLookbackLabel() + "过滤"
	}
	return "关闭" + aStockRecentLookbackLabel() + "过滤"
}

func aStockRecentLookbackLabel() string {
	return fmt.Sprintf("%d个交易日", aStockRecentLookbackDays)
}

func aStockRecentLookbackStatusPrefix() string {
	return aStockRecentLookbackLabel() + "内重复"
}

func aStockLimitUpFilterToggleHref(strategyDate string, period string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) string {
	return aStockPageHref(strategyDate, period, newsPage, ignoreRecent, !ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
}

func aStockLimitUpFilterToggleLabel(ignoreLimitUp bool) string {
	if ignoreLimitUp {
		return "启用涨停过滤"
	}
	return "关闭涨停过滤"
}

func aStockTodayMarketFilterToggleLabel(filterTodayMarket bool) string {
	if filterTodayMarket {
		return "关闭当日行情过滤"
	}
	return "启用当日行情过滤"
}

func aStockFundFlowFilterToggleLabel(ignoreFundFlow bool) string {
	if ignoreFundFlow {
		return "启用资金过滤"
	}
	return "关闭资金过滤"
}

func writeAStockFundFlowPreserveInput(b *strings.Builder, strategyDate string, ignoreFundFlow bool, explicit bool) {
	if b == nil || !explicit || ignoreFundFlow == aStockDefaultIgnoreFundFlow(strategyDate) {
		return
	}
	writeAStockFundFlowToggleInput(b, ignoreFundFlow)
}

func writeAStockFundFlowToggleInput(b *strings.Builder, ignoreFundFlow bool) {
	if b == nil {
		return
	}
	if ignoreFundFlow {
		b.WriteString(`<input type="hidden" name="ignore_fund_flow" value="1">`)
		return
	}
	b.WriteString(`<input type="hidden" name="filter_fund_flow" value="1">`)
}

func aStockPeriods() []aStockPeriod {
	return []aStockPeriod{
		{Key: "morning", Label: "上午推荐", WindowLabel: "08:00-09:30"},
		{Key: "afternoon", Label: "下午推荐", WindowLabel: "09:30-13:00"},
		{Key: "evening", Label: "晚间推荐", WindowLabel: "15:00-18:30"},
	}
}

func aStockPeriodKeys() []string {
	periods := aStockPeriods()
	keys := make([]string, 0, len(periods))
	for _, period := range periods {
		keys = append(keys, period.Key)
	}
	return keys
}

func normalizeAStockPeriod(raw string) aStockPeriod {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "after" || raw == "pm" {
		raw = "afternoon"
	}
	if raw == "night" || raw == "pm2" {
		raw = "evening"
	}
	for _, period := range aStockPeriods() {
		if period.Key == raw {
			return period
		}
	}
	return aStockPeriods()[0]
}
