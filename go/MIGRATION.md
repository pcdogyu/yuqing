# Java -> Go 迁移完成矩阵

## 结论

- 二期/三期 Java -> Go 主链路迁移截至 2026-06-12 已完成到“可运行 + 兼容收口”状态；四期核心生产化能力已开始落地，覆盖 scheduler 任务注册表、AOP 审计、健康检查和 Windows 运维脚本。
- 当前更准确的判断标准不是“是否已经有 Go 代码”，而是“是否已经脱离 legacy 兼容层，并能由 Go 正式接口稳定承接”。
- `DatafavoriteContoller` 已完成 Go 主链路收口。
- `PublicOptionContoller` 已完成 Go 工作台承接，旧 JSON / 路由兼容层仍保留；其中旧详情页和分析页入口已推进为 `410 Gone` 下线，因此状态保持“兼容完成”。
- `PlatformController` 已完成 Go 工作台承接，旧 JSON / SSE / 路由兼容层仍保留，因此状态更新为“兼容完成”。
- Java 全量高级全文检索剩余能力已补齐正式 Go API 承接面，旧 `fullsearch` JSON 入口已推进为 `410 Gone`，因此状态更新为“兼容完成”。
- 复杂传播 / 情感 / 专题分析已补齐正式聚合查询契约，并由 `PublicOption` 工作台优先消费，因此状态更新为“兼容完成”。
- OCR 与外部平台集成已补齐正式报告预览接口、能力清单和文档，平台工作台优先消费正式 `nlp-service` 能力；旧 Platform/NLP URL 仅作为外部契约兼容入口保留。
- 三期 legacy 收口已把注册表收敛到 `proxy=3`、`preserve=6`、`gone=66`、`delete=40`：剩余 `proxy` 均为 `timelysearch` 或外部平台契约，剩余 `preserve` 均为外部访问页面。

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
| `ImageController` | `兼容完成` | `portal-web /img/code` | 验证码入口已在 Go，但属于 legacy 兼容页体系。 |
| `MobileController` | `兼容完成` | `portal-web /mobile/*` | 移动端页面、二维码、跳转已由 Go 提供兼容入口。 |
| `DisplayBoardController` | `兼容完成` | `portal-web /displayboard` | 看板页可由 Go 打开，但仍是兼容页面形态。 |
| `VolumeController` | `兼容完成` | `portal-web /volume` | 页面和 `getproject` / `projectname` 仍按旧入口兼容。 |
| `HotNewsController` | `兼容完成` | `portal-web /hot/*` | 热点页和热点列表已在 Go，但仍保留旧访问方式。 |
| `UserAuthController` | `兼容完成` | `portal-web /dist/*` | 申请试用、跳转入口已在 Go 兼容层。 |
| `UserController` | `兼容完成` | `content-service /system/preferences` + `gateway-web /system` | 用户资料、偏好已迁移，但仍通过部分兼容入口暴露。 |
| `FullSearchController` | `兼容完成` | `gateway-web /fullsearch` + `content-service /api/v1/search/full` + `portal-web` | 全文搜索主能力已在 Go；旧结果页、特殊详情页、历史词、列表、元数据、特殊类型 JSON 入口均已下线为 `410 Gone`。 |
| `TimelySearchController` | `兼容完成` | `gateway-web /timelysearch` + `content-service /api/v1/search/timely` + `portal-web` | 即时搜索和模板执行可用，但仍依赖兼容路由与旧返回格式。 |
| `LSearchController` | `兼容完成` | `content-service /api/v1/search/metadata/*` + `/api/v1/search/full/facets` | `/industry` `/getevent` `/getProvinceList` `/getArticleCityList` 已下线为 `410 Gone`，调用方应使用正式搜索元数据和 facets 接口。 |
| `PublicOptionContoller` | `兼容完成` | `portal-web /publicoption/*` + `content-service /api/v1/public-options` | `/publicoption` 已切到统一“事件分析工作台”；列表、详情、创建、更新、删除、分析视图均由 Go 页面承接；旧 `reportdetail/*`、`*analysis*` 和 `loadInformation` 已返回 `410 Gone`；增改删旧 JSON 兼容接口仍保留。 |
| `PlatformController` | `兼容完成` | `portal-web /platform/*` + `content-service /system-*` + `content-service /platform/bindings/*` | `/platform` 与 `/platform/bindings` 已切到统一“平台工作台”；绑定、公告、最近平台操作审计、OCR、图像识别、写作标题生成、写作报告预览均可由 Go 页面承接；旧 JSON / SSE 兼容接口仍保留。 |
| OCR 与外部平台集成 | `兼容完成` | `nlp-service /api/v1/nlp/*` + `portal-web /platform/*` | OCR、图像识别、标题生成、报告预览、能力清单均已有正式 Go API；旧 `/platform/nlp/*`、`/platform/xie/*` 兼容入口仍保留。 |
| Java 全量高级全文检索剩余能力 | `兼容完成` | `content-service /api/v1/search/metadata/*` + `content-service /api/v1/search/special/*` + `portal-web` | 高级筛选、聚合面包屑、特殊类型列表/选项/详情已补到正式 Go API；旧 `fullsearch` JSON 已下线，`timelysearch` 因模板执行和 `return_to` 兼容继续保留。 |
| 复杂传播 / 情感 / 专题分析 | `兼容完成` | `analysis-service /api/v1/public-opinion/analysis` + `portal-web /publicoption/*` | 情感、传播、专题、事件概览、报告建议已形成统一聚合契约，`PublicOption` 页面优先消费正式接口；旧兼容页面与返回格式仍保留。 |
| legacy 路由清理与兼容层收口 | `兼容完成` | `gateway-web` + `portal-web` | 已建立集中式 legacy 路由注册表和执行清单；旧 `fullsearch` JSON、LSearch、`publicoption/loadInformation` 已下线为 `410 Gone`；剩余入口均标注为 `client-migrated` 过渡或 `external-contract` 长期兼容。 |

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
- legacy `fullsearch` / `timelysearch` 的类型筛选、聚合、面包屑、特殊实体查询现已优先转发到 `content-service` 正式搜索接口，只有在正式接口不可用或返回空载荷时才回退旧兼容逻辑。
- `analysis-service` 已新增 `GET /api/v1/public-opinion/analysis`，统一返回情感、传播、专题、事件概览、报告建议与 legacy 字符串结果；`portal-web` 事件分析工作台已优先消费此正式聚合契约，仅在失败时回退旧 `enrich` 接口。
- `nlp-service` 已新增 `POST /api/v1/nlp/report-preview` 和 `GET /api/v1/nlp/capabilities`；`portal-web` 平台工作台和 legacy `/platform/xie/report*` 已优先消费正式报告预览接口，仅在正式接口不可用时回退本地拼装逻辑。

