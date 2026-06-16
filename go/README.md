# Stonedt Go Platform

Go 版已经从早期 `jin10` 采集骨架收敛为一套可运行的一期多服务系统，核心范围包括：

- `gateway-web`：统一入口、搜索入口与兼容跳转
- `portal-web`：SSR 门户、正式工作台页面与 legacy 下线拦截
- `auth-service`：登录、会话、API Token
- `wechat-service`：微信登录、绑定、token 与公众号回调兼容接口
- `content-service`：项目组、项目、监测规则、文章、报告、公告、反馈、任务记录、偏好、弹窗、邮件配置
- `crawler-service`：抓取执行与抓取运行记录
- `analysis-service`：总览、趋势、来源分布、关键词热点、分析刷新
- `scheduler-service`：定时触发抓取和分析刷新
- `nlp-service`：轻量标题/摘要/关键词生成

二期/三期已经把 Go 主链路和 legacy 收口推进到可运行状态；四期收尾已将剩余 legacy `proxy` / `preserve` 入口统一下线为 `410 Gone`，当前注册表为 `proxy=0`、`preserve=0`、`gone=76`、`delete=40`。

当前二期已经补出的正式接口基线：

- 搜索增强：`/api/v1/search/full`、`/api/v1/search/timely`、`/api/v1/search/full/facets`、`/api/v1/search/history`、`/api/v1/search/suggestions`、`/api/v1/search/hot-keywords`、`/api/v1/search/metadata/types`、`/api/v1/search/metadata/polymerizations`、`/api/v1/search/metadata/breadcrumbs`
- 特殊详情：`/api/v1/search/details/{id}`、`/api/v1/search/special/{kind}`、`/api/v1/search/special/{kind}/options`、`/api/v1/search/special/{kind}/details/{id}`
- 公共舆情分析：`/api/v1/public-opinion/enrich`、`/api/v1/public-opinion/analysis`
- NLP/OCR：`/api/v1/nlp/title`、`/api/v1/nlp/summarize`、`/api/v1/nlp/keywords`、`/api/v1/nlp/ocr`、`/api/v1/nlp/image`、`/api/v1/nlp/report-preview`、`/api/v1/nlp/capabilities`

## 迁移完成矩阵

状态定义：

- `完成`：已由 Go 正式接口承接，主链路不再依赖旧 Java 兼容入口。
- `兼容完成`：Go 已可承接功能，但仍保留 legacy 路由、旧页面或旧返回格式兼容层。
- `部分完成`：只有部分子能力迁移到 Go，仍有明显缺口或兼容返回未收口。
- `未完成`：仍未形成可替代旧 Java 的 Go 实现，或只完成了很小一部分。

| 模块/能力 | 状态 | 说明 |
| --- | --- | --- |
| 登录、会话、API Token | `完成` | 已由 `auth-service` 和 Go 门户主链路承接。 |
| 项目、项目组、监测规则、文章、报告 | `完成` | 基础业务 CRUD 和查询主链路已在 Go。 |
| 总览、趋势、来源分布、关键词热点 | `完成` | 基础分析接口已由 `analysis-service` 承接。 |
| 系统公告、反馈、任务记录、偏好、预警子集 | `完成` | 核心系统页能力已在 Go 主链路。 |
| 微信登录/绑定 | `兼容完成` | Go 已实现，但仍保留兼容入口。 |
| 邮件配置、弹窗状态 | `兼容完成` | Go 已实现，旧接口格式仍在兼容。 |
| 移动端、大屏、热点、声量、申请试用 | `完成` | 旧 `/mobile/*`、`/displayboard*`、`/volume*`、`/hot/*`、`/dist/*`、`/img/code` 已统一下线为 `410 Gone`，正式能力走 Go 门户与正式 API。 |
| 全文搜索、即时搜索、LSearch 历史筛选接口 | `完成` | 旧 `fullsearch` JSON、LSearch 顶层聚合和 `timelysearch` 入口均已下线为 `410 Gone`，正式链路走 `/articles` 与 `/api/v1/search/*`。 |
| 平台设置、公共选项、收藏/已读等操作 | `完成` | Go 已承接主闭环；`publicoption/loadInformation`、`/platform/nlp/*`、`/platform/xie/*` 等旧入口已下线为 `410 Gone`。 |
| OCR 与外部平台集成 | `完成` | OCR、图像识别、标题生成、报告预览、能力清单均由正式 Go API 承接，旧兼容平台入口已下线。 |
| Java 全量高级全文检索剩余能力 | `兼容完成` | 高级筛选、聚合面包屑、特殊类型列表/选项/详情已补到正式 Go API，旧 `fullsearch` JSON 已下线。 |
| 复杂传播/情感/专题分析 | `完成` | 已有统一聚合查询契约，`PublicOption` 页面优先消费正式分析接口，旧分析兼容入口已下线。 |
| legacy 路由清理与兼容层收口 | `完成` | 注册表已收敛为 `proxy=0`、`preserve=0`、`gone=76`、`delete=40`；不再保留存活 legacy 业务入口。 |

详细矩阵见 [MIGRATION.md](MIGRATION.md)。

