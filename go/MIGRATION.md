# Java -> Go 模块映射

## 一期已承接

- `LoginController` -> `auth-service` + `gateway-web`
- `ProjectController` -> `content-service / projects + project-groups`
- `MonitorController` -> `content-service / monitor-rules + articles`
- `ReportController` -> `content-service / reports`
- `AnalysisController` -> `analysis-service`
- `ApiController` 的基础文章查询能力 -> `content-service / articles + search`
- `SystemController` 的公告/反馈/任务记录子集 -> `content-service / system-*`

## 暂不承接

- `FullSearchController`
- `TimelySearchController`
- `WechatController`
- `PlatformController`
- `MailController`
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