## 按状态汇总

| 状态 | 项数 | 范围 |
| --- | --- | --- |
| `完成` | 9 | 登录、项目、监测、报告、基础分析、基础文章查询、主搜索入口、系统页核心能力、Datafavorite 主链路 |
| `兼容完成` | 19 | 微信、邮件、弹窗、验证码、移动端、大屏、音量、热点、试用申请、用户资料、全文搜索、即时搜索、LSearch、PublicOption、Platform、高级全文检索剩余能力、复杂分析、OCR/外部平台、legacy 收口 |
| `部分完成` | 0 | - |
| `未完成` | 0 | - |

## 三期保留清单

### `兼容保留` 项

| 项目 | 当前缺口 | 建议动作 | 完成标准 |
| --- | --- | --- | --- |
| `timelysearch/*` | 仍承接模板执行、结果页、详情页和 `return_to` 兼容语义。 | 保持 `proxy / client-migrated`，新调用方使用 `/articles?mode=timely` 与 `/api/v1/search/timely`。 | 调用方迁移完成后再改为 `410 Gone`。 |
| `/platform/nlp/*`、`/platform/xie/*` | 作为 OCR、图像识别、写作工具和 SSE 外部契约兼容入口。 | 保持 `proxy / external-contract`，正式接入统一使用 `/api/v1/nlp/*`。 | 外部契约确认废弃后再下线。 |
| `/mobile/*`、`/displayboard*`、`/volume*`、`/hot/*`、`/dist/*`、`/img/code` | 外部访问页面或历史入口仍需保留。 | 保持 `preserve / external-contract`，不再计入未完成迁移缺口。 | 只有业务确认废弃后才删除。 |

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
  - 旧 `/platform/nlp/*`、`/platform/xie/*` JSON / SSE 接口仍保留，因此未提升为 `完成`。
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
  - `timelysearch` 仍保留旧结果页与详情页入口，以继续保障模板执行和 `return_to` 上下文兼容。
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
  - 代码注册表收敛为 `proxy=3`、`preserve=6`、`gone=66`、`delete=40`；剩余 `proxy` 为 `timelysearch` 与 Platform/NLP 外部契约，剩余 `preserve` 为外部访问页面。
  - [docs/legacy-route-inventory.md](docs/legacy-route-inventory.md) 已按注册表同步存活、下线和删除分组。

## 建议执行顺序

1. 四期核心已补齐 Quartz/AOP 等 Java 等价骨架，包括分析调度、预警调度、报表调度、声量/热点调度、微信调度、用户操作日志和系统访问日志。
2. 后续深化聚焦 Java cron 绝对时刻 1:1、生产数据对账脚本、外部集成限流策略和更细粒度的告警看板。

## 当前 Go 服务职责

- `gateway-web`
  - 统一入口、搜索入口、兼容跳转
- `portal-web`
  - SSR 门户、旧页面兼容入口、二维码、移动端、热点、看板页面
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
- `nlp-service`
  - 报告标题、摘要、关键词生成

## 四期完成记录

- 2026-06-12：四期核心生产化能力落地：
  - `scheduler-service` 新增统一 job registry 和管理 API，任务列表覆盖 Java Quartz 对应的抓取、分析、PublicOption、预警、报表、声量、热点、微信二维码清理和微信每日推送。
  - scheduler 外部 HTTP 调用统一设置超时与重试，任务执行成功/失败均写入 `task_runs`。
  - `content-service` 新增 HTTP 审计 middleware，对用户访问和操作写入 `audit_logs.detail_json`，包含 method/path/status/duration/ip/user_agent/module/operation，并脱敏 query 中的敏感字段。
  - 新增 PowerShell 运维脚本：`start-all.ps1`、`stop-all.ps1`、`health-check.ps1`、`backup-sqlite.ps1`、`smoke-test.ps1`。

## 下一步

- 深化四期：围绕 Java cron 绝对时刻、生产数据对账、外部集成限流和告警看板继续补齐。
- 对剩余 `proxy / preserve` 项保持注册表和文档同步；只有调用方确认迁移或外部契约废弃后才继续下线。
- 后续若迁移状态变更，先更新本文件，再同步 `README.md` 摘要。
