package com.jiansutech.yuqing.data

import kotlinx.serialization.builtins.ListSerializer
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
              "Days": [{"Close":"250.00","Return":"+4.17%"}],
              "BestReturn": "+4.17%",
              "Status": "已回测T+1"
            }
        """.trimIndent()

        val row = ApiFactory.json.decodeFromString(AStockBacktestRow.serializer(), payload)

        assertEquals("244.08", row.t0Close)
        assertEquals("+4.17%", row.days.first().returnPct)
    }
}
