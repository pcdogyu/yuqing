package com.jiansutech.yuqing.ui

import com.jiansutech.yuqing.data.AndroidDashboard
import com.jiansutech.yuqing.data.AStockBacktestCell
import com.jiansutech.yuqing.data.AStockBacktestRow
import com.jiansutech.yuqing.data.AStockRecommendation
import com.jiansutech.yuqing.data.AStockRecommendationSnapshot
import com.jiansutech.yuqing.data.ArticleItem
import com.jiansutech.yuqing.data.ItemListResult
import com.jiansutech.yuqing.data.StockResearch
import androidx.compose.material3.SwipeToDismissBoxValue
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.time.Instant
import java.time.LocalDate

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
    fun bottomNavSecretTapUnlocksSystemAfterTenConsecutiveTaps() {
        var state = BottomNavSecretTapState()
        var unlocked = false

        repeat(10) { index ->
            val result = nextBottomNavSecretTapState(state, "auction", now = 1_000L + index * 100L)
            state = result.state
            unlocked = result.unlocked
        }

        assertEquals(true, unlocked)
        assertEquals(BottomNavSecretTapState(), state)
    }

    @Test
    fun bottomNavSecretTapResetsWhenMenuChangesOrWindowExpires() {
        var state = BottomNavSecretTapState()

        repeat(9) { index ->
            state = nextBottomNavSecretTapState(state, "a_stock", now = 1_000L + index * 100L).state
        }
        val changedMenu = nextBottomNavSecretTapState(state, "auction", now = 2_000L)
        assertEquals(false, changedMenu.unlocked)
        assertEquals(BottomNavSecretTapState(key = "auction", count = 1, lastClickAt = 2_000L), changedMenu.state)

        val expiredWindow = nextBottomNavSecretTapState(changedMenu.state, "auction", now = 8_001L)
        assertEquals(false, expiredWindow.unlocked)
        assertEquals(BottomNavSecretTapState(key = "auction", count = 1, lastClickAt = 8_001L), expiredWindow.state)
    }

    @Test
    fun bottomNavSystemLockLastsThreeSeconds() {
        val lockedUntil = bottomNavLockUntilAfterSystemUnlock(now = 1_000L)

        assertEquals(4_000L, lockedUntil)
        assertEquals(true, isBottomNavLocked(lockedUntil, now = 3_999L))
        assertEquals(false, isBottomNavLocked(lockedUntil, now = 4_000L))
        assertEquals(0.38f, bottomNavAlpha(locked = true), 0.001f)
        assertEquals(1f, bottomNavAlpha(locked = false), 0.001f)
    }

    @Test
    fun aStockAuctionTrendDaysAcceptsOnlySupportedPeriods() {
        assertEquals(7, normalizeAStockAuctionTrendDays(7))
        assertEquals(14, normalizeAStockAuctionTrendDays(14))
        assertEquals(30, normalizeAStockAuctionTrendDays(30))
        assertEquals(DEFAULT_A_STOCK_AUCTION_TREND_DAYS, normalizeAStockAuctionTrendDays(0))
        assertEquals(DEFAULT_A_STOCK_AUCTION_TREND_DAYS, normalizeAStockAuctionTrendDays(15))
    }

    @Test
    fun aStockDateLabelSplitsDateAndWeekday() {
        assertEquals(
            AStockDateLabelParts(date = "2026-07-10", weekday = "周五"),
            formatAStockDateLabelParts("2026-07-10"),
        )
        assertEquals(AStockDateLabelParts(date = "bad-date"), formatAStockDateLabelParts("bad-date"))
    }

    @Test
    fun aStockRecommendationWindowRecognizesEveningAliases() {
        val evening = aStockRecommendationWindow("2026-07-10", "evening")

        assertEquals("evening", evening.period)
        assertEquals("晚间推荐", evening.periodLabel)
        assertEquals("15:00-18:30", evening.windowLabel)
        assertEquals("evening", aStockRecommendationWindow("2026-07-10", "night").period)
        assertEquals("evening", aStockRecommendationWindow("2026-07-10", "pm2").period)
        assertEquals("afternoon", aStockRecommendationWindow("2026-07-10", "pm").period)
    }

    @Test
    fun backtestValueToneUsesAStockRedUpGreenDownSemantics() {
        assertEquals(BacktestValueTone.Up, backtestValueTone("+10.44%"))
        assertEquals(BacktestValueTone.Down, backtestValueTone(" -3.27%"))
        assertEquals(BacktestValueTone.Flat, backtestValueTone("--"))
        assertEquals(BacktestValueTone.Flat, backtestValueTone(""))
    }

    @Test
    fun aStockBacktestAdjacentTargetsJoinMorningAndAfternoon() {
        val items = buildAStockBacktestNavigationItems(
            strategyDate = "2026-07-10",
            morningRecommendations = listOf(
                testAStockRecommendation("300054", "鼎龙股份", 3),
                testAStockRecommendation("688135", "利扬芯片", 4),
            ),
            morningBacktests = emptyList(),
            afternoonRecommendations = listOf(testAStockRecommendation("002747", "埃斯顿", 1)),
            afternoonBacktests = emptyList(),
        )

        val morningLastTargets = aStockBacktestAdjacentTargets(
            items[1].toDetailState(items),
            latestTradingDate = LocalDate.parse("2026-07-10"),
        )
        val afternoonFirstTargets = aStockBacktestAdjacentTargets(
            items[2].toDetailState(items),
            latestTradingDate = LocalDate.parse("2026-07-10"),
        )

        assertEquals("002747 埃斯顿", morningLastTargets.next?.label)
        assertEquals(AStockBacktestAdjacentTargetKind.Stock, morningLastTargets.next?.kind)
        assertEquals("688135 利扬芯片", afternoonFirstTargets.previous?.label)
        assertEquals(AStockBacktestAdjacentTargetKind.Stock, afternoonFirstTargets.previous?.kind)
    }

    @Test
    fun aStockBacktestNavigationJoinsMorningAfternoonAndEvening() {
        val items = buildAStockBacktestNavigationItems(
            strategyDate = "2026-07-10",
            morningRecommendations = listOf(testAStockRecommendation("300054", "鼎龙股份", 1)),
            morningBacktests = emptyList(),
            afternoonRecommendations = listOf(testAStockRecommendation("002747", "埃斯顿", 2)),
            afternoonBacktests = emptyList(),
            eveningRecommendations = listOf(testAStockRecommendation("600000", "浦发银行", 3)),
            eveningBacktests = emptyList(),
        )

        assertEquals(listOf("morning", "afternoon", "evening"), items.map { it.period })
        assertEquals("晚间推荐", items.last().sectionLabel)
        assertEquals("600000 浦发银行", aStockBacktestAdjacentTargets(items[1].toDetailState(items)).next?.label)
        assertEquals("002747 埃斯顿", aStockBacktestAdjacentTargets(items[2].toDetailState(items)).previous?.label)
    }

    @Test
    fun aStockBacktestDetailSnapshotKeepsEveningSectionLabel() {
        val items = buildAStockBacktestNavigationItems(
            strategyDate = "2026-07-10",
            morningRecommendations = emptyList(),
            morningBacktests = emptyList(),
            afternoonRecommendations = emptyList(),
            afternoonBacktests = emptyList(),
            eveningRecommendations = listOf(testAStockRecommendation("600000", "浦发银行", 1)),
            eveningBacktests = emptyList(),
        )
        val state = items.first().toDetailState(items)
        val snapshot = AStockRecommendationSnapshot(
            found = true,
            strategyDate = "2026-07-10",
            period = "evening",
            recommendationsJson = """
                [{"Rank":1,"Hotspot":"晚间量价筛选","Code":"600000","Name":"浦发银行","Reason":"量比 4.00"}]
            """.trimIndent(),
            backtestsJson = """[{"Stock":"600000 浦发银行","CurrentPrice":"11.20"}]""",
        )

        val updated = applyAStockBacktestDetailSnapshot(state, snapshot)

        assertEquals("evening", updated.period)
        assertEquals("晚间推荐", updated.sectionLabel)
        assertEquals("晚间推荐", updated.navigationItems.first().sectionLabel)
        assertEquals("600000", updated.recommendation.code)
    }

    @Test
    fun aStockBacktestAdjacentTargetsShowTradingDateBoundaries() {
        val todayItems = buildAStockBacktestNavigationItems(
            strategyDate = "2026-07-10",
            morningRecommendations = listOf(testAStockRecommendation("688249", "晶合集成", 1)),
            morningBacktests = emptyList(),
            afternoonRecommendations = listOf(testAStockRecommendation("002747", "埃斯顿", 1)),
            afternoonBacktests = emptyList(),
        )
        val todayFirstTargets = aStockBacktestAdjacentTargets(
            todayItems.first().toDetailState(todayItems),
            latestTradingDate = LocalDate.parse("2026-07-10"),
        )
        val todayLastTargets = aStockBacktestAdjacentTargets(
            todayItems.last().toDetailState(todayItems),
            latestTradingDate = LocalDate.parse("2026-07-10"),
        )
        val yesterdayItems = buildAStockBacktestNavigationItems(
            strategyDate = "2026-07-09",
            morningRecommendations = listOf(testAStockRecommendation("000977", "浪潮信息", 1)),
            morningBacktests = emptyList(),
            afternoonRecommendations = emptyList(),
            afternoonBacktests = emptyList(),
        )
        val yesterdayLastTargets = aStockBacktestAdjacentTargets(
            yesterdayItems.last().toDetailState(yesterdayItems),
            latestTradingDate = LocalDate.parse("2026-07-10"),
        )

        assertEquals("2026-07-09", todayFirstTargets.previous?.label)
        assertEquals(AStockBacktestTradingDatePick.Last, todayFirstTargets.previous?.tradingDatePick)
        assertNull(todayLastTargets.next)
        assertEquals("2026-07-10", yesterdayLastTargets.next?.label)
        assertEquals(AStockBacktestTradingDatePick.First, yesterdayLastTargets.next?.tradingDatePick)
    }

    @Test
    fun aStockBacktestAdjacentDetailUsesNavigationIndexWhenCodesRepeat() {
        val morning = testAStockRecommendation("300394", "天孚通信", 1)
        val afternoon = testAStockRecommendation("300394", "天孚通信", 1)
        val next = testAStockRecommendation("002747", "埃斯顿", 2)
        val items = buildAStockBacktestNavigationItems(
            strategyDate = "2026-07-10",
            morningRecommendations = listOf(morning),
            morningBacktests = emptyList(),
            afternoonRecommendations = listOf(afternoon, next),
            afternoonBacktests = emptyList(),
        )
        val afternoonState = items[1].toDetailState(items)

        val nextState = adjacentAStockBacktestDetail(afternoonState, 1)

        assertEquals("002747", nextState?.recommendation?.code)
        assertEquals("afternoon", nextState?.period)
        assertEquals("", aStockRecommendationCodeNameLabel(AStockRecommendation()))
    }

    @Test
    fun aStockBacktestAdjacentTargetsHideWhenNoSwitchTargetExists() {
        val emptyState = AStockBacktestDetailState(
            recommendation = testAStockRecommendation("002520", "日发精机", 1),
            row = null,
            strategyDate = "2026-07-10",
            period = "morning",
            sectionLabel = "上午推荐",
        )

        assertEquals(AStockBacktestAdjacentTargets(), aStockBacktestAdjacentTargets(emptyState))
        assertEquals("", aStockRecommendationCodeNameLabel(AStockRecommendation()))
    }

    @Test
    fun aStockBacktestAdjacentButtonUsesCompactLayoutDefaults() {
        assertEquals(0f, AStockBacktestAdjacentButtonSpacing.value, 0.001f)
        assertEquals(14f, AStockBacktestAdjacentButtonFontSize.value, 0.001f)
    }

    @Test
    fun aStockBacktestDetailCurrentValuesPreferRefreshedRowFields() {
        val row = AStockBacktestRow(
            stock = "300394 天孚通信",
            entryOpen = "281.00",
            currentPrice = "284.30",
            currentReturn = "+1.17%",
            currentMarketPct = "+1.17%",
        )

        assertEquals("284.30", aStockBacktestDetailCurrentPrice(row))
        assertEquals("+1.17%", aStockBacktestDetailRecommendedReturn(row))
    }

    @Test
    fun aStockBacktestDetailCurrentPriceDoesNotFallbackToLatestBacktestClose() {
        val row = AStockBacktestRow(
            stock = "300394 天孚通信",
            currentPrice = "--",
            currentReturn = "",
            days = listOf(AStockBacktestCell(close = "283.82", marketPct = "+1.00%")),
        )

        assertEquals("--", aStockBacktestDetailCurrentPrice(row))
        assertEquals("--", aStockBacktestDetailRecommendedReturn(row))
    }

    @Test
    fun aStockBacktestDetailCurrentPriceRejectsLegacyT5SnapshotValueWithoutRealtimePct() {
        val row = AStockBacktestRow(
            stock = "600276 恒瑞医药",
            entryOpen = "56.81",
            currentPrice = "54.73",
            currentReturn = "-3.66%",
            days = listOf(AStockBacktestCell(close = "54.73", returnPct = "-3.66%")),
            status = "已回测T+5",
        )

        assertEquals("--", aStockBacktestDetailCurrentPrice(row))
        assertEquals("--", aStockBacktestDetailRecommendedReturn(row))
    }

    @Test
    fun aStockBacktestDetailRecommendedReturnCalculatesFromCurrentAndEntryPrice() {
        val row = AStockBacktestRow(
            stock = "301017 漱玉平民",
            afternoonOpen = "13.70",
            currentPrice = "12.47",
            currentReturn = "",
            currentMarketPct = "-1.97%",
        )

        assertEquals("12.47", aStockBacktestDetailCurrentPrice(row))
        assertEquals("-8.98%", aStockBacktestDetailRecommendedReturn(row))
    }

    @Test
    fun stockResearchDisplayDatePrefersResearchDateThenPublishDate() {
        assertEquals(
            "2026-07-08",
            stockResearchDisplayDate(
                StockResearch(researchDate = "2026-07-08", publishTime = "2026-07-07T11:00:00Z"),
            ),
        )
        assertEquals(
            "2026-07-07",
            stockResearchDisplayDate(StockResearch(publishTime = "2026-07-07T11:00:00Z")),
        )
    }

    @Test
    fun stockResearchDetailBodyPrefersSourceTextThenPdfTextThenSummary() {
        assertEquals(
            "原文正文\n\n第二段",
            stockResearchDetailBody(StockResearch(sourceText = " 原文正文\n\n第二段 ", pdfText = "PDF正文", summary = "摘要")),
        )
        assertEquals(
            "PDF正文",
            stockResearchDetailBody(StockResearch(pdfText = " PDF正文 ", summary = "摘要")),
        )
        assertEquals(
            "摘要",
            stockResearchDetailBody(StockResearch(summary = " 摘要 ")),
        )
    }

    @Test
    fun stockResearchSourceLabelMarksInvestorRelations() {
        assertEquals(
            "投资者关系",
            stockResearchSourceLabel(StockResearch(sourceType = "cninfo_investor_relation", kind = "survey")),
        )
        assertEquals("调研", stockResearchSourceLabel(StockResearch(kind = "survey")))
    }

    @Test
    fun stockResearchHasDownloadablePdfRequiresStoredPathOrExternalUrl() {
        assertEquals(false, stockResearchHasDownloadablePdf(StockResearch(id = 4580)))
        assertEquals(true, stockResearchHasDownloadablePdf(StockResearch(pdfFilePath = "data/report.pdf")))
        assertEquals(true, stockResearchHasDownloadablePdf(StockResearch(pdfUrl = "https://example.com/report.pdf")))
    }

    @Test
    fun stockResearchTotalPagesRoundsUpAndHandlesEmptyTotal() {
        assertEquals(1, stockResearchTotalPages(total = 0, pageSize = 25))
        assertEquals(1, stockResearchTotalPages(total = 25, pageSize = 25))
        assertEquals(2, stockResearchTotalPages(total = 26, pageSize = 25))
        assertEquals(26, stockResearchTotalPages(total = 26, pageSize = 0))
    }

    @Test
    fun stockResearchOpenableUrlNormalizesWebUrlsOnly() {
        assertEquals("https://example.com/a", stockResearchOpenableUrl(" https://example.com/a "))
        assertEquals("https://example.com/a", stockResearchOpenableUrl("//example.com/a"))
        assertEquals("https://www.example.com/a", stockResearchOpenableUrl("www.example.com/a"))
        assertEquals("", stockResearchOpenableUrl("javascript:alert(1)"))
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
    fun filterInactiveArticlesDropsReadOrHiddenIds() {
        val result = ItemListResult(
            items = listOf(
                ArticleItem(id = 1, title = "read"),
                ArticleItem(id = 2, title = "visible"),
                ArticleItem(id = 3, title = "hidden"),
            ),
            total = 3,
        )

        val visible = filterInactiveArticles(result, setOf(1, 3))

        assertEquals(listOf("visible"), visible?.items?.map { it.title })
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
            inactiveArticleIds = setOf(2),
            referenceNow = Instant.parse("2026-06-24T10:00:00Z"),
            limit = 5,
        )

        assertEquals(listOf(3L, 4L, 1L), merged.map { it.id })
    }

    @Test
    fun appendVisibleArticlesCanFillFromLaterPages() {
        val firstPage = appendVisibleArticles(
            currentArticles = emptyList(),
            incomingArticles = listOf(
                ArticleItem(id = 1, title = "read"),
                ArticleItem(id = 2, title = "also read"),
                ArticleItem(id = 3, title = "visible first"),
            ),
            inactiveArticleIds = setOf(1, 2),
            limit = 3,
        )
        val filled = appendVisibleArticles(
            currentArticles = firstPage,
            incomingArticles = listOf(
                ArticleItem(id = 4, title = "visible second"),
                ArticleItem(id = 5, title = "visible third"),
            ),
            inactiveArticleIds = setOf(1, 2),
            limit = 3,
        )

        assertEquals(listOf(3L, 4L, 5L), filled.map { it.id })
    }

    @Test
    fun articleListSwipeDirectionMapsRightToHideAndLeftToClear() {
        assertEquals(ArticleListSwipeAction.Hide, articleListSwipeAction(SwipeToDismissBoxValue.StartToEnd))
        assertEquals(ArticleListSwipeAction.Clear, articleListSwipeAction(SwipeToDismissBoxValue.EndToStart))
        assertEquals(ArticleListSwipeAction.None, articleListSwipeAction(SwipeToDismissBoxValue.Settled))
    }

    @Test
    fun nextArticleAfterReturnsFollowingArticle() {
        val first = ArticleItem(id = 1, title = "first")
        val second = ArticleItem(id = 2, title = "second")
        val third = ArticleItem(id = 3, title = "third")

        val next = nextArticleAfter(first, listOf(first, second, third))

        assertEquals(second, next)
    }

    @Test
    fun previousArticleBeforeReturnsPreviousArticle() {
        val first = ArticleItem(id = 1, title = "first")
        val second = ArticleItem(id = 2, title = "second")
        val third = ArticleItem(id = 3, title = "third")

        val previous = previousArticleBefore(third, listOf(first, second, third))

        assertEquals(second, previous)
    }

    @Test
    fun articleDetailNavigationUsesListContainingCurrentDetail() {
        val dashboardArticle = ArticleItem(id = 1, title = "dashboard")
        val listArticle = ArticleItem(id = 2, title = "list")
        val listNext = ArticleItem(id = 3, title = "list next")
        val state = YuqingUiState(
            articleDetail = listArticle,
            dashboard = AndroidDashboard(
                articles = ItemListResult(items = listOf(dashboardArticle)),
            ),
            articleList = ItemListResult(items = listOf(listArticle, listNext)),
        )

        val articles = articleDetailNavigationArticles(state)

        assertEquals(listOf(listArticle, listNext), articles)
    }
}

private fun testAStockRecommendation(code: String, name: String, rank: Int): AStockRecommendation {
    return AStockRecommendation(rank = rank, code = code, name = name)
}
