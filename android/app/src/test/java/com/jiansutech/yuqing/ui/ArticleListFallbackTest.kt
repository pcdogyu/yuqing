package com.jiansutech.yuqing.ui

import com.jiansutech.yuqing.data.AndroidDashboard
import com.jiansutech.yuqing.data.ArticleItem
import com.jiansutech.yuqing.data.ItemListResult
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.time.Instant

class ArticleListFallbackTest {
    @Test
    fun fallbackKeepsCurrentArticleListFirst() {
        val current = ItemListResult(items = listOf(ArticleItem(id = 1, title = "current")))
        val dashboard = AndroidDashboard(
            articles = ItemListResult(items = listOf(ArticleItem(id = 2, title = "dashboard"))),
        )

        val fallback = fallbackArticleList(YuqingUiState(articleList = current, dashboard = dashboard))

        assertEquals("current", fallback?.items?.first()?.title)
    }

    @Test
    fun fallbackUsesDashboardArticlesWhenCurrentListIsEmpty() {
        val dashboard = AndroidDashboard(
            articles = ItemListResult(items = listOf(ArticleItem(id = 2, title = "dashboard"))),
        )

        val fallback = fallbackArticleList(YuqingUiState(articleList = null, dashboard = dashboard))

        assertEquals("dashboard", fallback?.items?.first()?.title)
    }

    @Test
    fun fallbackReturnsNullWhenNoArticlesExist() {
        val fallback = fallbackArticleList(YuqingUiState())

        assertNull(fallback)
    }

    @Test
    fun filterHiddenArticlesDropsMatchingIds() {
        val result = ItemListResult(
            items = listOf(
                ArticleItem(id = 1, title = "keep"),
                ArticleItem(id = 2, title = "hide"),
                ArticleItem(id = 3, title = "also keep"),
            ),
            total = 3,
        )

        val visible = filterHiddenArticles(result, setOf(2))

        assertEquals(listOf("keep", "also keep"), visible?.items?.map { it.title })
        assertEquals(3, visible?.total)
    }

    @Test
    fun articleStableKeyPrefersArticleId() {
        val key = articleStableKey(
            ArticleItem(
                id = 42,
                sourceUrl = "https://example.test/a",
                capturedAt = "2026-06-24T09:00:00Z",
                title = "headline",
            ),
        )

        assertEquals("id:42", key)
    }

    @Test
    fun articleStableKeyFallsBackToSourceTimeAndTitle() {
        val key = articleStableKey(
            ArticleItem(
                sourceUrl = "https://example.test/a",
                capturedAt = "2026-06-24T09:00:00Z",
                title = "headline",
            ),
        )

        assertEquals("https://example.test/a|2026-06-24T09:00:00Z|headline", key)
    }

    @Test
    fun mergeDashboardArticlesFiltersHiddenDedupesAndSorts() {
        val current = listOf(
            ArticleItem(id = 1, title = "current old", capturedAt = "2026-06-24T09:00:00Z"),
            ArticleItem(id = 2, title = "hidden", capturedAt = "2026-06-24T09:20:00Z"),
        )
        val incoming = listOf(
            ArticleItem(id = 1, title = "duplicate", capturedAt = "2026-06-24T09:10:00Z"),
            ArticleItem(id = 3, title = "incoming newest", capturedAt = "2026-06-24T09:30:00Z"),
            ArticleItem(id = 4, title = "incoming middle", capturedAt = "2026-06-24T09:15:00Z"),
        )

        val merged = mergeDashboardArticles(
            currentArticles = current,
            incomingArticles = incoming,
            hiddenArticleIds = setOf(2),
            referenceNow = Instant.parse("2026-06-24T10:00:00Z"),
            limit = 5,
        )

        assertEquals(listOf(3L, 4L, 1L), merged.map { it.id })
    }
}
