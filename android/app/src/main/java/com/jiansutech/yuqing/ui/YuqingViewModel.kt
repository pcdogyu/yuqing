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
import com.jiansutech.yuqing.data.ArticleUserActionDao
import com.jiansutech.yuqing.data.ArticleUserActionEntity
import com.jiansutech.yuqing.data.ApiFactory
import com.jiansutech.yuqing.data.DashboardCacheDao
import com.jiansutech.yuqing.data.DashboardCacheEntity
import com.jiansutech.yuqing.data.ItemListResult
import com.jiansutech.yuqing.data.NETWORK_HEARTBEAT_INTERVAL_MILLIS
import com.jiansutech.yuqing.data.NetworkEndpointPolicy
import com.jiansutech.yuqing.data.NetworkEndpoints
import com.jiansutech.yuqing.data.NetworkEnvironment
import com.jiansutech.yuqing.data.NetworkEnvironmentSelector
import com.jiansutech.yuqing.data.SearchResult
import com.jiansutech.yuqing.data.SessionState
import com.jiansutech.yuqing.data.SessionStore
import com.jiansutech.yuqing.data.YuqingApi
import kotlinx.coroutines.delay
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

private data class ArticleActionState(
    val readArticleIds: Set<Long>,
    val inactiveArticleIds: Set<Long>,
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
    val readArticleIds: Set<Long> = emptySet(),
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
    val networkEndpoints: NetworkEndpoints = NetworkEndpointPolicy.endpointsFor(NetworkEnvironment.External),
    val networkHeartbeatLoading: Boolean = false,
    val networkHeartbeatMessage: String = "",
)

