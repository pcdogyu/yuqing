package com.jiansutech.yuqing.ui

import android.os.SystemClock
import android.util.Log
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.jiansutech.yuqing.astock.AStockTradingCalendar
import com.jiansutech.yuqing.data.AndroidActionRequest
import com.jiansutech.yuqing.data.AndroidDashboard
import com.jiansutech.yuqing.data.AndroidModule
import com.jiansutech.yuqing.data.AStockAuctionListResult
import com.jiansutech.yuqing.data.AStockRecommendation
import com.jiansutech.yuqing.data.AStockRecommendationSnapshot
import com.jiansutech.yuqing.data.ArticleItem
import com.jiansutech.yuqing.data.ApiFactory
import com.jiansutech.yuqing.data.DashboardCacheDao
import com.jiansutech.yuqing.data.DashboardCacheEntity
import com.jiansutech.yuqing.data.ItemListResult
import com.jiansutech.yuqing.data.SearchResult
import com.jiansutech.yuqing.data.SessionState
import com.jiansutech.yuqing.data.SessionStore
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString
import java.time.Instant
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.LocalTime
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter

data class PendingAction(
    val action: String,
    val title: String,
    val params: Map<String, String> = emptyMap(),
)

data class ConnectionTestResult(
    val loading: Boolean = false,
    val ok: Boolean? = null,
    val message: String = "",
)

data class AStockRecommendationWindow(
    val date: String,
    val period: String,
    val periodLabel: String,
    val windowLabel: String,
)

data class YuqingUiState(
    val session: SessionState = SessionState(),
    val loading: Boolean = false,
    val articleLoading: Boolean = false,
    val error: String = "",
    val message: String = "",
    val modules: List<AndroidModule> = emptyList(),
    val selectedModuleKey: String = "dashboard",
    val dashboard: AndroidDashboard? = null,
    val articleList: ItemListResult? = null,
    val articleDetail: ArticleItem? = null,
    val articleDetailLoading: Boolean = false,
    val articleDetailError: String = "",
    val aStockAuction: AStockAuctionListResult = AStockAuctionListResult(),
    val aStockAuctionDate: String = AStockTradingCalendar.latestSelectableTradingDay(
        LocalDate.now(ZoneId.of("Asia/Shanghai")),
    ).toString(),
    val aStockRecommendation: AStockRecommendationSnapshot? = null,
    val aStockRecommendations: List<AStockRecommendation> = emptyList(),
    val morningAStockRecommendation: AStockRecommendationSnapshot? = null,
    val morningAStockRecommendations: List<AStockRecommendation> = emptyList(),
    val afternoonAStockRecommendation: AStockRecommendationSnapshot? = null,
    val afternoonAStockRecommendations: List<AStockRecommendation> = emptyList(),
    val aStockRecommendationWindow: AStockRecommendationWindow = currentAStockRecommendationWindow(),
    val searchKeyword: String = "",
    val searchResult: SearchResult? = null,
    val connectionTests: Map<String, ConnectionTestResult> = emptyMap(),
    val pendingAction: PendingAction? = null,
)