legacy 路由清单基线见 [docs/legacy-route-inventory.md](docs/legacy-route-inventory.md)。

NLP / OCR 正式接口说明见 [docs/nlp-platform-api.md](docs/nlp-platform-api.md)。

## Crypto 社媒接入

系统已支持 `crypto_x` 和 `crypto_telegram` 两个外部社媒抓取源。你可以接自己的代理层，也可以先用仓库内 mock 服务联调。
配置 `YUQING_CRYPTO_X_URL` 或 `YUQING_CRYPTO_TELEGRAM_URL` 后，`scheduler-service` 会分别启用 `crypto-x-crawl`、`crypto-telegram-crawl` 定时任务；未配置 URL 时任务会保留在 `/api/v1/scheduler/jobs` 但处于 disabled 状态。

## Crypto 新闻源

`/crypto` 也会抓取新闻证据源：

- `foresight_newsflash`：默认 `YUQING_FORESIGHT_NEWSFLASH_URL=https://foresightnews.pro/news`，scheduler 任务为 `foresight-newsflash-crawl`，默认间隔 `120s`。
- `coindesk_zh_latest`：默认 `YUQING_COINDESK_ZH_LATEST_URL=https://www.coindesk.com/zh/latest-crypto-news`，scheduler 任务为 `coindesk-zh-latest-crawl`，默认间隔 `300s`。
- `panews_newsflash`：默认 `YUQING_PANEWS_NEWSFLASH_URL=https://www.panewslab.com/rss.xml?lang=zh&type=NEWS`，scheduler 任务为 `panews-newsflash-crawl`，默认间隔 `120s`。

这两个源默认启用；如果需要禁用，把对应 URL 环境变量显式设为空。手动触发：

```powershell
Invoke-WebRequest -Method Post "http://127.0.0.1:8083/api/v1/admin/tasks/crawl?source_type=foresight_newsflash" -Headers @{"X-Service-Token"="stonedt-internal-token"}
Invoke-WebRequest -Method Post "http://127.0.0.1:8083/api/v1/admin/tasks/crawl?source_type=coindesk_zh_latest" -Headers @{"X-Service-Token"="stonedt-internal-token"}
Invoke-WebRequest -Method Post "http://127.0.0.1:8083/api/v1/admin/tasks/crawl?source_type=panews_newsflash" -Headers @{"X-Service-Token"="stonedt-internal-token"}
```

快速联调：

```powershell
cd D:\yuqing\go
powershell -ExecutionPolicy Bypass -File .\scripts\mock-crypto-social.ps1
```

另开一个终端配置：

```powershell
$env:YUQING_CRYPTO_X_URL = "http://127.0.0.1:19090/mock/x"
$env:YUQING_CRYPTO_TELEGRAM_URL = "http://127.0.0.1:19090/mock/telegram"
$env:YUQING_CRYPTO_X_INTERVAL_SEC = "90"
$env:YUQING_CRYPTO_TELEGRAM_INTERVAL_SEC = "90"
```

然后启动 `worker` 或直接调用：

```powershell
Invoke-WebRequest -Method Post "http://127.0.0.1:8083/api/v1/admin/tasks/crawl?source_type=crypto_x" -Headers @{"X-Service-Token"="stonedt-internal-token"}
Invoke-WebRequest -Method Post "http://127.0.0.1:8083/api/v1/admin/tasks/crawl?source_type=crypto_telegram" -Headers @{"X-Service-Token"="stonedt-internal-token"}
```

联调完成后可打开：

```text
http://127.0.0.1/crypto?pair=eth
```

页面会显示 ETH/USDT 的价格、相关新闻、社媒证据和空态诊断；如无证据，可直接在页面触发 X/Telegram 抓取或刷新分析。
页面也可直接触发 Foresight、CoinDesk 中文与 PANews 抓取。

对接字段和返回格式说明见 [docs/crypto-social-proxy.md](docs/crypto-social-proxy.md)。

## 默认端口

- `gateway-web`: `80`
- `auth-service`: `8081`
- `wechat-service`: `8087`
- `content-service`: `8082`
- `crawler-service`: `8083`
- `analysis-service`: `8084`
- `nlp-service`: `8085`
- `scheduler-service`: `8086`

## 数据

- 主数据库：SQLite
- 默认路径：`go/data/yuqing.db`
- 默认管理员：`admin / admin123`

## 启动

Windows PowerShell:

```powershell
cd D:\yuqing\go
go run .\cmd\auth-service
go run .\cmd\wechat-service
go run .\cmd\content-service
go run .\cmd\crawler-service
go run .\cmd\analysis-service
go run .\cmd\nlp-service
go run .\cmd\gateway-web
go run .\cmd\scheduler-service
```

生产化脚本：

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

或者直接运行：

```powershell
.\run.bat
```

浏览器打开：

```text
http://127.0.0.1
```

## 关键接口