class YuqingViewModel(
    private val sessionStore: SessionStore,
    private val dashboardCacheDao: DashboardCacheDao,
    private val articleUserActionDao: ArticleUserActionDao,
) : ViewModel() {
    private val _uiState = MutableStateFlow(YuqingUiState())
    val uiState: StateFlow<YuqingUiState> = _uiState
    private val networkEnvironmentSelector = NetworkEnvironmentSelector { baseUrl ->
        ApiFactory.testUrl(baseUrl, "healthz").ok
    }

    init {
        startNetworkHeartbeat()
        viewModelScope.launch {
            val initStartedAt = SystemClock.elapsedRealtime()
            Log.i(STARTUP_TAG, "YuqingViewModel.init start")
            val cacheQueryStartedAt = SystemClock.elapsedRealtime()
            Log.i(STARTUP_TAG, "YuqingViewModel.cache query start")
            val cached = dashboardCacheDao.get()?.payload
            val articleActions = loadArticleActionState()
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
                    val visibleDashboard = filterDashboardInactiveArticles(dashboard, articleActions.inactiveArticleIds)
                    _uiState.update {
                        it.copy(
                            dashboard = visibleDashboard,
                            readArticleIds = articleActions.readArticleIds,
                            aStockAuction = visibleDashboard.aStock.auction,
                            aStockAuctionDate = visibleDashboard.aStock.auction.date.ifBlank { it.aStockAuctionDate },
                            aStockRecommendation = visibleDashboard.aStock.recommendation.takeIf { snapshot -> snapshot.found },
                            aStockRecommendations = parseAStockRecommendations(visibleDashboard.aStock.recommendation.recommendationsJson),
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
                    detectNetworkEnvironment()
                    Log.i(STARTUP_TAG, "YuqingViewModel.init trigger refreshAll elapsedMs=${SystemClock.elapsedRealtime() - initStartedAt}")
                    refreshAll()
                }
            }
        }
    }

    private fun startNetworkHeartbeat() {
        viewModelScope.launch {
            while (true) {
                detectNetworkEnvironment()
                delay(NETWORK_HEARTBEAT_INTERVAL_MILLIS)
            }
        }
    }

    private suspend fun detectNetworkEnvironment() {
        _uiState.update {
            it.copy(
                networkHeartbeatLoading = true,
                networkHeartbeatMessage = "正在检测网络环境",
            )
        }
        runCatching {
            networkEnvironmentSelector.detect()
        }.onSuccess { endpoints ->
            _uiState.update {
                it.copy(
                    networkEndpoints = endpoints,
                    networkHeartbeatLoading = false,
                    networkHeartbeatMessage = "当前${endpoints.label}环境",
                )
            }
        }.onFailure { throwable ->
            val endpoints = NetworkEndpointPolicy.endpointsFor(NetworkEnvironment.External)
            _uiState.update {
                it.copy(
                    networkEndpoints = endpoints,
                    networkHeartbeatLoading = false,
                    networkHeartbeatMessage = throwable.message ?: "内网不可达，使用外网环境",
                )
            }
        }
    }

    private fun currentContentBaseUrl(): String {
        return _uiState.value.networkEndpoints.contentBaseUrl
    }

    private fun currentYuqingApi(session: SessionState) = ApiFactory.yuqing(currentContentBaseUrl(), session.token)

    private suspend fun loadArticleActionState(): ArticleActionState {
        val currentReadArticleIds = _uiState.value.readArticleIds
        val readArticleIds = articleUserActionDao.readArticleIds().toSet() + currentReadArticleIds
        val inactiveArticleIds = articleUserActionDao.inactiveArticleIds().toSet() + currentReadArticleIds
        return ArticleActionState(
            readArticleIds = readArticleIds,
            inactiveArticleIds = inactiveArticleIds,
        )
    }

    fun selectModule(key: String) {
        val wasSelected = _uiState.value.selectedModuleKey == key
        _uiState.update { it.copy(selectedModuleKey = key) }
        if (key == "dashboard" && wasSelected) {
            refreshAll()
        }
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
            val contentBaseUrl = currentContentBaseUrl()
            Log.i(
                STARTUP_TAG,
                "YuqingViewModel.refreshAll start selectedModule=${_uiState.value.selectedModuleKey} apiBaseUrl=$contentBaseUrl tokenPresent=${session.token.isNotBlank()}",
            )
            runCatching {
                val api = currentYuqingApi(session)
                val articleActions = loadArticleActionState()
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
                        .filterNot { article -> article.id in articleActions.inactiveArticleIds }
                    Log.i(
                        STARTUP_TAG,
                        "YuqingViewModel.refreshAll latestArticles loaded count=${latestArticles.size} firstPublishTime=${latestArticles.firstOrNull()?.publishTime.orEmpty()} firstCapturedAt=${latestArticles.firstOrNull()?.capturedAt.orEmpty()} firstTitle=${latestArticles.firstOrNull()?.title.orEmpty()} elapsedMs=${SystemClock.elapsedRealtime() - latestArticlesStartedAt}",
                    )
                    patchDashboardLatestArticles(dashboard, latestArticles)
                }.onFailure { throwable ->
                    Log.w(STARTUP_TAG, "YuqingViewModel.refreshAll latest article patch skipped", throwable)
                }.getOrDefault(dashboard)
                    .let { filterDashboardInactiveArticles(it, articleActions.inactiveArticleIds) }
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
                        readArticleIds = articleActions.readArticleIds,
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
                val result = currentYuqingApi(session).searchFull(keyword).data
                _uiState.update { it.copy(searchResult = result, message = "搜索完成") }
            }.onFailure { throwable ->
                _uiState.update { it.copy(error = throwable.message ?: "搜索失败") }
            }
            _uiState.update { it.copy(loading = false) }
        }
    }

    fun loadArticles(page: Int) {
        viewModelScope.launch {
            val articleActions = loadArticleActionState()
            _uiState.update {
                val fallback = filterInactiveArticles(fallbackArticleList(it), articleActions.inactiveArticleIds)
                it.copy(
                    loading = true,
                    articleLoading = true,
                    articleList = it.articleList ?: fallback,
                    readArticleIds = articleActions.readArticleIds,
                    error = "",
                    message = "",
                )
            }
            val session = sessionStore.state.first()
            runCatching {
                loadVisibleArticles(
                    api = currentYuqingApi(session),
                    page = page.coerceAtLeast(1),
                    inactiveArticleIds = articleActions.inactiveArticleIds,
                )
            }.onSuccess { result ->
                _uiState.update { it.copy(articleList = result, error = "", message = "") }
            }.onFailure { throwable ->
                _uiState.update {
                    val fallback = filterInactiveArticles(fallbackArticleList(it), articleActions.inactiveArticleIds)
                    it.copy(
                        articleList = fallback,
                        error = if (fallback == null) {
                            throwable.message ?: "文章加载失败"
                        } else {
                            "文章刷新失败，已显示缓存"
                        },
                        message = "",
                    )
                }
            }
            _uiState.update { it.copy(loading = false, articleLoading = false) }
        }
    }

    fun forceRefreshArticles() {
        loadArticles(1)
    }

    fun clearCache() {
        viewModelScope.launch {
            _uiState.update { it.copy(loading = true, error = "", message = "正在清除缓存") }
            runCatching {
                dashboardCacheDao.clear()
                articleUserActionDao.clearAll()
            }.onSuccess {
                _uiState.update {
                    it.copy(
                        dashboard = it.dashboard?.let { dashboard ->
                            filterDashboardInactiveArticles(dashboard, emptySet())
                        },
                        articleList = null,
                        readArticleIds = emptySet(),
                        message = "缓存已清除",
                    )
                }
                refreshAll()
            }.onFailure { throwable ->
                Log.w(STARTUP_TAG, "YuqingViewModel.clearCache failed", throwable)
                _uiState.update {
                    it.copy(
                        loading = false,
                        error = throwable.message ?: "清除缓存失败",
                        message = "",
                    )
                }
            }
        }
    }

    fun hideArticle(item: ArticleItem) {
        var shouldRefillDashboard = false
        _uiState.update {
            val nextDashboard = it.dashboard?.let { dashboard -> removeArticleFromDashboard(dashboard, item) }
            shouldRefillDashboard = nextDashboard != null &&
                nextDashboard.articles.items.size < DASHBOARD_VISIBLE_ARTICLE_LIMIT
            it.copy(
                articleList = removeArticleFromResult(it.articleList, item),
                dashboard = nextDashboard,
                readArticleIds = if (item.id > 0) it.readArticleIds + item.id else it.readArticleIds,
                articleDetail = it.articleDetail?.takeUnless { detail -> sameArticle(detail, item) },
            )
        }
        viewModelScope.launch {
            runCatching {
                if (item.id > 0) {
                    articleUserActionDao.upsert(
                        ArticleUserActionEntity(
                            articleId = item.id,
                            read = true,
                            hidden = true,
                            updatedAt = System.currentTimeMillis(),
                        ),
                    )
                }
            }.onFailure { throwable ->
                Log.w(STARTUP_TAG, "YuqingViewModel.hideArticle persist skipped", throwable)
            }
            if (shouldRefillDashboard) {
                refillDashboardArticlesIfNeeded(item.id.takeIf { id -> id > 0 })
            }
        }
    }

    fun clearArticleAction(item: ArticleItem) {
        if (item.id <= 0) {
            return
        }
        _uiState.update {
            it.copy(readArticleIds = it.readArticleIds - item.id)
        }
        viewModelScope.launch {
            runCatching {
                articleUserActionDao.upsert(
                    ArticleUserActionEntity(
                        articleId = item.id,
                        read = false,
                        hidden = false,
                        updatedAt = System.currentTimeMillis(),
                    ),
                )
            }.onFailure { throwable ->
                Log.w(STARTUP_TAG, "YuqingViewModel.clearArticleAction persist skipped", throwable)
            }
        }
    }

    private fun markArticleRead(item: ArticleItem) {
        if (item.id <= 0) {
            return
        }
        _uiState.update {
            it.copy(readArticleIds = it.readArticleIds + item.id)
        }
        viewModelScope.launch {
            runCatching {
                val existing = articleUserActionDao.get(item.id)
                articleUserActionDao.upsert(
                    ArticleUserActionEntity(
                        articleId = item.id,
                        read = true,
                        hidden = existing?.hidden ?: false,
                        updatedAt = System.currentTimeMillis(),
                    ),
                )
            }.onFailure { throwable ->
                Log.w(STARTUP_TAG, "YuqingViewModel.markArticleRead persist skipped", throwable)
            }
        }
    }

    private suspend fun refillDashboardArticlesIfNeeded(extraHiddenArticleId: Long?) {
        val dashboard = _uiState.value.dashboard ?: return
        if (dashboard.articles.items.size >= DASHBOARD_VISIBLE_ARTICLE_LIMIT) {
            return
        }
        val session = sessionStore.state.first()
        runCatching {
            val inactiveArticleIds = articleUserActionDao.inactiveArticleIds().toSet() +
                listOfNotNull(extraHiddenArticleId)
            val latestArticles = currentYuqingApi(session)
                .articles(page = 1, pageSize = DASHBOARD_REFILL_PAGE_SIZE)
                .data
                ?.items
                .orEmpty()
            mergeDashboardArticles(
                currentArticles = _uiState.value.dashboard?.articles?.items.orEmpty(),
                incomingArticles = latestArticles,
                inactiveArticleIds = inactiveArticleIds,
            )
        }.onSuccess { mergedArticles ->
            if (mergedArticles.isNotEmpty()) {
                _uiState.update {
                    val currentDashboard = it.dashboard ?: return@update it
                    it.copy(
                        dashboard = currentDashboard.copy(
                            articles = currentDashboard.articles.copy(items = mergedArticles),
                        ),
                    )
                }
            }
        }.onFailure { throwable ->
            Log.w(STARTUP_TAG, "YuqingViewModel.dashboard article refill skipped", throwable)
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
        markArticleRead(item)
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
                currentYuqingApi(session)
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

    fun openNextArticleDetail() {
        val state = _uiState.value
        val current = state.articleDetail ?: return
        val next = nextArticleAfter(current, articleDetailNavigationArticles(state))
        if (next == null) {
            _uiState.update { it.copy(message = "已经是最后一条新闻") }
            return
        }
        openArticleDetail(next)
    }

    fun openPreviousArticleDetail() {
        val state = _uiState.value
        val current = state.articleDetail ?: return
        val previous = previousArticleBefore(current, articleDetailNavigationArticles(state))
        if (previous == null) {
            _uiState.update { it.copy(message = "已经是第一条新闻") }
            return
        }
        openArticleDetail(previous)
    }

    fun loadAStockRecommendations(window: AStockRecommendationWindow = currentAStockRecommendationWindow()) {
        viewModelScope.launch {
            _uiState.update { it.copy(loading = true, error = "", message = "") }
            val session = sessionStore.state.first()
            runCatching {
                val result = currentYuqingApi(session)
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
                val api = currentYuqingApi(session)
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
                currentYuqingApi(session)
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
                val response = currentYuqingApi(session)
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
internal const val ARTICLE_PAGE_SIZE = 25
internal const val DASHBOARD_VISIBLE_ARTICLE_LIMIT = 5
internal const val DASHBOARD_ARTICLE_CACHE_LIMIT = 10
private const val DASHBOARD_REFILL_PAGE_SIZE = 50
private const val ARTICLE_REFILL_MAX_PAGES = 5
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

internal fun fallbackArticleList(state: YuqingUiState): ItemListResult? {
    state.articleList?.takeIf { it.items.isNotEmpty() }?.let { return it }
    return state.dashboard?.articles?.takeIf { it.items.isNotEmpty() }
}

private suspend fun loadVisibleArticles(
    api: YuqingApi,
    page: Int,
    inactiveArticleIds: Set<Long>,
): ItemListResult {
    val requestedPage = page.coerceAtLeast(1)
    var nextPage = requestedPage
    var firstResult: ItemListResult? = null
    var visibleArticles = emptyList<ArticleItem>()
    var loadedPages = 0
    while (visibleArticles.size < ARTICLE_PAGE_SIZE && loadedPages < ARTICLE_REFILL_MAX_PAGES) {
        val result = api.articles(page = nextPage, pageSize = ARTICLE_PAGE_SIZE)
            .data ?: error("文章数据为空")
        if (firstResult == null) {
            firstResult = result
        }
        visibleArticles = appendVisibleArticles(
            currentArticles = visibleArticles,
            incomingArticles = result.items,
            inactiveArticleIds = inactiveArticleIds,
            limit = ARTICLE_PAGE_SIZE,
        )
        loadedPages += 1
        if (result.items.isEmpty() || result.page * result.pageSize >= result.total) {
            break
        }
        nextPage += 1
    }
    return (firstResult ?: error("文章数据为空")).copy(
        items = visibleArticles,
        page = requestedPage,
        pageSize = ARTICLE_PAGE_SIZE,
    )
}

internal fun appendVisibleArticles(
    currentArticles: List<ArticleItem>,
    incomingArticles: List<ArticleItem>,
    inactiveArticleIds: Set<Long>,
    limit: Int,
): List<ArticleItem> {
    return (currentArticles + incomingArticles)
        .filterNot { article -> article.id in inactiveArticleIds }
        .distinctBy(::articleIdentity)
        .take(limit.coerceAtLeast(1))
}

internal fun filterHiddenArticles(result: ItemListResult?, hiddenArticleIds: Set<Long>): ItemListResult? {
    return filterInactiveArticles(result, hiddenArticleIds)
}

internal fun filterInactiveArticles(result: ItemListResult?, inactiveArticleIds: Set<Long>): ItemListResult? {
    if (result == null || inactiveArticleIds.isEmpty()) {
        return result
    }
    return result.copy(items = result.items.filterNot { article -> article.id in inactiveArticleIds })
}

private fun filterDashboardInactiveArticles(dashboard: AndroidDashboard, inactiveArticleIds: Set<Long>): AndroidDashboard {
    if (inactiveArticleIds.isEmpty()) {
        return dashboard
    }
    return dashboard.copy(
        articles = filterInactiveArticles(dashboard.articles, inactiveArticleIds) ?: dashboard.articles,
    )
}

private fun removeArticleFromDashboard(dashboard: AndroidDashboard, item: ArticleItem): AndroidDashboard {
    return dashboard.copy(
        articles = removeArticleFromResult(dashboard.articles, item) ?: dashboard.articles,
    )
}

private fun removeArticleFromResult(result: ItemListResult?, item: ArticleItem): ItemListResult? {
    if (result == null) {
        return null
    }
    return result.copy(items = result.items.filterNot { article -> sameArticle(article, item) })
}

private fun sameArticle(left: ArticleItem, right: ArticleItem): Boolean {
    if (left.id > 0 && right.id > 0) {
        return left.id == right.id
    }
    return left.title == right.title &&
        left.sourceUrl == right.sourceUrl &&
        left.capturedAt == right.capturedAt
}

internal fun articleDetailNavigationArticles(state: YuqingUiState): List<ArticleItem> {
    val current = state.articleDetail
    val sources = listOf(
        state.articleList?.items.orEmpty(),
        state.dashboard?.articles?.items.orEmpty(),
    )
    if (current != null) {
        sources.firstOrNull { articles -> articles.any { sameArticle(it, current) } }?.let { return it }
    }
    return sources.flatten().distinctBy(::articleIdentity)
}

internal fun nextArticleAfter(current: ArticleItem, candidates: List<ArticleItem>): ArticleItem? {
    val currentIndex = candidates.indexOfFirst { sameArticle(it, current) }
    if (currentIndex < 0) {
        return candidates.firstOrNull { !sameArticle(it, current) }
    }
    return candidates.drop(currentIndex + 1).firstOrNull { !sameArticle(it, current) }
}

internal fun previousArticleBefore(current: ArticleItem, candidates: List<ArticleItem>): ArticleItem? {
    val currentIndex = candidates.indexOfFirst { sameArticle(it, current) }
    if (currentIndex <= 0) {
        return null
    }
    return candidates.take(currentIndex).lastOrNull { !sameArticle(it, current) }
}

private fun patchDashboardLatestArticles(dashboard: AndroidDashboard, latestArticles: List<ArticleItem>): AndroidDashboard {
    val patchedItems = mergeDashboardArticles(
        currentArticles = emptyList(),
        incomingArticles = latestArticles,
        inactiveArticleIds = emptySet(),
    )
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

internal fun mergeDashboardArticles(
    currentArticles: List<ArticleItem>,
    incomingArticles: List<ArticleItem>,
    inactiveArticleIds: Set<Long>,
    referenceNow: Instant = Instant.now(),
    limit: Int = DASHBOARD_ARTICLE_CACHE_LIMIT,
): List<ArticleItem> {
    return (currentArticles + incomingArticles)
        .filterNot { article -> article.id in inactiveArticleIds }
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
        .take(limit.coerceAtLeast(1))
}

private fun articleIdentity(item: ArticleItem): String {
    if (item.id > 0) {
        return "id:${item.id}"
    }
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
    private val articleUserActionDao: ArticleUserActionDao,
) : ViewModelProvider.Factory {
    @Suppress("UNCHECKED_CAST")
    override fun <T : ViewModel> create(modelClass: Class<T>): T {
        return YuqingViewModel(sessionStore, dashboardCacheDao, articleUserActionDao) as T
    }
}
