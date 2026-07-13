package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"

	"github.com/pcdogyu/yuqing/go/internal/model"
	"github.com/pcdogyu/yuqing/go/internal/provider"
)

type Job struct {
	Name           string     `json:"name"`
	Group          string     `json:"group"`
	Description    string     `json:"description"`
	JavaQuartzName string     `json:"java_quartz_name"`
	Cron           string     `json:"cron"`
	IntervalSec    int64      `json:"interval_sec"`
	Enabled        bool       `json:"enabled"`
	TimeoutSec     int64      `json:"timeout_sec"`
	RetryCount     int        `json:"retry_count"`
	NextRunAt      *time.Time `json:"next_run_at,omitempty"`
	LastStatus     string     `json:"last_status,omitempty"`
	LastMessage    string     `json:"last_message,omitempty"`
	LastStartedAt  *time.Time `json:"last_started_at,omitempty"`
	LastFinishedAt *time.Time `json:"last_finished_at,omitempty"`
}

type jobDefinition struct {
	Name           string
	Group          string
	Description    string
	JavaQuartzName string
	Cron           string
	Interval       time.Duration
	Enabled        bool
	Run            func(context.Context) error
}

type crawlLinkHeartbeatSite struct {
	SourceType string
	Name       string
	URL        string
}

func (w *Worker) Jobs() []Job {
	defs := w.jobDefinitions()
	lastRuns := w.lastTaskRunsByName(context.Background())
	jobs := make([]Job, 0, len(defs))
	now := time.Now().UTC()
	for _, def := range defs {
		job := Job{
			Name:           def.Name,
			Group:          def.Group,
			Description:    def.Description,
			JavaQuartzName: def.JavaQuartzName,
			Cron:           def.Cron,
			IntervalSec:    int64(def.Interval / time.Second),
			Enabled:        def.Enabled,
			TimeoutSec:     int64(w.cfg.HTTPTimeout / time.Second),
			RetryCount:     2,
		}
		if run, ok := lastRuns[def.Name]; ok {
			job.LastStatus = run.Status
			job.LastMessage = run.Message
			job.LastStartedAt = &run.StartedAt
			job.LastFinishedAt = run.FinishedAt
			if def.Enabled {
				if next, err := nextCronRun(def.Cron, now); err == nil {
					job.NextRunAt = &next
				} else {
					job.LastStatus = "invalid_cron"
					job.LastMessage = err.Error()
				}
			}
		} else if def.Enabled {
			if next, err := nextCronRun(def.Cron, now); err == nil {
				job.NextRunAt = &next
			} else {
				job.LastStatus = "invalid_cron"
				job.LastMessage = err.Error()
			}
		}
		jobs = append(jobs, job)
	}
	return jobs
}

func (w *Worker) lastTaskRunsByName(ctx context.Context) map[string]model.TaskRun {
	result := map[string]model.TaskRun{}
	store, err := w.ensureStore()
	if err != nil {
		return result
	}
	runs, err := store.ListTaskRuns(ctx, 500)
	if err != nil {
		log.Warn().Err(err).Msg("load scheduler task runs failed")
		return result
	}
	for _, run := range runs {
		if _, exists := result[run.TaskName]; exists {
			continue
		}
		result[run.TaskName] = run
	}
	return result
}

func withJobMeta(def jobDefinition, javaQuartzName, cron string) jobDefinition {
	def.JavaQuartzName = javaQuartzName
	def.Cron = cron
	if override := schedulerEnv(def.Name, "CRON", ""); override != "" {
		if _, err := parseCronSchedules(override); err != nil {
			log.Warn().
				Err(err).
				Str("service", "scheduler-service").
				Str("task", def.Name).
				Str("cron", override).
				Str("fallback_cron", cron).
				Msg("invalid scheduler cron override ignored")
		} else {
			def.Cron = override
		}
	}
	def.Enabled = schedulerEnvBool(def.Name, "ENABLED", def.Enabled)
	return def
}

