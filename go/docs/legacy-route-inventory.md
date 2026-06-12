# Legacy Route Inventory

截至 `2026-06-12`，二期先建立收口基线，不在兼容层继续新增业务逻辑。

## 搜索与详情

| Legacy 入口 | 当前作用 | 正式 Go 替代 | 策略 |
| --- | --- | --- | --- |
| `/fullsearch/*` | 旧全文搜索页面、筛选、详情跳转 | `content-service /api/v1/search/full` + `portal-web /articles?mode=search` | `替换`，逐步降级为跳转 |
| `/timelysearch/*` | 旧即时搜索页面、模板执行、结果页 | `content-service /api/v1/search/timely` + `content-service /api/v1/crawl-templates` + `portal-web` SSR | `替换`，保留必要上下文跳转 |
| `/timelysearch/stream` | legacy SSE 实时抓取回显 | `crawler-service` / 模板执行正式接口 | `保留` 过渡，后续替换 |
| 各类 `*detailData` 旧详情 JSON | 特殊类型详情数据 | `content-service /api/v1/search/details/{id}` | `替换` |

## 平台与 NLP

| Legacy 入口 | 当前作用 | 正式 Go 替代 | 策略 |
| --- | --- | --- | --- |
| `/platform/nlp/ocr` | OCR 兼容 JSON | `nlp-service /api/v1/nlp/ocr` | `替换`，兼容层只做过渡 |
| `/platform/nlp/image` | 图像识别兼容 JSON | `nlp-service /api/v1/nlp/image` | `替换` |
| `/platform/xie/title/*` | 写作标题兼容入口 | `nlp-service /api/v1/nlp/title` | `替换` |
| `/platform/xie/report*` | 写作报告预览 / SSE | `nlp-service /api/v1/nlp/report-preview` + `portal-web` SSE 兼容包装 | `保留` 过渡，默认联调改走正式报告接口 |
| `/platform/notice` | 平台公告旧 JSON | `content-service /api/v1/system/notices` | `替换` |

## 公共舆情与分析

| Legacy 入口 | 当前作用 | 正式 Go 替代 | 策略 |
| --- | --- | --- | --- |
| `/publicoption/loadInformation` | 旧任务文章列表 | `content-service /api/v1/search/full` | `替换` |
| `/publicoption/*analysis*` | 旧分析结果页面/JSON | `analysis-service /api/v1/public-opinion/enrich` | `替换` |
| `/publicoption/reportdetail/*` | 旧详情工作台入口 | `portal-web /publicoption` Go 工作台 | `替换` |

## 兼容页面

| Legacy 入口 | 当前作用 | 正式 Go 替代 | 策略 |
| --- | --- | --- | --- |
| `/mobile/*` | 移动端兼容页 | `portal-web` 兼容 SSR | `保留`，三期评估是否继续长期支持 |
| `/displayboard*` | 大屏兼容页 | `portal-web` 兼容 SSR | `保留` |
| `/volume*` | 声量页兼容入口 | `portal-web` 兼容 SSR | `保留` |
| `/hot/*` | 热点兼容页 | `portal-web` 兼容 SSR | `保留` |
| `/img/code` | 验证码兼容入口 | Go 门户登录链路 | `保留`，直到旧登录页完全下线 |

## 二期执行规则

- 新增能力只落正式接口，不落 legacy handler。
- legacy handler 允许做跳转、转发、兼容返回，不允许新增业务计算。
- 文档、联调、测试默认优先使用正式 Go 接口。
