# Java -> Go 迁移完成矩阵

## 结论

- 二期 Java -> Go 迁移截至 2026-06-12 仍未全部完成。
- 当前更准确的判断标准不是“是否已经有 Go 代码”，而是“是否已经脱离 legacy 兼容层，并能由 Go 正式接口稳定承接”。
- `DatafavoriteContoller` 已完成 Go 主链路收口。
- `PublicOptionContoller` 已完成 Go 工作台承接，旧 JSON / 路由兼容层仍保留，因此状态更新为“兼容完成”。
- `PlatformController` 已完成 Go 工作台承接，旧 JSON / SSE / 路由兼容层仍保留，因此状态更新为“兼容完成”。

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
| `FullSearchController` | `兼容完成` | `gateway-web /fullsearch` + `content-service /api/v1/search/full` + `portal-web` | 全文搜索主能力已在 Go，但旧筛选、历史词和详情页兼容仍在。 |
| `TimelySearchController` | `兼容完成` | `gateway-web /timelysearch` + `content-service /api/v1/search/timely` + `portal-web` | 即时搜索和模板执行可用，但仍依赖兼容路由与旧返回格式。 |
| `LSearchController` | `兼容完成` | `gateway-web` compatibility endpoints | `/industry` `/getevent` `/getProvinceList` `/getArticleCityList` 仍是兼容接口。 |
| `PublicOptionContoller` | `兼容完成` | `portal-web /publicoption/*` + `content-service /api/v1/public-options` | `/publicoption` 已切到统一“事件分析工作台”；列表、详情、创建、更新、删除、分析视图均由 Go 页面承接；旧 `loadInformation`、旧 JSON 兼容接口仍保留。 |
| `PlatformController` | `兼容完成` | `portal-web /platform/*` + `content-service /system-*` + `content-service /platform/bindings/*` | `/platform` 与 `/platform/bindings` 已切到统一“平台工作台”；绑定、公告、最近平台操作审计、OCR、图像识别、写作标题生成、写作报告预览均可由 Go 页面承接；旧 JSON / SSE 兼容接口仍保留。 |
| OCR 与外部平台集成 | `未完成` | - | 仍未形成明确、稳定的 Go 对外承接面。 |
| Java 全量高级全文检索剩余能力 | `未完成` | 部分在 `content-service` + `portal-web` | Go 已覆盖基础与部分高级检索，但还未完全替代旧 Java 全量能力。 |
| 复杂传播 / 情感 / 专题分析 | `未完成` | 部分在 `analysis-service` | 已有基础分析接口，但复杂分析闭环尚未完成。 |
| legacy 路由清理与兼容层收口 | `未完成` | `gateway-web` + `portal-web` | 代码中仍存在大量 legacy endpoints 和兼容页面。 |

## 按状态汇总

| 状态 | 项数 | 范围 |
| --- | --- | --- |
| `完成` | 9 | 登录、项目、监测、报告、基础分析、基础文章查询、主搜索入口、系统页核心能力、Datafavorite 主链路 |
| `兼容完成` | 15 | 微信、邮件、弹窗、验证码、移动端、大屏、音量、热点、试用申请、用户资料、全文搜索、即时搜索、LSearch、PublicOption、Platform |
| `部分完成` | 0 | - |
| `未完成` | 4 | OCR/外部平台、高级全文检索剩余能力、复杂分析、legacy 收口 |

## 二期待办清单

### `未完成` 项

| 项目 | 当前缺口 | 建议动作 | 完成标准 |
| --- | --- | --- | --- |
| OCR 与外部平台集成 | `nlp-service` 内已有 OCR 处理代码线索，但尚未在迁移矩阵里形成明确、稳定的对外承接面。 | 确认 OCR 是否作为正式能力保留；补文档、鉴权、错误码和调用样例；梳理外部平台集成清单。 | OCR 与外部平台能力有明确接口、文档和启停策略，能独立上线验证。 |
| Java 全量高级全文检索剩余能力 | 当前 Go 已覆盖全文搜索、即时搜索和部分筛选/详情，但高级筛选、历史词、特殊类型详情仍散落在兼容层。 | 列出高级检索子能力清单；逐项判断保留、合并或下线；把仍需保留的能力改成 `content-service` 正式接口。 | `fullsearch` / `timelysearch` 仅保留必要跳转或完全下线，剩余能力有正式 Go API。 |
| 复杂传播 / 情感 / 专题分析 | 现有 `analysis-service` 已有 overview、emotions、propagation、themes 等接口和底层字段，但复杂分析闭环尚未明确。 | 先定义二期范围内必须保留的分析能力；按情感、传播、专题拆分输入输出；补快照刷新、查询和页面展示的联调验证。 | 分析能力边界清晰，复杂分析不再依赖旧 Java 页面拼装或隐式字段。 |
| legacy 路由清理与兼容层收口 | `gateway-web`、`portal-web` 内仍有大量 legacy endpoints、旧页面和旧 JSON 包装。 | 建立 legacy 路由清单；为每条路由标注“保留 / 替换 / 删除”；分批让测试覆盖 301/302/404/新接口替代行为。 | 兼容路由数量持续下降，最终只保留明确声明的过渡入口。 |

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
  - 旧 `loadInformation`、旧 JSON 兼容接口仍保留，因此未提升为 `完成`。
  - 已补 `portal` 测试覆盖工作台页面与增改删主链路。

## 建议执行顺序

1. 先处理高级全文检索和复杂分析，避免在兼容层继续堆逻辑。
2. 再批量清理 legacy 路由，并同步下线文档中的旧入口说明。

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
  - 定时触发抓取和分析任务
- `nlp-service`
  - 报告标题、摘要、关键词生成

## 下一步

- 优先处理 `部分完成` 和 `未完成` 项，避免继续新增兼容层。
- 为 `兼容完成` 项逐步建立“Go 正式接口替代 legacy 路由”的下线清单。
- 后续若迁移状态变更，先更新本文件，再同步 `README.md` 摘要。
