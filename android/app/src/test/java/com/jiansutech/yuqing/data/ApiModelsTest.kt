package com.jiansutech.yuqing.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class ApiModelsTest {
    @Test
    fun normalizeBaseUrlAddsTrailingSlash() {
        assertEquals("http://127.0.0.1:8082/", ApiFactory.normalizeBaseUrl("http://127.0.0.1:8082"))
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
}