class YuqingViewModel(
    private val sessionStore: SessionStore,
    private val dashboardCacheDao: DashboardCacheDao,
) : ViewModel() {
    private val _uiState = MutableStateFlow(YuqingUiState())
    val uiState: StateFlow<YuqingUiState> = _uiState

    init {
        viewModelScope.launch {
            val initStartedAt = SystemClock.elapsedRealtime()
            Log.i(STARTUP_TAG, "YuqingViewModel.init start")
            val cacheQueryStartedAt = SystemClock.elapsedRealtime()
            Log.i(STARTUP_TAG, "YuqingViewModel.cache query start")
            val cached = dashboardCacheDao.get()?.payload
            Log.i(
                STARTUP_TAG,
                "YuqingViewModel.cache query end elapsedMs=${SystemClock.elapsedRealtime() - cacheQueryStartedAt}",
            )
            Log.i(STARTUP_TAG, "YuqingViewModel.cache payloadPresent=${!cached.isNullOrBlank()} payloadLength=${cached?.length ?: 0}")
            if (!cached.isNullOrBlank()) {
                val decodeStartedAt = SystemClock.elapsedRealtime()
                Log.i(STARTUP_TAG, "YuqingViewModel.cache decode start")
                runCatching {
                    ApiFactory.json.decodeFromString<AndroidDashboard>(cached)
                }.onSuccess { dashboard ->
                    Log.i(
                        STARTUP_TAG,
                        "YuqingViewModel.cache decode success articleCount=${dashboard.overview.articleCount} elapsedMs=${SystemClock.elapsedRealtime() - decodeStartedAt}",
                    )
                    _uiState.update {
                        it.copy(
                            dashboard = dashboard,
                            aStockAuction = dashboard.aStock.auction,
                            aStockAuctionDate = dashboard.aStock.auction.date.ifBlank { it.aStockAuctionDate },
                            aStockRecommendation = dashboard.aStock.recommendation.takeIf { snapshot -> snapshot.found },
                            aStockRecommendations = parseAStockRecommendations(dashboard.aStock.recommendation.recommendationsJson),
                        )
                    }
                }.onFailure { throwable ->
                    Log.e(
                        STARTUP_TAG,
                        "YuqingViewModel.cache decode failure elapsedMs=${SystemClock.elapsedRealtime() - decodeStartedAt}",
                        throwable,
                    )
                }
            }
            var refreshed = false
            sessionStore.state.collect { session ->
                Log.i(
                    STARTUP_TAG,
                    "YuqingViewModel.session update authBaseUrl=${session.authBaseUrl} apiBaseUrl=${session.apiBaseUrl} tokenPresent=${session.token.isNotBlank()} refreshed=$refreshed",
                )
                _uiState.update { it.copy(session = session) }
                if (!refreshed) {
                    refreshed = true
                    Log.i(STARTUP_TAG, "YuqingViewModel.init trigger refreshAll elapsedMs=${SystemClock.elapsedRealtime() - initStartedAt}")
                    refreshAll()
                }
            }
        }
    }

    fun selectModule(key: String) {
        _uiState.update { it.copy(selectedModuleKey = key) }
        if (key == "articles") {
            loadArticles(1)
        }
        if (key == "a_stock") {
            resetAStockRecommendationDate()
        }
        if (key == "auction") {
            loadAStockAuction()
        }
    }

    fun updateSearchKeyword(value: String) {
        _uiState.update { it.copy(searchKeyword = value) }
    }

    fun testConnection(title: String, baseUrl: String) {
        val key = title.trim().ifBlank { baseUrl }
        if (baseUrl.isBlank()) {
            _uiState.update {
                it.copy(
                    connectionTests = it.connectionTests + (key to ConnectionTestResult(ok = false, message = "地址为空")),
                )
            }
            return
        }
        viewModelScope.launch {
            _uiState.update {
                it.copy(
                    connectionTests = it.connectionTests + (key to ConnectionTestResult(loading = true, message = "测试中")),
                )
            }
            val result = ApiFactory.testUrl(baseUrl)
            _uiState.update {
                it.copy(
                    connectionTests = it.connectionTests + (key to ConnectionTestResult(
                        loading = false,
                        ok = result.ok,
                        message = result.message,
                    )),
                )
            }
        }
    }

    fun refreshAll() {
        viewModelScope.launch {
            val startedAt = SystemClock.elapsedRealtime()
            _uiState.update { it.copy(loading = true, error = "", message = "") }
            val session = sessionStore.state.first()
            Log.i(
                STARTUP_TAG,
                "YuqingViewModel.refreshAll start selectedModule=${_uiState.value.selectedModuleKey} apiBaseUrl=${session.apiBaseUrl} tokenPresent=${session.token.isNotBlank()}",
            )
            runCatching {
                val api = ApiFactory.yuqing(session.apiBaseUrl, session.token)
                val bootstrapStartedAt = SystemClock.elapsedRealtime()
                val bootstrap = api.bootstrap().data
                Log.i(
                    STARTUP_TAG,
                    "YuqingViewModel.refreshAll bootstrap loaded moduleCount=${bootstrap?.modules.orEmpty().size} elapsedMs=${SystemClock.elapsedRealtime() - bootstrapStartedAt}",
                )
                val dashboardStartedAt = SystemClock.elapsedRealtime()
                val dashboard = api.dashboard().data ?: error("Dashboard 数据为空")
                Log.i(
                    STARTUP_TAG,
                    "YuqingViewModel.refreshAll dashboard loaded articleCount=${dashboard.articles.items.size} elapsedMs=${SystemClock.elapsedRealtime() - dashboardStartedAt}",
                )
                val correctedDashboard = runCatching {
                    val latestArticlesStartedAt = SystemClock.elapsedRealtime()
                    val latestArticles = api.articles(page = 1, pageSize = 50).data?.items.orEmpty()
                    Log.i(
                        STARTUP_TAG,
                        "YuqingViewModel.refreshAll latestArticles loaded count=${latestArticles.size} firstPublishTime=${latestArticles.firstOrNull()?.publishTime.orEmpty()} firstCapturedAt=${latestArticles.firstOrNull()?.capturedAt.orEmpty()} firstTitle=${latestArticles.firstOrNull()?.title.orEmpty()} elapsedMs=${SystemClock.elapsedRealtime() - latestArticlesStartedAt}",
                    )
                    patchDashboardLatestArticles(dashboard, latestArticles)
                }.onFailure { throwable ->
                    Log.w(STARTUP_TAG, "YuqingViewModel.refreshAll latest article patch skipped", throwable)
                }.getOrDefault(dashboard)
                dashboardCacheDao.upsert(
                    DashboardCacheEntity(
                        payload = ApiFactory.json.encodeToString(correctedDashboard),
                        savedAt = System.currentTimeMillis(),
                    ),
                )
                _uiState.update {
                    it.copy(
                        modules = bootstrap?.modules.orEmpty(),
                        dashboard = correctedDashboard,
                        articleList = if (it.selectedModuleKey == "articles") it.articleList else null,
                        aStockAuction = correctedDashboard.aStock.auction,
                        aStockAuctionDate = correctedDashboard.aStock.auction.date.ifBlank { it.aStockAuctionDate },
                        aStockRecommendation = correctedDashboard.aStock.recommendation.takeIf { snapshot -> snapshot.found },
                        aStockRecommendations = parseAStockRecommendations(correctedDashboard.aStock.recommendation.recommendationsJson),
                        message = "数据已刷新",
                    )
                }
                Log.i(
                    STARTUP_TAG,
                    "YuqingViewModel.refreshAll success modules=${bootstrap?.modules.orEmpty().size} articleCount=${correctedDashboard.overview.articleCount} taskCount=${correctedDashboard.overview.crawlRunCount}",
                )
            }.onFailure { throwable ->
                Log.e(STARTUP_TAG, "YuqingViewModel.refreshAll failure", throwable)
                _uiState.update { it.copy(error = throwable.message ?: "刷新失败") }
            }
            _uiState.update { it.copy(loading = false) }
            Log.i(
                STARTUP_TAG,
                "YuqingViewModel.refreshAll end elapsedMs=${SystemClock.elapsedRealtime() - startedAt} error=${_uiState.value.error.isNotBlank()}",
            )
            if (_uiState.value.selectedModuleKey == "articles") {
                loadArticles(1)
            }
            if (_uiState.value.selectedModuleKey == "a_stock") {
                loadAStockRecommendationDay()
            }
            if (_uiState.value.selectedModuleKey == "auction") {
                loadAStockAuction()
            }
        }
    }

    fun search() {
        val keyword = _uiState.value.searchKeyword.trim()
        if (keyword.isBlank()) {
            _uiState.update { it.copy(error = "请输入搜索关键词") }
            return
        }
        viewModelScope.launch {
            _uiState.update { it.copy(loading = true, error = "", message = "") }
            val session = sessionStore.state.first()
            runCatching {
                val result = ApiFactory.yuqing(session.apiBaseUrl, session.token).searchFull(keyword).data
                _uiState.update { it.copy(searchResult = result, message = "搜索完成") }
            }.onFailure { throwable ->
                _uiState.update { it.copy(error = throwable.message ?: "搜索失败") }
            }
            _uiState.update { it.copy(loading = false) }
        }
    }

    fun loadArticles(page: Int) {
        viewModelScope.launch {
            _uiState.update {
                it.copy(
                    loading = true,
                    articleLoading = true,
                    articleList = null,
                    error = "",
                    message = "",
                )
            }
            val session = sessionStore.state.first()
            runCatching {
                val result = ApiFactory.yuqing(session.apiBaseUrl, session.token)
                    .articles(page = page.coerceAtLeast(1), pageSize = 10)
                    .data ?: error("文章数据为空")
                _uiState.update { it.copy(articleList = result, message = "") }
            }.onFailure { throwable ->
                _uiState.update {
                    it.copy(
                        articleList = null,
                        error = throwable.message ?: "文章加载失败",
                        message = "",
                    )
                }
            }
            _uiState.update { it.copy(loading = false, articleLoading = false) }
        }
    }

    fun openArticleDetail(item: ArticleItem) {
        if (item.id <= 0) {
            _uiState.update {
                it.copy(
                    articleDetail = item,
                    articleDetailLoading = false,
                    articleDetailError = "文章ID无效，无法获取详情",
                )
            }
            return
        }
        _uiState.update {
            it.copy(
                articleDetail = item,
                articleDetailLoading = true,
                articleDetailError = "",
            )
        }
        viewModelScope.launch {
            val session = sessionStore.state.first()
            runCatching {
                ApiFactory.yuqing(session.apiBaseUrl, session.token)
                    .article(item.id)
                    .data ?: error("文章详情为空")
            }.onSuccess { detail ->
                _uiState.update {
                    it.copy(
                        articleDetail = detail,
                        articleDetailLoading = false,
                        articleDetailError = "",
                    )
                }
            }.onFailure { throwable ->
                _uiState.update {
                    it.copy(
                        articleDetailLoading = false,
                        articleDetailError = throwable.message ?: "文章详情加载失败",
                    )
                }
            }
        }
    }

    fun closeArticleDetail() {
        _uiState.update {
            it.copy(
                articleDetail = null,
                articleDetailLoading = false,
                articleDetailError = "",
            )
        }
    }

    fun loadAStockRecommendations(window: AStockRecommendationWindow = currentAStockRecommendationWindow()) {
        viewModelScope.launch {
            _uiState.update { it.copy(loading = true, error = "", message = "") }
            val session = sessionStore.state.first()
            runCatching {
                val result = ApiFactory.yuqing(session.apiBaseUrl, session.token)
                    .aStockRecommendations(date = window.date, period = window.period)
                    .data ?: error("推荐股票数据为空")
                val recommendations = parseAStockRecommendations(result.recommendationsJson)
                val resultWindow = aStockRecommendationWindow(
                    date = result.strategyDate.ifBlank { window.date },
                    period = result.period.ifBlank { window.period },
                )
                _uiState.update {
                    it.copy(
                        aStockRecommendation = result,
                        aStockRecommendations = recommendations,
                        aStockRecommendationWindow = resultWindow,
                        message = if (recommendations.isEmpty()) "当前推荐暂无股票" else "推荐股票已加载",
                    )
                }
            }.onFailure { throwable ->
                _uiState.update { it.copy(error = throwable.message ?: "推荐股票加载失败") }
            }
            _uiState.update { it.copy(loading = false) }
        }
    }

    fun selectAStockRecommendationPeriod(period: String) {
        val current = _uiState.value.aStockRecommendationWindow
        loadAStockRecommendations(aStockRecommendationWindow(current.date, period))
    }

    fun shiftAStockRecommendationDate(days: Long) {
        val zone = ZoneId.of("Asia/Shanghai")
        val latestTradingDay = AStockTradingCalendar.latestSelectableTradingDay(LocalDate.now(zone))
        val current = _uiState.value.aStockRecommendationWindow
        val currentDate = runCatching { LocalDate.parse(current.date) }.getOrDefault(latestTradingDay)
        val currentTradingDate = AStockTradingCalendar.previousOrSameTradingDay(currentDate)
        var targetDate = currentTradingDate
        repeat(kotlin.math.abs(days).toInt()) {
            targetDate = if (days < 0) {
                AStockTradingCalendar.previousTradingDay(targetDate)
            } else if (days > 0) {
                AStockTradingCalendar.nextTradingDay(targetDate)
                    .let { if (it.isAfter(latestTradingDay)) latestTradingDay else it }
            } else {
                targetDate
            }
        }
        loadAStockRecommendationDay(targetDate.toString())
    }

    fun resetAStockRecommendationDate() {
        val today = AStockTradingCalendar.latestSelectableTradingDay(LocalDate.now(ZoneId.of("Asia/Shanghai")))
        loadAStockRecommendationDay(today.toString())
    }

    fun loadAStockRecommendationDay(date: String = _uiState.value.aStockRecommendationWindow.date) {
        viewModelScope.launch {
            _uiState.update { it.copy(loading = true, error = "", message = "") }
            val session = sessionStore.state.first()
            val zone = ZoneId.of("Asia/Shanghai")
            val latestTradingDay = AStockTradingCalendar.latestSelectableTradingDay(LocalDate.now(zone))
            val parsedDate = runCatching { LocalDate.parse(date) }.getOrDefault(latestTradingDay)
            val requestedTradingDate = AStockTradingCalendar.previousOrSameTradingDay(parsedDate)
                .let { if (it.isAfter(latestTradingDay)) latestTradingDay else it }
            val requestedDate = requestedTradingDate.toString()
            runCatching {
                val api = ApiFactory.yuqing(session.apiBaseUrl, session.token)
                val morning = api.aStockRecommendations(date = requestedDate, period = "morning")
                    .data ?: error("上午推荐股票数据为空")
                val afternoon = api.aStockRecommendations(date = requestedDate, period = "afternoon")
                    .data ?: error("下午推荐股票数据为空")
                Pair(morning, afternoon)
            }.onSuccess { (morning, afternoon) ->
                val morningItems = parseAStockRecommendations(morning.recommendationsJson)
                val afternoonItems = parseAStockRecommendations(afternoon.recommendationsJson)
                _uiState.update {
                    it.copy(
                        morningAStockRecommendation = morning,
                        morningAStockRecommendations = morningItems,
                        afternoonAStockRecommendation = afternoon,
                        afternoonAStockRecommendations = afternoonItems,
                        aStockRecommendation = afternoon.takeIf { snapshot -> snapshot.found }
                            ?: morning.takeIf { snapshot -> snapshot.found },
                        aStockRecommendations = morningItems + afternoonItems,
                        aStockRecommendationWindow = aStockRecommendationWindow(requestedDate, "morning"),
                        message = if (morningItems.isEmpty() && afternoonItems.isEmpty()) {
                            "当日推荐暂无股票"
                        } else {
                            "当日推荐股票已加载"
                        },
                    )
                }
            }.onFailure { throwable ->
                _uiState.update { it.copy(error = throwable.message ?: "推荐股票加载失败") }
            }
            _uiState.update { it.copy(loading = false) }
        }
    }

    fun loadAStockAuction(date: String = _uiState.value.aStockAuctionDate) {
        viewModelScope.launch {
            _uiState.update { it.copy(loading = true, error = "", message = "") }
            val session = sessionStore.state.first()
            val requestedDate = date.ifBlank { LocalDate.now(ZoneId.of("Asia/Shanghai")).toString() }
            runCatching {
                ApiFactory.yuqing(session.apiBaseUrl, session.token)
                    .aStockAuction(date = requestedDate, page = 1, pageSize = 6000)
                    .data ?: error("集合竞价数据为空")
            }.onSuccess { result ->
                _uiState.update {
                    it.copy(
                        aStockAuction = result,
                        aStockAuctionDate = result.date.ifBlank { requestedDate },
                        message = "集合竞价已加载",
                    )
                }
            }.onFailure { throwable ->
                _uiState.update { it.copy(error = throwable.message ?: "集合竞价加载失败") }
            }
            _uiState.update { it.copy(loading = false) }
        }
    }

    fun requestAction(action: String, title: String, params: Map<String, String> = emptyMap()) {
        _uiState.update { it.copy(pendingAction = PendingAction(action, title, params), error = "", message = "") }
    }

    fun dismissAction() {
        _uiState.update { it.copy(pendingAction = null) }
    }

    fun runPendingAction() {
        val pending = _uiState.value.pendingAction ?: return
        viewModelScope.launch {
            _uiState.update { it.copy(loading = true, pendingAction = null, error = "", message = "") }
            val session = sessionStore.state.first()
            runCatching {
                val response = ApiFactory.yuqing(session.apiBaseUrl, session.token)
                    .runAction(pending.action, AndroidActionRequest(pending.params))
                val status = response.data?.status ?: response.message
                _uiState.update { it.copy(message = "${pending.title}: $status") }
            }.onFailure { throwable ->
                _uiState.update { it.copy(error = throwable.message ?: "操作失败") }
            }
            _uiState.update { it.copy(loading = false) }
        }
    }
}