- `POST /api/v1/auth/login`
- `GET|POST|PUT|DELETE /api/v1/project-groups`
- `GET|POST|PUT|DELETE /api/v1/projects`
- `GET|POST|PUT|DELETE /api/v1/monitor-rules`
- `GET /api/v1/articles`
- `GET /api/v1/articles/{id}`
- `GET /api/v1/articles/{id}/related`
- `GET /api/v1/search/articles`
- `GET /api/v1/search/full`
- `GET /api/v1/search/timely`
- `GET /api/v1/search/full/facets`
- `GET /api/v1/search/history`
- `GET /api/v1/search/suggestions`
- `GET /api/v1/search/hot-keywords`
- `GET /api/v1/search/metadata/types`
- `GET /api/v1/search/metadata/polymerizations`
- `GET /api/v1/search/metadata/breadcrumbs`
- `GET /api/v1/search/details/{id}`
- `GET /api/v1/search/special/{kind}`
- `GET /api/v1/search/special/{kind}/options`
- `GET /api/v1/search/special/{kind}/details/{id}`
- `GET /api/v1/analysis/overview`
- `GET /api/v1/analysis/trends`
- `GET /api/v1/analysis/sources`
- `GET /api/v1/public-opinion/analysis`
- `GET /api/v1/public-opinion/enrich`
- `GET|POST /api/v1/reports`
- `GET /api/v1/reports/{id}`
- `GET /api/v1/system/notices`
- `POST /api/v1/system/feedback`
- `GET /api/v1/system/task-runs`
- `GET /api/v1/system/audit-logs`
- `GET /api/v1/system/operations`
- `GET /api/v1/system/alerts`
- `GET /api/v1/scheduler/jobs`
- `POST /api/v1/scheduler/jobs/{name}/run`
- `POST /api/v1/admin/tasks/crawl`
- `GET /api/v1/admin/tasks/crawl/runs`
- `POST /api/v1/admin/tasks/analysis/refresh`
- `GET /api/v1/crypto/social`
- `GET /api/v1/crypto/insights`
- `POST /api/v1/nlp/title`
- `POST /api/v1/nlp/summarize`
- `POST /api/v1/nlp/keywords`
- `POST /api/v1/nlp/ocr`
- `POST /api/v1/nlp/image`
- `POST /api/v1/nlp/report-preview`
- `GET /api/v1/nlp/capabilities`

## 已完成的一期范围

- 账号登录、会话守卫、API Token
- 项目组/项目/监测规则的基础 CRUD API
- 文章列表、详情、相关文章、FTS 搜索
- 按监测规则把抓取结果关联到项目
- 总览、趋势、来源分布、关键词热点
- 报告生成与详情查看
- 系统公告、反馈、任务记录页面
- Windows 启动脚本和多服务入口

## 四期核心完成范围

- `scheduler-service` 已提供统一任务注册表、健康检查、任务列表和手动触发接口。
- `GET /api/v1/scheduler/jobs` 已返回 Java Quartz 对应关系、cron 描述、下次执行时间和最近运行结果。
- Quartz 等价任务已覆盖抓取、分析、PublicOption 预热、预警扫描、报表、声量、热点、微信二维码清理和微信每日推送。
- `content-service` 已增加 HTTP 审计 middleware，操作日志写入 `audit_logs` 并脱敏 query 中的 token/password/secret/key。
- 外部 HTTP 调用已在 scheduler 侧增加超时和重试，失败写入 `task_runs`，不拖垮后台循环。
- `/system?section=operations` 已提供服务健康、scheduler、失败任务、审计、抓取健康和 legacy 注册表视图。
- Windows PowerShell 已补齐启动、停止、健康检查、SQLite 备份校验、生产对账和 smoke test 脚本。
- 四期收尾已把剩余旧入口统一下线：`/timelysearch/*`、`/platform/nlp/*`、`/platform/xie/*`、`/mobile/*`、`/displayboard*`、`/volume*`、`/hot/*`、`/dist/*`、`/img/code` 均返回 `410 Gone`。

## 后续深化

- 五期已把 scheduler 实际触发切到 Java Quartz cron 等价调度，`next_run_at` 由 cron 表达式计算。
- 五期已新增上线验收闭环：`/api/v1/system/operations`、`/api/v1/system/alerts`、baseline 对账、备份恢复演练和 `release-check.ps1`。
- Java 原系统直接数据差异对账通过 `ops-check --baseline` 接收导出的 JSON/CSV 基线；五期二批已支持 Java 领域名映射和恢复库表计数一致性检查。
- 五期三批已统一外部 HTTP 重试策略，支持 `YUQING_EXTERNAL_RETRY_COUNT`、`YUQING_EXTERNAL_RETRY_WAIT_MS` 和 `YUQING_CRYPTO_SOCIAL_RATE_LIMIT_MS`。
- 五期四批已深化 operations/alerts：统一返回 scheduler 摘要、连续失败任务、审计新鲜度、legacy 410 探测、外部集成健康和备份年龄。
- 五期五批已将 `release-check.ps1` 收口为纯 JSON 发布验收报告；`ready=true` 才视为上线验收通过，可通过 `-OutputPath` 保存报告。备份脚本复制前会执行 WAL checkpoint，停止脚本会清理 `go run` 留下的服务子进程。

说明：具体模块状态以 [MIGRATION.md](MIGRATION.md) 的详细矩阵为准。
