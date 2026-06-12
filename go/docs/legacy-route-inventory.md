# Legacy Route Inventory

截至 `2026-06-12`，三期按“代码注册表 + 文档执行清单”统一管理 legacy 路由。新增兼容入口必须先补注册表与本清单；禁止继续新增匿名 legacy handler。

当前代码注册表基线：`proxy=3`、`preserve=6`、`gone=66`、`delete=40`、`redirect=0`。`go/internal/portal/legacy_routes.go` 是精确真相源，本清单按执行批次合并展示。

## 执行规则

- `strategy` 只允许：`preserve`、`proxy`、`redirect`、`gone`、`delete`
- `removal_gate` 只允许：`ui-migrated`、`client-migrated`、`usage-zero`、`external-contract`
- 页面入口优先 `302`
- 已过渡完成且仍被访问的 API 优先 `410`
- 确认无调用方的入口直接删除

## 存活 legacy 路由

| legacy_path | owner_module | formal_target | current_behavior | strategy | removal_gate |
| --- | --- | --- | --- | --- | --- |
| `/timelysearch/*` | `TimelySearchController` | `/articles?mode=timely` | 旧即时搜索页面、结果页与兼容数据流 | `proxy` | `client-migrated` |
| `/platform/nlp/*` | `PlatformController` | `/api/v1/nlp/*` | OCR / 图像识别兼容入口 | `proxy` | `external-contract` |
| `/platform/xie/*` | `PlatformController` | `/api/v1/nlp/*` | 标题 / 报告预览 / SSE 兼容入口 | `proxy` | `external-contract` |
| `/mobile/*` | `MobileController` | `portal SSR pages` | 移动端兼容页 | `preserve` | `external-contract` |
| `/displayboard*` | `DisplayBoardController` | `portal SSR pages` | 大屏兼容页 | `preserve` | `external-contract` |
| `/volume*` | `VolumeController` | `portal SSR pages` | 声量兼容页 | `preserve` | `external-contract` |
| `/hot/*` | `HotNewsController` | `portal SSR pages` | 热点兼容页 | `preserve` | `external-contract` |
| `/dist/*` | `UserAuthController` | `portal SSR pages` | 试用申请兼容页 | `preserve` | `external-contract` |
| `/img/code` | `ImageController` | `/login` | 验证码兼容入口 | `preserve` | `external-contract` |

## 已下线 legacy 路由