private const val STARTUP_TAG = "YuqingStartup"
private val articleTimeFormats = listOf(
    DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss"),
    DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm"),
    DateTimeFormatter.ofPattern("yyyy/MM/dd HH:mm:ss"),
    DateTimeFormatter.ofPattern("yyyy/MM/dd HH:mm"),
)

private fun currentAStockRecommendationWindow(): AStockRecommendationWindow {
    val zone = ZoneId.of("Asia/Shanghai")
    val now = LocalTime.now(zone)
    val period = if (now.isBefore(LocalTime.of(9, 31))) "morning" else "afternoon"
    return aStockRecommendationWindow(
        date = AStockTradingCalendar.latestSelectableTradingDay(LocalDate.now(zone)).toString(),
        period = period,
    )
}

private fun aStockRecommendationWindow(date: String, period: String): AStockRecommendationWindow {
    val normalizedPeriod = if (period == "afternoon" || period == "pm" || period == "after") "afternoon" else "morning"
    return AStockRecommendationWindow(
        date = date,
        period = normalizedPeriod,
        periodLabel = if (normalizedPeriod == "afternoon") "下午推荐" else "上午推荐",
        windowLabel = if (normalizedPeriod == "afternoon") "09:30-13:00" else "08:00-09:30",
    )
}

