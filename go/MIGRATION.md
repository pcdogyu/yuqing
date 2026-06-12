# Java -> Go 迁移完成矩阵

## 结论

- 二期/三期 Java -> Go 主链路迁移截至 2026-06-12 已完成到“可运行 + 兼容收口”状态；四期核心生产化能力已开始落地，覆盖 scheduler 任务注册表、AOP 审计、健康检查和 Windows 运维脚本。
- 当前更准确的判断标准不是“是否已经有 Go 代码”，而是“是否已经脱离 legacy 兼容层，并能由 Go 正式接口稳定承接”。
- `DatafavoriteContoller` 已完成 Go 主链路收口。
- `PublicOptionContoller` 已完成 Go 工作台承接，旧详情页、分析页和 `loadInformation` 已推进为 `410 Gone` 下线。
- `PlatformController` 已完成 Go 工作台承接，旧 `/platform/nlp/*`、`/platform/xie/*` JSON / SSE 入口已在四期收尾中推进为 `410 Gone`。
- Java 全量高级全文检索剩余能力已补齐正式 Go API 承接面，旧 `fullsearch` 与 `timelysearch` 兼容入口均已推进为 `410 Gone`。
- 复杂传播 / 情感 / 专题分析已补齐正式聚合查询契约，并由 `PublicOption` 工作台优先消费；旧兼容页面入口已下线。
- OCR 与外部平台集成已补齐正式报告预览接口、能力清单和文档，正式接入统一使用 `nlp-service /api/v1/nlp/*`。
- 四期收尾已把 legacy 注册表收敛到 `proxy=0`、`preserve=0`、`gone=75`、`delete=40`：不再保留存活旧入口。

## 状态定义

| 状态 | 定义 |
| --- | --- |
| `完成` | 已由 Go 正式接口承接，主链路不再依赖旧 Java 兼容入口。 |
| `兼容完成` | Go 已可承接主要功能，但仍保留 legacy 路由、旧页面或旧返回格式兼容层。 |
| `部分完成` | 只有部分子能力迁移到 Go，仍有明显缺口或兼容返回未收口。 |
| `未完成` | 尚未形成可替代旧 Java 的 Go 实现，或只完成了很小一部分。 |

## 迁移完成矩阵