| legacy_path | owner_module | formal_target | current_behavior | strategy | removal_gate |
| --- | --- | --- | --- | --- | --- |
| `/jumpLogin` | `LoginController` | `/login` | 已移除旧登录跳转 | `delete` | `usage-zero` |
| `/wechatJumpLogin` | `LoginController` | `/wechat/checkLogin` | 已移除旧微信登录跳转 | `delete` | `usage-zero` |
| `/onlinestatistical` | `LoginController` | `/system?section=account` | 已移除旧在线统计入口 | `delete` | `usage-zero` |
| `/user/save` | `UserController` | `/system?section=account` | 已移除旧用户保存入口 | `delete` | `usage-zero` |
| `/user/getToken` | `UserController` | `/api/v1/auth/login` | 已移除旧 token 入口 | `delete` | `usage-zero` |
| `/fullsearch/result` | `FullSearchController` | `/articles?mode=full` | 已下线旧全文搜索结果页入口，访问返回 `410 Gone` | `gone` | `ui-migrated` |
| `/fullsearch/index` | `FullSearchController` | `/articles?mode=full` | 已下线旧全文搜索首页入口，访问返回 `410 Gone` | `gone` | `ui-migrated` |
| `/fullsearch/*Detail/*` | `FullSearchController` | `/articles/{id}` | 已下线旧特殊详情页入口，访问返回 `410 Gone`；详情主链路使用 `/articles/{id}` | `gone` | `ui-migrated` |
| `/fullsearch/informationList*`、`/fullsearch/hotList`、`/fullsearch/*List` | `FullSearchController` | `/api/v1/search/full`、`/api/v1/search/special/{kind}` | 已下线旧全文搜索列表类 JSON，访问返回 `410 Gone` | `gone` | `client-migrated` |
| `/fullsearch/*DetailData`、`/fullsearch/companyDetails`、`/fullsearch/getresearch-report-detail` | `FullSearchController` | `/api/v1/search/special/{kind}/details/{id}` | 已下线旧特殊详情数据 JSON，访问返回 `410 Gone` | `gone` | `client-migrated` |
| `/fullsearch/search`、`/fullsearch/listFullType*`、`/fullsearch/listFullPolymerization`、`/fullsearch/getBreadCrumbs` | `FullSearchController` | `/api/v1/search/history`、`/api/v1/search/metadata/*` | 已下线旧历史词与元数据 JSON，访问返回 `410 Gone` | `gone` | `client-migrated` |
| `/fullsearch/*Industry`、`/fullsearch/*CaseType`、`/fullsearch/*Type`、`/fullsearch/announcementrtype` | `FullSearchController` | `/api/v1/search/special/{kind}/options` | 已下线旧特殊类型选项 JSON，访问返回 `410 Gone` | `gone` | `client-migrated` |
| `/industry`、`/getevent`、`/getProvinceList`、`/getArticleCityList` | `LSearchController` | `/api/v1/search/metadata/*`、`/api/v1/search/full/facets` | 已下线旧 LSearch 聚合 JSON，访问返回 `410 Gone` | `gone` | `client-migrated` |
| `/publicoption/reportdetail/*` | `PublicOptionContoller` | `/publicoption?id={id}` | 已下线旧详情页入口，访问返回 `410 Gone` | `gone` | `ui-migrated` |
| `/publicoption/backanalysis` | `PublicOptionContoller` | `/publicoption?id={id}&section=backanalysis` | 已下线旧分析页入口，访问返回 `410 Gone` | `gone` | `ui-migrated` |
| `/publicoption/eventContext` | `PublicOptionContoller` | `/publicoption?id={id}&section=eventContext` | 已下线旧分析页入口，访问返回 `410 Gone` | `gone` | `ui-migrated` |
| `/publicoption/eventTrace` | `PublicOptionContoller` | `/publicoption?id={id}&section=eventTrace` | 已下线旧分析页入口，访问返回 `410 Gone` | `gone` | `ui-migrated` |
| `/publicoption/hotAnalysis` | `PublicOptionContoller` | `/publicoption?id={id}&section=hotAnalysis` | 已下线旧分析页入口，访问返回 `410 Gone` | `gone` | `ui-migrated` |
| `/publicoption/netizensAnalysis` | `PublicOptionContoller` | `/publicoption?id={id}&section=netizensAnalysis` | 已下线旧分析页入口，访问返回 `410 Gone` | `gone` | `ui-migrated` |
| `/publicoption/statistics` | `PublicOptionContoller` | `/publicoption?id={id}&section=statistics` | 已下线旧分析页入口，访问返回 `410 Gone` | `gone` | `ui-migrated` |
| `/publicoption/propagationAnalysis` | `PublicOptionContoller` | `/publicoption?id={id}&section=propagationAnalysis` | 已下线旧分析页入口，访问返回 `410 Gone` | `gone` | `ui-migrated` |
| `/publicoption/thematicAnalysis` | `PublicOptionContoller` | `/publicoption?id={id}&section=thematicAnalysis` | 已下线旧分析页入口，访问返回 `410 Gone` | `gone` | `ui-migrated` |
| `/publicoption/unscrambleContent` | `PublicOptionContoller` | `/publicoption?id={id}&section=unscrambleContent` | 已下线旧分析页入口，访问返回 `410 Gone` | `gone` | `ui-migrated` |
| `/publicoption/popular_feelings_analys` | `PublicOptionContoller` | `/publicoption?id={id}&section=popular_feelings_analys` | 已下线旧分析页入口，访问返回 `410 Gone` | `gone` | `ui-migrated` |
| `/publicoption/loadInformation` | `PublicOptionContoller` | `/api/v1/search/full` | 已下线旧任务文章列表 JSON，访问返回 `410 Gone` | `gone` | `client-migrated` |
| `/api/getToken`、`/api/getArticle`、`/api/getMergeArticle`、`/api/detail` | `ApiController` | `/api/v1/auth/login`、`/api/v1/articles*` | 已移除旧开放 API，访问返回 `404 Not Found` | `delete` | `usage-zero` |
| `/monitor/exportarticle` | `MonitorController` | `/articles` | 已移除旧导出入口，访问返回 `404 Not Found` | `delete` | `usage-zero` |
| `/project/*` 旧项目 JSON 入口 | `ProjectController` | `/projects`、`/projects/{id}` | 已移除旧项目管理 JSON，访问返回 `404 Not Found` | `delete` | `usage-zero` |
| `/mail/*` 旧邮件配置入口 | `MailController` | `/system?section=mail` | 已移除旧邮件配置 JSON，访问返回 `404 Not Found` | `delete` | `usage-zero` |
| `/popUp/*` 旧弹窗入口 | `PopUpController` | `/system?section=popup` | 已移除旧弹窗 JSON，访问返回 `404 Not Found` | `delete` | `usage-zero` |
| `/datamonitor/*` 旧文章操作入口 | `DatafavoriteContoller` | `/articles` | 已移除旧收藏、已读、情感、分享、删除入口，访问返回 `404 Not Found` | `delete` | `usage-zero` |
