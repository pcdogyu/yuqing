# NLP / OCR 正式接口

本文档覆盖二期收口后的正式接口：

- `POST /api/v1/nlp/title`
- `POST /api/v1/nlp/summarize`
- `POST /api/v1/nlp/keywords`
- `POST /api/v1/nlp/ocr`
- `POST /api/v1/nlp/image`
- `POST /api/v1/nlp/report-preview`
- `GET /api/v1/nlp/capabilities`

## 鉴权

- 服务直连：当前接口默认允许服务内网直接调用。
- 平台工作台：通过 `portal-web` 读取 `platform_bindings` 中的 `nlp` / `xie` 绑定信息后转发调用。
- 兼容入口：旧 `/platform/nlp/*`、`/platform/xie/*` 入口仍保留，但只作为过渡代理或 SSE 包装层。

说明：

- `secret-id` / `secret-key` 头目前主要用于兼容旧工作台请求形态。
- 正式 API 的推荐调用方式是直接访问 `nlp-service`，不要再依赖 legacy URL 理解能力边界。

## 错误码

`title` / `summarize` / `keywords` / `report-preview` 使用统一 JSON envelope：

```json
{
  "code": 200,
  "message": "ok",
  "data": {}
}
```

`ocr` / `image` 兼容旧返回格式：

```json
{
  "code": 200,
  "msg": "ok",
  "results": {}
}
```

约定错误：

- `400`：请求体无效、缺少 `text`、缺少图片载荷
- `200 + code!=200`：兼容接口透传 legacy 业务失败
- `500`：服务端异常或兼容代理失败

## 调用示例

标题生成：

```powershell
Invoke-RestMethod -Method Post http://127.0.0.1:8085/api/v1/nlp/title `
  -ContentType "application/json" `
  -Body '{"text":"这是需要生成标题的正文"}'
```

报告预览：

```powershell
Invoke-RestMethod -Method Post http://127.0.0.1:8085/api/v1/nlp/report-preview `
  -ContentType "application/json" `
  -Body '{"title":"测试标题","text":"平台工作台报告预览正文。","relatedword":"AI","publish_time":"2026-06-10 12:00:00"}'
```

OCR：

```powershell
$form = @{
  images = Get-Item .\screenshot.png
}
Invoke-RestMethod -Method Post http://127.0.0.1:8085/api/v1/nlp/ocr -Form $form
```

能力清单：

```powershell
Invoke-RestMethod http://127.0.0.1:8085/api/v1/nlp/capabilities
```

## 启停与降级策略

- `nlp-service` 不可用时：
  - `portal-web` 工作台标题生成和报告预览会先尝试正式接口，再回退到本地简化逻辑。
  - 旧 `/platform/xie/report*` SSE 入口仍保留，但流内容优先来自 `report-preview` 正式接口。
- `portal-web` 不可用时：
  - 外部调用方仍可直接访问 `nlp-service` 正式接口。
- 下线顺序：
  1. 新联调路径统一改用 `nlp-service` 正式 API
  2. 旧 `/platform/nlp/*`、`/platform/xie/*` 仅保留过渡兼容
  3. 完成调用方切换后再清理 legacy URL