| Java 控制器 / 能力 | 状态 | Go 承接 | 现状说明 |
| --- | --- | --- | --- |
| `LoginController` | `完成` | `auth-service` + `gateway-web` | 登录、会话、API Token 已由 Go 正式接口承接。 |
| `ProjectController` | `完成` | `content-service / projects + project-groups` | 项目组、项目 CRUD 已在 Go 主链路。 |
| `MonitorController` | `完成` | `content-service / monitor-rules + articles` | 监测规则和文章主数据已完成 Go 化。 |
| `ReportController` | `完成` | `content-service / reports` | 报告列表、详情、生成主链路已由 Go 承接。 |
| `AnalysisController` | `完成` | `analysis-service` | 总览、趋势、来源分布、关键词热点已接入 Go。 |
| `ApiController` 基础文章查询 | `完成` | `content-service / articles + search` | 基础文章查询与搜索 API 已稳定可用。 |
| `SearchController` | `完成` | `gateway-web /search` + `gateway-web /articles?mode=search` | 主搜索入口已切到 Go。 |
| `SystemController` 公告/反馈/任务记录/偏好/预警子集 | `完成` | `content-service / system-*` + `gateway-web /system` | 核心系统页能力已在 Go 主链路。 |
| `DatafavoriteContoller` | `完成` | `portal-web /articles` + `content-service /articles` | 已读、收藏、分享、情感标记、删除均已走正式 Go 动作；旧 `datamonitor/*` 特殊兼容返回不再作为主链路。 |
| `WechatController` | `兼容完成` | `portal-web /wechat/*` + `auth-service /api/v1/wechat/*` | Go 已实现登录、绑定、token 等能力，但仍保留兼容入口。 |
| `MailController` | `兼容完成` | `gateway-web /mail/*` + `content-service /system/mail-config` | 邮件配置已落到 Go，但旧接口格式仍在兼容。 |
| `PopUpController` | `兼容完成` | `gateway-web /popUp/*` + `content-service /system/popup` | 弹窗状态由 Go 存取，但旧入口仍保留。 |
| `ImageController` | `完成` | `/login` | 旧 `/img/code` 验证码兼容入口已下线为 `410 Gone`。 |
| `MobileController` | `完成` | `portal-web /articles` + 主门户页面 | 旧 `/mobile/*` 移动端兼容入口已下线为 `410 Gone`。 |
| `DisplayBoardController` | `完成` | `portal-web /` + `/system?section=operations` | 旧 `/displayboard*` 大屏兼容入口已下线为 `410 Gone`。 |
| `VolumeController` | `完成` | `analysis-service /api/v1/analysis/*` + `/system?section=operations` | 旧 `/volume*` 声量兼容入口已下线为 `410 Gone`。 |
| `HotNewsController` | `完成` | `content-service /api/v1/search/hot-keywords` | 旧 `/hot/*` 热点兼容入口已下线为 `410 Gone`。 |
| `UserAuthController` | `完成` | `/login` + 正式账号流程 | 旧 `/dist/*` 试用申请兼容入口已下线为 `410 Gone`。 |
| `UserController` | `兼容完成` | `content-service /system/preferences` + `gateway-web /system` | 用户资料、偏好已迁移，但仍通过部分兼容入口暴露。 |
| `FullSearchController` | `兼容完成` | `gateway-web /fullsearch` + `content-service /api/v1/search/full` + `portal-web` | 全文搜索主能力已在 Go；旧结果页、特殊详情页、历史词、列表、元数据、特殊类型 JSON 入口均已下线为 `410 Gone`。 |
| `TimelySearchController` | `完成` | `portal-web /articles?mode=timely` + `content-service /api/v1/search/timely` | 即时搜索正式入口可用，旧 `/timelysearch/*` 兼容路由已下线为 `410 Gone`。 |
| `LSearchController` | `兼容完成` | `content-service /api/v1/search/metadata/*` + `/api/v1/search/full/facets` | `/industry` `/getevent` `/getProvinceList` `/getArticleCityList` 已下线为 `410 Gone`，调用方应使用正式搜索元数据和 facets 接口。 |
| `PublicOptionContoller` | `兼容完成` | `portal-web /publicoption/*` + `content-service /api/v1/public-options` | `/publicoption` 已切到统一“事件分析工作台”；列表、详情、创建、更新、删除、分析视图均由 Go 页面承接；旧 `reportdetail/*`、`*analysis*` 和 `loadInformation` 已返回 `410 Gone`；增改删旧 JSON 兼容接口仍保留。 |
| `PlatformController` | `完成` | `portal-web /platform/bindings` + `content-service /platform/bindings/*` + `nlp-service /api/v1/nlp/*` | 平台工作台、绑定、公告、审计、OCR、图像识别、写作标题生成、写作报告预览均由正式 Go 页面/API 承接；旧 `/platform/nlp/*`、`/platform/xie/*` 已下线为 `410 Gone`。 |
| OCR 与外部平台集成 | `完成` | `nlp-service /api/v1/nlp/*` + `portal-web /platform/bindings` | OCR、图像识别、标题生成、报告预览、能力清单均已有正式 Go API；旧 Platform/NLP 兼容入口已下线。 |
| Java 全量高级全文检索剩余能力 | `完成` | `content-service /api/v1/search/metadata/*` + `content-service /api/v1/search/special/*` + `portal-web` | 高级筛选、聚合面包屑、特殊类型列表/选项/详情已补到正式 Go API；旧 `fullsearch` 与 `timelysearch` 兼容入口均已下线。 |
| 复杂传播 / 情感 / 专题分析 | `完成` | `analysis-service /api/v1/public-opinion/analysis` + `portal-web /publicoption/*` | 情感、传播、专题、事件概览、报告建议已形成统一聚合契约，`PublicOption` 页面优先消费正式接口；旧兼容页面已下线。 |
| legacy 路由清理与兼容层收口 | `完成` | `gateway-web` + `portal-web` | 已建立集中式 legacy 路由注册表和执行清单；四期收尾后 `proxy=0`、`preserve=0`，所有存活旧入口均已改为 `410 Gone` 或 `404 Not Found`。 |

