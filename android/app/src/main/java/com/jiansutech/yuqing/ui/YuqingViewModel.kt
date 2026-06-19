package com.jiansutech.yuqing.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.jiansutech.yuqing.data.AndroidActionRequest
import com.jiansutech.yuqing.data.AndroidDashboard
import com.jiansutech.yuqing.data.AndroidModule
import com.jiansutech.yuqing.data.ApiFactory
import com.jiansutech.yuqing.data.DashboardCacheDao
import com.jiansutech.yuqing.data.DashboardCacheEntity
import com.jiansutech.yuqing.data.LoginRequest
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

data class PendingAction(
    val action: String,
    val title: String,
    val params: Map<String, String> = emptyMap(),
)

data class YuqingUiState(
    val session: SessionState = SessionState(),
    val loading: Boolean = false,
    val error: String = "",
    val message: String = "",
    val modules: List<AndroidModule> = emptyList(),
    val selectedModuleKey: String = "dashboard",
    val dashboard: AndroidDashboard? = null,
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
                    _uiState.update { it.copy(dashboard = dashboard) }
                }
            }
            sessionStore.state.collect { session ->
                val wasLoggedOut = !_uiState.value.session.loggedIn
                _uiState.update { it.copy(session = session) }
                if (session.loggedIn && wasLoggedOut) {
                    refreshAll()
                }
            }
        }
    }

    fun selectModule(key: String) {
        _uiState.update { it.copy(selectedModuleKey = key) }
    }

    fun updateSearchKeyword(value: String) {
        _uiState.update { it.copy(searchKeyword = value) }
    }

    fun login(username: String, password: String, authBaseUrl: String, apiBaseUrl: String) {
        viewModelScope.launch {
            _uiState.update { it.copy(loading = true, error = "", message = "") }
            runCatching {
                sessionStore.saveServers(authBaseUrl, apiBaseUrl)
                val response = ApiFactory.auth(authBaseUrl).login(LoginRequest(username.trim(), password))
                val data = response.data ?: error(response.message.ifBlank { "登录失败" })
                if (data.sessionToken.isBlank()) error("登录响应缺少 session token")
                sessionStore.saveLogin(data.sessionToken, data.user.username.ifBlank { username.trim() }, authBaseUrl, apiBaseUrl)
            }.onFailure { throwable ->
                _uiState.update { it.copy(error = throwable.message ?: "登录失败") }
            }
            _uiState.update { it.copy(loading = false) }
        }
    }

    fun logout() {
        viewModelScope.launch {
            sessionStore.clear()
            _uiState.update { YuqingUiState(session = it.session.copy(token = "", username = "")) }
        }
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
                        message = "数据已刷新",
                    )
                }
            }.onFailure { throwable ->
                _uiState.update { it.copy(error = throwable.message ?: "刷新失败") }
            }
            _uiState.update { it.copy(loading = false) }
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

class YuqingViewModelFactory(
    private val sessionStore: SessionStore,
    private val dashboardCacheDao: DashboardCacheDao,
) : ViewModelProvider.Factory {
    @Suppress("UNCHECKED_CAST")
    override fun <T : ViewModel> create(modelClass: Class<T>): T {
        return YuqingViewModel(sessionStore, dashboardCacheDao) as T
    }
}
