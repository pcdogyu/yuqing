# Phase 4 Productionization

四期目标是把三期 legacy 收口后的 Go 系统推进到可生产运行状态。本页记录四期已落地的调度、审计、健康检查和 Windows 运维能力；五期上线验收闭环见 [phase5-release-readiness.md](phase5-release-readiness.md)。

## Scheduler

`scheduler-service` 同时运行后台任务和管理 API：

- `GET /healthz`
- `GET /api/v1/scheduler/jobs`
- `POST /api/v1/scheduler/jobs/{name}/run`

手动触发任务必须携带 `X-Service-Token`。任务执行结果统一写入 `task_runs`。

`GET /api/v1/scheduler/jobs` 返回每个任务的生产运行态：

- `java_quartz_name`
- `cron`
- `next_run_at`
- `last_status`
- `last_message`
- `last_started_at`
- `last_finished_at`

当前 job registry 覆盖：

- `flash-crawl`
- `headline-crawl`
- `analysis-refresh`
- `publicoption-analysis-refresh`
- `warning-scan-20m`
- `monitor-warning-hourly`
- `monitor-warning-realtime-10m`
- `report-data-refresh`
- `day-report`
- `week-report`
- `month-report`
- `volume-refresh`
- `volume-pt-refresh`
- `hot-data-refresh`
- `wechat-challenge-cleanup`
- `wechat-daily-push`

## Audit Logs

`content-service` 已启用 HTTP 审计 middleware。除 `/healthz`、`/api/v1/system/audit-logs`、`/api/v1/system/task-runs` 外，请求会写入 `audit_logs`：

- `action`: `http.<method>`
- `resource`: 请求 path
- `detail_json`: method、path、query、status、duration_ms、ip、user_agent、module、operation

Query 中的 `token`、`password`、`secret`、`key` 会脱敏。

## Windows Scripts

从 `D:\yuqing\go` 运行：

```powershell
.\scripts\start-all.ps1
.\scripts\health-check.ps1
.\scripts\smoke-test.ps1
.\scripts\reconcile-production.ps1
.\scripts\backup-sqlite.ps1
.\scripts\restore-sqlite.ps1
.\scripts\release-check.ps1
.\scripts\stop-all.ps1
```

脚本默认使用 `YUQING_DB_PATH`；未设置时使用 `data\yuqing.db`。

`reconcile-production.ps1` 调用 `cmd/ops-check` 做 SQLite 只读对账，覆盖 `items`、`projects`、`reports`、`items_fts`、`task_runs`、`audit_logs`、`platform_bindings`、crypto social 来源、失败任务和最近审计。五期后可通过 `-BaselinePath` 对 Java 导出的 JSON/CSV 基线做差异比较；五期第二批已支持 Java 领域名到 Go 指标的映射。`backup-sqlite.ps1` 会先执行 WAL checkpoint，再复制数据库并对备份库执行同一套校验，`restore-sqlite.ps1` 会对备份执行临时恢复演练，并可通过 `-SourceDatabasePath` 比较源库与恢复库表计数。

五期第五批后，`gateway-web` 提供正式 `GET /healthz` JSON 健康检查；`release-check.ps1` 会在本机默认 `8079` 或 `8086` 端口被占用时自动切换 gateway / scheduler 到空闲验收端口，并把端口配置同步给启动的服务。

## Operations View

`portal-web /system?section=operations` 展示生产运行闭环：

- 服务健康，包括 `scheduler-service`
- scheduler jobs 的 Java Quartz 对应关系、cron 描述、下次执行时间和最近结果
- 最近失败任务、最近审计、抓取健康
- legacy 注册表策略汇总；四期收尾后 `proxy / preserve` 均为 `0`

五期后该页面优先消费 `GET /api/v1/system/operations`，脚本、页面和 JSON API 使用同一份运行数据。`GET /api/v1/system/alerts` 输出上线告警与 `ready` 标志。五期第四批后，operations 数据进一步包含 scheduler 摘要、任务连续失败数、审计新鲜度、legacy 410 探测、crypto social 最近抓取/入库统计和备份文件年龄。

五期第三批后，外部 HTTP 访问的超时、重试和 crypto social 限流配置已统一收口；具体参数和验收口径见 [phase5-release-readiness.md](phase5-release-readiness.md)。

## Verification

四期代码变更后固定验证：

```powershell
cd D:\yuqing\go
gofmt -w <changed-go-files>
go test ./...
.\scripts\smoke-test.ps1
```

`smoke-test.ps1` 需要服务已启动，覆盖服务健康、scheduler jobs 运行态、正式搜索/分析/NLP API、NLP capabilities、审计写入和 legacy 410 探测。五期第五批后，smoke 会使用同一份 `DatabasePath` 做备份/恢复演练，对 scheduler jobs 启动空窗做短重试，并可通过 `YUQING_CRYPTO_MOCK_URL` 检查 crypto social 非 200、坏 JSON、空数据和重复数据 mock 样本。旧 `fullsearch/getSearchResult`、`timelysearch`、`platform/nlp`、`platform/xie`、`mobile`、`displayboard`、`volume`、`hot`、`dist`、`img/code` 入口已统一返回 `410 Gone`。