func schedulerEnv(jobName, suffix, fallback string) string {
	key := "YUQING_SCHEDULER_" + strings.ToUpper(strings.ReplaceAll(jobName, "-", "_")) + "_" + suffix
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func schedulerEnvBool(jobName, suffix string, fallback bool) bool {
	raw := schedulerEnv(jobName, suffix, "")
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func quartzCronSpec(spec string) string {
	fields := strings.Fields(strings.TrimSpace(spec))
	switch len(fields) {
	case 5:
		fields = append([]string{"0"}, fields...)
		fallthrough
	case 6:
		for i, field := range fields {
			if field == "?" {
				fields[i] = "*"
			}
		}
		return strings.Join(fields, " ")
	}
	return spec
}

func cronParser() cron.Parser {
	return cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
}

func nextCronRun(spec string, from time.Time) (time.Time, error) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.Local
	}
	schedules, err := parseCronSchedules(spec)
	if err != nil {
		return time.Time{}, err
	}
	localFrom := from.In(location)
	var next time.Time
	for _, schedule := range schedules {
		candidate := schedule.Next(localFrom)
		if next.IsZero() || candidate.Before(next) {
			next = candidate
		}
	}
	return next.UTC(), nil
}

func parseCronSchedule(spec string) (cron.Schedule, error) {
	return cronParser().Parse(quartzCronSpec(spec))
}

func parseCronSchedules(spec string) ([]cron.Schedule, error) {
	parts := cronSpecParts(spec)
	if len(parts) == 0 {
		return nil, errors.New("empty cron spec")
	}
	schedules := make([]cron.Schedule, 0, len(parts))
	for _, part := range parts {
		schedule, err := parseCronSchedule(part)
		if err != nil {
			return nil, err
		}
		schedules = append(schedules, schedule)
	}
	return schedules, nil
}