private fun parseAStockRecommendations(raw: String): List<AStockRecommendation> {
    val payload = raw.trim().ifBlank { "[]" }
    return runCatching {
        ApiFactory.json.decodeFromString<List<AStockRecommendation>>(payload)
    }.getOrDefault(emptyList())
}

private fun patchDashboardLatestArticles(dashboard: AndroidDashboard, latestArticles: List<ArticleItem>): AndroidDashboard {
    val referenceNow = Instant.now()
    val patchedItems = latestArticles
        .distinctBy(::articleIdentity)
        .map { item ->
            DashboardLatestArticleSortEntry(
                item = item,
                sortTime = articleSortTime(item, referenceNow) ?: Instant.EPOCH,
            )
        }
        .sortedWith(
            compareByDescending<DashboardLatestArticleSortEntry> { it.sortTime }
                .thenByDescending { it.item.publishTime }
                .thenByDescending { it.item.publishTimeText }
                .thenByDescending { it.item.capturedAt }
                .thenByDescending { it.item.id },
        )
        .map { it.item }
        .take(10)
    if (patchedItems.isEmpty()) {
        return dashboard
    }
    val topItem = patchedItems.first()
    Log.i(
        STARTUP_TAG,
        "YuqingViewModel.latestArticles patched size=${patchedItems.size} topTitle=${topItem.title} topCapturedAt=${topItem.capturedAt} topPublishTime=${topItem.publishTime} topPublishTimeText=${topItem.publishTimeText}",
    )
    return dashboard.copy(
        articles = dashboard.articles.copy(items = patchedItems),
    )
}

