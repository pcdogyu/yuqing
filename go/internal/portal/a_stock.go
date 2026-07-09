package portal

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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
	FundFlowFilterEnabled        bool
	FundFlowFiltered             int
	FundFlowMissingCount         int
	TodayMarketFilterEnabled     bool
	NoTodayMarketCount           int
	MarketCandidateStatus        string
	MarketCandidateCount         int
	GeneratedRecommendationCount int
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
	EntryTime       string
}

type aStockMarketBar struct {
	Code                string
	Date                string
	Open                float64
	Close               float64
	Pct                 float64
	EntryPrice          float64
	AfternoonEntryPrice float64
	SessionPrices       map[string]float64
}

type aStockBacktestCell struct {
	Close       string
	Return      string
	ReturnClass string
}

type aStockBacktestRow struct {
	Stock           string
	EntryOpen       string
	AfternoonOpen   string
	T0Return        string
	T0Close         string
	T0ReturnClass   string
	Days            []aStockBacktestCell
	BestReturn      string
	BestReturnClass string
	Status          string
}

type aStockTopicRule struct {
	Name     string
	Keywords []string
	Stocks   []aStockStockPick
}

type aStockStockPick struct {
	Code string
	Name string
}

type aStockMarketCandidate struct {
	Code          string
	Name          string
	TradeDate     string
	Rank          int
	AuctionAmount float64
	AuctionVolume float64
	MatchedScore  int
	Evidence      int
	Keywords      []string
	Fallback      bool
	FixedPool     bool
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
	auctionResults            map[string]aStockAuctionResultCacheEntry
	sectorConstituents        map[string]aStockSectorConstituentCodesCacheEntry
	codeNames                 map[string]map[string]string
	sourceRuns                []aStockSourceRun
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

type aStockAuctionResultCacheEntry struct {
	result model.AStockAuctionListResult
	found  bool
}

type aStockSectorConstituentCodesCacheEntry struct {
	codes map[string]struct{}
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
	RecommendationCount      int    `json:"recommendation_count"`
	GeneratedCount           int    `json:"generated_count"`
	BacktestStatus           string `json:"backtest_status"`
	RecentFiltered           int    `json:"recent_filtered"`
	RecentLookbackDays       int    `json:"recent_lookback_days"`
	SameDayMorningFiltered   int    `json:"same_day_morning_filtered"`
	LimitUpFiltered          int    `json:"limit_up_filtered"`
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
	aStockDrawdownFilterThreshold      = -15.0
	aStockSectorDrawdownPenalty        = 15
	aStockFundFlowBonusThreshold       = 30000000.0
	aStockFundFlowStrongBonusThreshold = 100000000.0
	aStockNegativeNewsPenalty          = 30
	aStockNewsPageSize                 = 10
	aStockArticleFetchPageSize         = 1000
	aStockArticleFetchMaxPages         = 100
	aStockRecentLookbackDays           = 31
	aStockFundFlowFilterCookieName     = "yuqing_astock_fund_flow_filter"
	aStockFundFlowFilterCookieEnabled  = "enabled"
	aStockFundFlowFilterCookieDisabled = "disabled"
	aStockAuctionCandidateCacheTTL     = 5 * time.Minute
	aStockMarketCandidateLimit         = 5000
	aStockRecommendationLimit          = 12
	aStockReplacementPoolLimit         = 36
	aStockReplacementPerHotspot        = 12
	aStockHotspotTopStockLimit         = 9
	aStockHotspotScoredCandidateLimit  = 240
	aStockHotspotLimit                 = 3
	aStockMarketRankScoreBase          = 200
	aStockStocksPerHotspot             = 3
	aStockRecommendationPhasePreopen   = "preopen"
	aStockRecommendationPhaseFinal     = "final"
)

var (
	aStockNow               = time.Now
	aStockEastmoneyKlineURL = "https://push2his.eastmoney.com/api/qt/stock/kline/get"
	aStockEastmoneyQuoteURL = "https://push2.eastmoney.com/api/qt/stock/get"
	aStockTencentMinuteURL  = "https://web.ifzq.gtimg.cn/appstock/app/minute/query"
	aStockSinaMinuteURL     = "https://quotes.sina.cn/cn/api/jsonp_v2.php/=/CN_MarketDataService.getKLineData"
	aStockYahooChartURL     = "https://query1.finance.yahoo.com/v8/finance/chart/"
	aStockMarketHolidays    = map[string]struct{}{
		"2026-01-01": {},
		"2026-01-02": {},
		"2026-01-03": {},
		"2026-02-15": {},
		"2026-02-16": {},
		"2026-02-17": {},
		"2026-02-18": {},
		"2026-02-19": {},
		"2026-02-20": {},
		"2026-02-21": {},
		"2026-02-22": {},
		"2026-02-23": {},
		"2026-04-04": {},
		"2026-04-05": {},
		"2026-04-06": {},
		"2026-05-01": {},
		"2026-05-02": {},
		"2026-05-03": {},
		"2026-05-04": {},
		"2026-05-05": {},
		"2026-06-19": {},
		"2026-06-20": {},
		"2026-06-21": {},
		"2026-09-25": {},
		"2026-09-26": {},
		"2026-09-27": {},
		"2026-10-01": {},
		"2026-10-02": {},
		"2026-10-03": {},
		"2026-10-04": {},
		"2026-10-05": {},
		"2026-10-06": {},
		"2026-10-07": {},
	}
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
	ignoreFundFlow, fundFlowExplicit := normalizeAStockIgnoreFundFlowFromRequest(r)
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
		payload = s.buildAStockPageFragment(strategyDate, period.Key, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh, strings.TrimSpace(r.URL.Query().Get("msg")))
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
		.astock-recommendation-table{width:100%;min-width:1280px;table-layout:fixed}
		.astock-recommendation-table th,.astock-recommendation-table td{vertical-align:top}
		.astock-recommendation-table th:nth-child(2),.astock-recommendation-table td:nth-child(2){width:7.5%;white-space:nowrap}
		.astock-recommendation-table th:last-child,.astock-recommendation-table td:last-child{width:36%}
		.astock-date-tabs{display:flex;gap:8px;flex-wrap:nowrap;margin:14px 0 18px;overflow-x:auto;padding-bottom:6px;scrollbar-width:thin}
		.astock-tabs{display:flex;gap:8px;flex-wrap:wrap;margin-top:12px}
		.astock-tab{display:inline-flex;align-items:center;flex:0 0 auto;padding:8px 12px;border:1px solid #d6ccbb;border-radius:8px;color:#214e34;text-decoration:none;background:#fff}
		.astock-tab.active{background:#214e34;color:#fff;border-color:#214e34}
		.astock-tab.disabled{color:#9a9388;border-color:#ece7dc;background:#faf8f2;pointer-events:none}
		.astock-history-actions{display:flex;gap:10px;flex-wrap:wrap;align-items:stretch;margin:0 0 18px}
		.astock-history-actions form{display:flex;margin:0}
		.astock-history-actions button,.astock-history-actions .astock-filter-toggle{display:inline-flex;align-items:center;justify-content:center;box-sizing:border-box;min-height:38px;margin:0;padding:8px 12px;line-height:1.2}
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

func (s *Server) buildAStockPageFragment(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, forceRecommendationRefresh bool, message string) aStockPartialPayload {
	requestCache := newAStockRequestCache()
	ctx := s.loadAStockContextReadOnlyWithCache(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh, requestCache)
	morningCtx := ctx
	if ctx.Period != "morning" {
		morningCtx = s.loadAStockCompanionContextReadOnlyWithCache(strategyDate, "morning", 1, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh, requestCache)
	}
	afternoonCtx := ctx
	if ctx.Period != "afternoon" {
		afternoonCtx = s.loadAStockCompanionContextReadOnlyWithCache(strategyDate, "afternoon", 1, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh, requestCache)
	}
	if !morningCtx.FastReadOnly {
		s.applyAStockRecommendationFundFlow5DToContext(&morningCtx, requestCache)
	}
	if !afternoonCtx.FastReadOnly {
		s.applyAStockRecommendationFundFlow5DToContext(&afternoonCtx, requestCache)
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
	renderAStockDateTabs(&b, ctx.Date, ctx.Period, ctx.IgnoreRecent, ctx.IgnoreLimitUp, ctx.IgnoreFundFlow, ctx.TodayMarketFilterEnabled, false)
	renderAStockOverviewSection(&b, morningCtx, afternoonCtx)
	renderAStockRecommendationSection(&b, morningCtx, afternoonCtx)
	renderAStockActionSection(&b, ctx)
	renderAStockHotspotSection(&b, ctx.Hotspots)
	b.WriteString(`</div>`)

	return aStockPartialPayload{
		HTML:         b.String(),
		CanonicalURL: aStockCanonicalPageURL(ctx.Date, ctx.Period, ctx.NewsPage, ctx.IgnoreRecent, ctx.IgnoreLimitUp, ctx.IgnoreFundFlow, ctx.TodayMarketFilterEnabled),
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
		writeAStockFundFlowFilterInput(b, ctx.IgnoreFundFlow)
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

func aStockCanonicalPageURL(strategyDate string, period string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) string {
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
	if ignoreFundFlow {
		query.Set("ignore_fund_flow", "1")
	}
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
	ignoreFundFlow, fundFlowExplicit := normalizeAStockIgnoreFundFlowFromRequest(r)
	setAStockFundFlowFilterCookie(w, ignoreFundFlow, fundFlowExplicit)
	filterTodayMarket := normalizeAStockFilterTodayMarket(r.URL.Query())
	requestCache := newAStockRequestCache()
	ctx := s.loadAStockBacktestSnapshotContextWithCache(strategyDate, period.Key, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, requestCache)
	morningCtx := ctx
	if ctx.Period != "morning" {
		morningCtx = s.loadAStockBacktestSnapshotContextWithCache(strategyDate, "morning", 1, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, requestCache)
	}
	afternoonCtx := ctx
	if ctx.Period != "afternoon" {
		afternoonCtx = s.loadAStockBacktestSnapshotContextWithCache(strategyDate, "afternoon", 1, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, requestCache)
	}
	message := strings.TrimSpace(r.URL.Query().Get("msg"))
	if message == "" {
		message = ctx.LoadMessage
	}
	detail := strings.TrimSpace(r.URL.Query().Get("detail"))

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
		.astock-history-actions button,.astock-history-actions .astock-filter-toggle{display:inline-flex;align-items:center;justify-content:center;box-sizing:border-box;min-height:38px;margin:0;padding:8px 12px;border:1px solid #214e34;border-radius:8px;background:#214e34;color:#fff;font-weight:700;line-height:1.2;text-decoration:none}
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
	renderAStockBacktestSectionForPath(&b, "/a-stock/backtest", ctx.Date, ctx.Period, ctx.IgnoreRecent, ctx.IgnoreLimitUp, ctx.IgnoreFundFlow, ctx.TodayMarketFilterEnabled, morningCtx, afternoonCtx)

	_ = s.writeSimplePage(w, "a-stock-backtest", "A股回测", b.String())
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
	ignoreFundFlow, fundFlowExplicit := normalizeAStockIgnoreFundFlowFromForm(r)
	setAStockFundFlowFilterCookie(w, ignoreFundFlow, fundFlowExplicit)
	filterTodayMarket := normalizeAStockBool(r.FormValue("filter_today_market"))
	if ignoreRecent {
		query.Set("ignore_recent", "1")
	}
	if ignoreLimitUp {
		query.Set("ignore_limit_up", "1")
	}
	if ignoreFundFlow {
		query.Set("ignore_fund_flow", "1")
	} else if fundFlowExplicit {
		query.Set("filter_fund_flow", "1")
	}
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
		query.Set("msg", s.repairAStockActionRecommendationNames(strategyDate, period.Key, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket))
	case "backfill_auction":
		query.Set("msg", s.triggerAStockAuctionBackfillDate(strategyDate))
		persistRecommendation = true
	case "refresh_backtest":
		query.Set("msg", "上午和下午消息回测已按当前推荐股票、13:01价格和行情收益重新刷新。")
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
		periods = []string{"morning", "afternoon"}
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
		periods = []string{"morning", "afternoon"}
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
	repaired, skipped := s.repairAStockPersistedRecommendationsWithCache(ctx.Date, ctx.Recommendations, cache)
	ctx.Recommendations = repaired
	if len(ctx.Recommendations) == 0 {
		summary := ctx.PeriodLabel + "行情收益未补齐：当前推荐股票名称无效。"
		if skipped > 0 {
			summary = fmt.Sprintf("%s 跳过无有效名称股票 %d 只。", strings.TrimSuffix(summary, "。"), skipped)
		}
		return summary, summary
	}
	ctx.Recommendations, ctx.Backtests, ctx.BacktestStatus, ctx.LimitUpFiltered = s.loadAStockLockedMarketView(ctx.Date, ctx.Period, ctx.Recommendations)
	ctx.EmptyReason = aStockRecommendationEmptyReason(ctx)
	if err := s.saveAStockRecommendationSnapshot(ctx); err != nil {
		summary := ctx.PeriodLabel + "行情收益保存失败：" + err.Error()
		return summary, summary
	}
	updatedCells := countAStockBacktestUpdatedCells(previousByCode, ctx.Backtests)
	summary := fmt.Sprintf("%s行情收益已按当前推荐股票重新补齐：读取 %d 只，写入 %d 行，补齐/更新 %d 个收益字段。", ctx.PeriodLabel, len(ctx.Recommendations), len(ctx.Backtests), updatedCells)
	if skipped > 0 {
		summary = fmt.Sprintf("%s 跳过无有效名称股票 %d 只。", summary, skipped)
	}
	detail := formatAStockBacktestRefreshDetail(ctx, previousByCode, updatedCells)
	return summary, detail
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
	for i := 0; i < 5; i++ {
		var previousValue string
		if i < len(previous.Days) {
			previousValue = previous.Days[i].Return
		}
		var currentValue string
		if i < len(current.Days) {
			currentValue = current.Days[i].Return
		}
		if aStockBacktestRefreshFieldChanged(previousValue, currentValue) {
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
	fmt.Fprintf(&b, "获取：date=%s period=%s codes=%s source=%s\n", ctx.Date, ctx.Period, strings.Join(aStockRecommendationCodes(ctx.Recommendations), ","), aStockMarketConfigHint())
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
		formatAStockBacktestRefreshField("T+0", previous.T0Return, current.T0Return),
	}
	for i := 0; i < 5; i++ {
		var previousValue string
		if i < len(previous.Days) {
			previousValue = previous.Days[i].Return
		}
		var currentValue string
		if i < len(current.Days) {
			currentValue = current.Days[i].Return
		}
		parts = append(parts, formatAStockBacktestRefreshField(fmt.Sprintf("T+%d", i+1), previousValue, currentValue))
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

func (s *Server) repairAStockActionRecommendationNames(strategyDate string, periodKey string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) string {
	if strings.TrimSpace(s.cfg.ContentURL) == "" {
		return "内容服务未配置，无法补股票名称。"
	}
	cache := newAStockRequestCache()
	ctx, ok, loadMessage := s.loadAStockRecommendationNameRepairContext(strategyDate, periodKey, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, cache)
	if loadMessage != "" {
		return loadMessage
	}
	if !ok || len(ctx.Recommendations) == 0 {
		return ctx.PeriodLabel + "暂无推荐股票可补名称。"
	}
	originalNames := aStockRecommendationNameMap(ctx.Recommendations)
	recommendations, skipped := s.repairAStockPersistedRecommendationsWithCache(ctx.Date, ctx.Recommendations, cache)
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
	if isAStockOfficialSelectionContext(ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket) {
		if err := s.saveAStockRecommendationSelections(ctx); err != nil {
			return ctx.PeriodLabel + "股票名称补齐失败：" + err.Error()
		}
	}
	if err := s.saveAStockRecommendationSnapshot(ctx); err != nil {
		return ctx.PeriodLabel + "股票名称补齐失败：" + err.Error()
	}
	changed := countAStockRecommendationNameChanges(originalNames, recommendations)
	message := fmt.Sprintf("%s股票名称已从集合竞价名称库补齐 %d 只", ctx.PeriodLabel, changed)
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
	b.WriteString(`function schedulePopupChecks(){clearPopupTimers();fetchPopup();if(!popupEligible){return;}popupTimer=window.setInterval(fetchPopup,5000);var now=new Date();[[9,15],[9,27],[12,45],[12,57]].forEach(function(parts){var target=new Date();target.setHours(parts[0],parts[1],0,0);if(now<target){popupExactTimers.push(window.setTimeout(fetchPopup,Math.max(0,target.getTime()-now.getTime()+100)));}});}`)
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

func renderAStockOverviewSection(b *strings.Builder, morningCtx aStockContext, afternoonCtx aStockContext) {
	auctionAmount := firstNonEmpty(morningCtx.AuctionAmountLabel, afternoonCtx.AuctionAmountLabel)
	if strings.TrimSpace(auctionAmount) == "" {
		auctionAmount = "--"
	}
	b.WriteString(`<section><div class="astock-overview-header"><h2>顶部概览</h2><div class="astock-overview-summary">`)
	writeAStockOverviewSummaryItem(b, "策略日期", morningCtx.Date)
	writeAStockOverviewSummaryItem(b, "集合竞价金额", auctionAmount)
	b.WriteString(`</div></div><div class="astock-scroll"><table class="astock-overview-table"><tr>`)
	writeAStockOverviewPeriodCells(b, morningCtx)
	b.WriteString(`</tr><tr>`)
	writeAStockOverviewPeriodCells(b, afternoonCtx)
	b.WriteString(`</tr></table></div></section>`)
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
	writeAStockFundFlowFilterInput(b, ctx.IgnoreFundFlow)
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
	writeAStockFundFlowFilterInput(b, ctx.IgnoreFundFlow)
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
	writeAStockFundFlowFilterInput(b, !ctx.IgnoreFundFlow)
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
		writeAStockFundFlowFilterInput(b, ctx.IgnoreFundFlow)
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
		reasons = append(reasons, fmt.Sprintf("过滤上午同股票/热点名额 %d", ctx.SameDayMorningFiltered))
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

func renderAStockNewsSections(b *strings.Builder, morningCtx aStockContext, afternoonCtx aStockContext) {
	b.WriteString(`<section>`)
	renderAStockNewsStatsContent(b, morningCtx, afternoonCtx)
	b.WriteString(`</section>`)
}

func renderAStockNewsStatsContent(b *strings.Builder, morningCtx aStockContext, afternoonCtx aStockContext) {
	b.WriteString(`<h2>财经新闻来源统计</h2><div class="astock-news-grid">`)
	renderAStockNewsWindow(b, morningCtx)
	renderAStockNewsWindow(b, afternoonCtx)
	b.WriteString(`</div>`)
}

func renderAStockNewsWindow(b *strings.Builder, ctx aStockContext) {
	newsArticles := aStockNewsArticles(ctx)
	b.WriteString(`<div class="astock-news-window"><h3>`)
	b.WriteString(html.EscapeString(ctx.WindowLabel))
	b.WriteString(` 财经新闻</h3>`)
	if len(newsArticles) == 0 {
		b.WriteString(`<div class="astock-empty">暂无数据：请点击“抓取 A 股新闻”，或确认 `)
		b.WriteString(html.EscapeString(ctx.Date))
		b.WriteString(` `)
		b.WriteString(html.EscapeString(ctx.WindowLabel))
		b.WriteString(` 窗口内已有财经新闻源入库。</div>`)
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

func renderAStockRecommendationSection(b *strings.Builder, morningCtx aStockContext, afternoonCtx aStockContext) {
	b.WriteString(`<section><h2>推荐股票</h2>`)
	renderAStockRecommendationSubsection(b, morningCtx)
	b.WriteString(`<hr class="astock-section-divider">`)
	renderAStockRecommendationSubsection(b, afternoonCtx)
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
		b.WriteString(`</td><td><span class="`)
		b.WriteString(html.EscapeString(rec.FundFlow5DClass))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(rec.FundFlow5D))
		b.WriteString(`</span></td><td>`)
		b.WriteString(html.EscapeString(rec.Reason))
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</table></div>`)
}

func renderAStockBacktestSection(b *strings.Builder, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, morningCtx aStockContext, afternoonCtx aStockContext) {
	renderAStockBacktestSectionForPath(b, "/a-stock", strategyDate, period, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, morningCtx, afternoonCtx)
}

func renderAStockBacktestSectionForPath(b *strings.Builder, targetPath string, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, morningCtx aStockContext, afternoonCtx aStockContext) {
	if strings.TrimSpace(strategyDate) == "" {
		strategyDate = nonEmpty(morningCtx.Date, afternoonCtx.Date)
	}
	b.WriteString(`<section><h2>消息回测</h2><p class="astock-muted">上午推荐按上午开盘价计算，下午推荐按下午开盘价计算；T+0 到 T+5 及五日内最高收益均按对应推荐窗口的基准价回测。</p>`)
	renderAStockRecommendationHistoryTabsForPath(b, targetPath, strategyDate, period, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
	renderAStockRecommendationHistoryActionsForPath(b, targetPath, strategyDate, period, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
	mergedRows := combineAStockBacktestRows(morningCtx, afternoonCtx)
	b.WriteString(`<div class="astock-scroll"><table class="astock-table"><tr><th>推荐窗口</th><th>股票</th><th>上午开盘价</th><th>下午开盘价</th><th>T+0 收益</th><th>T+1 收益</th><th>T+2 收益</th><th>T+3 收益</th><th>T+4 收益</th><th>T+5 收益</th><th>五日内最高收益</th><th>命中状态</th></tr>`)
	if len(mergedRows) == 0 {
		b.WriteString(`<tr><td colspan="12">`)
		b.WriteString(html.EscapeString(aStockBacktestEmptyReason(period, morningCtx, afternoonCtx)))
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

func renderAStockRecommendationHistoryTabs(b *strings.Builder, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) {
	renderAStockRecommendationHistoryTabsForPath(b, "/a-stock", strategyDate, period, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
}

func renderAStockRecommendationHistoryTabsForPath(b *strings.Builder, targetPath string, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) {
	renderAStockDateTabsForPath(b, targetPath, strategyDate, period, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, true)
}

func renderAStockDatePeriodTabs(b *strings.Builder, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, withHeading bool) {
	renderAStockDateTabsForPath(b, "/a-stock", strategyDate, period, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, withHeading)
}

func renderAStockDateTabs(b *strings.Builder, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, withHeading bool) {
	renderAStockDateTabsForPath(b, "/a-stock", strategyDate, period, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, withHeading)
}

func renderAStockDateTabsForPath(b *strings.Builder, targetPath string, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, withHeading bool) {
	if withHeading {
		b.WriteString(`<h3>推荐历史</h3>`)
	}
	b.WriteString(`<div class="astock-date-tabs">`)
	normalizedPeriod := normalizeAStockPeriod(period).Key
	tabs := aStockDateTabs(strategyDate)
	today := aStockTodayDate()
	if len(tabs) == 0 {
		writeAStockDateTab(b, targetPath, "今日", today, normalizedPeriod, strategyDate == today, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
		b.WriteString(`</div>`)
		return
	}
	hasToday := false
	for _, tab := range tabs {
		if tab.Date == today {
			hasToday = true
		}
		active := strategyDate == tab.Date
		writeAStockDateTab(b, targetPath, tab.Label, tab.Date, normalizedPeriod, active, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
	}
	if !hasToday {
		writeAStockDateTab(b, targetPath, "今日", today, normalizedPeriod, strategyDate == today, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
	}
	b.WriteString(`</div>`)
}

func renderAStockRecommendationHistoryActions(b *strings.Builder, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) {
	renderAStockRecommendationHistoryActionsForPath(b, "/a-stock", strategyDate, period, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
}

func renderAStockPeriodSwitchTabsForPath(b *strings.Builder, targetPath string, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) {
	current := normalizeAStockPeriod(period).Key
	b.WriteString(`<div class="astock-tabs">`)
	for _, option := range aStockPeriods() {
		b.WriteString(`<a class="astock-tab`)
		if option.Key == current {
			b.WriteString(` active`)
		}
		b.WriteString(`" data-preserve-scroll="1" href="`)
		b.WriteString(aStockPageHrefForPath(targetPath, strategyDate, option.Key, 1, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(option.Label))
		b.WriteString(`</a>`)
	}
	b.WriteString(`</div>`)
}

func renderAStockRecommendationHistoryActionsForPath(b *strings.Builder, targetPath string, strategyDate string, period string, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) {
	targetPath = aStockPagePath(targetPath)
	b.WriteString(`<div class="astock-history-actions">`)
	b.WriteString(`<a class="astock-filter-toggle" data-preserve-scroll="1" href="`)
	b.WriteString(html.EscapeString(aStockFilterToggleHrefForPath(targetPath, strategyDate, period, 1, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)))
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
		writeAStockFundFlowFilterInput(b, ignoreFundFlow)
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

func writeAStockDateTab(b *strings.Builder, targetPath string, label string, date string, period string, active bool, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) {
	b.WriteString(`<a class="astock-tab`)
	if active {
		b.WriteString(` active`)
	}
	b.WriteString(`" data-preserve-scroll="1" href="`)
	b.WriteString(aStockPageHrefForPath(targetPath, date, period, 1, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket))
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

func combineAStockBacktestRows(morningCtx aStockContext, afternoonCtx aStockContext) []aStockBacktestDisplayRow {
	morningRows := aStockBacktestRowsForDisplay(morningCtx)
	afternoonRows := aStockBacktestRowsForDisplay(afternoonCtx)
	rows := make([]aStockBacktestDisplayRow, 0, len(morningRows)+len(afternoonRows))
	for _, row := range morningRows {
		rows = append(rows, aStockBacktestDisplayRow{
			PeriodLabel: morningCtx.PeriodLabel,
			PeriodKey:   morningCtx.Period,
			Row:         row,
		})
	}
	for _, row := range afternoonRows {
		rows = append(rows, aStockBacktestDisplayRow{
			PeriodLabel: afternoonCtx.PeriodLabel,
			PeriodKey:   afternoonCtx.Period,
			Row:         row,
		})
	}
	return rows
}

func aStockBacktestEmptyReason(period string, morningCtx aStockContext, afternoonCtx aStockContext) string {
	switch normalizeAStockPeriod(period).Key {
	case "morning":
		if reason := strings.TrimSpace(morningCtx.EmptyReason); reason != "" {
			return reason
		}
	case "afternoon":
		if reason := strings.TrimSpace(afternoonCtx.EmptyReason); reason != "" {
			return reason
		}
	}
	if reason := strings.TrimSpace(morningCtx.EmptyReason); reason != "" {
		return reason
	}
	if reason := strings.TrimSpace(afternoonCtx.EmptyReason); reason != "" {
		return reason
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
		Stock:           aStockRecommendationStockLabel(rec),
		EntryOpen:       "--",
		AfternoonOpen:   "--",
		T0Return:        "--",
		T0Close:         "--",
		T0ReturnClass:   "astock-flat",
		BestReturn:      "--",
		BestReturnClass: "astock-flat",
		Status:          "等待行情同步",
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
	refreshMode := aStockRecommendationRebuild
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("refresh_mode")), string(aStockRecommendationPreserveLocked)) {
		refreshMode = aStockRecommendationPreserveLocked
	}
	ctx := s.loadAStockContextWithRecommendationPhasePersistenceMode(strategyDate, period.Key, 1, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, true, phase, newAStockRequestCache(), true, true, refreshMode)
	writeRawJSON(w, http.StatusOK, map[string]any{
		"code":    http.StatusOK,
		"message": "ok",
		"data": aStockRecommendationGenerateResult{
			StrategyDate:             ctx.Date,
			Period:                   ctx.Period,
			Phase:                    phase,
			RecommendationCount:      len(ctx.Recommendations),
			GeneratedCount:           ctx.GeneratedRecommendationCount,
			BacktestStatus:           ctx.BacktestStatus,
			RecentFiltered:           ctx.RecentFiltered,
			RecentLookbackDays:       aStockRecentLookbackDays,
			SameDayMorningFiltered:   ctx.SameDayMorningFiltered,
			LimitUpFiltered:          ctx.LimitUpFiltered,
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
	writeRawJSON(w, http.StatusOK, map[string]any{"ok": true, "key": key, "dismissed": updated.Dismissed})
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
		auctionResults:            make(map[string]aStockAuctionResultCacheEntry),
		sectorConstituents:        make(map[string]aStockSectorConstituentCodesCacheEntry),
		codeNames:                 make(map[string]map[string]string),
	}
}

func (s *Server) loadAStockContextWithCache(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, forceRecommendationRefresh bool, cache *aStockRequestCache) aStockContext {
	return s.loadAStockContextWithRecommendationPhasePersistence(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh, aStockRecommendationPhaseFinal, cache, true, true)
}

func (s *Server) loadAStockFastReadOnlyContextWithCache(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, includeHotspotTopStocks bool, cache *aStockRequestCache) (aStockContext, bool) {
	ctx := newAStockBaseContext(strategyDate, periodKey, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, aStockRecommendationPhaseFinal)
	ctx.FastReadOnly = true
	ctx.SourceRuns = s.loadAStockSourceRunsWithCache(cache)
	if err := s.populateAStockContextArticleStatsWithCache(&ctx, newsPage, cache); err != nil {
		ctx.LoadMessage = err.Error()
		return ctx, false
	}
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
	ctx.MarketCandidateCount = len(fixedPoolAStockMarketCandidates(aStockRecommendationHotspotSlice(ctx.Hotspots), candidates))
	if auctionLabel := normalizeAStockAuctionSummaryLabel(formatAStockAuctionSummaryAmount(auctionResult)); auctionLabel != "" {
		ctx.AuctionAmountLabel = auctionLabel
	}
	if includeHotspotTopStocks {
		ctx.Hotspots = buildAStockHotspotsWithTopStocks(ctx.Hotspots, candidates, aStockHotspotTopStockLimit)
		ctx.Hotspots = s.applyAStockHotspotRecommendationDatesWithCache(ctx.Hotspots, ctx.Date, ctx.Period, cache)
	}
	recommendationTarget := 0
	var recentReplacementPool []aStockRecommendation
	var recentCodes map[string]struct{}
	recentReplacementStatus := ""
	baseRecommendations := buildAStockSnapshotRecommendationsWithPhase(ctx.Date, ctx.Period, aStockRecommendationPhaseFinal, ctx.Articles, candidates)
	recommendationTarget = len(baseRecommendations)
	ctx.GeneratedRecommendationCount = recommendationTarget
	ctx.Recommendations = baseRecommendations
	if ctx.Period == "morning" && !ctx.IgnoreRecent && recommendationTarget > 0 {
		recentReplacementPool = buildAStockSnapshotReplacementRecommendations(ctx.Date, ctx.Period, aStockRecommendationPhaseFinal, ctx.Articles, candidates)
	}
	if ctx.Period == "afternoon" && len(ctx.Recommendations) > 0 {
		s.applyAStockAfternoonSameDayCapsWithCache(&ctx, candidates, cache)
	}
	if len(ctx.Recommendations) > 0 && !ctx.IgnoreRecent {
		recentCodes = s.loadRecentAStockRecommendationCodesForPeriodWithCache(ctx.Date, ctx.Period, aStockRecentLookbackDays, cache)
		if ctx.Period == "morning" && len(recentReplacementPool) > 0 && recommendationTarget > 0 {
			result := filterRecentAStockRecommendationsWithReplenishment(ctx.Recommendations, recentReplacementPool, recentCodes, recommendationTarget)
			ctx.Recommendations = result.Recommendations
			ctx.RecentFiltered = result.Filtered
			ctx.RecentReplenished = result.Replenished
			ctx.RecentReplenishShortfall = result.Shortfall
			recentReplacementStatus = formatAStockRecentReplenishmentStatus(result.Filtered, result.Replenished, result.Shortfall)
		} else {
			ctx.Recommendations, ctx.RecentFiltered = filterRecentAStockRecommendations(ctx.Recommendations, recentCodes)
		}
	}
	ctx.Recommendations = withAStockRecommendationEntryTimes(ctx.Recommendations, ctx.Period, "")
	ctx.Recommendations = initializeAStockRecommendationMarket(ctx.Recommendations)
	ctx.Backtests = buildAStockBacktestRows(ctx.Date, ctx.Period, ctx.Recommendations, nil)
	ctx.BacktestStatus = "只读快速推荐，等待行情同步"
	if recentReplacementStatus != "" {
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, recentReplacementStatus)
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
	ctx.Recommendations = rerankAStockRecommendations(recommendations)
	ctx.Backtests = filterAStockBacktestsForSnapshotRecommendations(backtests, ctx.Recommendations)
	ctx.BacktestStatus = nonEmpty(snapshot.BacktestStatus, "已读取推荐快照")
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
	ctx.RecentFiltered = snapshot.RecentFiltered
	ctx.SameDayMorningFiltered = snapshot.SameDayMorningFiltered
	ctx.LimitUpFilterEnabled = snapshot.LimitUpFilterEnabled
	ctx.LimitUpFiltered = snapshot.LimitUpFiltered
	ctx.FundFlowFilterEnabled = snapshot.FundFlowFilterEnabled
	ctx.IgnoreFundFlow = !snapshot.FundFlowFilterEnabled
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
	ctx.Hotspots = buildAStockHotspots(ctx.Articles)
	return nil
}

func (s *Server) loadAStockNewsStatsContextWithCache(strategyDate string, periodKey string, cache *aStockRequestCache) aStockContext {
	period := normalizeAStockPeriod(periodKey)
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	ctx := newAStockBaseContext(strategyDate, period.Key, 1, false, false, false, false, aStockRecommendationPhaseFinal)
	ctx.SourceRuns = s.loadAStockSourceRunsWithCache(cache)
	if err := s.populateAStockContextArticleStatsWithCache(&ctx, 1, cache); err != nil {
		ctx.LoadMessage = err.Error()
	}
	return ctx
}

func (s *Server) renderAStockNewsStatsHTML(strategyDate string) string {
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	cache := newAStockRequestCache()
	morningCtx := s.loadAStockNewsStatsContextWithCache(strategyDate, "morning", cache)
	afternoonCtx := s.loadAStockNewsStatsContextWithCache(strategyDate, "afternoon", cache)
	var b strings.Builder
	for _, msg := range []string{morningCtx.LoadMessage, afternoonCtx.LoadMessage} {
		msg = strings.TrimSpace(msg)
		if msg == "" {
			continue
		}
		b.WriteString(`<div class="astock-empty">`)
		b.WriteString(html.EscapeString(msg))
		b.WriteString(`</div>`)
	}
	renderAStockNewsStatsContent(&b, morningCtx, afternoonCtx)
	return b.String()
}

func (s *Server) loadAStockContextWithRecommendationPhasePersistenceModeEntryTime(strategyDate string, periodKey string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, forceRecommendationRefresh bool, recommendationPhase string, cache *aStockRequestCache, includeHotspotTopStocks bool, persist bool, refreshMode aStockRecommendationRefreshMode, entryTimeOverride string) aStockContext {
	period := normalizeAStockPeriod(periodKey)
	phase := normalizeAStockRecommendationPhase(recommendationPhase)
	strategyDate = normalizeAStockStrategyDate(strategyDate)
	ctx := newAStockBaseContext(strategyDate, period.Key, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, phase)
	ctx.SourceRuns = s.loadAStockSourceRunsWithCache(cache)
	if err := s.populateAStockContextArticleStatsWithCache(&ctx, newsPage, cache); err != nil {
		ctx.LoadMessage = err.Error()
		return ctx
	}
	ctx.AuctionAmountLabel = s.loadAStockAuctionAmountLabelWithCache(strategyDate, cache)
	var marketCandidates []aStockMarketCandidate
	if len(ctx.Hotspots) > 0 && includeHotspotTopStocks {
		candidates, candidateStatus, auctionResult := s.loadAStockMarketCandidatesWithStatusWithCache(strategyDate, cache)
		marketCandidates = candidates
		ctx.Hotspots = buildAStockHotspotsWithTopStocks(ctx.Hotspots, marketCandidates, aStockHotspotTopStockLimit)
		ctx.Hotspots = s.applyAStockHotspotRecommendationDatesWithCache(ctx.Hotspots, strategyDate, period.Key, cache)
		ctx.MarketCandidateStatus = candidateStatus
		ctx.MarketCandidateCount = len(fixedPoolAStockMarketCandidates(aStockRecommendationHotspotSlice(ctx.Hotspots), marketCandidates))
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
	allowPersistedRecommendations := phase == aStockRecommendationPhaseFinal && isAStockOfficialSelectionContext(ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
	rebuildRecommendations := refreshMode == aStockRecommendationRebuild
	if allowPersistedRecommendations && !forceRecommendationRefresh && s.applyAStockRecommendationSnapshotWithCache(&ctx, cache) {
		return ctx
	}
	if allowPersistedRecommendations && !rebuildRecommendations && s.applyAStockRecommendationSelectionsWithCache(&ctx, cache) {
		ctx.Recommendations = s.applyAStockHoldingSummariesWithCache(ctx.Recommendations, cache)
		fundFlowStatus := ""
		if ctx.FundFlowFilterEnabled && len(ctx.Recommendations) > 0 {
			result := s.applyAStockRecommendationFundFlowFilterWithCache(strategyDate, ctx.Recommendations, nil, nil, len(ctx.Recommendations), cache)
			ctx.Recommendations = result.Recommendations
			ctx.FundFlowFiltered = result.Filtered
			ctx.FundFlowMissingCount = result.Missing
			fundFlowStatus = formatAStockFundFlowFilterStatus(result.Filtered, result.Replenished, result.Missing, result.Shortfall)
		}
		ctx.Recommendations, ctx.Backtests, ctx.BacktestStatus, ctx.LimitUpFiltered = s.loadAStockLockedMarketView(strategyDate, ctx.Period, ctx.Recommendations)
		if forceRecommendationRefresh {
			s.restoreAStockBacktestsFromSnapshotWithCache(&ctx, cache)
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
		ctx.Recommendations = s.applyAStockHoldingSummariesWithCache(ctx.Recommendations, cache)
		ctx.Recommendations, ctx.Backtests, ctx.BacktestStatus, ctx.LimitUpFiltered = s.loadAStockLockedMarketView(strategyDate, ctx.Period, ctx.Recommendations)
		if forceRecommendationRefresh {
			s.restoreAStockBacktestsFromSnapshotWithCache(&ctx, cache)
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
	var recentReplacementPool []aStockRecommendation
	var fundFlowReplacementPool []aStockRecommendation
	var recentCodes map[string]struct{}
	recentReplacementStatus := ""
	fundFlowStatus := ""
	if len(ctx.Hotspots) > 0 {
		if marketCandidates == nil {
			candidates, candidateStatus, auctionResult := s.loadAStockMarketCandidatesWithStatusWithCache(strategyDate, cache)
			marketCandidates = candidates
			ctx.MarketCandidateStatus = candidateStatus
			ctx.MarketCandidateCount = len(fixedPoolAStockMarketCandidates(aStockRecommendationHotspotSlice(ctx.Hotspots), marketCandidates))
			if auctionLabel := normalizeAStockAuctionSummaryLabel(formatAStockAuctionSummaryAmount(auctionResult)); auctionLabel != "" {
				ctx.AuctionAmountLabel = auctionLabel
			}
		}
		candidates := marketCandidates
		sectorGate := s.loadAStockHotspotSectorGateWithCache(ctx.Hotspots, cache)
		baseRecommendations := buildAStockSnapshotRecommendationsWithPhaseAndSectorGate(strategyDate, period.Key, phase, ctx.Articles, candidates, sectorGate)
		recommendationTarget = len(baseRecommendations)
		ctx.GeneratedRecommendationCount = recommendationTarget
		ctx.Recommendations = baseRecommendations
		if period.Key == "morning" && !ctx.IgnoreRecent && recommendationTarget > 0 {
			recentReplacementPool = buildAStockSnapshotReplacementRecommendationsWithSectorGate(strategyDate, period.Key, phase, ctx.Articles, candidates, sectorGate)
		}
		if ctx.FundFlowFilterEnabled && recommendationTarget > 0 {
			fundFlowReplacementPool = buildAStockSnapshotReplacementRecommendationsWithSectorGate(strategyDate, period.Key, phase, ctx.Articles, candidates, sectorGate)
		}
		if ctx.LimitUpFilterEnabled && recommendationTarget > 0 {
			replacementPool := buildAStockSnapshotRecommendationsWithPhaseAndLimitAndSectorGate(strategyDate, period.Key, phase, ctx.Articles, candidates, aStockReplacementPoolLimit, aStockReplacementPerHotspot, sectorGate)
			ctx.Recommendations = mergeAStockLimitUpReplacementPool(ctx.Recommendations, replacementPool)
		}
		if period.Key == "afternoon" && len(ctx.Recommendations) > 0 {
			s.applyAStockAfternoonSameDayCapsWithCache(&ctx, candidates, cache)
		}
	}
	if len(ctx.Recommendations) > 0 && !ctx.IgnoreRecent {
		recentCodes = s.loadRecentAStockRecommendationCodesForPeriodWithCache(strategyDate, period.Key, aStockRecentLookbackDays, cache)
		if period.Key == "morning" && len(recentReplacementPool) > 0 && recommendationTarget > 0 {
			result := filterRecentAStockRecommendationsWithReplenishment(ctx.Recommendations, recentReplacementPool, recentCodes, recommendationTarget)
			ctx.Recommendations = result.Recommendations
			ctx.RecentFiltered = result.Filtered
			ctx.RecentReplenished = result.Replenished
			ctx.RecentReplenishShortfall = result.Shortfall
			recentReplacementStatus = formatAStockRecentReplenishmentStatus(result.Filtered, result.Replenished, result.Shortfall)
		} else {
			ctx.Recommendations, ctx.RecentFiltered = filterRecentAStockRecommendations(ctx.Recommendations, recentCodes)
		}
	}
	ctx.Recommendations = withAStockRecommendationEntryTimes(ctx.Recommendations, period.Key, entryTimeOverride)
	ctx.Recommendations = s.applyAStockHoldingSummariesWithCache(ctx.Recommendations, cache)
	if ctx.FundFlowFilterEnabled && len(ctx.Recommendations) > 0 {
		if len(fundFlowReplacementPool) > 0 {
			fundFlowReplacementPool = withAStockRecommendationEntryTimes(fundFlowReplacementPool, period.Key, entryTimeOverride)
			fundFlowReplacementPool = s.applyAStockHoldingSummariesWithCache(fundFlowReplacementPool, cache)
		}
		result := s.applyAStockRecommendationFundFlowFilterWithCache(strategyDate, ctx.Recommendations, fundFlowReplacementPool, recentCodes, recommendationTarget, cache)
		ctx.Recommendations = result.Recommendations
		ctx.FundFlowFiltered = result.Filtered
		ctx.FundFlowMissingCount = result.Missing
		fundFlowStatus = formatAStockFundFlowFilterStatus(result.Filtered, result.Replenished, result.Missing, result.Shortfall)
	}
	ctx.Recommendations, ctx.Backtests, ctx.BacktestStatus, ctx.LimitUpFiltered, ctx.NoTodayMarketCount = s.loadAStockMarketView(strategyDate, ctx.Period, ctx.Recommendations, ctx.LimitUpFilterEnabled, ctx.TodayMarketFilterEnabled, recommendationTarget)
	if forceRecommendationRefresh {
		s.restoreAStockBacktestsFromSnapshotWithCache(&ctx, cache)
	}
	if recentReplacementStatus != "" {
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, recentReplacementStatus)
	}
	if fundFlowStatus != "" {
		ctx.BacktestStatus = appendAStockBacktestStatus(ctx.BacktestStatus, fundFlowStatus)
	}
	if persist && shouldPersistAStockRecommendationSelections(ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket, forceRecommendationRefresh) && (len(ctx.Recommendations) > 0 || rebuildRecommendations) {
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

func isAStockOfficialSelectionContext(ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) bool {
	return !ignoreRecent && !ignoreLimitUp && !ignoreFundFlow && !filterTodayMarket
}

func shouldPersistAStockRecommendationSelections(ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool, forceRecommendationRefresh bool) bool {
	return forceRecommendationRefresh && isAStockOfficialSelectionContext(ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
}

func (s *Server) loadRecentAStockRecommendationCodesForPeriodWithCache(strategyDate string, period string, lookbackDays int, cache *aStockRequestCache) map[string]struct{} {
	result := s.loadRecentAStockRecommendationCodesWithCache(strategyDate, lookbackDays, cache)
	if normalizeAStockPeriod(period).Key == "afternoon" {
		if len(result) == 0 {
			result = make(map[string]struct{})
		}
		for code := range s.loadPersistedAStockRecommendationCodesWithCache(strategyDate, "morning", false, cache) {
			result[code] = struct{}{}
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
	if ctx == nil || !isAStockOfficialSelectionContext(ctx.IgnoreRecent, ctx.IgnoreLimitUp, ctx.IgnoreFundFlow, ctx.TodayMarketFilterEnabled) {
		return false
	}
	result, ok := s.loadAStockRecommendationSelectionsWithCache(ctx.Date, ctx.Period, cache)
	if !ok || len(result.Items) == 0 {
		return false
	}
	ctx.Recommendations, _ = s.repairAStockPersistedRecommendationsWithCache(ctx.Date, aStockRecommendationSelectionsToRecommendations(result.Items, ctx.Period), cache)
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
	if ctx == nil || strings.TrimSpace(s.cfg.ContentURL) == "" || !isAStockOfficialSelectionContext(ctx.IgnoreRecent, ctx.IgnoreLimitUp, ctx.IgnoreFundFlow, ctx.TodayMarketFilterEnabled) {
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
	ctx.Recommendations, _ = s.repairAStockPersistedRecommendationsWithCache(ctx.Date, recommendations, cache)
	if len(ctx.Recommendations) == 0 {
		return false
	}
	ctx.GeneratedRecommendationCount = snapshot.GeneratedCount
	if ctx.GeneratedRecommendationCount <= 0 {
		ctx.GeneratedRecommendationCount = len(ctx.Recommendations)
	}
	ctx.RecentFiltered = snapshot.RecentFiltered
	ctx.SameDayMorningFiltered = snapshot.SameDayMorningFiltered
	ctx.LimitUpFilterEnabled = snapshot.LimitUpFilterEnabled
	ctx.LimitUpFiltered = snapshot.LimitUpFiltered
	ctx.FundFlowFilterEnabled = snapshot.FundFlowFilterEnabled
	ctx.IgnoreFundFlow = !snapshot.FundFlowFilterEnabled
	ctx.FundFlowFiltered = snapshot.FundFlowFiltered
	ctx.FundFlowMissingCount = snapshot.FundFlowMissingCount
	ctx.TodayMarketFilterEnabled = snapshot.TodayMarketFilterEnabled
	ctx.NoTodayMarketCount = snapshot.NoTodayMarketCount
	ctx.MarketCandidateStatus = snapshot.MarketCandidateStatus
	ctx.MarketCandidateCount = snapshot.MarketCandidateCount
	if auctionLabel := normalizeAStockAuctionSummaryLabel(snapshot.AuctionAmountLabel); auctionLabel != "" {
		ctx.AuctionAmountLabel = auctionLabel
	}
	ctx.EmptyReason = snapshot.EmptyReason
	ctx.SnapshotUpdatedAt = snapshot.UpdatedAt
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
		ctx.GeneratedRecommendationCount = snapshot.GeneratedCount
		ctx.RecentFiltered = snapshot.RecentFiltered
		ctx.SameDayMorningFiltered = snapshot.SameDayMorningFiltered
		ctx.LimitUpFilterEnabled = snapshot.LimitUpFilterEnabled
		ctx.LimitUpFiltered = snapshot.LimitUpFiltered
		ctx.FundFlowFilterEnabled = snapshot.FundFlowFilterEnabled
		ctx.IgnoreFundFlow = !snapshot.FundFlowFilterEnabled
		ctx.FundFlowFiltered = snapshot.FundFlowFiltered
		ctx.FundFlowMissingCount = snapshot.FundFlowMissingCount
		ctx.TodayMarketFilterEnabled = snapshot.TodayMarketFilterEnabled
		ctx.NoTodayMarketCount = snapshot.NoTodayMarketCount
		ctx.MarketCandidateStatus = snapshot.MarketCandidateStatus
		ctx.MarketCandidateCount = snapshot.MarketCandidateCount
		if auctionLabel := normalizeAStockAuctionSummaryLabel(snapshot.AuctionAmountLabel); auctionLabel != "" {
			ctx.AuctionAmountLabel = auctionLabel
		}
		ctx.EmptyReason = nonEmpty(snapshot.EmptyReason, aStockRecommendationEmptyReason(*ctx))
		ctx.SnapshotUpdatedAt = snapshot.UpdatedAt
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
	ctx.Recommendations, _ = s.repairAStockPersistedRecommendationsWithCache(ctx.Date, recommendations, cache)
	if len(ctx.Recommendations) == 0 {
		return false
	}
	ctx.Backtests = filterAStockBacktestsForRecommendations(backtests, ctx.Recommendations)
	ctx.BacktestStatus = nonEmpty(snapshot.BacktestStatus, "已读取推荐快照")
	ctx.GeneratedRecommendationCount = snapshot.GeneratedCount
	ctx.RecentFiltered = snapshot.RecentFiltered
	ctx.SameDayMorningFiltered = snapshot.SameDayMorningFiltered
	ctx.LimitUpFilterEnabled = snapshot.LimitUpFilterEnabled
	ctx.LimitUpFiltered = snapshot.LimitUpFiltered
	ctx.FundFlowFilterEnabled = snapshot.FundFlowFilterEnabled
	ctx.IgnoreFundFlow = !snapshot.FundFlowFilterEnabled
	ctx.FundFlowFiltered = snapshot.FundFlowFiltered
	ctx.FundFlowMissingCount = snapshot.FundFlowMissingCount
	ctx.TodayMarketFilterEnabled = snapshot.TodayMarketFilterEnabled
	ctx.NoTodayMarketCount = snapshot.NoTodayMarketCount
	ctx.MarketCandidateStatus = snapshot.MarketCandidateStatus
	ctx.MarketCandidateCount = snapshot.MarketCandidateCount
	if auctionLabel := normalizeAStockAuctionSummaryLabel(snapshot.AuctionAmountLabel); auctionLabel != "" {
		ctx.AuctionAmountLabel = auctionLabel
	}
	ctx.EmptyReason = snapshot.EmptyReason
	ctx.SnapshotUpdatedAt = snapshot.UpdatedAt
	if ctx.EmptyReason == "" {
		ctx.EmptyReason = aStockRecommendationEmptyReason(*ctx)
	}
	return true
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
	recommendations, _ := s.repairAStockRecommendationsForPersistence(ctx.Date, ctx.Recommendations)
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
	recommendations, _ := s.repairAStockRecommendationsForPersistence(ctx.Date, ctx.Recommendations)
	recommendationsJSON, err := json.Marshal(recommendations)
	if err != nil {
		return err
	}
	backtestsJSON, err := json.Marshal(filterAStockBacktestsForRecommendations(ctx.Backtests, recommendations))
	if err != nil {
		return err
	}
	newsSummaryJSON, err := buildAStockSnapshotNewsSummaryJSON(ctx)
	if err != nil {
		return err
	}
	snapshot := model.AStockRecommendationSnapshot{
		StrategyDate:             ctx.Date,
		Period:                   ctx.Period,
		IgnoreRecent:             ctx.IgnoreRecent,
		RecommendationsJSON:      normalizeAStockSnapshotJSONArray(string(recommendationsJSON)),
		BacktestsJSON:            normalizeAStockSnapshotJSONArray(string(backtestsJSON)),
		NewsSummaryJSON:          newsSummaryJSON,
		BacktestStatus:           ctx.BacktestStatus,
		GeneratedCount:           ctx.GeneratedRecommendationCount,
		RecentFiltered:           ctx.RecentFiltered,
		SameDayMorningFiltered:   ctx.SameDayMorningFiltered,
		LimitUpFilterEnabled:     ctx.LimitUpFilterEnabled,
		LimitUpFiltered:          ctx.LimitUpFiltered,
		TodayMarketFilterEnabled: ctx.TodayMarketFilterEnabled,
		NoTodayMarketCount:       ctx.NoTodayMarketCount,
		FundFlowFilterEnabled:    ctx.FundFlowFilterEnabled,
		FundFlowFiltered:         ctx.FundFlowFiltered,
		FundFlowMissingCount:     ctx.FundFlowMissingCount,
		MarketCandidateStatus:    ctx.MarketCandidateStatus,
		MarketCandidateCount:     ctx.MarketCandidateCount,
		AuctionAmountLabel:       ctx.AuctionAmountLabel,
		EmptyReason:              ctx.EmptyReason,
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
	for _, value := range []string{row.EntryOpen, row.AfternoonOpen, row.T0Return, row.T0Close, row.BestReturn} {
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
		recommendations, _ := s.repairAStockPersistedRecommendationsWithCache(strategyDate, aStockRecommendationSelectionsToRecommendations(result.Items, normalizedPeriod), nil)
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
	recommendations, _ = s.repairAStockPersistedRecommendationsWithCache(strategyDate, recommendations, nil)
	if len(recommendations) == 0 {
		return nil, time.Time{}, false
	}
	return recommendations, snapshot.UpdatedAt, true
}

func aStockPreopenPopupWindows() []aStockPreopenPopupWindow {
	return []aStockPreopenPopupWindow{
		{
			Period:      "morning",
			KeyPrefix:   "a-stock-morning-preopen-recommendation-",
			Title:       "09:27 上午盘前推荐股票",
			WindowLabel: "08:00-09:26:59",
			StartHour:   9,
			StartMinute: 15,
			EndHour:     9,
			EndMinute:   30,
		},
		{
			Period:      "afternoon",
			KeyPrefix:   "a-stock-afternoon-preopen-recommendation-",
			Title:       "12:57 下午盘前推荐股票",
			WindowLabel: "09:30-12:56:59",
			StartHour:   12,
			StartMinute: 45,
			EndHour:     13,
			EndMinute:   0,
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
	case "crawl", "backfill_window_news", "backfill_morning_stock", "generate_morning_stock", "generate_afternoon_stock", "generate_ignore_recent_stock", "generate", "refresh_current_backtest", "recalculate":
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
	day, err := time.ParseInLocation("2006-01-02", date, aStockLocation())
	if err != nil {
		return false
	}
	if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
		return false
	}
	_, holiday := aStockMarketHolidays[date]
	return !holiday
}

func localAStockAdjacentTradingDay(date string, direction int) string {
	day, err := time.ParseInLocation("2006-01-02", date, aStockLocation())
	if err != nil {
		return ""
	}
	if direction == 0 {
		if isLocalAStockTradingDay(date) {
			return date
		}
		return localAStockAdjacentTradingDay(date, -1)
	}
	step := 1
	if direction < 0 {
		step = -1
	}
	for offset := step; offset >= -14 && offset <= 14; offset += step {
		candidate := day.AddDate(0, 0, offset).Format("2006-01-02")
		if isLocalAStockTradingDay(candidate) {
			return candidate
		}
	}
	return ""
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
		return fmt.Sprintf("暂无推荐股票：%s %s 已生成候选，但过滤上午同股票或已满热点名额 %d 只。", ctx.PeriodLabel, windowLabel, ctx.SameDayMorningFiltered)
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

func normalizeAStockIgnoreFundFlowFromRequest(r *http.Request) (bool, bool) {
	if r == nil {
		return false, false
	}
	if ignoreFundFlow, explicit := parseAStockFundFlowFilterValues(r.URL.Query()); explicit {
		return ignoreFundFlow, true
	}
	return aStockFundFlowFilterFromCookie(r)
}

func normalizeAStockIgnoreFundFlowFromForm(r *http.Request) (bool, bool) {
	if r == nil {
		return false, false
	}
	if ignoreFundFlow, explicit := parseAStockFundFlowFilterValues(r.Form); explicit {
		return ignoreFundFlow, true
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
	cacheKey := dateKey + "|" + fmt.Sprint(lookbackDays) + "|calendar"
	if cache != nil {
		if cached, ok := cache.recentCodes[cacheKey]; ok {
			return cached
		}
	}
	dates := recentAStockCalendarDates(dateKey, lookbackDays)
	result := make(map[string]struct{})
	for _, date := range dates {
		for _, period := range aStockPeriods() {
			for code := range s.loadPersistedAStockRecommendationCodesWithCache(date, period.Key, false, cache) {
				result[code] = struct{}{}
			}
		}
	}
	if cache != nil {
		cache.recentCodes[cacheKey] = result
	}
	return result
}

func recentAStockCalendarDates(strategyDate string, lookbackDays int) []string {
	day, err := time.ParseInLocation("2006-01-02", normalizeAStockStrategyDate(strategyDate), aStockLocation())
	if err != nil || lookbackDays <= 0 {
		return nil
	}
	dates := make([]string, 0, lookbackDays)
	for offset := 1; offset <= lookbackDays; offset++ {
		dates = append(dates, day.AddDate(0, 0, -offset).Format("2006-01-02"))
	}
	return dates
}

func (s *Server) applyAStockAfternoonSameDayCaps(ctx *aStockContext, candidates []aStockMarketCandidate) {
	s.applyAStockAfternoonSameDayCapsWithCache(ctx, candidates, nil)
}

func (s *Server) applyAStockAfternoonSameDayCapsWithCache(ctx *aStockContext, candidates []aStockMarketCandidate, cache *aStockRequestCache) {
	if ctx == nil || ctx.Period != "afternoon" || len(ctx.Recommendations) == 0 {
		return
	}
	morningRecommendations := s.loadSameDayMorningAStockRecommendationsWithCache(ctx.Date, candidates, cache)
	if len(morningRecommendations) == 0 {
		return
	}
	filtered, sameCodeFiltered := filterAStockRecommendationsByCodes(ctx.Recommendations, aStockRecommendationCodeSet(morningRecommendations))
	filtered, hotspotQuotaFiltered := filterAStockRecommendationsByMorningHotspotQuota(filtered, aStockRecommendationHotspotCounts(morningRecommendations), aStockStocksPerHotspot)
	ctx.Recommendations = filtered
	ctx.SameDayMorningFiltered += sameCodeFiltered + hotspotQuotaFiltered
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

func (s *Server) loadPersistedAStockRecommendations(strategyDate string, period string, ignoreRecent bool) []aStockRecommendation {
	return s.loadPersistedAStockRecommendationsWithCache(strategyDate, period, ignoreRecent, nil)
}

func (s *Server) loadPersistedAStockRecommendationsWithCache(strategyDate string, period string, ignoreRecent bool, cache *aStockRequestCache) []aStockRecommendation {
	if !ignoreRecent {
		if result, ok := s.loadAStockRecommendationSelectionsWithCache(strategyDate, period, cache); ok && len(result.Items) > 0 {
			recommendations, _ := s.repairAStockPersistedRecommendationsWithCache(strategyDate, aStockRecommendationSelectionsToRecommendations(result.Items, period), cache)
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
	recommendations, _ = s.repairAStockPersistedRecommendationsWithCache(strategyDate, recommendations, cache)
	return recommendations
}

func (s *Server) applyAStockHoldingSummaries(recommendations []aStockRecommendation) []aStockRecommendation {
	return s.applyAStockHoldingSummariesWithCache(recommendations, nil)
}

func (s *Server) applyAStockHoldingSummariesWithCache(recommendations []aStockRecommendation, cache *aStockRequestCache) []aStockRecommendation {
	if len(recommendations) == 0 {
		return recommendations
	}
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
		recommendations[i].MarketScore += bonus
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
	ctx.Recommendations = s.applyAStockRecommendationFundFlow5DWithCache(ctx.Date, ctx.Recommendations, cache)
}

func (s *Server) applyAStockRecommendationFundFlow5DWithCache(strategyDate string, recommendations []aStockRecommendation, cache *aStockRequestCache) []aStockRecommendation {
	if len(recommendations) == 0 {
		return recommendations
	}
	for i := range recommendations {
		code := normalizeAStockCode(recommendations[i].Code)
		if code == "" {
			recommendations[i] = applyAStockFundFlow5DAssessmentToRecommendation(recommendations[i], aStockFundFlow5DAssessment{Missing: true}, false)
			continue
		}
		assessment := s.assessAStockRecommendationFundFlow5DWithCache(strategyDate, code, cache)
		recommendations[i] = applyAStockFundFlow5DAssessmentToRecommendation(recommendations[i], assessment, false)
	}
	return recommendations
}

type aStockFundFlow5DAssessment struct {
	Total        float64
	NegativeDays int
	Missing      bool
}

type aStockFundFlowRecommendationFilterResult struct {
	Recommendations []aStockRecommendation
	Filtered        int
	Missing         int
	Replenished     int
	Shortfall       bool
}

func (s *Server) assessAStockRecommendationFundFlow5DWithCache(strategyDate string, code string, cache *aStockRequestCache) aStockFundFlow5DAssessment {
	code = normalizeAStockCode(code)
	if code == "" {
		return aStockFundFlow5DAssessment{Missing: true}
	}
	result, err := s.loadAStockStockFundFlowTrendWithCache(strategyDate, code, 5, cache)
	if err != nil || len(result.Items) == 0 {
		return aStockFundFlow5DAssessment{Missing: true}
	}
	assessment := aStockFundFlow5DAssessment{}
	for _, item := range result.Items {
		assessment.Total += item.MainNetInflow
		if item.MainNetInflow < 0 {
			assessment.NegativeDays++
		}
	}
	return assessment
}

func (s *Server) applyAStockRecommendationFundFlowFilterWithCache(strategyDate string, base []aStockRecommendation, replacementPool []aStockRecommendation, recentCodes map[string]struct{}, target int, cache *aStockRequestCache) aStockFundFlowRecommendationFilterResult {
	if target <= 0 {
		target = len(base)
	}
	result := aStockFundFlowRecommendationFilterResult{}
	if len(base) == 0 || target <= 0 {
		result.Recommendations = rerankAStockRecommendations(base)
		return result
	}
	seen := make(map[string]struct{}, len(base))
	baseHotspotCounts := make(map[string]int)
	keptHotspotCounts := make(map[string]int)
	kept := make([]aStockRecommendation, 0, minInt(len(base), target))
	isRecent := func(code string) bool {
		if len(recentCodes) == 0 {
			return false
		}
		_, ok := recentCodes[normalizeAStockCode(code)]
		return ok
	}
	appendRecommendation := func(rec aStockRecommendation, countFiltered bool, replenished bool) bool {
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
		assessment := s.assessAStockRecommendationFundFlow5DWithCache(strategyDate, code, cache)
		if !assessment.Missing && isAStockFundFlowHardFiltered(assessment) {
			if countFiltered {
				result.Filtered++
			}
			return false
		}
		rec = applyAStockFundFlow5DAssessmentToRecommendation(rec, assessment, true)
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
		appendRecommendation(rec, true, false)
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
			if appendRecommendation(rec, false, true) {
				hotspotDeficits[hotspot]--
			}
			if len(kept) >= target {
				break
			}
		}
		if len(kept) < target {
			for _, rec := range candidates {
				if appendRecommendation(rec, false, true) && len(kept) >= target {
					break
				}
			}
		}
	}
	result.Shortfall = len(kept) < target
	result.Recommendations = sortAStockRecommendationsByScore(kept)
	return result
}

func applyAStockFundFlow5DAssessmentToRecommendation(rec aStockRecommendation, assessment aStockFundFlow5DAssessment, applyScore bool) aStockRecommendation {
	if assessment.Missing {
		rec.FundFlow5D = "--"
		rec.FundFlow5DClass = "astock-flat"
		return rec
	}
	rec.FundFlow5D = formatSectorFundFlowMoney(assessment.Total)
	rec.FundFlow5DClass = aStockPctClass(assessment.Total)
	if !applyScore {
		return rec
	}
	scoreDelta := aStockFundFlowScoreAdjustment(assessment.Total)
	if scoreDelta == 0 {
		return rec
	}
	baseScore := rec.MarketScore
	if baseScore == 0 {
		baseScore = rec.HotspotScore
	}
	rec.MarketScore = baseScore + scoreDelta
	rec.Reason = appendAStockReason(rec.Reason, formatAStockFundFlowScoreReason(assessment.Total, scoreDelta))
	return rec
}

func aStockFundFlowScoreAdjustment(total float64) int {
	switch {
	case total >= aStockFundFlowStrongBonusThreshold:
		return 10
	case total >= aStockFundFlowBonusThreshold:
		return 5
	case total < 0:
		return -5
	default:
		return 0
	}
}

func isAStockFundFlowHardFiltered(assessment aStockFundFlow5DAssessment) bool {
	return assessment.Total < 0
}

func formatAStockFundFlowScoreReason(total float64, scoreDelta int) string {
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
	return fmt.Sprintf("5日主力资金%s %s，资金%s %d", direction, formatSectorFundFlowMoney(total), action, points)
}

func formatAStockFundFlowFilterStatus(filtered int, replenished int, missing int, shortfall bool) string {
	if filtered <= 0 && missing <= 0 {
		return ""
	}
	parts := make([]string, 0, 4)
	if filtered > 0 {
		parts = append(parts, fmt.Sprintf("资金过滤 %d 只", filtered))
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
	cacheKey := aStockAuctionCandidateCacheKey(strategyDate)
	if cache != nil && cache.auctionResults != nil {
		if cached, ok := cache.auctionResults[cacheKey]; ok {
			return cached.result, cached.found
		}
		if result, found, ok := s.loadCachedAStockAuctionCandidateResult(cacheKey); ok {
			cache.auctionResults[cacheKey] = aStockAuctionResultCacheEntry{result: result, found: found}
			return result, found
		}
		result, found := s.fetchAStockMarketCandidateResult(strategyDate)
		s.storeCachedAStockAuctionCandidateResult(cacheKey, result, found)
		cache.auctionResults[cacheKey] = aStockAuctionResultCacheEntry{result: result, found: found}
		return result, found
	}
	if result, found, ok := s.loadCachedAStockAuctionCandidateResult(cacheKey); ok {
		return result, found
	}
	result, found := s.fetchAStockMarketCandidateResult(strategyDate)
	s.storeCachedAStockAuctionCandidateResult(cacheKey, result, found)
	return result, found
}

func aStockAuctionCandidateCacheKey(strategyDate string) string {
	if strings.TrimSpace(strategyDate) == "" {
		return "__latest__"
	}
	return normalizeAStockStrategyDate(strategyDate)
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
	}
	delete(s.aStockAuctions, "__latest__")
}

func (s *Server) fetchAStockMarketCandidateResult(strategyDate string) (model.AStockAuctionListResult, bool) {
	var result model.AStockAuctionListResult
	query := "/api/v1/a-stock/auction?page=1&page_size=" + fmt.Sprint(aStockMarketCandidateLimit)
	if strings.TrimSpace(strategyDate) != "" {
		query += "&date=" + url.QueryEscape(normalizeAStockStrategyDate(strategyDate))
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
		candidates = append(candidates, aStockMarketCandidate{
			Code:          code,
			Name:          name,
			TradeDate:     nonEmpty(item.TradeDate, result.Date),
			Rank:          i + 1,
			AuctionAmount: item.AuctionAmount,
			AuctionVolume: item.AuctionVolume,
		})
	}
	return candidates
}

func filterRecentAStockRecommendations(recommendations []aStockRecommendation, recentCodes map[string]struct{}) ([]aStockRecommendation, int) {
	result := filterRecentAStockRecommendationsWithReplenishment(recommendations, nil, recentCodes, len(recommendations))
	return result.Recommendations, result.Filtered
}

type aStockRecentRecommendationFilterResult struct {
	Recommendations []aStockRecommendation
	Filtered        int
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
	recordRecent := func(code string) {
		code = normalizeAStockCode(code)
		if code == "" {
			return
		}
		if _, exists := filteredCodes[code]; exists {
			return
		}
		filteredCodes[code] = struct{}{}
		result.Filtered++
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
			recordRecent(code)
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
			recordRecent(code)
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

func formatAStockRecentReplenishmentStatus(filtered int, replenished int, shortfall bool) string {
	if filtered <= 0 {
		return ""
	}
	parts := []string{fmt.Sprintf("%s过滤 %d 只", aStockRecentLookbackStatusPrefix(), filtered)}
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
	recommendations = initializeAStockRecommendationMarket(recommendations)
	if len(recommendations) == 0 {
		return recommendations, nil, "无推荐股票", 0, 0
	}
	endpoint := aStockMarketEndpoint()
	codes := make([]string, 0, len(recommendations))
	for _, rec := range recommendations {
		codes = append(codes, rec.Code)
	}
	bars, err := s.loadAStockMarketBars(strategyDate, codes, endpoint)
	if err != nil {
		return recommendations, buildAStockBacktestRows(strategyDate, period, recommendations, nil), "行情读取失败", 0, 0
	}
	s.enrichAStockRecommendationEntryPrices(strategyDate, period, recommendations, bars)
	return applyAStockMarketBars(strategyDate, period, recommendations, bars, filterLimitUp, filterTodayMarket, maxRecommendations)
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
		return bars, err
	}
	s.enrichAStockSessionPrices(strategyDate, codes, bars)
	return bars, nil
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
	byCode := groupAStockMarketBars(bars)
	normalizedPeriod := normalizeAStockPeriod(period).Key
	withPrev := 0
	sectorPenalties := make(map[string]int)
	filteredCount := 0
	limitUpFilteredCount := 0
	noEntryPriceCount := 0
	waitingEntryPriceCount := 0
	filtered := make([]aStockRecommendation, 0, len(recommendations))
	for i := range recommendations {
		blockedByDrawdown := false
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
				continue
			}
		}
		if ok && entry.Close > 0 {
			recommendations[i].CurrentPrice = formatAStockPrice(entry.Close)
			recommendations[i].TodayPct = formatAStockPct(entry.Pct)
			recommendations[i].TodayPctClass = aStockPctClass(entry.Pct)
		}
		if ok && filterLimitUp && isAStockLimitUpPct(recommendations[i].Code, recommendations[i].Name, entry.Pct) {
			limitUpFilteredCount++
			continue
		}
		if prev, ok := previousAStockBar(byCode[recommendations[i].Code], strategyDate); ok {
			recommendations[i].PrevClose = formatAStockPrice(prev.Close)
			recommendations[i].PrevPct = formatAStockPct(prev.Pct)
			recommendations[i].PrevPctClass = aStockPctClass(prev.Pct)
			if change, ok := aStockLookbackChange(byCode[recommendations[i].Code], strategyDate, 30, prev.Close); ok {
				recommendations[i].Change30 = formatAStockPct(change)
				recommendations[i].Change30Class = aStockPctClass(change)
				if change <= aStockDrawdownFilterThreshold {
					blockedByDrawdown = true
				}
			}
			if change, ok := aStockLookbackChange(byCode[recommendations[i].Code], strategyDate, 60, prev.Close); ok {
				recommendations[i].Change60 = formatAStockPct(change)
				recommendations[i].Change60Class = aStockPctClass(change)
				if change <= aStockDrawdownFilterThreshold {
					blockedByDrawdown = true
				}
			}
			withPrev++
		}
		if blockedByDrawdown {
			sectorPenalties[recommendations[i].Hotspot] += aStockSectorDrawdownPenalty
			filteredCount++
			continue
		}
		filtered = append(filtered, recommendations[i])
	}
	recommendations = filtered
	for i := range recommendations {
		penalty := sectorPenalties[recommendations[i].Hotspot]
		baseScore := recommendations[i].MarketScore
		if baseScore == 0 {
			baseScore = recommendations[i].HotspotScore
		}
		recommendations[i].MarketScore = baseScore - penalty
		if penalty > 0 {
			recommendations[i].Reason = fmt.Sprintf("%s，板块回撤减分 %d，调整分 %d", recommendations[i].Reason, penalty, recommendations[i].MarketScore)
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
		status = fmt.Sprintf("%s，过滤回撤股票 %d", status, filteredCount)
	}
	if limitUpFilteredCount > 0 {
		status = fmt.Sprintf("%s，过滤涨停股票 %d", status, limitUpFilteredCount)
	}
	if noEntryPriceCount > 0 {
		if filterTodayMarket {
			status = fmt.Sprintf("%s，过滤无当日行情股票 %d", status, noEntryPriceCount)
		} else {
			status = fmt.Sprintf("%s，缺少当日行情股票 %d", status, noEntryPriceCount)
		}
	}
	if waitingEntryPriceCount > 0 {
		status = fmt.Sprintf("%s，等待下午开盘价股票 %d", status, waitingEntryPriceCount)
	}
	if len(recommendations) == 0 && limitUpFilteredCount > 0 {
		status = fmt.Sprintf("涨停过滤后无推荐股票，过滤涨停股票 %d", limitUpFilteredCount)
	}
	if len(recommendations) == 0 && filteredCount > 0 {
		status = fmt.Sprintf("回撤过滤后无推荐股票，过滤回撤股票 %d", filteredCount)
	}
	if filterTodayMarket && len(recommendations) == 0 && noEntryPriceCount > 0 {
		status = fmt.Sprintf("无当日行情可推荐，过滤无当日行情股票 %d", noEntryPriceCount)
		if filteredCount > 0 {
			status = fmt.Sprintf("%s，过滤回撤股票 %d", status, filteredCount)
		}
	}
	if filterLimitUp && limitUpFilteredCount > 0 && maxRecommendations > 0 {
		if preLimitCount >= maxRecommendations {
			status = fmt.Sprintf("%s，已按热度递补", status)
		} else {
			status = fmt.Sprintf("%s，候选不足未补满", status)
		}
	}
	if withPrev == 0 && completed == 0 && filteredCount == 0 && limitUpFilteredCount == 0 && noEntryPriceCount == 0 {
		status = "无匹配行情"
	}
	return recommendations, rows, status, limitUpFilteredCount, noEntryPriceCount
}

func applyAStockLockedMarketBars(strategyDate string, period string, recommendations []aStockRecommendation, bars []aStockMarketBar) ([]aStockRecommendation, []aStockBacktestRow, string, int) {
	byCode := groupAStockMarketBars(bars)
	normalizedPeriod := normalizeAStockPeriod(period).Key
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
		if prev, ok := previousAStockBar(codeBars, strategyDate); ok {
			recommendations[i].PrevClose = formatAStockPrice(prev.Close)
			recommendations[i].PrevPct = formatAStockPct(prev.Pct)
			recommendations[i].PrevPctClass = aStockPctClass(prev.Pct)
			if change, ok := aStockLookbackChange(codeBars, strategyDate, 30, prev.Close); ok {
				recommendations[i].Change30 = formatAStockPct(change)
				recommendations[i].Change30Class = aStockPctClass(change)
			}
			if change, ok := aStockLookbackChange(codeBars, strategyDate, 60, prev.Close); ok {
				recommendations[i].Change60 = formatAStockPct(change)
				recommendations[i].Change60Class = aStockPctClass(change)
			}
		}
	}
	rows := buildAStockBacktestRows(strategyDate, period, recommendations, byCode)
	completed := 0
	waitingEntry := 0
	noData := 0
	for _, row := range rows {
		switch {
		case row.Status == "已回测" || strings.HasPrefix(row.Status, "已回测"):
			completed++
		case row.Status == "等待下午开盘价" || row.Status == "等待当日开盘价":
			waitingEntry++
		case row.Status == "无行情数据":
			noData++
		}
	}
	status := fmt.Sprintf("已锁定推荐股票，已回测 %d/%d", completed, len(rows))
	if waitingEntry > 0 {
		if normalizedPeriod == "afternoon" {
			status = fmt.Sprintf("%s，等待下午开盘价股票 %d", status, waitingEntry)
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
	return recommendations, rows, status, 0
}

func isAStockLimitUpPct(code string, name string, pct float64) bool {
	return pct >= aStockLimitUpPctThreshold(code, name)
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
	for _, bar := range bars {
		if bar.Date == strategyDate && aStockEntryPriceForRecommendation(bar, period, rec) > 0 {
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
			Stock:           rec.Code + " " + rec.Name,
			EntryOpen:       "--",
			AfternoonOpen:   "--",
			T0Return:        "--",
			T0Close:         "--",
			T0ReturnClass:   "astock-flat",
			Days:            make([]aStockBacktestCell, 5),
			BestReturn:      "--",
			BestReturnClass: "astock-flat",
			Status:          "等待行情接口配置",
		}
		for i := range row.Days {
			row.Days[i] = aStockBacktestCell{Close: "--", Return: "--", ReturnClass: "astock-flat"}
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
				Close:       formatAStockPrice(bar.Close),
				Return:      formatAStockPct(ret),
				ReturnClass: aStockPctClass(ret),
			}
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

func aStockEntryBarIndex(bars []aStockMarketBar, strategyDate string, period string, rec aStockRecommendation) int {
	for i, bar := range bars {
		if bar.Date == strategyDate && aStockEntryPriceForRecommendation(bar, period, rec) > 0 {
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
						Open  []*float64 `json:"open"`
						Close []*float64 `json:"close"`
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
		bars = append(bars, aStockMarketBar{
			Code:  code,
			Date:  time.Unix(ts, 0).In(location).Format("2006-01-02"),
			Open:  open,
			Close: closeValue,
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
	closeValue, ok := firstFloat(row, "close", "close_price", "pre_close")
	pct, _ := firstFloat(row, "pct", "pct_chg", "change_pct")
	entryPrice, _ := firstFloat(row, "entry_price", "entry", "open0930", "open_0930", "price0930", "price_0930", "minute0930", "minute_0930")
	afternoonEntryPrice, _ := firstFloat(row, "afternoon_entry_price", "afternoon_entry", "afternoon_open", "afternoon_open_price", "open1300", "open_1300", "price1300", "price_1300", "minute1300", "minute_1300")
	if code == "" || date == "" || !ok {
		return aStockMarketBar{}, false
	}
	return aStockMarketBar{Code: code, Date: date, Open: open, Close: closeValue, Pct: pct, EntryPrice: entryPrice, AfternoonEntryPrice: afternoonEntryPrice}, true
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
	pct := parseAStockFloat(parts[8])
	return aStockMarketBar{Date: normalizeAStockMarketDate(parts[0]), Open: open, Close: closeValue, Pct: pct}, true
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

func aStockMarketEndpoint() string {
	return strings.TrimSpace(os.Getenv("YUQING_ASTOCK_MARKET_URL"))
}

func aStockMarketConfigHint() string {
	if aStockMarketEndpoint() == "" {
		return "默认东方财富日 K，失败后使用 Yahoo Finance 日线；也可用 YUQING_ASTOCK_MARKET_URL 覆盖"
	}
	return "YUQING_ASTOCK_MARKET_URL，失败后回退东方财富日 K / Yahoo Finance 日线"
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
		negativeNewsPenalty := negativeNewsCount * aStockNegativeNewsPenalty
		score := len(matches)*10 + len(keywords)*3 - negativeNewsPenalty
		if score < 1 {
			score = 1
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
	if len(hotspots) > 8 {
		return hotspots[:8]
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
	return fmt.Sprintf("负面新闻 %d 条，板块减分 %d", hotspot.NegativeNewsCount, hotspot.NegativeNewsPenalty)
}

func buildAStockHotspotsWithTopStocks(hotspots []aStockHotspot, marketCandidates []aStockMarketCandidate, limit int) []aStockHotspot {
	if len(hotspots) == 0 {
		return hotspots
	}
	if limit <= 0 {
		limit = aStockHotspotTopStockLimit
	}
	candidates := aStockHotspotTopStockCandidates(hotspots, marketCandidates)
	result := make([]aStockHotspot, len(hotspots))
	copy(result, hotspots)
	seen := make(map[string]struct{})
	for i := range result {
		result[i].TopStocks = buildAStockHotspotTopStocksWithSeen(result[i], candidates.scored, candidates.fallback, limit, seen)
	}
	return result
}

func buildAStockHotspotTopStocks(hotspot aStockHotspot, candidates []aStockMarketCandidate, limit int) []aStockHotspotStock {
	return buildAStockHotspotTopStocksWithSeen(hotspot, candidates, sortedAStockHotspotFallbackCandidates(candidates), limit, nil)
}

func buildAStockHotspotTopStocksWithSeen(hotspot aStockHotspot, scoredCandidates []aStockMarketCandidate, fallbackCandidates []aStockMarketCandidate, limit int, seen map[string]struct{}) []aStockHotspotStock {
	if limit <= 0 {
		limit = aStockHotspotTopStockLimit
	}
	scored := scoreAStockMarketCandidates(hotspot, scoredCandidates)
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
		stocks = appendAStockHotspotTopStock(stocks, stock, hotspot.Score+aStockMarketRankScore(stock.Rank), limit, seen, rowSeen)
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
	fixedCandidates := fixedPoolAStockMarketCandidates(hotspots, marketCandidates)
	fallbackCandidates := sortedAStockHotspotFallbackCandidates(marketCandidates)
	scoredCandidates := make([]aStockMarketCandidate, 0, len(fixedCandidates)+minInt(aStockHotspotScoredCandidateLimit, len(fallbackCandidates)))
	seen := make(map[string]struct{}, len(fixedCandidates)+minInt(aStockHotspotScoredCandidateLimit, len(fallbackCandidates)))
	for _, candidate := range fixedCandidates {
		code := normalizeAStockCode(candidate.Code)
		if code != "" {
			scoredCandidates = append(scoredCandidates, candidate)
			seen[code] = struct{}{}
		}
	}
	for _, candidate := range fallbackCandidates {
		if len(scoredCandidates) >= len(fixedCandidates)+aStockHotspotScoredCandidateLimit {
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

func buildAStockRecommendations(hotspots []aStockHotspot, candidates []aStockMarketCandidate) []aStockRecommendation {
	return buildAStockRecommendationsWithLimit(hotspots, candidates, aStockRecommendationLimit, aStockStocksPerHotspot)
}

func buildAStockRecommendationsWithLimit(hotspots []aStockHotspot, candidates []aStockMarketCandidate, maxRecommendations int, maxPerHotspot int) []aStockRecommendation {
	return buildAStockRecommendationsWithLimitAndSectorGate(hotspots, candidates, maxRecommendations, maxPerHotspot, nil)
}

func buildAStockRecommendationsWithLimitAndSectorGate(hotspots []aStockHotspot, candidates []aStockMarketCandidate, maxRecommendations int, maxPerHotspot int, sectorGate *aStockHotspotSectorGate) []aStockRecommendation {
	if len(hotspots) > aStockHotspotLimit {
		hotspots = hotspots[:aStockHotspotLimit]
	}
	if maxRecommendations <= 0 {
		maxRecommendations = aStockRecommendationLimit
	}
	if maxPerHotspot <= 0 {
		maxPerHotspot = aStockStocksPerHotspot
	}
	candidates = aStockRecommendationCandidatesForLimit(hotspots, candidates, maxRecommendations, maxPerHotspot)
	if len(candidates) == 0 {
		return nil
	}
	recommendations := make([]aStockRecommendation, 0)
	seen := make(map[string]struct{})
	for _, hotspot := range hotspots {
		stocks := scoreAStockMarketCandidatesWithSectorGate(hotspot, candidates, sectorGate)
		picked := 0
		for _, stock := range stocks {
			if _, exists := seen[stock.Code]; exists {
				continue
			}
			seen[stock.Code] = struct{}{}
			marketScore := hotspot.Score + stock.MatchedScore
			reason := ""
			if stock.FixedPool {
				reason = fmt.Sprintf(
					"命中 %s，证据新闻 %d 条，热度分 %d；使用原始固定股票池，个股证据 %d 条，匹配分 %d，综合分 %d",
					strings.Join(hotspot.Keywords, "、"),
					hotspot.Evidence,
					hotspot.Score,
					stock.Evidence,
					stock.MatchedScore,
					marketScore,
				)
				if stock.AuctionAmount > 0 || stock.AuctionVolume > 0 {
					reason = fmt.Sprintf("%s，集合竞价排名 %d，成交额 %s", reason, stock.Rank, formatAStockAuctionMoney(stock.AuctionAmount))
				}
			} else if stock.Fallback {
				reason = fmt.Sprintf(
					"命中 %s，证据新闻 %d 条，热度分 %d；使用实时新闻明确提及股票，个股证据 %d 条，匹配分 %d，综合分 %d",
					strings.Join(hotspot.Keywords, "、"),
					hotspot.Evidence,
					hotspot.Score,
					stock.Evidence,
					stock.MatchedScore,
					marketScore,
				)
			} else {
				reason = fmt.Sprintf(
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
			if len(stock.Keywords) > 0 && !stock.Fallback && !stock.FixedPool {
				reason = fmt.Sprintf("%s，股票名命中 %s", reason, strings.Join(stock.Keywords, "、"))
			}
			reason = appendAStockReason(reason, formatAStockHotspotNegativeNewsPenaltyReason(hotspot))
			recommendations = append(recommendations, aStockRecommendation{
				Rank:         len(recommendations) + 1,
				Hotspot:      hotspot.Name,
				Code:         stock.Code,
				Name:         stock.Name,
				HotspotScore: hotspot.Score,
				MarketScore:  marketScore,
				Reason:       reason,
			})
			picked++
			if picked >= maxPerHotspot || len(recommendations) >= maxRecommendations {
				break
			}
		}
		if len(recommendations) >= maxRecommendations {
			return recommendations
		}
	}
	return recommendations
}

func aStockRecommendationCandidatesForLimit(hotspots []aStockHotspot, marketCandidates []aStockMarketCandidate, maxRecommendations int, maxPerHotspot int) []aStockMarketCandidate {
	fixedCandidates := fixedPoolAStockMarketCandidates(hotspots, marketCandidates)
	if maxRecommendations <= aStockRecommendationLimit && maxPerHotspot <= aStockStocksPerHotspot {
		return fixedCandidates
	}
	nameResolver := newAStockRecommendationNameResolver(marketCandidates)
	candidates := make([]aStockMarketCandidate, 0, len(fixedCandidates)+aStockHotspotScoredCandidateLimit)
	seen := make(map[string]struct{}, len(fixedCandidates)+aStockHotspotScoredCandidateLimit)
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
	for _, candidate := range fixedCandidates {
		appendCandidate(candidate)
	}
	for _, candidate := range newsDerivedAStockMarketCandidates(hotspots) {
		appendCandidate(candidate)
	}
	for _, candidate := range sortedAStockHotspotFallbackCandidates(marketCandidates) {
		if len(candidates) >= len(fixedCandidates)+aStockHotspotScoredCandidateLimit {
			break
		}
		appendCandidate(candidate)
	}
	return candidates
}

func newAStockRecommendationNameResolver(marketCandidates []aStockMarketCandidate) map[string]string {
	names := make(map[string]string)
	for _, rule := range aStockTopicRulesByName() {
		for _, stock := range rule.Stocks {
			addAStockRecommendationResolvedName(names, stock.Code, stock.Name)
		}
	}
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
	if name == resolved {
		return true
	}
	return strings.Contains(resolved, name) || strings.Contains(name, resolved)
}

func hasResolvedAStockRecommendationName(code string, name string) bool {
	name = astockcode.DisplayName(code, name)
	return astockcode.HasResolvedName(code, name)
}

func isAStockRecommendationPlaceholderName(name string) bool {
	return astockcode.IsPlaceholderName(name)
}

func fixedPoolAStockMarketCandidates(hotspots []aStockHotspot, marketCandidates []aStockMarketCandidate) []aStockMarketCandidate {
	rules := aStockTopicRulesByName()
	marketByCode := make(map[string]aStockMarketCandidate, len(marketCandidates))
	for _, candidate := range marketCandidates {
		code := normalizeAStockCode(candidate.Code)
		if !astockcode.IsShanghaiShenzhen(code) {
			continue
		}
		candidate.Code = code
		marketByCode[code] = candidate
	}
	candidates := make([]aStockMarketCandidate, 0)
	seen := make(map[string]struct{})
	fallbackRank := 1
	for _, hotspot := range hotspots {
		rule, ok := rules[hotspot.Name]
		if !ok {
			continue
		}
		for _, stock := range rule.Stocks {
			code := normalizeAStockCode(stock.Code)
			name := astockcode.DisplayName(code, stock.Name)
			if !astockcode.IsShanghaiShenzhen(code) || !hasResolvedAStockRecommendationName(code, name) {
				continue
			}
			if isBlockedAStockRecommendationStock(code, name) {
				continue
			}
			if _, exists := seen[code]; exists {
				continue
			}
			seen[code] = struct{}{}
			candidate := aStockMarketCandidate{
				Code:      code,
				Name:      name,
				Rank:      fallbackRank,
				Keywords:  append([]string(nil), rule.Keywords...),
				FixedPool: true,
			}
			fallbackRank++
			if marketCandidate, ok := marketByCode[code]; ok {
				candidate.TradeDate = marketCandidate.TradeDate
				candidate.Rank = marketCandidate.Rank
				candidate.AuctionAmount = marketCandidate.AuctionAmount
				candidate.AuctionVolume = marketCandidate.AuctionVolume
				if marketName := astockcode.DisplayName(code, marketCandidate.Name); hasResolvedAStockRecommendationName(code, marketName) {
					candidate.Name = marketName
				}
			}
			candidates = append(candidates, candidate)
		}
	}
	return candidates
}

func aStockRecommendationHotspotSlice(hotspots []aStockHotspot) []aStockHotspot {
	if len(hotspots) > aStockHotspotLimit {
		return hotspots[:aStockHotspotLimit]
	}
	return hotspots
}

func aStockTopicRulesByName() map[string]aStockTopicRule {
	rules := aStockTopicRules()
	byName := make(map[string]aStockTopicRule, len(rules))
	for _, rule := range rules {
		byName[rule.Name] = rule
	}
	return byName
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
	snapshots := aStockRecommendationSnapshots(strategyDate, periodKey, phase)
	if len(snapshots) == 0 {
		return withAStockRecommendationEntryTimes(buildAStockRecommendationsWithLimitAndSectorGate(buildAStockHotspots(articles), candidates, maxRecommendations, maxPerHotspot, sectorGate), periodKey, "")
	}
	if maxRecommendations <= 0 {
		maxRecommendations = aStockRecommendationLimit
	}
	if maxPerHotspot <= 0 {
		maxPerHotspot = aStockStocksPerHotspot
	}
	combined := make([]aStockRecommendation, 0)
	seen := make(map[string]struct{})
	hotspotCounts := make(map[string]int)
	for _, snapshot := range snapshots {
		snapshotArticles := filterAStockArticlesByPublishWindow(articles, snapshot.Start, snapshot.End)
		if len(snapshotArticles) == 0 {
			continue
		}
		for _, rec := range buildAStockRecommendationsWithLimitAndSectorGate(buildAStockHotspots(snapshotArticles), candidates, maxRecommendations, maxPerHotspot, sectorGate) {
			code := normalizeAStockCode(rec.Code)
			if code == "" {
				continue
			}
			if _, exists := seen[code]; exists {
				continue
			}
			hotspotKey := normalizeAStockRecommendationHotspot(rec.Hotspot)
			if hotspotKey != "" {
				if _, exists := hotspotCounts[hotspotKey]; !exists && len(hotspotCounts) >= aStockHotspotLimit {
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
		return withAStockRecommendationEntryTimes(buildAStockRecommendationsWithLimitAndSectorGate(buildAStockHotspots(articles), candidates, maxRecommendations, maxPerHotspot, sectorGate), periodKey, "")
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
	if g == nil || candidate.FixedPool || !candidate.Fallback {
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
	sectorType := normalizeSectorFundFlowSectorType(alias.SectorType)
	sectorName := strings.TrimSpace(alias.SectorName)
	if sectorName == "" {
		return nil
	}
	key := sectorType + "|" + sectorName
	if cache != nil {
		if cache.sectorConstituents == nil {
			cache.sectorConstituents = make(map[string]aStockSectorConstituentCodesCacheEntry)
		}
		if entry, ok := cache.sectorConstituents[key]; ok && entry.found {
			return entry.codes
		}
	}
	codes := s.loadAStockCachedSectorConstituentCodes(sectorType, sectorName)
	if len(codes) == 0 {
		items, _, err := s.refreshSectorFundFlowConstituents(model.AStockSectorFundFlowListResult{
			SectorType: sectorType,
			Indicator:  "今日",
		}, sectorName)
		if err == nil {
			codes = aStockSectorConstituentCodes(items)
		}
	}
	if cache != nil {
		cache.sectorConstituents[key] = aStockSectorConstituentCodesCacheEntry{codes: codes, found: true}
	}
	return codes
}

func (s *Server) loadAStockCachedSectorConstituentCodes(sectorType string, sectorName string) map[string]struct{} {
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
	return aStockSectorConstituentCodes(result.Items)
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

func aStockRecommendationSnapshots(strategyDate string, periodKey string, phase string) []aStockRecommendationSnapshot {
	location := aStockLocation()
	day, err := time.ParseInLocation("2006-01-02", normalizeAStockStrategyDate(strategyDate), location)
	if err != nil {
		return nil
	}
	period := normalizeAStockPeriod(periodKey)
	normalizedPhase := normalizeAStockRecommendationPhase(phase)
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
		rec.MarketScore = lowScore - len(merged) - 1
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
		filtered = append(filtered, rec)
	}
	return rerankAStockRecommendations(filtered), skipped
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

func (s *Server) repairAStockRecommendationsForPersistence(strategyDate string, recommendations []aStockRecommendation) ([]aStockRecommendation, int) {
	return s.repairAStockPersistedRecommendationsWithCache(strategyDate, recommendations, newAStockRequestCache())
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
		evidence := evidenceIndex.Count(candidate)
		keywords := aStockCandidateKeywordMatches(candidate.Name, hotspot.Keywords)
		if (candidate.Fallback || candidate.FixedPool) && len(keywords) == 0 {
			keywords = intersectAStockKeywords(candidate.Keywords, hotspot.Keywords)
		}
		if evidence == 0 && len(keywords) == 0 {
			continue
		}
		if !sectorGate.Allows(hotspot, candidate) {
			continue
		}
		candidate.Evidence = evidence
		candidate.Keywords = keywords
		candidate.MatchedScore = aStockMarketRankScore(candidate.Rank) + evidence*25 + len(keywords)*12
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
	if rank <= 0 {
		return 0
	}
	score := (aStockMarketRankScoreBase - rank + 1) / 5
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
		})
	}
	return index
}

func (idx aStockStockEvidenceIndex) Count(candidate aStockMarketCandidate) int {
	if len(idx.items) == 0 {
		return 0
	}
	code := normalizeAStockCode(candidate.Code)
	name := strings.ToLower(strings.TrimSpace(candidate.Name))
	if code == "" && name == "" {
		return 0
	}
	count := 0
	for _, item := range idx.items {
		if code != "" {
			if _, ok := item.codes[code]; ok {
				count++
				continue
			}
		}
		if name != "" && strings.Contains(item.text, name) {
			count++
		}
	}
	return count
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
		{Name: "人工智能", Keywords: []string{"AI", "人工智能", "大模型", "算力", "AIGC", "机器人"}, Stocks: []aStockStockPick{{Code: "002230", Name: "科大讯飞"}, {Code: "603019", Name: "中科曙光"}, {Code: "601138", Name: "工业富联"}}},
		{Name: "半导体", Keywords: []string{"半导体", "芯片", "光刻", "晶圆", "存储", "先进封装"}, Stocks: []aStockStockPick{{Code: "688981", Name: "中芯国际"}, {Code: "002371", Name: "北方华创"}, {Code: "603986", Name: "兆易创新"}}},
		{Name: "新能源", Keywords: []string{"新能源", "锂电", "储能", "光伏", "风电", "充电桩"}, Stocks: []aStockStockPick{{Code: "300750", Name: "宁德时代"}, {Code: "300274", Name: "阳光电源"}, {Code: "601012", Name: "隆基绿能"}}},
		{Name: "低空经济", Keywords: []string{"低空经济", "eVTOL", "无人机", "通航", "飞行汽车"}, Stocks: []aStockStockPick{{Code: "000099", Name: "中信海直"}, {Code: "002085", Name: "万丰奥威"}, {Code: "300124", Name: "汇川技术"}}},
		{Name: "金融券商", Keywords: []string{"券商", "证券", "银行", "保险", "降准", "降息", "资本市场"}, Stocks: []aStockStockPick{{Code: "300059", Name: "东方财富"}, {Code: "600030", Name: "中信证券"}, {Code: "600036", Name: "招商银行"}}},
		{Name: "黄金有色", Keywords: []string{"黄金", "有色", "铜", "铝", "稀土", "贵金属"}, Stocks: []aStockStockPick{{Code: "600547", Name: "山东黄金"}, {Code: "601899", Name: "紫金矿业"}, {Code: "600111", Name: "北方稀土"}}},
		{Name: "医药生物", Keywords: []string{"医药", "创新药", "疫苗", "医疗器械", "CXO"}, Stocks: []aStockStockPick{{Code: "600276", Name: "恒瑞医药"}, {Code: "300760", Name: "迈瑞医疗"}, {Code: "603259", Name: "药明康德"}}},
		{Name: "消费电子", Keywords: []string{"消费电子", "苹果", "华为", "手机", "MR", "AR", "VR"}, Stocks: []aStockStockPick{{Code: "002475", Name: "立讯精密"}, {Code: "000725", Name: "京东方A"}, {Code: "300433", Name: "蓝思科技"}}},
		{Name: "房地产", Keywords: []string{"房地产", "地产", "房贷", "楼市", "保障房"}, Stocks: []aStockStockPick{{Code: "000002", Name: "万科A"}, {Code: "001979", Name: "招商蛇口"}, {Code: "600048", Name: "保利发展"}}},
		{Name: "军工航天", Keywords: []string{"军工", "航天", "卫星", "商业航天", "航空发动机"}, Stocks: []aStockStockPick{{Code: "600760", Name: "中航沈飞"}, {Code: "600893", Name: "航发动力"}, {Code: "002179", Name: "中航光电"}}},
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
	if ignoreFundFlow {
		href += "&ignore_fund_flow=1"
	}
	if filterTodayMarket {
		href += "&filter_today_market=1"
	}
	return href
}

func aStockFilterToggleHref(strategyDate string, period string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) string {
	return aStockFilterToggleHrefForPath("/a-stock", strategyDate, period, newsPage, ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
}

func aStockFilterToggleHrefForPath(targetPath string, strategyDate string, period string, newsPage int, ignoreRecent bool, ignoreLimitUp bool, ignoreFundFlow bool, filterTodayMarket bool) string {
	return aStockPageHrefForPath(targetPath, strategyDate, period, newsPage, !ignoreRecent, ignoreLimitUp, ignoreFundFlow, filterTodayMarket)
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
	return fmt.Sprintf("%d日", aStockRecentLookbackDays)
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

func writeAStockFundFlowFilterInput(b *strings.Builder, ignoreFundFlow bool) {
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
	}
}

func normalizeAStockPeriod(raw string) aStockPeriod {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "after" || raw == "pm" {
		raw = "afternoon"
	}
	for _, period := range aStockPeriods() {
		if period.Key == raw {
			return period
		}
	}
	return aStockPeriods()[0]
}
