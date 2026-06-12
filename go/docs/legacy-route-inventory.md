# Legacy Route Inventory

截至 `2026-06-12`，三期开始按“代码注册表 + 文档执行清单”统一管理 legacy 路由。新增兼容入口必须先补注册表与本清单；禁止继续新增匿名 legacy handler。

## 执行规则

- `strategy` 只允许：`preserve`、`proxy`、`redirect`、`gone`、`delete`
- `removal_gate` 只允许：`ui-migrated`、`client-migrated`、`usage-zero`、`external-contract`
- 页面入口优先 `302`
- 已过渡完成且仍被访问的 API 优先 `410`
- 确认无调用方的入口直接删除

## 存活 legacy 路由

| legacy_path | owner_module | formal_target | current_behavior | strategy | removal_gate |
| --- | --- | --- | --- | --- | --- |
| `/fullsearch/*` | `FullSearchController` | `/articles?mode=full` | 旧全文搜索结果页与特殊详情页入口已改为跳转；筛选、类型、详情数据等兼容 JSON 仍保留 | `redirect` | `ui-migrated` |
| `/timelysearch/*` | `TimelySearchController` | `/articles?mode=timely` | 旧即时搜索页面、结果页与兼容数据流 | `proxy` | `client-migrated` |
| `/industry` | `LSearchController` | `/api/v1/search/metadata/types` | 行业聚合兼容 JSON | `proxy` | `client-migrated` |
| `/getevent` | `LSearchController` | `/api/v1/search/metadata/breadcrumbs` | 事件聚合兼容 JSON | `proxy` | `client-migrated` |
| `/getProvinceList` | `LSearchController` | `/api/v1/search/full/facets` | 省份聚合兼容 JSON | `proxy` | `client-migrated` |
| `/getArticleCityList` | `LSearchController` | `/api/v1/search/full/facets` | 城市聚合兼容 JSON | `proxy` | `client-migrated` |
| `/publicoption/reportdetail/*` | `PublicOptionContoller` | `/publicoption?id={id}` | 旧详情页入口，现仅保留跳转 | `redirect` | `ui-migrated` |
| `/publicoption/backanalysis` | `PublicOptionContoller` | `/publicoption?id={id}&section=backanalysis` | 旧分析页入口，现仅保留跳转 | `redirect` | `ui-migrated` |
| `/publicoption/eventContext` | `PublicOptionContoller` | `/publicoption?id={id}&section=eventContext` | 旧分析页入口，现仅保留跳转 | `redirect` | `ui-migrated` |
| `/publicoption/eventTrace` | `PublicOptionContoller` | `/publicoption?id={id}&section=eventTrace` | 旧分析页入口，现仅保留跳转 | `redirect` | `ui-migrated` |
| `/publicoption/hotAnalysis` | `PublicOptionContoller` | `/publicoption?id={id}&section=hotAnalysis` | 旧分析页入口，现仅保留跳转 | `redirect` | `ui-migrated` |
| `/publicoption/netizensAnalysis` | `PublicOptionContoller` | `/publicoption?id={id}&section=netizensAnalysis` | 旧分析页入口，现仅保留跳转 | `redirect` | `ui-migrated` |
| `/publicoption/statistics` | `PublicOptionContoller` | `/publicoption?id={id}&section=statistics` | 旧分析页入口，现仅保留跳转 | `redirect` | `ui-migrated` |
| `/publicoption/propagationAnalysis` | `PublicOptionContoller` | `/publicoption?id={id}&section=propagationAnalysis` | 旧分析页入口，现仅保留跳转 | `redirect` | `ui-migrated` |
| `/publicoption/thematicAnalysis` | `PublicOptionContoller` | `/publicoption?id={id}&section=thematicAnalysis` | 旧分析页入口，现仅保留跳转 | `redirect` | `ui-migrated` |
| `/publicoption/unscrambleContent` | `PublicOptionContoller` | `/publicoption?id={id}&section=unscrambleContent` | 旧分析页入口，现仅保留跳转 | `redirect` | `ui-migrated` |
| `/publicoption/popular_feelings_analys` | `PublicOptionContoller` | `/publicoption?id={id}&section=popular_feelings_analys` | 旧分析页入口，现仅保留跳转 | `redirect` | `ui-migrated` |
| `/publicoption/loadInformation` | `PublicOptionContoller` | `/api/v1/search/full` | 旧任务文章列表 JSON | `proxy` | `client-migrated` |
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
