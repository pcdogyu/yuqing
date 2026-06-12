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

## Operations APIs

`content-service` 新增只读接口：

- `GET /api/v1/system/operations`
- `GET /api/v1/system/alerts`

`operations` 返回服务健康、最近任务、失败任务、最近审计、legacy 注册表、外部集成状态、备份状态和 `ready`。`alerts` 返回上线告警，覆盖服务不可用、失败任务、审计缺失、备份缺失和 legacy 存活入口。

`portal-web /system?section=operations` 优先消费同一份 operations 数据。

## External Integration Resilience

外部调用使用固定错误分类：

- `external_timeout`
- `external_non_200`
- `external_invalid_json`
- `external_empty_data`
- `external_duplicate_data`
- `external_disabled`

crypto social provider 对非 200、坏 JSON、空数据返回分类错误；重复数据会去重并记录结构化日志。NLP capabilities 已明确 `/platform/*` 旧入口为 `410 Gone`，不再描述过渡代理。

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
