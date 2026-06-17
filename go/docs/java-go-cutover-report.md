# Java-Go 全量收口对账报告

生成日期：2026-06-17
对账基线：`3fd5f8f8c43fd14219cccf0fce45ce80581dfe21`

## 结论

当前仓库的线上运行链路已经收口为 Go 多服务，Java 旧入口不再作为业务兼容面保留。`src/main/java`、根 `pom.xml`、`mvnw`、`mvnw.cmd` 和 `.mvn` 已从活跃源码树退场；启动、调度、门户、API、审计、备份恢复和发布验收均以 `go/` 下的 Go 服务为准。

Go 侧 legacy 注册表保持收口基线：

| 策略 | 数量 |
| --- | ---: |
| `proxy` | 0 |
| `preserve` | 0 |
| `redirect` | 0 |
| `gone` | 76 |
| `delete` | 40 |

## Java 入口盘点

退场前静态盘点结果：

| 类别 | 数量 | 退场判定 |
| --- | ---: | --- |
| Java 源文件 | 247 | 已从活跃源码树移除 |
| Controller/API 文件 | 24 | 已由 Go 正式入口、`410 Gone` 或 `404 Not Found` 覆盖 |
| Spring 路由注解 | 269 | 不再继续保留 Java 旧 URL 的业务兼容 |
| Quartz/Job 相关文件 | 16 | 已由 `scheduler-service` 等价调度承接 |
| AOP/Aspect 文件 | 4 | 已由 Go HTTP 审计、任务记录和 operations 页面承接 |

## 模块判定

| Java 模块 / 能力 | 最终状态 | Go 承接或下线策略 |
| --- | --- | --- |
| Login/Auth | Go 正式承接 | `auth-service`、`gateway-web` 登录会话和 API Token |
| Project/Monitor/Article/Report | Go 正式承接 | `content-service` 与 Go 门户页面 |
| Search/FullSearch/TimelySearch/LSearch | Go 正式承接 | `/articles`、`/api/v1/search/*`；旧入口 `410 Gone` |
| PublicOption/复杂分析 | Go 正式承接 | `/publicoption/*`、`analysis-service /api/v1/public-opinion/*`；旧分析入口 `410 Gone` |
| Platform/NLP/OCR | Go 正式承接 | `/platform/bindings`、`nlp-service /api/v1/nlp/*`；旧 `/platform/nlp/*`、`/platform/xie/*` 为 `410 Gone` |
| Wechat | Go 正式承接 | `wechat-service /api/v1/wechat/*` 与 Go 门户绑定链路 |
| Mail/PopUp/User | Go 正式承接 | `content-service` 系统配置、弹窗、偏好能力 |
| Mobile/DisplayBoard/Volume/Hot/Dist/Image | 已下线 | 对应旧入口保持 `410 Gone` 或 `404 Not Found` |
| Quartz 调度 | Go 等价调度 | `scheduler-service` job registry、cron 覆盖和 `task_runs` |
| AOP/系统日志 | Go 等价审计 | HTTP audit middleware、`audit_logs`、`/system?section=operations` |
| Service/DAO/Entity/Mapper | 无需保留 Java 运行时 | Go store/service 层和 PostgreSQL/SQLite schema 承接业务数据 |

## 保留项说明

- `src/main/resources` 已随根目录 Go 版清理移除；历史 MyBatis XML、Flyway SQL 和旧静态资源不再保留在活跃源码树中。
- `docs/legacy-route-inventory.md` 继续作为旧入口 `410/404` 基线。
- Go 发布验收入口仍是 `go/scripts/release-check.ps1` 或等价健康检查。

## 验收口径

1. `src/main/java`、根 `pom.xml`、`mvnw`、`mvnw.cmd`、`.mvn` 不再存在于活跃树。
2. `go test -count=1 ./internal/portal` 通过。
3. `go test -count=1 ./internal/scheduler` 通过。
4. `go test -count=1 ./...` 通过。
5. legacy 注册表保持 `proxy=0`、`preserve=0`、`redirect=0`。