func cronSpecParts(spec string) []string {
	rawParts := strings.Split(spec, ";")
	parts := make([]string, 0, len(rawParts))
	for _, raw := range rawParts {
		part := strings.TrimSpace(raw)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func (w *Worker) jobDefinitions() []jobDefinition {
	return []jobDefinition{
		withJobMeta(jobDefinition{
			Name:        "crawl-link-heartbeat",
			Group:       "crawl",
			Description: "所有抓取站点链接心跳检测，每 300 秒验证一次已配置站点可访问性",
			Interval:    5 * time.Minute,
			Enabled:     true,
			Run:         w.runCrawlLinkHeartbeat,
		}, "CrawlLinkHeartbeat", "0 0/5 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "flash-crawl",
			Group:       "crawl",
			Description: "金十快讯抓取，延续 Java 实时采集链路",
			Interval:    w.cfg.FlashInterval,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.runCrawl(ctx, provider.SourceTypeFlash)
			},
		}, "FlashCrawlerQuartz", "0/15 * * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "headline-crawl",
			Group:       "crawl",
			Description: "金十资讯抓取，延续 Java 新闻采集链路",
			Interval:    w.cfg.HeadlineInterval,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.runCrawl(ctx, provider.SourceTypeHeadline)
			},
		}, "HeadlineCrawlerQuartz", "0 0/1 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "eastmoney-kuaixun-crawl",
			Group:       "crawl",
			Description: "东方财富快讯抓取，每 5 分钟抓取近期财经快讯并补充详情页全文",
			Interval:    5 * time.Minute,
			Enabled:     strings.TrimSpace(w.cfg.EastMoneyKuaixunURL) != "",
			Run: func(ctx context.Context) error {
				return w.runAStockIncrementalNewsCrawl(ctx, provider.SourceTypeEastMoneyKuaixun)
			},
		}, "EastMoneyKuaixunCrawler", "0 0/5 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "eastmoney-full-crawl",
			Group:       "crawl",
			Description: "东方财富全站财经新闻增量抓取，每 5 分钟抓取全站搜索新闻并补充详情页全文",
			Interval:    5 * time.Minute,
			Enabled:     strings.TrimSpace(w.cfg.EastMoneyKuaixunURL) != "",
			Run: func(ctx context.Context) error {
				return w.runAStockIncrementalNewsCrawl(ctx, provider.SourceTypeEastMoneyFull)
			},
		}, "EastMoneyFullCrawler", "0 1/5 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "jin10-full-crawl",
			Group:       "crawl",
			Description: "金十全站财经新闻增量抓取，每 5 分钟抓取近期快讯和资讯窗口",
			Interval:    5 * time.Minute,
			Enabled:     w.cfg.Jin10FullEnabled,
			Run: func(ctx context.Context) error {
				return w.runAStockIncrementalNewsCrawl(ctx, provider.SourceTypeJin10Full)
			},
		}, "Jin10FullCrawler", "0 0/5 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "wallstreetcn-a-stock-crawl",
			Group:       "crawl",
			Description: "华尔街见闻 A股快讯增量抓取，每 5 分钟抓取近期财经新闻",
			Interval:    5 * time.Minute,
			Enabled:     strings.TrimSpace(w.cfg.WallStreetCNAStockURL) != "",
			Run: func(ctx context.Context) error {
				return w.runAStockIncrementalNewsCrawl(ctx, provider.SourceTypeWallStreetCNAStock)
			},
		}, "WallStreetCNAStockCrawler", "0 0/5 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "cls-telegraph-crawl",
			Group:       "crawl",
			Description: "财联社电报增量抓取，每 5 分钟抓取近期财经新闻",
			Interval:    5 * time.Minute,
			Enabled:     strings.TrimSpace(w.cfg.CLSTelegraphURL) != "",
			Run: func(ctx context.Context) error {
				return w.runAStockIncrementalNewsCrawl(ctx, provider.SourceTypeCLSTelegraph)
			},
		}, "CLSTelegraphCrawler", "0 0/5 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "sina-finance-7x24-crawl",
			Group:       "crawl",
			Description: "新浪财经 7x24 增量抓取，每 5 分钟抓取近期财经新闻",
			Interval:    5 * time.Minute,
			Enabled:     strings.TrimSpace(w.cfg.SinaFinance7x24URL) != "",
			Run: func(ctx context.Context) error {
				return w.runAStockIncrementalNewsCrawl(ctx, provider.SourceTypeSinaFinance7x24)
			},
		}, "SinaFinance7x24Crawler", "0 0/5 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "crypto-x-crawl",
			Group:       "crawl",
			Description: "Crypto X 社媒抓取，补充币对社媒证据",
			Interval:    w.cfg.CryptoXInterval,
			Enabled:     strings.TrimSpace(w.cfg.CryptoXURL) != "",
			Run: func(ctx context.Context) error {
				return w.runCrawl(ctx, "crypto_x")
			},
		}, "CryptoXSocialCrawler", "0 0/2 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "crypto-telegram-crawl",
			Group:       "crawl",
			Description: "Crypto Telegram 社媒抓取，补充币对社媒证据",
			Interval:    w.cfg.CryptoTelegramInterval,
			Enabled:     strings.TrimSpace(w.cfg.CryptoTelegramURL) != "",
			Run: func(ctx context.Context) error {
				return w.runCrawl(ctx, "crypto_telegram")
			},
		}, "CryptoTelegramSocialCrawler", "0 1/2 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "foresight-newsflash-crawl",
			Group:       "crawl",
			Description: "Foresight News 快讯抓取，补充币对新闻证据",
			Interval:    w.cfg.ForesightNewsflashInterval,
			Enabled:     strings.TrimSpace(w.cfg.ForesightNewsflashURL) != "",
			Run: func(ctx context.Context) error {
				return w.runCrawl(ctx, "foresight_newsflash")
			},
		}, "ForesightNewsflashCrawler", "0 0/2 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "coindesk-zh-latest-crawl",
			Group:       "crawl",
			Description: "CoinDesk 中文最新新闻抓取，补充币对新闻证据",
			Interval:    w.cfg.CoinDeskZHLatestInterval,
			Enabled:     strings.TrimSpace(w.cfg.CoinDeskZHLatestURL) != "",
			Run: func(ctx context.Context) error {
				return w.runCrawl(ctx, "coindesk_zh_latest")
			},
		}, "CoinDeskZHLatestCrawler", "0 0/5 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "panews-newsflash-crawl",
			Group:       "crawl",
			Description: "PANews 快讯抓取，补充币对新闻证据",
			Interval:    w.cfg.PANewsNewsflashInterval,
			Enabled:     strings.TrimSpace(w.cfg.PANewsNewsflashURL) != "",
			Run: func(ctx context.Context) error {
				return w.runCrawl(ctx, "panews_newsflash")
			},
		}, "PANewsNewsflashCrawler", "0 1/2 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "theblock-latest-crawl",
			Group:       "crawl",
			Description: "The Block 最新加密新闻抓取，补充币对新闻证据",
			Interval:    w.cfg.TheBlockLatestInterval,
			Enabled:     strings.TrimSpace(w.cfg.TheBlockLatestURL) != "",
			Run: func(ctx context.Context) error {
				return w.runCrawl(ctx, "theblock_latest")
			},
		}, "TheBlockLatestCrawler", "0 2/5 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "a-stock-morning-news-crawl",
			Group:       "a-stock",
			Description: "A股上午新闻预抓：09:05 抓取 08:00-09:26:59 财经新闻，便于开盘前页面已有多源数据",
			Interval:    24 * time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.runAStockWindowNewsCrawl(ctx, "morning", "preopen")
			},
		}, "AStockMorningNewsCrawl", "0 5 9 * * ?"),
		withJobMeta(jobDefinition{
			Name:        "a-stock-morning-recommendation-preview",
			Group:       "a-stock",
			Description: "A股上午盘前推荐：09:27 生成上午盘前推荐快照并触发弹窗",
			Interval:    24 * time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.runAStockRecommendation(ctx, "morning", "preopen")
			},
		}, "AStockMorningRecommendationPreview", "0 27 9 * * ?"),
		withJobMeta(jobDefinition{
			Name:        "a-stock-morning-recommendation",
			Group:       "a-stock",
			Description: "A股上午推荐补全：09:32 抓取 08:00-09:26:59 财经新闻并补全推荐快照",
			Interval:    24 * time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.runAStockRecommendation(ctx, "morning", "final")
			},
		}, "AStockMorningRecommendation", "0 32 9 * * ?"),
		withJobMeta(jobDefinition{
			Name:        "a-stock-afternoon-recommendation-preview",
			Group:       "a-stock",
			Description: "A股下午盘前推荐：12:57 生成下午盘前推荐快照并触发弹窗",
			Interval:    24 * time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.runAStockRecommendation(ctx, "afternoon", "preopen")
			},
		}, "AStockAfternoonRecommendationPreview", "0 57 12 * * ?"),
		withJobMeta(jobDefinition{
			Name:        "a-stock-midday-news-crawl",
			Group:       "a-stock",
			Description: "A股午间新闻补抓：12:30 抓取 09:30-13:00 财经新闻，补齐午间来源统计",
			Interval:    24 * time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.runAStockWindowNewsCrawl(ctx, "afternoon", "final")
			},
		}, "AStockMiddayNewsCrawl", "0 30 12 * * ?"),
		withJobMeta(jobDefinition{
			Name:        "a-stock-afternoon-recommendation",
			Group:       "a-stock",
			Description: "A股下午推荐补全：13:02 抓取 09:30-13:00 财经新闻并补全推荐快照",
			Interval:    24 * time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.runAStockRecommendation(ctx, "afternoon", "final")
			},
		}, "AStockAfternoonRecommendation", "0 2 13 * * ?"),
		withJobMeta(jobDefinition{
			Name:        "a-stock-afternoon-open-refresh",
			Group:       "a-stock",
			Description: "A股下午开盘价补全：13:05 自动补当日下午推荐 13:01 开盘价并刷新回测快照",
			Interval:    24 * time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.runAStockAfternoonOpenRefresh(ctx)
			},
		}, "AStockAfternoonOpenRefresh", "0 5 13 * * ?"),
		withJobMeta(jobDefinition{
			Name:        "a-stock-daily-backtest-refresh",
			Group:       "a-stock",
			Description: "A股收盘回测补全：15:05 自动补当日 T+0，并刷新前 5 个交易日 T+1 到 T+5 收益",
			Interval:    24 * time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.runAStockDailyBacktestRefresh(ctx)
			},
		}, "AStockDailyBacktestRefresh", "0 5 15 * * ?"),
		withJobMeta(jobDefinition{
			Name:        "a-stock-exact-snapshot-backfill",
			Group:       "a-stock",
			Description: "A股精确快照补齐：按资金过滤开启/关闭状态自动生成缺失快照，页面日期切换保持只读",
			Interval:    24 * time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.runAStockExactSnapshotBackfill(ctx)
			},
		}, "AStockExactSnapshotBackfill", "0 29 2 * * ?; 0 29 9 * * ?; 0 59 12 * * ?; 0 35 15 * * ?"),
		withJobMeta(jobDefinition{
			Name:        "a-stock-auction-crawl",
			Group:       "a-stock",
			Description: "A股集合竞价金额：09:25:05、09:30:05 通过 AKShare 抓取全市场集合竞价成交金额",
			Interval:    24 * time.Hour,
			Enabled:     strings.TrimSpace(w.cfg.AStockAuctionURL) != "",
			Run: func(ctx context.Context) error {
				return w.runAStockAuctionCrawl(ctx)
			},
		}, "AStockAuctionCrawl", "5 25 9 * * ?; 5 30 9 * * ?"),
		withJobMeta(jobDefinition{
			Name:        "a-stock-sector-fund-flow-crawl",
			Group:       "a-stock",
			Description: "A股版块/个股资金：09:31、10:01、11:01、12:01、13:01、14:01、15:01 抓取资金流并聚合",
			Interval:    time.Hour,
			Enabled:     strings.TrimSpace(w.cfg.AStockAuctionURL) != "",
			Run: func(ctx context.Context) error {
				return w.runAStockSectorFundFlowCrawl(ctx)
			},
		}, "AStockSectorFundFlowCrawl", "0 31 9 * * ?; 0 1 10-15 * * ?"),
		withJobMeta(jobDefinition{
			Name:        "stock-research-crawl",
			Group:       "a-stock",
			Description: "上市公司研报调研：抓取 AKShare/TuShare、东方财富、新浪财经、搜狐财经研报调研数据",
			Interval:    24 * time.Hour,
			Enabled:     strings.TrimSpace(w.cfg.StockResearchURL) != "" || w.cfg.StockResearchPublicEnabled,
			Run: func(ctx context.Context) error {
				return w.runStockResearchCrawl(ctx)
			},
		}, "StockResearchCrawl", "0 30 16 * * ?"),
		withJobMeta(jobDefinition{
			Name:        "investor-relations-crawl",
			Group:       "a-stock",
			Description: "CNINFO 投资者关系活动记录抓取，下载 PDF 并解析文本评分",
			Interval:    24 * time.Hour,
			Enabled:     strings.TrimSpace(w.cfg.InvestorRelationsURL) != "",
			Run: func(ctx context.Context) error {
				return w.runInvestorRelationsCrawl(ctx)
			},
		}, "InvestorRelationsCrawl", "0 45 16 * * ?"),
		withJobMeta(jobDefinition{
			Name:        "a-stock-holdings-crawl",
			Group:       "a-stock",
			Description: "A股机构持仓：按最近 4 个报告期抓取基金、机构、社保、QFII 等季度持仓",
			Interval:    24 * time.Hour,
			Enabled:     strings.TrimSpace(w.cfg.AStockHoldingURL) != "",
			Run: func(ctx context.Context) error {
				return w.runAStockHoldingsCrawl(ctx)
			},
		}, "AStockHoldingsCrawl", "0 35 2 * * ?"),
		withJobMeta(jobDefinition{
			Name:        "analysis-refresh",
			Group:       "analysis",
			Description: "AnalysisQuartz 等价的系统分析快照刷新",
			Interval:    w.cfg.AnalysisInterval,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodPost, w.cfg.AnalysisURL+"/api/v1/admin/tasks/analysis/refresh", nil)
			},
		}, "AnalysisQuartz", "0 0/2 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "publicoption-analysis-refresh",
			Group:       "analysis",
			Description: "publicoptionQuartz 等价的舆情分析数据预热",
			Interval:    10 * time.Minute,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.AnalysisURL+"/api/v1/public-opinion/analysis?page_size=20", nil)
			},
		}, "publicoptionQuartz", "0 0/10 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "warning-scan-20m",
			Group:       "warning",
			Description: "WarningSchedule 等价的 20 分钟预警扫描",
			Interval:    20 * time.Minute,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.ContentURL+"/api/v1/articles?page_size=1", nil)
			},
		}, "WarningSchedule", "0 0/20 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "monitor-warning-hourly",
			Group:       "warning",
			Description: "MonitorWarningQuartz 小时级预警设置检查",
			Interval:    time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.ContentURL+"/api/v1/projects", nil)
			},
		}, "MonitorWarningQuartz", "0 0 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "monitor-warning-realtime-10m",
			Group:       "warning",
			Description: "MonitorWarningQuartz 实时推送配置检查",
			Interval:    10 * time.Minute,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.ContentURL+"/api/v1/search/hot-keywords?limit=10", nil)
			},
		}, "MonitorWarningQuartz", "0 0/10 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "report-data-refresh",
			Group:       "report",
			Description: "ReportDataSchedule 等价的报表数据刷新检查",
			Interval:    time.Minute,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.ContentURL+"/api/v1/reports", nil)
			},
		}, "ReportDataSchedule", "0 0/1 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "day-report",
			Group:       "report",
			Description: "ReportListSchedule 日报任务等价入口",
			Interval:    24 * time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.ContentURL+"/api/v1/reports", nil)
			},
		}, "ReportListSchedule.day", "0 0 8 * * ?"),
		withJobMeta(jobDefinition{
			Name:        "week-report",
			Group:       "report",
			Description: "ReportListSchedule 周报任务等价入口",
			Interval:    7 * 24 * time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.ContentURL+"/api/v1/reports", nil)
			},
		}, "ReportListSchedule.week", "0 0 8 ? * MON"),
		withJobMeta(jobDefinition{
			Name:        "month-report",
			Group:       "report",
			Description: "ReportListSchedule 月报任务等价入口",
			Interval:    30 * 24 * time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.ContentURL+"/api/v1/reports", nil)
			},
		}, "ReportListSchedule.month", "0 0 8 1 * ?"),
		withJobMeta(jobDefinition{
			Name:        "volume-refresh",
			Group:       "operation",
			Description: "VolumeSchedule 等价的声量统计刷新",
			Interval:    24 * time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.AnalysisURL+"/api/v1/analysis/sources", nil)
			},
		}, "VolumeSchedule", "0 5 0 * * ?"),
		withJobMeta(jobDefinition{
			Name:        "volume-pt-refresh",
			Group:       "operation",
			Description: "VolumePTSchedule 等价的 5 分钟声量预热",
			Interval:    5 * time.Minute,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.AnalysisURL+"/api/v1/analysis/trends", nil)
			},
		}, "VolumePTSchedule", "0 0/5 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "hot-data-refresh",
			Group:       "operation",
			Description: "HotDataSchedule 等价的热点数据刷新",
			Interval:    30 * time.Minute,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.ContentURL+"/api/v1/search/hot-keywords?limit=10", nil)
			},
		}, "HotDataSchedule", "0 0/30 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "wechat-challenge-cleanup",
			Group:       "wechat",
			Description: "WechatqrcodeSchedule 等价的二维码挑战清理",
			Interval:    w.cfg.WechatCleanupInterval,
			Enabled:     true,
			Run:         w.cleanupExpiredWechatChallenges,
		}, "WechatqrcodeSchedule", "0 0 * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "wechat-daily-push",
			Group:       "wechat",
			Description: "WechatSchedule 等价的每日热点微信推送",
			Interval:    w.cfg.WechatPushInterval,
			Enabled:     w.cfg.WechatPushEnabled,
			Run:         w.pushWechatDailySummary,
		}, "WechatSchedule", "0 30 8 * * ?"),
	}
}

