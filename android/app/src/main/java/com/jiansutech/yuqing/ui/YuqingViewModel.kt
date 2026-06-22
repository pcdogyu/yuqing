package com.jiansutech.yuqing.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.jiansutech.yuqing.data.AndroidActionRequest
import com.jiansutech.yuqing.data.AndroidDashboard
import com.jiansutech.yuqing.data.AndroidModule
import com.jiansutech.yuqing.data.AStockRecommendation
import com.jiansutech.yuqing.data.AStockRecommendationSnapshot
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
import java.time.LocalDate
import java.time.LocalTime
import java.time.ZoneId

data class PendingAction(
    val action: String,
    val title: String,
    val params: Map<String, String> = emptyMap(),
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
    val error: String = "",
    val message: String = "",
    val modules: List<AndroidModule> = emptyList(),
    val selectedModuleKey: String = "dashboard",
    val dashboard: AndroidDashboard? = null,
    val articleList: ItemListResult? = null,
    val aStockRecommendation: AStockRecommendationSnapshot? = null,
    val aStockRecommendations: List<AStockRecommendation> = emptyList(),
    val aStockRecommendationWindow: AStockRecommendationWindow = currentAStockRecommendationWindow(),
    val searchKeyword: String = "",
    val searchResult: SearchResult? = null,
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
            val cached = dashboardCacheDao.get()?.payload
            if (!cached.isNullOrBlank()) {
                runCatching {
                    ApiFactory.json.decodeFromString<AndroidDashboard>(cached)
                }.onSuccess { dashboard ->
                    _uiState.update {
                        it.copy(
                            dashboard = dashboard,
                            aStockRecommendation = dashboard.aStock.recommendation.takeIf { snapshot -> snapshot.found },
                            aStockRecommendations = parseAStockRecommendations(dashboard.aStock.recommendation.recommendationsJson),
                        )
                    }
                }
            }
            var refreshed = false
            sessionStore.state.collect { session ->
                _uiState.update { it.copy(session = session) }
                if (!refreshed) {
                    refreshed = true
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
        if (key == "a_stock" && _uiState.value.aStockRecommendation == null) {
            loadAStockRecommendations()
        }
    }

    fun updateSearchKeyword(value: String) {
        _uiState.update { it.copy(searchKeyword = value) }
    }

    fun refreshAll() {
        viewModelScope.launch {
            _uiState.update { it.copy(loading = true, error = "", message = "") }
            val session = sessionStore.state.first()
            runCatching {
                val api = ApiFactory.yuqing(session.apiBaseUrl, session.token)
                val bootstrap = api.bootstrap().data
                val dashboard = api.dashboard().data ?: error("Dashboard 数据为空")
                dashboardCacheDao.upsert(
                    DashboardCacheEntity(
                        payload = ApiFactory.json.encodeToString(dashboard),
                        savedAt = System.currentTimeMillis(),
                    ),
                )
                _uiState.update {
                    it.copy(
                        modules = bootstrap?.modules.orEmpty(),
                        dashboard = dashboard,
                        articleList = if (it.selectedModuleKey == "articles") it.articleList else null,
                        aStockRecommendation = dashboard.aStock.recommendation.takeIf { snapshot -> snapshot.found },
                        aStockRecommendations = parseAStockRecommendations(dashboard.aStock.recommendation.recommendationsJson),
                        message = "数据已刷新",
                    )
                }
            }.onFailure { throwable ->
                _uiState.update { it.copy(error = throwable.message ?: "刷新失败") }
            }
            _uiState.update { it.copy(loading = false) }
            if (_uiState.value.selectedModuleKey == "articles") {
                loadArticles(1)
            }
            if (_uiState.value.selectedModuleKey == "a_stock") {
                loadAStockRecommendations()
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
            _uiState.update { it.copy(loading = true, error = "", message = "") }
            val session = sessionStore.state.first()
            runCatching {
                val result = ApiFactory.yuqing(session.apiBaseUrl, session.token)
                    .articles(page = page.coerceAtLeast(1), pageSize = 10)
                    .data ?: error("文章数据为空")
                _uiState.update { it.copy(articleList = result, message = "文章已加载") }
            }.onFailure { throwable ->
                _uiState.update { it.copy(error = throwable.message ?: "文章加载失败") }
            }
            _uiState.update { it.copy(loading = false) }
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

private fun currentAStockRecommendationWindow(): AStockRecommendationWindow {
    val zone = ZoneId.of("Asia/Shanghai")
    val now = LocalTime.now(zone)
    val period = if (now.isBefore(LocalTime.of(9, 31))) "morning" else "afternoon"
    return aStockRecommendationWindow(
        date = LocalDate.now(zone).toString(),
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

class YuqingViewModelFactory(
    private val sessionStore: SessionStore,
    private val dashboardCacheDao: DashboardCacheDao,
) : ViewModelProvider.Factory {
    @Suppress("UNCHECKED_CAST")
    override fun <T : ViewModel> create(modelClass: Class<T>): T {
        return YuqingViewModel(sessionStore, dashboardCacheDao) as T
    }
}