## 二期接口基线

- 搜索增强正式接口：
  - `GET /api/v1/search/full`
  - `GET /api/v1/search/timely`
  - `GET /api/v1/search/full/facets`
  - `GET /api/v1/search/history`
  - `GET /api/v1/search/suggestions`
  - `GET /api/v1/search/hot-keywords`
  - `GET /api/v1/search/metadata/types`
  - `GET /api/v1/search/metadata/polymerizations`
  - `GET /api/v1/search/metadata/breadcrumbs`
- 特殊类型详情正式接口：
  - `GET /api/v1/search/details/{id}`
  - `GET /api/v1/search/special/{kind}`
  - `GET /api/v1/search/special/{kind}/options`
  - `GET /api/v1/search/special/{kind}/details/{id}`
- 公共舆情分析正式接口：
  - `GET /api/v1/public-opinion/enrich`
  - `GET /api/v1/public-opinion/analysis`
- NLP / OCR 正式接口：
  - `POST /api/v1/nlp/title`
  - `POST /api/v1/nlp/summarize`
  - `POST /api/v1/nlp/keywords`
  - `POST /api/v1/nlp/ocr`
  - `POST /api/v1/nlp/image`
  - `POST /api/v1/nlp/report-preview`
  - `GET /api/v1/nlp/capabilities`

说明：

- `PublicOption` 工作台的分析富化已优先走 `analysis-service` 正式接口，`portal-web` 本地拼装只作为降级兜底。
- 特殊详情数据不再只依赖 legacy `*detailData` 接口，`content-service` 已提供统一详情出口。
- legacy `fullsearch` / `timelysearch` 入口已下线为 `410 Gone`；调用方应使用 `content-service` 正式搜索接口和 `/articles` 页面。
- `analysis-service` 已新增 `GET /api/v1/public-opinion/analysis`，统一返回情感、传播、专题、事件概览、报告建议与 legacy 字符串结果；`portal-web` 事件分析工作台已优先消费此正式聚合契约，仅在失败时回退旧 `enrich` 接口。
- `nlp-service` 已新增 `POST /api/v1/nlp/report-preview` 和 `GET /api/v1/nlp/capabilities`；`portal-web` 平台工作台优先消费正式报告预览接口，仅在正式接口不可用时回退本地拼装逻辑，旧 `/platform/xie/report*` 已下线。

## 按状态汇总

| 状态 | 项数 | 范围 |
| --- | --- | --- |
| `完成` | 19 | 登录、项目、监测、报告、基础分析、基础文章查询、主搜索入口、系统页核心能力、Datafavorite 主链路、验证码旧入口下线、移动端旧入口下线、大屏旧入口下线、声量旧入口下线、热点旧入口下线、试用申请旧入口下线、即时搜索正式入口、Platform、OCR/外部平台、legacy 收口 |
| `兼容完成` | 9 | 微信、邮件、弹窗、用户资料、全文搜索、LSearch、PublicOption、高级分析相关工作台、复杂分析历史数据契约 |
| `部分完成` | 0 | - |
| `未完成` | 0 | - |

## 三期保留清单

### `兼容保留` 项

无。四期收尾后 `proxy / preserve` 入口均已推进为 `410 Gone`。

当前 legacy 路由清单基线见 [docs/legacy-route-inventory.md](docs/legacy-route-inventory.md)。

## 本次更新

