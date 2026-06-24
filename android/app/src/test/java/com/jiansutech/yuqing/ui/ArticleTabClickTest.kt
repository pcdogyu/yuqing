package com.jiansutech.yuqing.ui

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ArticleTabClickTest {
    @Test
    fun articleTabSecondClickWithinWindowIsDoubleClick() {
        assertTrue(
            isArticleTabDoubleClick(
                clickedKey = "articles",
                currentKey = "articles",
                lastArticleTabClickAt = 1_000L,
                now = 1_350L,
            ),
        )
    }

    @Test
    fun firstClickOrDifferentTabIsNotDoubleClick() {
        assertFalse(
            isArticleTabDoubleClick(
                clickedKey = "articles",
                currentKey = "dashboard",
                lastArticleTabClickAt = 1_000L,
                now = 1_100L,
            ),
        )
        assertFalse(
            isArticleTabDoubleClick(
                clickedKey = "dashboard",
                currentKey = "articles",
                lastArticleTabClickAt = 1_000L,
                now = 1_100L,
            ),
        )
    }

    @Test
    fun articleTabClickAfterWindowIsSingleClick() {
        assertFalse(
            isArticleTabDoubleClick(
                clickedKey = "articles",
                currentKey = "articles",
                lastArticleTabClickAt = 1_000L,
                now = 1_401L,
            ),
        )
    }
}
