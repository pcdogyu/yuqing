package com.jiansutech.yuqing.data

import kotlinx.serialization.builtins.ListSerializer
import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ApiModelsTest {
    @Test
    fun normalizeBaseUrlAddsTrailingSlash() {
        assertEquals("http://127.0.0.1:8082/", ApiFactory.normalizeBaseUrl("http://127.0.0.1:8082"))
    }

    @Test
    fun normalizeBaseUrlUsesFallbackForBlankValue() {
        assertEquals(
            "http://yuqin.jiansutech.com:8081/",
            ApiFactory.normalizeAuthBaseUrl("", "http://yuqin.jiansutech.com:8081/"),
        )
    }

    @Test
    fun normalizeBaseUrlAddsFallbackPortForBareHost() {
        assertEquals(
            "http://yuqin.jiansutech.com:8081/",
            ApiFactory.normalizeAuthBaseUrl("yuqin.jiansutech.com", "http://yuqin.jiansutech.com:8081/"),
        )
        assertEquals(
            "http://yuqin.jiansutech.com:8082/",
            ApiFactory.normalizeApiBaseUrl("yuqin.jiansutech.com", "http://yuqin.jiansutech.com:8082/"),
        )
    }

    @Test
    fun normalizeBaseUrlAddsFallbackPortWhenSchemeIsProvidedWithoutPort() {
        assertEquals(
            "http://yuqin.jiansutech.com:8082/",
            ApiFactory.normalizeApiBaseUrl("http://yuqin.jiansutech.com", "http://yuqin.jiansutech.com:8082/"),
        )
    }

    @Test
    fun normalizeBaseUrlKeepsExplicitPort() {
        assertEquals(
            "http://yuqin.jiansutech.com:18082/",
            ApiFactory.normalizeApiBaseUrl("http://yuqin.jiansutech.com:18082", "http://yuqin.jiansutech.com:8082/"),
        )
    }

    @Test
    fun articlesRequestDefaultsToRealtimeSync() = runTest {
        val server = MockWebServer()
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "application/json")
                .setBody("""{"code":200,"message":"ok","data":{"items":[],"page":1,"page_size":25,"total":0}}"""),
        )
        server.start()
        try {
            val result = ApiFactory.yuqing(server.url("/").toString())
                .articles(page = 1)
                .data
            val request = server.takeRequest()

            assertEquals(0, result?.total)
            assertEquals("/api/v1/articles", request.requestUrl?.encodedPath)
            assertEquals("25", request.requestUrl?.queryParameter("page_size"))
            assertEquals("captured_at", request.requestUrl?.queryParameter("time_field"))
            assertEquals("captured_at_desc", request.requestUrl?.queryParameter("sort"))
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun articleDetailRequestUsesArticleIdPath() = runTest {
        val server = MockWebServer()
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "application/json")
                .setBody(
                    """
                    {
                      "code": 200,
                      "message": "ok",
                      "data": {
                        "id": 42,
                        "title": "detail title",
                        "content": "full body",
                        "source_type": "flash",
                        "captured_at": "2026-06-24T08:30:01Z"
                      }
                    }
                    """.trimIndent(),
                ),
        )
        server.start()
        try {
            val result = ApiFactory.yuqing(server.url("/").toString())
                .article(42)
                .data
            val request = server.takeRequest()

            assertEquals("/api/v1/articles/42", request.requestUrl?.encodedPath)
            assertEquals("detail title", result?.title)
            assertEquals("full body", result?.content)
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun aStockAuctionRequestSendsTrendDays() = runTest {
        val server = MockWebServer()
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "application/json")
                .setBody("""{"code":200,"message":"ok","data":{"items":[],"page":1,"page_size":6000,"total":0,"date":"2026-07-09","trend":[]}}"""),
        )
        server.start()
        try {
            val result = ApiFactory.yuqing(server.url("/").toString())
                .aStockAuction(date = "2026-07-09", page = 1, pageSize = 6000, trendDays = 30)
                .data
            val request = server.takeRequest()

            assertEquals("2026-07-09", result?.date)
            assertEquals("/api/v1/a-stock/auction", request.requestUrl?.encodedPath)
            assertEquals("2026-07-09", request.requestUrl?.queryParameter("date"))
            assertEquals("6000", request.requestUrl?.queryParameter("page_size"))
            assertEquals("30", request.requestUrl?.queryParameter("trend_days"))
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun aStockRecommendationRequestSendsDefaultFilterState() = runTest {
        val server = MockWebServer()
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "application/json")
                .setBody("""{"code":200,"message":"ok","data":{"found":true,"strategy_date":"2026-07-10","period":"morning","recommendations_json":"[]","backtests_json":"[]"}}"""),
        )
        server.start()
        try {
            ApiFactory.yuqing(server.url("/").toString())
                .aStockRecommendations(date = "2026-07-10", period = "morning")
                .data
            val request = server.takeRequest()

            assertEquals("/api/v1/a-stock/recommendations", request.requestUrl?.encodedPath)
            assertEquals("2026-07-10", request.requestUrl?.queryParameter("date"))
            assertEquals("morning", request.requestUrl?.queryParameter("period"))
            assertEquals("false", request.requestUrl?.queryParameter("ignore_recent"))
            assertEquals("false", request.requestUrl?.queryParameter("limit_up_filter_enabled"))
            assertEquals("false", request.requestUrl?.queryParameter("today_market_filter_enabled"))
            assertEquals("true", request.requestUrl?.queryParameter("fund_flow_filter_enabled"))
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun dashboardSerializationKeepsPortalCounts() {
        val dashboard = AndroidDashboard(
            overview = Overview(articleCount = 3, projectCount = 2, reportCount = 1),
            projects = listOf(Project(id = 1, name = "project")),
            articles = ItemListResult(items = listOf(ArticleItem(id = 9, title = "headline")), total = 1),
        )

        val payload = ApiFactory.json.encodeToString(AndroidDashboard.serializer(), dashboard)
        val decoded = ApiFactory.json.decodeFromString(AndroidDashboard.serializer(), payload)

        assertEquals(3, decoded.overview.articleCount)
        assertEquals(2, decoded.overview.projectCount)
        assertEquals("headline", decoded.articles.items.first().title)
        assertTrue(payload.contains("article_count"))
    }

    @Test
    fun aStockRecommendationSnapshotParsesStoredRecommendationJson() {
        val payload = """
            {
              "found": true,
              "strategy_date": "2026-06-22",
              "period": "afternoon",
              "recommendations_json": "[{\"Rank\":1,\"Hotspot\":\"金融券商\",\"Code\":\"600000\",\"Name\":\"浦发银行\",\"MarketScore\":238,\"CurrentPrice\":\"10.20\",\"TodayPct\":\"+2.1%\",\"Reason\":\"命中券商，综合分238\"}]",
              "backtest_status": "已回测 1/1"
            }
        """.trimIndent()

        val snapshot = ApiFactory.json.decodeFromString(AStockRecommendationSnapshot.serializer(), payload)
        val recommendations = ApiFactory.json.decodeFromString(
            ListSerializer(AStockRecommendation.serializer()),
            snapshot.recommendationsJson,
        )

        assertTrue(snapshot.found)
        assertEquals("afternoon", snapshot.period)
        assertEquals("600000", recommendations.first().code)
        assertEquals(238, recommendations.first().marketScore)
    }

    @Test
    fun aStockBacktestRowParsesT0Close() {
        val payload = """
            {
              "Stock": "603083 剑桥科技",
              "EntryOpen": "240.00",
              "T0Return": "+1.70%",
              "T0Close": "244.08",
              "CurrentPrice": "245.00",
              "CurrentReturn": "+2.08%",
              "CurrentReturnClass": "astock-up",
              "Days": [{"Close":"250.00","Return":"+4.17%"}],
              "BestReturn": "+4.17%",
              "Status": "已回测T+1"
            }
        """.trimIndent()

        val row = ApiFactory.json.decodeFromString(AStockBacktestRow.serializer(), payload)

        assertEquals("244.08", row.t0Close)
        assertEquals("245.00", row.currentPrice)
        assertEquals("+2.08%", row.currentReturn)
        assertEquals("astock-up", row.currentReturnClass)
        assertEquals("+4.17%", row.days.first().returnPct)
    }

    @Test
    fun aStockBacktestRefreshPricePostsJsonBody() = runTest {
        val server = MockWebServer()
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "application/json")
                .setBody(
                    """
                    {
                      "code": 200,
                      "message": "ok",
                      "data": {
                        "summary": "价格已刷新",
                        "detail": "明细",
                        "snapshot": {
                          "found": true,
                          "strategy_date": "2026-07-10",
                          "period": "morning",
                          "recommendations_json": "[]",
                          "backtests_json": "[]"
                        }
                      }
                    }
                    """.trimIndent(),
                ),
        )
        server.start()
        try {
            val result = ApiFactory.yuqing(server.url("/").toString())
                .refreshAStockBacktestPrice(
                    AStockBacktestPriceRefreshRequest(
                        date = "2026-07-10",
                        period = "morning",
                        code = "300394",
                    ),
                )
                .data
            val request = server.takeRequest()

            assertEquals("/api/v1/a-stock/backtests/refresh-price", request.requestUrl?.encodedPath)
            assertEquals("POST", request.method)
            assertTrue(request.body.readUtf8().contains("\"code\":\"300394\""))
            assertEquals("价格已刷新", result?.summary)
            assertEquals("morning", result?.snapshot?.period)
        } finally {
            server.shutdown()
        }
    }

    @Test
    fun stockResearchParsesStoredSourceText() {
        val payload = """
            {
              "id": 7,
              "title": "研报标题",
              "source_text": "第一段\n\n第二段",
              "source_fetch_status": "parsed",
              "source_fetched_at": "2026-07-08T01:00:00Z"
            }
        """.trimIndent()

        val item = ApiFactory.json.decodeFromString(StockResearch.serializer(), payload)

        assertEquals("第一段\n\n第二段", item.sourceText)
        assertEquals("parsed", item.sourceFetchStatus)
        assertEquals("2026-07-08T01:00:00Z", item.sourceFetchedAt)
    }

	@Test
	fun stockResearchPdfFileNameUsesIdAndSanitizesLabel() {
		val fileName = stockResearchPdfFileName(
			StockResearch(id = 7, code = "002497", name = "雅化/集团:*?"),
		)

		assertEquals("7-002497-雅化-集团.pdf", fileName)
	}

	@Test
	fun releasePackageMetadataParsesLatestResponse() {
        val payload = """
            {
              "version_name": "20260625-120000-abcdef12",
              "version_code": 123,
              "file_name": "yuqing-20260625-120000-abcdef12-release.apk",
              "download_url": "http://yuqin.jiansutech.com:8099/release/yuqing-20260625-120000-abcdef12-release.apk",
              "size_bytes": 12345678,
              "sha256": "abc123",
              "modified_at": "2026-06-25T04:00:00Z"
            }
        """.trimIndent()

        val release = ApiFactory.json.decodeFromString(ReleasePackage.serializer(), payload)

        assertEquals("20260625-120000-abcdef12", release.versionName)
        assertEquals(123, release.versionCode)
        assertEquals("yuqing-20260625-120000-abcdef12-release.apk", release.fileName)
        assertEquals("abc123", release.sha256)
    }
}