- 2026-06-11：`DatafavoriteContoller` 从 `部分完成` 更新为 `完成`。
- 2026-06-11：文档同步反映以下已落地能力：
  - 文章列表页与详情页统一通过 Go 动作处理文章操作。
  - 已补齐 `favorite` / `read` / `share` / `emotion` / `delete`。
  - 已补测试覆盖情感标记更新、删除后回跳、删除后列表消失等主链路。
- 2026-06-12：`PlatformController` 现状说明更新。
- 2026-06-12：文档同步反映以下已落地能力：
  - `/platform/bindings` 已切到统一“平台工作台”页面。
  - 页面已整合平台绑定状态、平台公告、最近平台操作审计。
  - 保存平台绑定时已写入 `platform.*` 审计日志，并有 `portal` 测试覆盖。
- 2026-06-12：`PlatformController` 从 `部分完成` 更新为 `兼容完成`。
- 2026-06-12：文档同步反映以下已落地能力：
  - `/platform` 根路径已直接进入统一“平台工作台”。
  - OCR、图像识别、写作标题生成、写作报告预览已并入 Go 工作台。
  - 旧 `/platform/nlp/*`、`/platform/xie/*` JSON / SSE 接口当时仍保留，因此未提升为 `完成`。
  - 已补 `portal` 测试覆盖平台工作台主链路。
- 2026-06-12：`PublicOptionContoller` 从 `部分完成` 更新为 `兼容完成`。
- 2026-06-12：文档同步反映以下已落地能力：
  - `/publicoption` 已切到统一“事件分析工作台”页面。
  - 列表、详情、创建、更新、删除、分析视图已统一由 Go 页面承接。
  - 旧 `loadInformation` 当时仍保留，后续三期第三批已推进到 `410 Gone`；增改删旧 JSON 兼容接口仍保留，因此未提升为 `完成`。
  - 已补 `portal` 测试覆盖工作台页面与增改删主链路。
- 2026-06-12：三期收口基线开始落地：
  - `portal-web` 已新增集中式 legacy 路由注册表，作为兼容策略真相源。
  - [docs/legacy-route-inventory.md](docs/legacy-route-inventory.md) 已升级为包含 `legacy_path / owner_module / formal_target / current_behavior / strategy / removal_gate` 的执行清单。
  - `/publicoption/reportdetail/*` 和各类 `publicoption/*analysis*` 旧页面入口已统一跳转到 `/publicoption?id=...&section=...`。
  - 第一批 PublicOption 页面型旧入口收口完成后，上述旧 URL 已进一步调整为 `410 Gone`，正式主链路保留 `/publicoption?id=...&section=...`。
- 2026-06-12：二期接口基线补充：
  - `content-service` 新增 `GET /api/v1/search/details/{id}` 作为统一详情出口。
  - `analysis-service` 新增 `GET /api/v1/public-opinion/enrich`，`PublicOption` 分析富化优先改为正式接口消费。
  - `README.md` 与 `docs/legacy-route-inventory.md` 已同步正式接口与兼容清单基线。
- 2026-06-12：`Java 全量高级全文检索剩余能力` 从 `未完成` 更新为 `兼容完成`。
- 2026-06-12：文档同步反映以下已落地能力：
  - `content-service` 新增 `GET /api/v1/search/metadata/types`、`/polymerizations`、`/breadcrumbs`。
  - `content-service` 新增 `GET /api/v1/search/special/{kind}`、`/options`、`/details/{id}`。
  - `portal-web` 兼容搜索入口已优先改为消费正式搜索增强接口，仅在正式接口缺失或返回空载荷时回退旧逻辑。
- 2026-06-12：三期第一批搜索收口进展：
  - `fullsearch/result`、`/fullsearch/*Detail/*` 等旧页面型入口已统一跳转到 `/articles` 或 `/articles/{id}`。
  - `fullsearch` 兼容列表返回的详情链接已默认切到 `/articles/{id}`，不再把 legacy 详情页作为主目标。
  - 当时 `timelysearch` 仍保留旧结果页与详情页入口，用于保障模板执行和 `return_to` 上下文兼容；四期收尾后该入口已下线为 `410 Gone`。
