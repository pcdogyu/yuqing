# Stonedt Go Platform

Go 版已经从早期 `jin10` 采集骨架收敛为一套可运行的一期多服务系统，核心范围包括：

- `gateway-web`：SSR 门户
- `auth-service`：登录、会话、API Token
- `content-service`：项目组、项目、监测规则、文章、报告、公告、反馈、任务记录
- `crawler-service`：抓取执行与抓取运行记录
- `analysis-service`：总览、趋势、来源分布、关键词热点、分析刷新
- `scheduler-service`：定时触发抓取和分析刷新
- `nlp-service`：轻量标题/摘要/关键词生成

二期已经开始推进，重点是把剩余 Java 兼容入口和未迁移功能继续收口到 Go；旧兼容路由会逐步下线，以 Go 正式接口为准。

## Crypto 社媒接入

系统已支持 `crypto_x` 和 `crypto_telegram` 两个外部社媒抓取源。你可以接自己的代理层，也可以先用仓库内 mock 服务联调。

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

对接字段和返回格式说明见 [docs/crypto-social-proxy.md](docs/crypto-social-proxy.md)。

## 默认端口

- `gateway-web`: `80`
- `auth-service`: `8081`
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
go run .\cmd\content-service
go run .\cmd\crawler-service
go run .\cmd\analysis-service
go run .\cmd\nlp-service
go run .\cmd\gateway-web
go run .\cmd\scheduler-service
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
- `GET /api/v1/analysis/overview`
- `GET /api/v1/analysis/trends`
- `GET /api/v1/analysis/sources`
- `GET|POST /api/v1/reports`
- `GET /api/v1/reports/{id}`
- `GET /api/v1/system/notices`
- `POST /api/v1/system/feedback`
- `GET /api/v1/system/task-runs`
- `POST /api/v1/admin/tasks/crawl`
- `GET /api/v1/admin/tasks/crawl/runs`
- `POST /api/v1/admin/tasks/analysis/refresh`
- `GET /api/v1/crypto/social`
- `GET /api/v1/crypto/insights`

## 已完成的一期范围

- 账号登录、会话守卫、API Token
- 项目组/项目/监测规则的基础 CRUD API
- 文章列表、详情、相关文章、FTS 搜索
- 按监测规则把抓取结果关联到项目
- 总览、趋势、来源分布、关键词热点
- 报告生成与详情查看
- 系统公告、反馈、任务记录页面
- Windows 启动脚本和多服务入口

## 暂未迁移

- 微信登录/绑定
- OCR 与外部平台集成
- 邮件配置
- 大屏和移动端
- Java 全量高级全文检索
- 复杂传播/情感/专题分析