func (w *Worker) RunJobByName(ctx context.Context, name string) error {
	for _, job := range w.jobDefinitions() {
		if job.Name == name {
			if !job.Enabled {
				return fmt.Errorf("scheduler job %s is disabled", name)
			}
			return w.runJob(ctx, job)
		}
	}
	return fmt.Errorf("scheduler job %s not found", name)
}

func (w *Worker) runJob(ctx context.Context, job jobDefinition) error {
	startedAt := time.Now().UTC()
	err := job.Run(ctx)
	finishedAt := time.Now().UTC()
	status := "success"
	message := "scheduler job completed"
	if err != nil {
		var skipped jobSkippedError
		if errors.As(err, &skipped) {
			status = "skipped"
			message = skipped.Error()
			err = nil
		} else {
			status = "failed"
			message = err.Error()
		}
	}
	if recordErr := w.recordTaskRun(ctx, job.Name, status, message, startedAt, &finishedAt); recordErr != nil {
		log.Warn().Err(recordErr).Str("task", job.Name).Msg("record scheduler task run failed")
	}
	return err
}

func (w *Worker) recordTaskRun(ctx context.Context, name, status, message string, startedAt time.Time, finishedAt *time.Time) error {
	store, err := w.ensureStore()
	if err != nil {
		if errors.Is(err, errStoreDisabled) {
			return nil
		}
		return err
	}
	return store.RecordTaskRun(ctx, name, status, message, startedAt, finishedAt)
}