- 2026-06-12：三期第二批 FullSearch 页面型旧入口收口：
  - `/fullsearch/result`、`/fullsearch/index`、`/fullsearch/*Detail/*` 已从跳转兼容推进到 `410 Gone` 下线。
  - `/articles?mode=full`、`/articles/{id}` 仍是全文搜索列表与详情正式主链路。
  - `fullsearch` 的 `informationList*`、`hotList`、`*List`、`*DetailData`、类型/元数据等 JSON 兼容入口进入下一批 `client-migrated` 下线清单。
  - `timelysearch`、LSearch、Platform、移动端等 legacy 入口当时仍在，因此 `legacy 路由清理与兼容层收口` 当时仍保持 `未完成`。
- 2026-06-12：`复杂传播 / 情感 / 专题分析` 从 `未完成` 更新为 `兼容完成`。
- 2026-06-12：文档同步反映以下已落地能力：
  - `analysis-service` 新增 `GET /api/v1/public-opinion/analysis` 聚合契约，统一返回情感、传播、专题、事件概览、报告建议和 legacy 字符串结果。
  - `portal-web` 事件分析工作台已优先消费正式聚合契约，仅在正式接口失败时回退旧 `enrich` 接口。
  - 已补 `analysis` 与 `portal` 测试覆盖正式契约和页面消费链路。
- 2026-06-12：`OCR 与外部平台集成` 从 `未完成` 更新为 `兼容完成`。
- 2026-06-12：文档同步反映以下已落地能力：
  - `nlp-service` 新增 `POST /api/v1/nlp/report-preview` 与 `GET /api/v1/nlp/capabilities`。
  - `portal-web` 平台工作台与 legacy `/platform/xie/report*` 已优先消费正式报告预览接口，仅在正式接口不可用时回退本地拼装。
  - 已新增 [docs/nlp-platform-api.md](docs/nlp-platform-api.md) 说明鉴权、错误码、调用样例与降级策略。
- 2026-06-12：三期第三批 legacy 入口收口：
  - `gateway-web` 显式注册 `/fullsearch` 与 `/timelysearch`，并在鉴权前按注册表返回 `410 Gone` / `404 Not Found`，避免旧 URL 落入 dashboard。
  - `fullsearch` 剩余 JSON 兼容入口、LSearch 顶层聚合入口、`publicoption/loadInformation` 已推进到 `410 Gone`。
  - 当时代码注册表收敛为 `proxy=3`、`preserve=6`、`gone=66`、`delete=40`；四期收尾后已推进到 `proxy=0`、`preserve=0`、`gone=75`、`delete=40`。
  - [docs/legacy-route-inventory.md](docs/legacy-route-inventory.md) 已按注册表同步存活、下线和删除分组。
- 2026-06-12：四期收尾移除“兼容完成”旧入口：
  - `/timelysearch/*`、`/platform/nlp/*`、`/platform/xie/*`、`/mobile/*`、`/displayboard*`、`/volume*`、`/hot/*`、`/dist/*`、`/img/code` 已统一下线为 `410 Gone`。
  - `portal-web` 路由层已在进入旧 handler 前按 legacy 注册表拦截，避免旧入口继续对外提供兼容访问。
  - 代码注册表收敛为 `proxy=0`、`preserve=0`、`gone=75`、`delete=40`。
  - [docs/legacy-route-inventory.md](docs/legacy-route-inventory.md)、[docs/nlp-platform-api.md](docs/nlp-platform-api.md)、[docs/phase4-productionization.md](docs/phase4-productionization.md) 已同步旧入口下线状态。

## 建议执行顺序

1. 四期核心已补齐 Quartz/AOP 等 Java 等价骨架，包括分析调度、预警调度、报表调度、声量/热点调度、微信调度、用户操作日志和系统访问日志。
2. 五期已补齐 Java cron 实际调度、baseline 对账、备份恢复、外部错误分类和 operations/alerts API；后续深化聚焦生产实测阈值、告警通知渠道和真实 Java 导出基线沉淀。

