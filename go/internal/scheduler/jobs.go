package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"

	"github.com/pcdogyu/yuqing/go/internal/model"
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
		if _, err := parseCronSchedule(override); err != nil {
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
	schedule, err := parseCronSchedule(spec)
	if err != nil {
		return time.Time{}, err
	}
	return schedule.Next(from.In(location)).UTC(), nil
}

func parseCronSchedule(spec string) (cron.Schedule, error) {
	return cronParser().Parse(quartzCronSpec(spec))
}

func (w *Worker) jobDefinitions() []jobDefinition {
	return []jobDefinition{
		withJobMeta(jobDefinition{
			Name:        "flash-crawl",
			Group:       "crawl",
			Description: "金十快讯抓取，延续 Java 实时采集链路",
			Interval:    w.cfg.FlashInterval,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.runCrawl(ctx, "flash")
			},
		}, "FlashCrawlerQuartz", "0/15 * * * * ?"),
		withJobMeta(jobDefinition{
			Name:        "headline-crawl",
			Group:       "crawl",
			Description: "金十头条抓取，延续 Java 新闻采集链路",
			Interval:    w.cfg.HeadlineInterval,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.runCrawl(ctx, "headline")
			},
		}, "HeadlineCrawlerQuartz", "0 0/1 * * * ?"),
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
		status = "failed"
		message = err.Error()
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
	resp, err := w.crawlClient.R().
		SetContext(ctx).
		SetQueryParam("source_type", sourceType).
		Post(w.cfg.CrawlerURL + "/api/v1/admin/tasks/crawl")
	if err != nil {
		return err
	}
	if !resp.IsSuccess() {
		return fmt.Errorf("crawl %s failed: %s", sourceType, resp.Status())
	}
	return nil
}