func (w *Worker) runCrawl(ctx context.Context, sourceType string) error {
	return w.runCrawlWithOptions(ctx, sourceType, model.CrawlOptions{})
}

func (w *Worker) runCrawlWithOptions(ctx context.Context, sourceType string, options model.CrawlOptions) error {
	req := w.crawlClient.R().
		SetContext(ctx).
		SetQueryParam("source_type", sourceType)
	if start := strings.TrimSpace(options.Start); start != "" {
		req.SetQueryParam("start", start)
	}
	if end := strings.TrimSpace(options.End); end != "" {
		req.SetQueryParam("end", end)
	}
	if timeField := strings.TrimSpace(options.TimeField); timeField != "" {
		req.SetQueryParam("time_field", timeField)
	}
	resp, err := req.Post(w.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		detail := strings.TrimSpace(resp.String())
		var envelope struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(resp.Body(), &envelope); err == nil && strings.TrimSpace(envelope.Message) != "" {
			detail = strings.TrimSpace(envelope.Message)
		}
		if detail == "" {
			detail = resp.Status()
		}
		return fmt.Errorf("crawl %s failed: %s", sourceType, detail)
	}
	return nil
}

func (w *Worker) runCrawlLinkHeartbeat(ctx context.Context) error {
	return w.runCrawlLinkHeartbeatForSites(ctx, w.crawlLinkHeartbeatSites())
}

