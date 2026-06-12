package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

type Job struct {
	Name        string `json:"name"`
	Group       string `json:"group"`
	Description string `json:"description"`
	IntervalSec int64  `json:"interval_sec"`
	Enabled     bool   `json:"enabled"`
}

type jobDefinition struct {
	Name        string
	Group       string
	Description string
	Interval    time.Duration
	Enabled     bool
	Run         func(context.Context) error
}

func (w *Worker) Jobs() []Job {
	defs := w.jobDefinitions()
	jobs := make([]Job, 0, len(defs))
	for _, def := range defs {
		jobs = append(jobs, Job{
			Name:        def.Name,
			Group:       def.Group,
			Description: def.Description,
			IntervalSec: int64(def.Interval / time.Second),
			Enabled:     def.Enabled,
		})
	}
	return jobs
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

func (w *Worker) jobDefinitions() []jobDefinition {
	return []jobDefinition{
		{
			Name:        "flash-crawl",
			Group:       "crawl",
			Description: "金十快讯抓取，延续 Java 实时采集链路",
			Interval:    w.cfg.FlashInterval,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.runCrawl("flash")
			},
		},
		{
			Name:        "headline-crawl",
			Group:       "crawl",
			Description: "金十头条抓取，延续 Java 新闻采集链路",
			Interval:    w.cfg.HeadlineInterval,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.runCrawl("headline")
			},
		},
		{
			Name:        "analysis-refresh",
			Group:       "analysis",
			Description: "AnalysisQuartz 等价的系统分析快照刷新",
			Interval:    w.cfg.AnalysisInterval,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodPost, w.cfg.AnalysisURL+"/api/v1/admin/tasks/analysis/refresh", nil)
			},
		},
		{
			Name:        "publicoption-analysis-refresh",
			Group:       "analysis",
			Description: "publicoptionQuartz 等价的舆情分析数据预热",
			Interval:    10 * time.Minute,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.AnalysisURL+"/api/v1/public-opinion/analysis?page_size=20", nil)
			},
		},
		{
			Name:        "warning-scan-20m",
			Group:       "warning",
			Description: "WarningSchedule 等价的 20 分钟预警扫描",
			Interval:    20 * time.Minute,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.ContentURL+"/api/v1/articles?page_size=1", nil)
			},
		},
		{
			Name:        "monitor-warning-hourly",
			Group:       "warning",
			Description: "MonitorWarningQuartz 小时级预警设置检查",
			Interval:    time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.ContentURL+"/api/v1/projects", nil)
			},
		},
		{
			Name:        "monitor-warning-realtime-10m",
			Group:       "warning",
			Description: "MonitorWarningQuartz 实时推送配置检查",
			Interval:    10 * time.Minute,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.ContentURL+"/api/v1/search/hot-keywords?limit=10", nil)
			},
		},
		{
			Name:        "report-data-refresh",
			Group:       "report",
			Description: "ReportDataSchedule 等价的报表数据刷新检查",
			Interval:    time.Minute,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.ContentURL+"/api/v1/reports", nil)
			},
		},
		{
			Name:        "day-report",
			Group:       "report",
			Description: "ReportListSchedule 日报任务等价入口",
			Interval:    24 * time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.ContentURL+"/api/v1/reports", nil)
			},
		},
		{
			Name:        "week-report",
			Group:       "report",
			Description: "ReportListSchedule 周报任务等价入口",
			Interval:    7 * 24 * time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.ContentURL+"/api/v1/reports", nil)
			},
		},
		{
			Name:        "month-report",
			Group:       "report",
			Description: "ReportListSchedule 月报任务等价入口",
			Interval:    30 * 24 * time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.ContentURL+"/api/v1/reports", nil)
			},
		},
		{
			Name:        "volume-refresh",
			Group:       "operation",
			Description: "VolumeSchedule 等价的声量统计刷新",
			Interval:    24 * time.Hour,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.AnalysisURL+"/api/v1/analysis/sources", nil)
			},
		},
		{
			Name:        "volume-pt-refresh",
			Group:       "operation",
			Description: "VolumePTSchedule 等价的 5 分钟声量预热",
			Interval:    5 * time.Minute,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.AnalysisURL+"/api/v1/analysis/trends", nil)
			},
		},
		{
			Name:        "hot-data-refresh",
			Group:       "operation",
			Description: "HotDataSchedule 等价的热点数据刷新",
			Interval:    30 * time.Minute,
			Enabled:     true,
			Run: func(ctx context.Context) error {
				return w.request(http.MethodGet, w.cfg.ContentURL+"/api/v1/search/hot-keywords?limit=10", nil)
			},
		},
		{
			Name:        "wechat-challenge-cleanup",
			Group:       "wechat",
			Description: "WechatqrcodeSchedule 等价的二维码挑战清理",
			Interval:    w.cfg.WechatCleanupInterval,
			Enabled:     true,
			Run:         w.cleanupExpiredWechatChallenges,
		},
		{
			Name:        "wechat-daily-push",
			Group:       "wechat",
			Description: "WechatSchedule 等价的每日热点微信推送",
			Interval:    w.cfg.WechatPushInterval,
			Enabled:     w.cfg.WechatPushEnabled,
			Run:         w.pushWechatDailySummary,
		},
	}
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

func (w *Worker) runCrawl(sourceType string) error {
	resp, err := w.client.R().
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
