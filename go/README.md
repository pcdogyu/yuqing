# Jin10 Go Crawler

这个目录是新增的独立 Go 子系统，用于采集金十公开数据源并写入 SQLite。

当前覆盖：

- `https://www.jin10.com/` 快讯
- `https://xnews.jin10.com/` 头条

## 目录

- `cmd/server`: REST API
- `cmd/worker`: 常驻采集 worker
- `internal/provider/jin10flash`: 快讯抓取与解析
- `internal/provider/jin10xnews`: 头条抓取与解析
- `internal/store/sqlite`: SQLite 持久化
- `internal/service`: 抓取编排与去重
- `internal/httpapi`: HTTP 接口

## 运行要求

- Go `1.22+`

## 环境变量

- `JIN10_LISTEN_ADDR`: API 监听地址，默认 `:8090`
- `JIN10_DB_PATH`: SQLite 路径，默认 `data/jin10.db`
- `JIN10_FLASH_URL`: 快讯页地址，默认 `https://www.jin10.com/`
- `JIN10_HEADLINE_URL`: 头条页地址，默认 `https://xnews.jin10.com/`
- `JIN10_HTTP_TIMEOUT_SEC`: HTTP 超时秒数，默认 `20`
- `JIN10_FLASH_INTERVAL_SEC`: 快讯轮询间隔，默认 `15`
- `JIN10_HEADLINE_INTERVAL_SEC`: 头条轮询间隔，默认 `60`
- `JIN10_LOG_LEVEL`: `debug|info|warn|error`

## 启动

```bash
cd go
go run ./cmd/server
```

```bash
cd go
go run ./cmd/worker
```

## 常用接口

```bash
curl http://127.0.0.1:8090/healthz
curl "http://127.0.0.1:8090/api/v1/items?page=1&page_size=20&source_type=headline"
curl "http://127.0.0.1:8090/api/v1/items/latest?limit=5&source_type=flash"
curl -X POST "http://127.0.0.1:8090/api/v1/crawl/run?source_type=flash"
curl http://127.0.0.1:8090/api/v1/crawl/runs
```

## 说明

- 第一阶段只采集公开内容，不处理登录态和 VIP 解锁。
- 快讯优先解析首页嵌入 `data-list`，找不到时降级到 HTML 链接兜底。
- 头条当前采集公开列表，不做深分页。