## 当前 Go 服务职责

- `gateway-web`
  - 统一入口、搜索入口、兼容跳转
- `portal-web`
  - SSR 门户、正式工作台页面、legacy 入口下线拦截
- `auth-service`
  - 登录、登出、session、token、微信登录 / 绑定
- `content-service`
  - 业务主数据与后台接口，包括偏好、弹窗、邮件配置等系统能力
- `crawler-service`
  - 采集和抓取运行记录
- `analysis-service`
  - 聚合分析和快照刷新
- `scheduler-service`
  - 定时触发抓取、分析、PublicOption 预热、预警扫描、报表、声量、热点和微信任务
  - 提供 `/healthz`、`GET /api/v1/scheduler/jobs`、`POST /api/v1/scheduler/jobs/{name}/run`
  - job 列表返回 Java Quartz 对应关系、cron 描述、下次执行时间和最近运行结果
- `nlp-service`
  - 报告标题、摘要、关键词生成

## 四期完成记录

- 2026-06-12：四期核心生产化能力落地：
  - `scheduler-service` 新增统一 job registry 和管理 API，任务列表覆盖 Java Quartz 对应的抓取、分析、PublicOption、预警、报表、声量、热点、微信二维码清理和微信每日推送。
  - scheduler 外部 HTTP 调用统一设置超时与重试，任务执行成功/失败均写入 `task_runs`。
  - `content-service` 新增 HTTP 审计 middleware，对用户访问和操作写入 `audit_logs.detail_json`，包含 method/path/status/duration/ip/user_agent/module/operation，并脱敏 query 中的敏感字段。
  - 新增 PowerShell 运维脚本：`start-all.ps1`、`stop-all.ps1`、`health-check.ps1`、`backup-sqlite.ps1`、`smoke-test.ps1`。
- 2026-06-12：四期生产化深化：
  - `GET /api/v1/scheduler/jobs` 已补齐 `java_quartz_name`、`cron`、`next_run_at`、`last_status`、`last_message`、`last_started_at`、`last_finished_at`。
  - 新增 `cmd/ops-check` 和 `scripts/reconcile-production.ps1`，对 SQLite 生产数据做只读对账，覆盖文章、项目、报告、FTS、任务、审计、平台绑定和 crypto social 来源。
  - `backup-sqlite.ps1` 已扩展为备份后校验，输出备份路径、大小和校验 JSON。
  - `/system?section=operations` 已增加生产运行视图，展示服务健康、scheduler jobs、失败任务、审计、抓取健康和 legacy 注册表。
  - `smoke-test.ps1` 已扩展覆盖 scheduler 运行态、NLP capabilities、审计写入和 legacy 410 探测。
- 2026-06-12：五期生产上线验收闭环：
  - `scheduler-service` 改为使用 `robfig/cron/v3` 按 Java Quartz cron 实际调度，默认时区 `Asia/Shanghai`，支持 `YUQING_SCHEDULER_<JOB>_CRON` 和 `YUQING_SCHEDULER_<JOB>_ENABLED` 覆盖。
  - `cmd/ops-check` 支持 `--baseline` JSON/CSV 基线对账，输出 `success`、`failed`、`diff`、`missing`、`extra`、`warnings`。
  - 新增 `GET /api/v1/system/operations` 与 `GET /api/v1/system/alerts`，门户生产运行页优先消费同一份 operations 数据。
  - 新增 `restore-sqlite.ps1` 与 `release-check.ps1`，`smoke-test.ps1` 扩展覆盖 operations/alerts、备份和恢复演练。
