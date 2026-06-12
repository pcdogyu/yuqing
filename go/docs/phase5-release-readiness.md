# Phase 5 Release Readiness

五期目标是把四期生产化骨架推进到可上线验收闭环。范围包括 cron 等价调度、跨系统基线对账、备份恢复演练、外部集成错误分类、运维 JSON API 和发布验收脚本。

## Scheduler Cron

`scheduler-service` 使用 `robfig/cron/v3` 按 Java Quartz cron 实际调度，默认时区为 `Asia/Shanghai`。

- `GET /api/v1/scheduler/jobs` 继续返回 `java_quartz_name`、`cron`、`next_run_at`、最近状态和最近错误。
- `next_run_at` 由 cron 表达式计算，不再用 interval 近似。
- 每个 job 支持环境变量覆盖：
  - `YUQING_SCHEDULER_<JOB>_CRON`
  - `YUQING_SCHEDULER_<JOB>_ENABLED`
- 手动触发仍使用 `POST /api/v1/scheduler/jobs/{name}/run` 和 `X-Service-Token`。

## Reconciliation And Restore

从 `D:\yuqing\go` 执行：

```powershell
.\scripts\reconcile-production.ps1 -BaselinePath .\baseline\java-counts.json
.\scripts\backup-sqlite.ps1
.\scripts\restore-sqlite.ps1
```

`cmd/ops-check --baseline` 支持 JSON/CSV 基线，输出：

- `success`
- `failed`
- `diff`
- `missing`
- `extra`
- `warnings`

推荐 JSON 基线格式：

```json
{
  "counts": {
    "items": 100,
    "projects": 10,
    "reports": 5,
    "items_fts": 100,
    "task_runs": 20,
    "audit_logs": 50,
    "platform_bindings": 2
  }
}
```

也支持 Java 导出侧的领域命名，工具会映射到 Go SQLite 指标：

- `articles` -> `items`
- `search_index` -> `items_fts`
- `task_records` -> `task_runs`
- `audit_logs` -> `audit_logs`
- `platform_bindings` -> `platform_bindings`
- `crypto_social` -> `crypto_social_sources`
- `nlp_status` -> `nlp_capabilities`

CSV 可使用 `metric,label,expected` 或 `name,count`。嵌套 JSON 可使用：

```json
{
  "articles": { "count": 100 },
  "search_index": { "expected": 100 }
}
```

`restore-sqlite.ps1` 会把备份复制到临时库执行只读校验，并在源库存在时比较源库与恢复库的表计数，输出 `table_counts_match` 和 `table_count_diff`。

五期第二批检查结论：

- 跨系统对账已从“单库健康检查”深化为 Java 导出 baseline 与 Go SQLite 指标对比。
- baseline 允许 Java 侧领域命名，不要求导出字段名完全等同 Go 表名。
- 恢复演练已从“备份可打开”深化为“临时恢复库可校验，并可与源库表计数比对”。
- 发布验收链路中 `release-check.ps1` 会把 `DatabasePath` 传入恢复演练，保证恢复检查使用同一份上线目标库。

二批最小验收命令：

```powershell
cd D:\yuqing\go
go test ./cmd/ops-check
.\scripts\backup-sqlite.ps1
.\scripts\restore-sqlite.ps1 -SourceDatabasePath .\data\yuqing.db
```

## Operations APIs

`content-service` 新增只读接口：

- `GET /api/v1/system/operations`
- `GET /api/v1/system/alerts`

`operations` 返回服务健康、scheduler 摘要、任务摘要、最近任务、失败任务、审计摘要、最近审计、legacy 注册表、legacy 410 探测、外部集成状态、备份状态和 `ready`。`alerts` 返回上线告警，覆盖服务不可用、失败任务、连续任务失败、审计缺失或过期、备份缺失或过期、legacy 非 410、外部集成失败和 crypto social 长时间无入库。

`portal-web /system?section=operations` 优先消费同一份 operations 数据。

四批检查结论：

- `operations.scheduler_jobs` 直接来自 scheduler API，`/system?section=operations` 优先消费同一份数据，页面与 JSON API 口径一致。
- `operations.task_summary` 输出最近任务数、失败数和连续失败数，`alerts` 在连续失败达到 2 次时输出 `consecutive_task_failures`。
- `operations.audit_summary` 输出最近审计数量、最后动作和距今秒数，超过 24 小时无新审计会输出 `recent_audit_stale`。
- `operations.legacy_route_probes` 对关键旧入口执行 `410 Gone` 探测，非 410 会输出 `legacy_non_410`。
- `operations.external_integrations` 对 crypto social 输出最近抓取时间、抓取数、入库数、更新数和重复数；最近一次成功抓取超过 6 小时仍无入库会输出 `crypto_social_no_recent_insert`。
- `operations.backup` 输出备份文件、大小、最近备份时间和年龄小时数。

四批最小验收命令：

```powershell
cd D:\yuqing\go
go test ./internal/content ./internal/portal
```

## External Integration Resilience

外部调用使用固定错误分类：

- `external_timeout`
- `external_non_200`
- `external_invalid_json`
- `external_empty_data`
- `external_duplicate_data`
- `external_disabled`

五期第三批检查结论：

- crawler 和 scheduler 的外部 HTTP 客户端统一使用 `YUQING_HTTP_TIMEOUT_SEC`、`YUQING_EXTERNAL_RETRY_COUNT`、`YUQING_EXTERNAL_RETRY_WAIT_MS`。
- 仅超时、HTTP `429` 和 `5xx` 会重试；`4xx` 客户端错误不重试。
- crypto social provider 支持 `YUQING_CRYPTO_SOCIAL_RATE_LIMIT_MS`，用于限制同一 provider 连续请求间隔。
- crypto social 对禁用端点、非 200、坏 JSON、空数据返回固定分类错误；重复数据会去重并记录 `external_duplicate_data` 结构化日志。
- 启动日志输出 `external_retry_count`、`external_retry_wait`、`crypto_social_rate_limit`，便于生产核对当前韧性配置。
- NLP capabilities 已明确 `/platform/*` 旧入口为 `410 Gone`，不再描述过渡代理。

相关环境变量：

```powershell
$env:YUQING_EXTERNAL_RETRY_COUNT = "2"
$env:YUQING_EXTERNAL_RETRY_WAIT_MS = "500"
$env:YUQING_CRYPTO_SOCIAL_RATE_LIMIT_MS = "0"
```

三批最小验收命令：

```powershell
cd D:\yuqing\go
go test ./internal/external ./internal/config ./internal/app ./internal/provider/cryptosocial ./internal/scheduler
```

验收口径：

- `external.ShouldRetryResponse` 覆盖超时、`429`、`5xx`，并排除普通 `4xx`。
- `cryptosocial.Provider` 对禁用端点、非 200、坏 JSON、空数据返回可分类错误。
- `cryptosocial.Provider` 在配置 `YUQING_CRYPTO_SOCIAL_RATE_LIMIT_MS` 后会限制连续请求间隔。
- `scheduler.Worker` 与 `app.NewCrawler` 统一使用配置化 retry count / wait。

## Release Check

发布前执行：

```powershell
.\scripts\release-check.ps1
```

脚本串联：

- `go test ./...`
- `start-all.ps1`
- `health-check.ps1`
- `smoke-test.ps1`
- `reconcile-production.ps1`
- `backup-sqlite.ps1`
- `restore-sqlite.ps1`
- `stop-all.ps1`

输出 JSON 中 `ready=true` 才视为五期发布验收通过。
