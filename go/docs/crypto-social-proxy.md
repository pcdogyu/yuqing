# Crypto Social Proxy 接入说明

本文档约束 `crypto_x` 与 `crypto_telegram` 两个抓取源的代理返回格式，方便外部聚合服务、内部网关或 mock 服务直接接入当前 Go 系统。

## 配置项

在运行 `worker`、`server` 或 `crawler-service` 前配置：

```powershell
$env:YUQING_CRYPTO_X_URL = "http://127.0.0.1:19090/mock/x"
$env:YUQING_CRYPTO_X_TOKEN = "optional-token"
$env:YUQING_CRYPTO_X_INTERVAL_SEC = "90"

$env:YUQING_CRYPTO_TELEGRAM_URL = "http://127.0.0.1:19090/mock/telegram"
$env:YUQING_CRYPTO_TELEGRAM_TOKEN = "optional-token"
$env:YUQING_CRYPTO_TELEGRAM_INTERVAL_SEC = "90"
```

未配置 URL 时，对应抓取源不会启动。

## 请求约定

- 方法：`GET`
- Header：
  - `Authorization: Bearer <token>`，仅当配置了 token 才会发送
  - `User-Agent` 复用系统全局抓取 UA
- 返回状态：`200 OK`
- 返回编码：`application/json; charset=utf-8`

## 支持的返回结构

当前 provider 支持两类 JSON：

1. 直接数组

```json
[
  {
    "url": "https://x.com/alpha/status/1",
    "content": "BTC ETF approval chatter grows",
    "author": "@alpha",
    "created_at": "2026-06-04T10:00:00Z"
  }
]
```

2. 包装对象

```json
{
  "items": [
    {
      "url": "https://t.me/channel/1",
      "message": "ETF inflow discussion",
      "channel": "tg-alpha",
      "created_at": "2026-06-04 10:05:00"
    }
  ]
}
```

外层对象支持以下字段名之一：

- `items`
- `data`
- `results`
- `posts`
- `messages`

## 单条记录字段映射

系统会按顺序尝试以下字段：

- 正文：`content` / `text` / `body` / `message` / `full_text` / `description`
- 标题：`title` / `headline` / `subject`
- 链接：`url` / `link` / `source_url` / `detail_url` / `permalink` / `tweet_url` / `message_url`
- 作者：`author` / `username` / `screen_name` / `channel` / `from` / `account`
- 时间：`publish_time` / `posted_at` / `created_at` / `timestamp` / `date`
- 摘要：`summary` / `excerpt` / `snippet`
- 标签：`tags` / `symbols` / `tickers`

布尔字段支持：

- `has_image`
- `image`
- `media`
- `photo`

## 时间格式

推荐直接返回 RFC3339：

```text
2026-06-04T10:00:00Z
```

当前也兼容：

- `2006-01-02 15:04:05`
- `2006-01-02 15:04`
- Unix 秒时间戳
- Unix 毫秒时间戳

## 入库结果

归一化后数据写入现有 `items` 表：

- `source_type = crypto_x` 或 `crypto_telegram`
- `from_text` 存作者/频道
- `external_source_host` 存外部域名
- `raw_payload` 存原始 JSON

系统按 `detail_url/source_url + publish_time + title + content` 生成去重键，同一帖重复抓取会落成更新，不会无限插入。

## 推荐返回字段

建议代理至少返回：

- `url`
- `content`
- `author`
- `created_at`
- `tags`

如果能补充：

- `summary`
- `has_image`
- `title`

则后续 `/crypto` 页面上的社媒证据、热度排序和摘要展示会更稳定。

## 五期三批韧性配置

crypto social provider 复用全局外部 HTTP 超时和重试策略：

- `YUQING_HTTP_TIMEOUT_SEC`
- `YUQING_EXTERNAL_RETRY_COUNT`
- `YUQING_EXTERNAL_RETRY_WAIT_MS`
- `YUQING_CRYPTO_SOCIAL_RATE_LIMIT_MS`

重试只覆盖超时、HTTP `429` 和 `5xx`。禁用端点、非 200、坏 JSON、空数据会返回固定错误分类；重复数据会去重并记录 `external_duplicate_data` 结构化日志。最近抓取状态、抓取数、入库数和错误摘要会通过 `crawl_runs` 进入 operations 视图。