private fun articleIdentity(item: ArticleItem): String {
    return listOf(item.id.toString(), item.sourceUrl, item.capturedAt, item.title)
        .joinToString("|")
}

private data class DashboardLatestArticleSortEntry(
    val item: ArticleItem,
    val sortTime: Instant,
)

private fun articleSortTime(item: ArticleItem, referenceNow: Instant): Instant? {
    return parseArticleInstant(item.publishTime, referenceNow)
        ?: parseArticleInstant(item.publishTimeText, referenceNow)
        ?: parseArticleInstant(item.capturedAt, referenceNow)
}

private fun parseArticleInstant(raw: String, referenceNow: Instant): Instant? {
    val value = raw.trim()
    if (value.isBlank()) {
        return null
    }
    if (value == "刚刚") {
        return referenceNow
    }
    Regex("""^(\d+)\s*分钟前$""").matchEntire(value)?.groupValues?.getOrNull(1)?.toLongOrNull()?.let {
        return referenceNow.minusSeconds(it * 60)
    }
    Regex("""^(\d+)\s*小时前$""").matchEntire(value)?.groupValues?.getOrNull(1)?.toLongOrNull()?.let {
        return referenceNow.minusSeconds(it * 3600)
    }
    runCatching { OffsetDateTime.parse(value).toInstant() }.getOrNull()?.let { return it }
    runCatching { Instant.parse(value) }.getOrNull()?.let { return it }
    val zone = ZoneId.of("Asia/Shanghai")
    for (formatter in articleTimeFormats) {
        runCatching { LocalDateTime.parse(value, formatter).atZone(zone).toInstant() }.getOrNull()?.let { return it }
    }
    return null
}

class YuqingViewModelFactory(
    private val sessionStore: SessionStore,
    private val dashboardCacheDao: DashboardCacheDao,
) : ViewModelProvider.Factory {
    @Suppress("UNCHECKED_CAST")
    override fun <T : ViewModel> create(modelClass: Class<T>): T {
        return YuqingViewModel(sessionStore, dashboardCacheDao) as T
    }
}