- 2026-06-12：五期第二批跨系统对账与恢复演练深化：
  - `cmd/ops-check --baseline` 支持 Java 导出领域名映射，包括 `articles`、`search_index`、`task_records`、`crypto_social`、`nlp_status` 等。
  - baseline JSON 支持 `counts`、顶层数值、嵌套 `{count}` / `{expected}`；CSV 支持 `name,count` 和 `metric,label,expected`。
  - `restore-sqlite.ps1` 支持 `-SourceDatabasePath`，恢复后对源库与恢复库的表计数输出 `table_counts_match` 和 `table_count_diff`。
  - `release-check.ps1` 已把源库路径传入恢复演练，发布验收可以同时确认备份可读和恢复计数一致。
- 2026-06-12：五期第二批文档复核：
  - 已同步 [docs/phase5-release-readiness.md](docs/phase5-release-readiness.md) 的二批检查结论、baseline 命名映射、恢复演练输出和最小验收命令。
  - 已同步 [docs/phase4-productionization.md](docs/phase4-productionization.md) 的五期二批对账/恢复说明，避免四期文档继续只描述单库健康检查。
- 2026-06-12：五期第三批外部集成韧性标准化：
  - crawler 与 scheduler 外部 HTTP 客户端统一使用 `YUQING_EXTERNAL_RETRY_COUNT`、`YUQING_EXTERNAL_RETRY_WAIT_MS`，仅对超时、HTTP `429` 和 `5xx` 重试。
  - crypto social provider 新增 `YUQING_CRYPTO_SOCIAL_RATE_LIMIT_MS` 请求限流，禁用端点、非 200、坏 JSON、空数据和重复数据均有固定分类或结构化日志。
  - 启动日志新增 `external_retry_count`、`external_retry_wait`、`crypto_social_rate_limit`，便于生产核对实际外部集成策略。
- 2026-06-12：五期第三批文档复核：
  - 已同步 [docs/phase5-release-readiness.md](docs/phase5-release-readiness.md) 的三批最小验收命令和验收口径。
  - 已同步 [docs/crypto-social-proxy.md](docs/crypto-social-proxy.md)、[docs/nlp-platform-api.md](docs/nlp-platform-api.md)、[docs/phase4-productionization.md](docs/phase4-productionization.md) 的外部访问韧性说明。
- 2026-06-12：五期第四批告警与运维 JSON API 深化：
  - `GET /api/v1/system/operations` 已补充 `scheduler_jobs`、`task_summary`、`audit_summary`、`legacy_route_probes`、结构化 `backup` 和 crypto social 最近抓取/入库统计。
  - `GET /api/v1/system/alerts` 已覆盖连续任务失败、审计过期、legacy 非 410、外部集成失败和 crypto social 长时间无入库。
  - `/system?section=operations` 的 scheduler 数据优先消费同一份 operations API，减少页面和脚本口径分叉。
- 2026-06-12：五期第四批文档复核：
  - 已同步 [docs/phase5-release-readiness.md](docs/phase5-release-readiness.md) 的 operations 页面/API 同源口径。
  - 已同步 [docs/crypto-social-proxy.md](docs/crypto-social-proxy.md) 的 `external_integrations` 字段和 `crypto_social_no_recent_insert` 告警说明。

## 下一步

- 五期上线验收闭环已落地：scheduler 实际触发切为 Java Quartz cron 等价调度，`ops-check --baseline` 支持 Java 导出基线对账和领域名映射，`restore-sqlite.ps1` 和 `release-check.ps1` 支持备份恢复与源/恢复库计数一致性检查。
- 新增只读生产运行接口：`GET /api/v1/system/operations`、`GET /api/v1/system/alerts`，`/system?section=operations` 优先消费同一份 operations 数据；第四批后告警口径已覆盖 scheduler、审计、legacy、外部集成和备份状态。
- 外部集成韧性已标准化；后续如接入真实第三方 SLA，可在现有错误分类基础上增加告警阈值和通知渠道。
- legacy 注册表已无剩余 `proxy / preserve` 项；后续新增旧入口必须先更新注册表和文档，并给出明确下线门槛。
- 后续若迁移状态变更，先更新本文件，再同步 `README.md` 摘要。
