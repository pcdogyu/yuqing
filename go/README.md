# Stonedt Go Platform

这个目录现在是 Go 版舆情门户的主实现，不再只是单独的金十采集子系统。当前版本已经具备一套可运行的多服务骨架：

- `portal-web`
  Go SSR 门户，提供登录页、总览、项目中心、文章中心、报告中心。
- `content-service`
  承接 `/api/v1/auth/*`、项目、监测、文章、搜索、分析、报告、公告等 API。
- `crawler-service`
  承接采集任务与文章最新数据接口，当前已接入金十快讯和头条。
- `nlp-service`
  提供摘要、标题、关键词的本地化轻量 NLP 接口。
- `scheduler-worker`
  定时触发采集和分析快照刷新。

## 目录

- `cmd/portal-web`
- `cmd/content-service`
- `cmd/crawler-service`
- `cmd/nlp-service`
- `cmd/scheduler-worker`
- `internal/store/sqlite`
- `internal/provider/jin10flash`
- `internal/provider/jin10xnews`
- `openapi.yaml`

## 数据与运行

- 主数据库：SQLite，默认 `go/data/jin10.db`
- 检索：SQLite FTS5
- 默认管理员：`admin / admin123`
- 服务默认端口：
  - `portal-web`: `8080`
  - `content-service`: `8081`
  - `crawler-service`: `8082`
  - `nlp-service`: `8083`

## 启动

```bash
cd go
go run ./cmd/content-service
```

```bash
cd go
go run ./cmd/crawler-service
```

```bash
cd go
go run ./cmd/nlp-service
```

```bash
cd go
go run ./cmd/portal-web
```

```bash
cd go
go run ./cmd/scheduler-worker
```

## 已实现能力

- 账号密码登录、会话校验、API token 发放
- 项目创建与项目列表
- 监测规则列表与创建
- 金十头条/快讯采集、采集运行记录、文章最新接口
- 文章分页、关键词过滤、SQLite FTS 搜索
- 总览分析快照刷新
- 报告生成与报告列表
- Go SSR 门户页面
- OpenAPI 文档骨架

## 验证命令

```bash
go test ./...
```

```bash
curl http://127.0.0.1:8081/healthz
curl -X POST http://127.0.0.1:8081/api/v1/auth/login -H "Content-Type: application/json" -d "{\"username\":\"admin\",\"password\":\"admin123\"}"
curl -X POST "http://127.0.0.1:8082/api/v1/admin/tasks/crawl?source_type=headline" -H "X-Service-Token: stonedt-internal-token"
```

## 当前边界

- 已经完成全量 Go 化的主架构切换和核心骨架，不再依赖 Java 运行。
- 业务上仍是首版重构，不等价覆盖原 Java 的全部 269 条路由和全部页面细节。
- 微信扫码、OCR、复杂 AI 写作、原大屏视觉和高级舆情分析仍需继续补齐。