func (w *Worker) crawlLinkHeartbeatSites() []crawlLinkHeartbeatSite {
	sites := []crawlLinkHeartbeatSite{
		{SourceType: provider.SourceTypeFlash, Name: "金十快讯", URL: "https://www.jin10.com/"},
		{SourceType: provider.SourceTypeHeadline, Name: "金十资讯", URL: "https://xnews.jin10.com/"},
	}
	if w.cfg.Jin10FullEnabled {
		sites = append(sites, crawlLinkHeartbeatSite{SourceType: "jin10_full", Name: "金十全站", URL: "https://www.jin10.com/"})
	}
	sites = append(sites,
		crawlLinkHeartbeatSite{SourceType: provider.SourceTypeEastMoneyKuaixun, Name: "东方财富快讯", URL: w.cfg.EastMoneyKuaixunURL},
		crawlLinkHeartbeatSite{SourceType: provider.SourceTypeEastMoneyFull, Name: "东方财富全站", URL: w.cfg.EastMoneyKuaixunURL},
		crawlLinkHeartbeatSite{SourceType: "wallstreetcn_a_stock", Name: "华尔街见闻 A股快讯", URL: w.cfg.WallStreetCNAStockURL},
		crawlLinkHeartbeatSite{SourceType: "cls_telegraph", Name: "财联社电报", URL: w.cfg.CLSTelegraphURL},
		crawlLinkHeartbeatSite{SourceType: "sina_finance_7x24", Name: "新浪财经 7x24", URL: w.cfg.SinaFinance7x24URL},
		crawlLinkHeartbeatSite{SourceType: "foresight_newsflash", Name: "Foresight News 快讯", URL: w.cfg.ForesightNewsflashURL},
		crawlLinkHeartbeatSite{SourceType: "coindesk_zh_latest", Name: "CoinDesk 中文最新", URL: w.cfg.CoinDeskZHLatestURL},
		crawlLinkHeartbeatSite{SourceType: "panews_newsflash", Name: "PANews 快讯", URL: w.cfg.PANewsNewsflashURL},
		crawlLinkHeartbeatSite{SourceType: "theblock_latest", Name: "The Block 最新新闻", URL: theBlockHeartbeatURL(w.cfg.TheBlockLatestURL)},
		crawlLinkHeartbeatSite{SourceType: "crypto_x", Name: "Crypto X", URL: w.cfg.CryptoXURL},
		crawlLinkHeartbeatSite{SourceType: "crypto_telegram", Name: "Crypto Telegram", URL: w.cfg.CryptoTelegramURL},
		crawlLinkHeartbeatSite{SourceType: "a_stock_auction", Name: "A股集合竞价行情", URL: w.cfg.AStockAuctionURL},
		crawlLinkHeartbeatSite{SourceType: "a_stock_holdings", Name: "A股机构持仓", URL: w.cfg.AStockHoldingURL},
	)
	return slices.DeleteFunc(sites, func(site crawlLinkHeartbeatSite) bool {
		return strings.TrimSpace(site.URL) == ""
	})
}

func theBlockHeartbeatURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.Contains(raw, "theblock.co/latest-crypto-news") {
		return "https://www.theblock.co/rss.xml"
	}
	return raw
}

func (w *Worker) runCrawlLinkHeartbeatForSites(ctx context.Context, sites []crawlLinkHeartbeatSite) error {
	startedAt := time.Now().UTC()
	okCount := 0
	failed := make([]string, 0)
	for _, site := range sites {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		site.URL = strings.TrimSpace(site.URL)
		begin := time.Now()
		resp, err := w.client.R().
			SetContext(ctx).
			SetHeader("Accept", "*/*").
			SetHeader("User-Agent", "yuqing-crawl-heartbeat/1.0").
			Get(site.URL)
		elapsed := time.Since(begin)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", site.SourceType, err))
			log.Warn().
				Err(err).
				Str("service", "scheduler-service").
				Str("task", "crawl-link-heartbeat").
				Str("source_type", site.SourceType).
				Str("site", site.Name).
				Str("url", site.URL).
				Dur("duration", elapsed).
				Msg("crawl link heartbeat failed")
			continue
		}
		if resp.StatusCode() < http.StatusOK || resp.StatusCode() >= http.StatusBadRequest {
			failed = append(failed, fmt.Sprintf("%s: %s", site.SourceType, resp.Status()))
			log.Warn().
				Str("service", "scheduler-service").
				Str("task", "crawl-link-heartbeat").
				Str("source_type", site.SourceType).
				Str("site", site.Name).
				Str("url", site.URL).
				Int("status", resp.StatusCode()).
				Dur("duration", elapsed).
				Msg("crawl link heartbeat failed")
			continue
		}
		okCount++
		log.Info().
			Str("service", "scheduler-service").
			Str("task", "crawl-link-heartbeat").
			Str("source_type", site.SourceType).
			Str("site", site.Name).
			Str("url", site.URL).
			Int("status", resp.StatusCode()).
			Dur("duration", elapsed).
			Msg("crawl link heartbeat ok")
	}

	finishedAt := time.Now().UTC()
	status := "success"
	message := fmt.Sprintf("crawl link heartbeat ok=%d failed=%d", okCount, len(failed))
	if len(failed) > 0 {
		status = "failed"
		message = message + "; " + strings.Join(failed, "; ")
	}
	if recordErr := w.recordTaskRun(ctx, "crawl-link-heartbeat", status, message, startedAt, &finishedAt); recordErr != nil {
		log.Warn().Err(recordErr).Str("task", "crawl-link-heartbeat").Msg("record scheduler task run failed")
	}
	return nil
}
