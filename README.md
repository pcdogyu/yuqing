# 思通舆情 Go 版

思通舆情当前分支已经完成 Java 到 Go 的运行链路收口。站点、API、调度任务、启动脚本和发布验收均由 `go/` 目录下的 Go 服务承接；根 Maven 工程、Java 主源码和遗留 Java 测试文件已退场。

## 当前状态

- 运行语言：Go
- 活跃分支：`golang`
- Java 运行入口：已删除
- Maven 工程：已删除
- Go 服务：`gateway-web`、`portal-web`、`auth-service`、`wechat-service`、`content-service`、`crawler-service`、`analysis-service`、`scheduler-service`、`nlp-service`
- 发布验收入口：`go/scripts/release-check.ps1`
- 迁移对账报告：[go/docs/java-go-cutover-report.md](go/docs/java-go-cutover-report.md)

## 核心功能

- 舆情门户：项目、文章、报告、搜索、系统运行页
- A 股策略：财经新闻抓取、热点归纳、股票推荐、消息回测
- 集合竞价：09:30 抓取并展示全市场开盘集合竞价金额
- 研报调研：上市公司研报和机构调研搜索入口
- Crypto：新闻、社媒、行情和策略观察
- 系统运维：健康检查、调度任务、审计日志、备份恢复、数据库切换

## 服务端口

| 服务 | 默认端口 | 说明 |
| --- | ---: | --- |
| `gateway-web` | `80` | 统一入口 |
| `auth-service` | `8081` | 登录、会话、Token |
| `content-service` | `8082` | 业务数据与内部 API |
| `crawler-service` | `8083` | 抓取任务 |
| `analysis-service` | `8084` | 聚合分析 |
| `nlp-service` | `8085` | 摘要、关键词、OCR 能力 |
| `scheduler-service` | `8086` | 调度任务 |
| `akshare-service` | `8087` | AKShare HTTP 适配 |
| `wechat-service` | `8088` | 微信登录与绑定 |

## 快速启动

Windows PowerShell：

```powershell
cd D:\yuqing\go
.\run.bat
```

启动后访问：

```text
http://127.0.0.1
```

停止服务：

```powershell
cd D:\yuqing\go
.\stop.bat
```

## 配置

常用环境变量：

| 变量 | 说明 |
| --- | --- |
| `YUQING_DB_DRIVER` | 数据库驱动，支持 `sqlite` / `postgres` |
| `YUQING_DB_PATH` | SQLite 数据库路径 |
| `YUQING_POSTGRES_DSN` | PostgreSQL DSN |
| `YUQING_SERVICE_TOKEN` | 内部服务调用 Token |
| `YUQING_ASTOCK_MARKET_URL` | A 股行情数据源覆盖 |
| `YUQING_ASTOCK_AUCTION_URL` | 集合竞价 AKShare HTTP 服务 |
| `YUQING_STOCK_RESEARCH_URL` | 研报调研 HTTP 服务 |
| `YUQING_TUSHARE_TOKEN` | TuShare 增强数据 Token |

PostgreSQL 生产配置建议：

```powershell
$env:YUQING_DB_DRIVER = "postgres"
$env:YUQING_POSTGRES_DSN = "postgres://user:password@127.0.0.1:5432/yuqing?sslmode=disable"
cd D:\yuqing\go
.\run.bat
```

## 主要页面

| 页面 | 地址 |
| --- | --- |
| 总览 | `/` |
| A 股策略 | `/a-stock` |
| 集合竞价 | `/a-stock/auction` |
| 研报调研 | `/stock-research` |
| Crypto | `/crypto` |
| 系统运行 | `/system?section=operations` |
| 日志 | `/logs` |

## 调度任务

调度任务由 `scheduler-service` 统一管理。可通过系统运行页查看，也可以访问内部 API：

```powershell
Invoke-RestMethod "http://127.0.0.1:8086/api/v1/scheduler/jobs" -Headers @{"X-Service-Token"="stonedt-internal-token"}
```

重点任务：

- `a-stock-morning-recommendation`：上午 A 股新闻和推荐
- `a-stock-afternoon-recommendation`：下午 A 股新闻和推荐
- `a-stock-auction-crawl`：集合竞价金额抓取
- `stock-research-crawl`：研报调研增量抓取
- `crypto-*`：Crypto 新闻和社媒抓取

## 验证

常规测试：

```powershell
cd D:\yuqing\go
go test -count=1 ./...
```

发布验收：

```powershell
cd D:\yuqing\go
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\release-check.ps1
```

验收通过时输出中应包含：

```json
{
  "ready": true,
  "status": "ok"
}
```

## 目录说明

| 路径 | 说明 |
| --- | --- |
| `go/cmd` | 各服务入口 |
| `go/internal` | 核心业务实现 |
| `go/scripts` | 启动、健康检查、发布验收和备份恢复脚本 |
| `go/docs` | 迁移、接口和运行文档 |
| `go/db` | 数据库 schema |
| `go/services` | 外部数据适配服务 |
| `src/main/resources` | 历史资源归档，非 Java 运行入口 |

## Java 退场说明

当前分支不再保留 Java 业务源码、Maven 包装器或 Maven 构建文件。历史 Java 入口的最终状态分为 Go 正式承接、`410 Gone`、`404 Not Found` 或无需迁移；详细对账见 [go/docs/java-go-cutover-report.md](go/docs/java-go-cutover-report.md)。
