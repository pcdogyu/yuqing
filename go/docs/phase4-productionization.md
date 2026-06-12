# Phase 4 Productionization

四期目标是把三期 legacy 收口后的 Go 系统推进到可生产运行状态。本页记录当前已落地的调度、审计、健康检查和 Windows 运维能力。

## Scheduler

`scheduler-service` 同时运行后台任务和管理 API：

- `GET /healthz`
- `GET /api/v1/scheduler/jobs`
- `POST /api/v1/scheduler/jobs/{name}/run`

手动触发任务必须携带 `X-Service-Token`。任务执行结果统一写入 `task_runs`。

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
.\scripts\backup-sqlite.ps1
.\scripts\stop-all.ps1
```

脚本默认使用 `YUQING_DB_PATH`；未设置时使用 `data\yuqing.db`。

## Verification

四期代码变更后固定验证：

```powershell
cd D:\yuqing\go
gofmt -w <changed-go-files>
go test ./...
.\scripts\smoke-test.ps1
```

`smoke-test.ps1` 需要服务已启动，覆盖服务健康、scheduler jobs、正式搜索/分析/NLP API 和 legacy 410 探测。
