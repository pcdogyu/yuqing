package com.jiansutech.yuqing.ui

import com.jiansutech.yuqing.data.AndroidDashboard
import com.jiansutech.yuqing.data.ArticleItem
import com.jiansutech.yuqing.data.ItemListResult
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

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
}
