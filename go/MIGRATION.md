# Java -> Go 模块映射

## 一期已承接

- `LoginController` -> `auth-service` + `gateway-web`
- `ProjectController` -> `content-service / projects + project-groups`
- `MonitorController` -> `content-service / monitor-rules + articles`
- `ReportController` -> `content-service / reports`
- `AnalysisController` -> `analysis-service`
- `ApiController` 的基础文章查询能力 -> `content-service / articles + search`
- `SearchController` -> `gateway-web /search` 兼容跳转 + `gateway-web /articles?mode=search`
- `SystemController` 的公告/反馈/任务记录/偏好/弹窗/邮件/预警子集 -> `content-service / system-*` + `gateway-web /system`

## 部分承接

- `FullSearchController` -> `gateway-web /fullsearch` 兼容跳转 + `gateway-web /articles?mode=full` + `content-service /api/v1/search/full`
- `TimelySearchController` -> `gateway-web /timelysearch` 兼容跳转 + `gateway-web /articles?mode=timely` + `content-service /api/v1/search/timely`
- `LSearchController` -> `gateway-web` legacy compatibility endpoints `/industry` `/getevent` `/getProvinceList` `/getArticleCityList`
- `PlatformController` -> 平台设置/通知/绑定能力已部分落到 `content-service /system-*` 与 `gateway-web /system`
- `MailController` -> 邮件配置已落到 `content-service /system/mail-config` 与 `gateway-web /system`
- `PopUpController` -> 弹窗状态已落到 `content-service /system/popup` 与 `gateway-web /system`
- `PublicOptionContoller` -> 话题/偏好/系统配置的主要闭环已在 `content-service` 与 `gateway-web /system`
- `DatafavoriteContoller` -> 已读/收藏/分享的文章态能力已落到 `content-service /articles`，但老接口未完全对齐
- `UserController` -> 用户资料/偏好已落到 `content-service /system/preferences` 与 `gateway-web /system`

## 暂不承接

- `WechatController`
- `MobileController`
- `DisplayBoardController`
- `VolumeController`
- `HotNewsController`
- `UserAuthController`

## 当前 Go 服务职责

- `gateway-web`
  - 门户页面和会话跳转
- `auth-service`
  - 登录、登出、session、token
- `content-service`
  - 业务主数据与后台接口
- `crawler-service`
  - 采集和抓取运行记录
- `analysis-service`
  - 聚合分析和快照刷新
- `scheduler-service`
  - 定时触发抓取和分析任务
- `nlp-service`
  - 报告标题/摘要/关键词生成
