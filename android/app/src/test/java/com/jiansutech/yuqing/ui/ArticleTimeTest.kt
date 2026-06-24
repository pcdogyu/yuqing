package com.jiansutech.yuqing.ui

import org.junit.Assert.assertEquals
import org.junit.Test
import java.time.LocalDateTime

class ArticleTimeTest {
    private val now = LocalDateTime.of(2026, 6, 23, 10, 0, 0)

    @Test
    fun absoluteTimestampFormatsAsMinutesWhenUnderOneHour() {
        assertEquals(
            "17分钟之前",
            formatArticleRelativeTime("2026-06-23 09:43:00", now),
        )
    }

    @Test
    fun absoluteTimestampFormatsAsHoursWhenOverOneHour() {
        assertEquals(
            "2小时之前",
            formatArticleRelativeTime("2026-06-23 07:50:00", now),
        )
    }

    @Test
    fun relativeMinutesConvertToExpectedLabel() {
        assertEquals("5分钟之前", formatArticleRelativeTime("5分钟前", now))
        assertEquals("60分钟之前", formatArticleRelativeTime("60分钟前", now))
        assertEquals("1小时之前", formatArticleRelativeTime("90分钟前", now))
    }

    @Test
    fun relativeHoursAndJustNowStayReadable() {
        assertEquals("2小时之前", formatArticleRelativeTime("2小时前", now))
        assertEquals("刚刚", formatArticleRelativeTime("刚刚", now))
        assertEquals("", formatArticleRelativeTime("未知时间", now))
    }

    @Test
    fun capturedTimestampConvertsToBeijingClockWhenTimezoneIsPresent() {
        assertEquals("2026-06-23 18:20", formatArticleCapturedTime("2026-06-23T10:20:01Z"))
        assertEquals("2026-06-23 10:20", formatArticleCapturedTime("2026-06-23 10:20:01"))
    }

    @Test
    fun publishTimestampFormatsAbsoluteTimeWithoutRelativeText() {
        assertEquals("2026-06-24 09:29", formatArticlePublishTime("2026-06-24 09:29:30"))
        assertEquals("2026-06-24 09:30", formatArticlePublishTime("2026-06-24T01:30:12Z"))
        assertEquals("5分钟前", formatArticlePublishTime("5分钟前"))
    }

    @Test
    fun articleTitleAppendsRelativeTime() {
        assertEquals("新闻标题 5分钟之前", articleTitleWithRelativeTime(" 新闻标题 ", "5分钟之前"))
        assertEquals("新闻标题", articleTitleWithRelativeTime("新闻标题", ""))
    }
}
