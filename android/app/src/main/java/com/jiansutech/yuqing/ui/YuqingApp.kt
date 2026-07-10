package com.jiansutech.yuqing.ui

import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.pdf.PdfRenderer
import android.net.Uri
import android.os.ParcelFileDescriptor
import android.os.SystemClock
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.gestures.detectDragGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.OpenInNew
import androidx.compose.material.icons.filled.Article
import androidx.compose.material.icons.filled.Assessment
import androidx.compose.material.icons.filled.Business
import androidx.compose.material.icons.filled.Dashboard
import androidx.compose.material.icons.filled.Description
import androidx.compose.material.icons.filled.Groups
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.ShowChart
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SwipeToDismissBox
import androidx.compose.material3.SwipeToDismissBoxValue
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.rememberSwipeToDismissBoxState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.graphics.vector.path
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import kotlin.math.abs
import com.jiansutech.yuqing.BuildConfig
import com.jiansutech.yuqing.astock.AStockTradingCalendar
import com.jiansutech.yuqing.data.AndroidDashboard
import com.jiansutech.yuqing.data.AndroidModule
import com.jiansutech.yuqing.data.AStockAuctionAmount
import com.jiansutech.yuqing.data.AStockAuctionListResult
import com.jiansutech.yuqing.data.AStockBacktestRow
import com.jiansutech.yuqing.data.AStockRecommendation
import com.jiansutech.yuqing.data.AStockRecommendationSnapshot
import com.jiansutech.yuqing.data.ArticleItem
import com.jiansutech.yuqing.data.ApiFactory
import com.jiansutech.yuqing.data.ItemListResult
import com.jiansutech.yuqing.data.Project
import com.jiansutech.yuqing.data.Report
import com.jiansutech.yuqing.data.SchedulerJob
import com.jiansutech.yuqing.data.ServiceStatus
import com.jiansutech.yuqing.data.StockHolding
import com.jiansutech.yuqing.data.StockResearch
import com.jiansutech.yuqing.data.TaskRun
import kotlinx.coroutines.delay
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.decodeFromString
import java.io.File
import java.time.DayOfWeek
import java.time.LocalDate
import java.time.ZoneId

@Composable
fun YuqingApp(
    viewModel: YuqingViewModel,
    versionUpgradeState: VersionUpgradeUiState = VersionUpgradeUiState(),
    onCheckUpgrade: () -> Unit = {},
) {
    val state by viewModel.uiState.collectAsState()
    YuqingTheme {
        PortalScreen(state, viewModel, versionUpgradeState, onCheckUpgrade)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun PortalScreen(
    state: YuqingUiState,
    viewModel: YuqingViewModel,
    versionUpgradeState: VersionUpgradeUiState,
    onCheckUpgrade: () -> Unit,
) {
    val modules = state.modules.ifEmpty { fallbackModules() }
    val fallback = fallbackModules()
    val selected = modules.firstOrNull { it.key == state.selectedModuleKey }
        ?: fallback.firstOrNull { it.key == state.selectedModuleKey }
        ?: modules.first()
    var backtestDetail by remember { mutableStateOf<AStockBacktestDetailState?>(null) }
    var lastArticleTabClickAt by remember { mutableStateOf(0L) }
    var bottomNavSecretTapState by remember { mutableStateOf(BottomNavSecretTapState()) }
    var bottomNavLockedUntil by remember { mutableStateOf(0L) }
    LaunchedEffect(bottomNavLockedUntil) {
        val remaining = bottomNavLockedUntil - SystemClock.elapsedRealtime()
        if (remaining > 0) {
            delay(remaining)
            bottomNavLockedUntil = 0L
        }
    }
    val detail = backtestDetail
    val articleDetail = state.articleDetail
    val stockResearchDetail = state.stockResearchDetail
    BackHandler(enabled = detail != null || articleDetail != null || stockResearchDetail != null) {
        if (detail != null) {
            backtestDetail = null
        } else if (articleDetail != null) {
            viewModel.closeArticleDetail()
        } else {
            viewModel.closeStockResearchDetail()
        }
    }
    fun refreshBacktestDetailPrice(target: AStockBacktestDetailState) {
        viewModel.refreshAStockBacktestPrice(
            date = target.strategyDate,
            period = target.period,
            code = target.recommendation.code,
        ) { snapshot ->
            backtestDetail = backtestDetail?.let { current ->
                if (sameAStockBacktestDetailTarget(current, target)) {
                    applyAStockBacktestDetailSnapshot(current, snapshot)
                } else {
                    current
                }
            }
        }
    }
    LaunchedEffect(detail?.strategyDate, detail?.period, detail?.recommendation?.code) {
        detail?.let { refreshBacktestDetailPrice(it) }
    }
    val hideTopBar = selected.key == "dashboard" ||
        selected.key == "articles" ||
        selected.key == "search" ||
        selected.key == "a_stock" ||
        selected.key == "auction" ||
        selected.key == "stock_research" ||
        selected.key == "system"
    val hideModuleStrip = hideTopBar || selected.key == "system"
    Scaffold(
        topBar = if (detail != null) {
            {
                TopAppBar(
                    title = { Text("${detail.recommendation.code} ${detail.recommendation.name}", maxLines = 1, overflow = TextOverflow.Ellipsis) },
                    navigationIcon = {
                        IconButton(onClick = { backtestDetail = null }) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回")
                        }
                    },
                )
            }
        } else if (articleDetail != null) {
            {
                TopAppBar(
                    title = { Text("新闻详情", maxLines = 1, overflow = TextOverflow.Ellipsis) },
                    navigationIcon = {
                        IconButton(onClick = viewModel::closeArticleDetail) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回")
                        }
                    },
                )
            }
        } else if (stockResearchDetail != null) {
            {
                TopAppBar(
                    title = { Text("${stockResearchSourceLabel(stockResearchDetail)}详情", maxLines = 1, overflow = TextOverflow.Ellipsis) },
                    navigationIcon = {
                        IconButton(onClick = viewModel::closeStockResearchDetail) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "返回")
                        }
                    },
                )
            }
        } else if (hideTopBar) {
            {}
        } else {
            {
            TopAppBar(
                title = {
                    Column {
                        Text(selected.title, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text(state.session.username, style = MaterialTheme.typography.labelMedium)
                    }
                },
                actions = {
                    IconButton(onClick = viewModel::refreshAll) { Icon(Icons.Default.Refresh, contentDescription = "刷新") }
                },
            )
            }
        },
        bottomBar = {
            if (detail == null && articleDetail == null && stockResearchDetail == null) {
                val bottomNavLocked = isBottomNavLocked(bottomNavLockedUntil, SystemClock.elapsedRealtime())
                NavigationBar(modifier = Modifier.alpha(bottomNavAlpha(bottomNavLocked))) {
                    bottomNavigationKeys.forEach { key ->
                        val module = modules.firstOrNull { it.key == key }
                            ?: fallback.firstOrNull { it.key == key }
                            ?: AndroidModule(key = key, title = key)
                        val navTitle = bottomNavigationTitle(key, module.title)
                        NavigationBarItem(
                            selected = selected.key == key,
                            enabled = !bottomNavLocked,
                            onClick = {
                                val now = SystemClock.elapsedRealtime()
                                if (!isBottomNavLocked(bottomNavLockedUntil, now)) {
                                    val secretTap = nextBottomNavSecretTapState(bottomNavSecretTapState, key, now)
                                    bottomNavSecretTapState = secretTap.state
                                    if (secretTap.unlocked) {
                                        bottomNavLockedUntil = bottomNavLockUntilAfterSystemUnlock(now)
                                        viewModel.selectModule("system")
                                    } else {
                                        if (isArticleTabDoubleClick(key, selected.key, lastArticleTabClickAt, now)) {
                                            viewModel.forceRefreshArticles()
                                        } else {
                                            viewModel.selectModule(key)
                                        }
                                        if (key == "articles") {
                                            lastArticleTabClickAt = now
                                        }
                                    }
                                }
                            },
                            icon = { Icon(moduleIcon(key), contentDescription = navTitle) },
                            label = { Text(navTitle, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                        )
                    }
                }
            }
        },
    ) { padding ->
        val contentModifier = if (detail != null || articleDetail != null || stockResearchDetail != null) {
            Modifier.padding(padding).fillMaxSize()
        } else if (hideTopBar) {
            Modifier.padding(padding).fillMaxSize().statusBarsPadding()
        } else {
            Modifier.padding(padding).fillMaxSize()
        }
        Column(contentModifier) {
            if (!hideModuleStrip) {
                StatusMessages(state)
                ModuleStrip(modules, selected.key, viewModel)
            } else if (!hideTopBar) {
                StatusMessages(state)
            }
            if (state.loading) {
                Row(Modifier.fillMaxWidth().padding(12.dp), horizontalArrangement = Arrangement.Center) {
                    CircularProgressIndicator()
                }
            }
            if (detail != null) {
                AStockBacktestDetailScreen(
                    state = detail,
                    onSwipeDown = {
                        backtestDetail = adjacentAStockBacktestDetail(detail, 1) ?: detail
                    },
                    onSwipeUp = {
                        backtestDetail = adjacentAStockBacktestDetail(detail, -1) ?: detail
                    },
                    onPreviousStock = {
                        backtestDetail = adjacentAStockBacktestDetail(detail, -1) ?: detail
                    },
                    onNextStock = {
                        backtestDetail = adjacentAStockBacktestDetail(detail, 1) ?: detail
                    },
                    refreshingPrice = state.aStockBacktestPriceRefreshing,
                    onRefreshPrice = {
                        refreshBacktestDetailPrice(detail)
                    },
                )
            } else if (articleDetail != null) {
                ArticleDetailScreen(
                    item = articleDetail,
                    loading = state.articleDetailLoading,
                    error = state.articleDetailError,
                    onSwipeLeft = viewModel::closeArticleDetail,
                    onSwipeDown = viewModel::openNextArticleDetail,
                    onSwipeUp = viewModel::openPreviousArticleDetail,
                )
            } else if (stockResearchDetail != null) {
                StockResearchDetailScreen(
                    item = stockResearchDetail,
                    loading = state.stockResearchDetailLoading,
                    error = state.stockResearchDetailError,
                    pdfState = state.stockResearchPdf,
                    onDownloadPdf = { viewModel.downloadStockResearchPdf(force = false) },
                    onRedownloadPdf = { viewModel.downloadStockResearchPdf(force = true) },
                    onPdfError = viewModel::reportStockResearchPdfError,
                )
            } else {
                ModuleContent(
                    key = selected.key,
                    dashboard = state.dashboard,
                    state = state,
                    viewModel = viewModel,
                    versionUpgradeState = versionUpgradeState,
                    onCheckUpgrade = onCheckUpgrade,
                    onOpenAStockBacktest = { backtestDetail = it },
                )
            }
        }
    }
    state.pendingAction?.let { pending ->
        AlertDialog(
            onDismissRequest = viewModel::dismissAction,
            title = { Text("确认操作") },
            text = { Text("将执行：${pending.title}") },
            confirmButton = { Button(onClick = viewModel::runPendingAction) { Text("执行") } },
            dismissButton = { TextButton(onClick = viewModel::dismissAction) { Text("取消") } },
        )
    }
}

internal const val ARTICLE_TAB_DOUBLE_CLICK_MS = 400L
internal const val BOTTOM_NAV_SYSTEM_UNLOCK_TAPS = 10
internal const val BOTTOM_NAV_SYSTEM_UNLOCK_WINDOW_MS = 5_000L
internal const val BOTTOM_NAV_SYSTEM_LOCK_MS = 3_000L

private val bottomNavigationKeys = listOf("dashboard", "articles", "a_stock", "stock_research", "auction")
private val bottomNavigationTitles = mapOf(
    "dashboard" to "总览",
    "articles" to "文章",
    "a_stock" to "A股",
    "stock_research" to "研报",
    "auction" to "集合",
)

private fun bottomNavigationTitle(key: String, fallback: String): String = bottomNavigationTitles[key] ?: fallback

internal data class BottomNavSecretTapState(
    val key: String = "",
    val count: Int = 0,
    val lastClickAt: Long = 0L,
)

internal data class BottomNavSecretTapResult(
    val state: BottomNavSecretTapState,
    val unlocked: Boolean,
)

internal fun isArticleTabDoubleClick(
    clickedKey: String,
    currentKey: String,
    lastArticleTabClickAt: Long,
    now: Long,
): Boolean {
    return clickedKey == "articles" &&
        currentKey == "articles" &&
        lastArticleTabClickAt > 0 &&
        now - lastArticleTabClickAt in 0..ARTICLE_TAB_DOUBLE_CLICK_MS
}

internal fun nextBottomNavSecretTapState(
    current: BottomNavSecretTapState,
    clickedKey: String,
    now: Long,
    targetTaps: Int = BOTTOM_NAV_SYSTEM_UNLOCK_TAPS,
    windowMillis: Long = BOTTOM_NAV_SYSTEM_UNLOCK_WINDOW_MS,
): BottomNavSecretTapResult {
    val isConsecutive = current.key == clickedKey &&
        current.lastClickAt > 0 &&
        now - current.lastClickAt in 0..windowMillis
    val nextCount = if (isConsecutive) current.count + 1 else 1
    if (nextCount >= targetTaps) {
        return BottomNavSecretTapResult(BottomNavSecretTapState(), true)
    }
    return BottomNavSecretTapResult(
        BottomNavSecretTapState(
            key = clickedKey,
            count = nextCount,
            lastClickAt = now,
        ),
        false,
    )
}

internal fun bottomNavLockUntilAfterSystemUnlock(
    now: Long,
    lockMillis: Long = BOTTOM_NAV_SYSTEM_LOCK_MS,
): Long = now + lockMillis

internal fun isBottomNavLocked(lockedUntil: Long, now: Long): Boolean = now < lockedUntil

internal fun bottomNavAlpha(locked: Boolean): Float = if (locked) 0.38f else 1f

@Composable
private fun StatusMessages(state: YuqingUiState) {
    if (state.error.isNotBlank()) {
        Text(state.error, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(vertical = 8.dp))
    }
    if (state.message.isNotBlank()) {
        Text(state.message, color = MaterialTheme.colorScheme.primary, modifier = Modifier.padding(vertical = 8.dp))
    }
}

@Composable
private fun ModuleStrip(modules: List<AndroidModule>, selectedKey: String, viewModel: YuqingViewModel) {
    LazyRow(
        modifier = Modifier.height(64.dp),
        contentPadding = PaddingValues(horizontal = 12.dp, vertical = 8.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        items(modules) { module ->
            FilterChip(
                selected = module.key == selectedKey,
                onClick = { viewModel.selectModule(module.key) },
                label = { Text(module.title, maxLines = 1) },
            )
        }
    }
}

@Composable
private fun ModuleContent(
    key: String,
    dashboard: AndroidDashboard?,
    state: YuqingUiState,
    viewModel: YuqingViewModel,
    versionUpgradeState: VersionUpgradeUiState = VersionUpgradeUiState(),
    onCheckUpgrade: () -> Unit = {},
    onOpenAStockBacktest: (AStockBacktestDetailState) -> Unit = {},
) {
    if (dashboard == null) {
        EmptyState("暂无缓存数据，请刷新")
        return
    }
    when (key) {
        "dashboard" -> DashboardModule(dashboard, state.readArticleIds, viewModel)
        "projects" -> ProjectsModule(dashboard.projects, dashboard.rules)
        "articles" -> ArticlesModule(state.articleList, state.articleLoading, state.error, state.readArticleIds, viewModel)
        "search" -> SearchModule(state, viewModel)
        "auction" -> AStockAuctionModule(state.aStockAuction, state.aStockAuctionTrendDays, viewModel)
        "analysis" -> AnalysisModule(dashboard)
        "reports" -> ReportsModule(dashboard.reports, viewModel)
        "a_stock" -> AStockModule(state, viewModel, onOpenAStockBacktest)
        "stock_research" -> StockResearchModule(state, viewModel)
        "holdings" -> HoldingsModule(dashboard.holdings.items)
        "system" -> SystemModule(dashboard, state, viewModel, versionUpgradeState, onCheckUpgrade)
        else -> GenericModule(key, dashboard)
    }
}

@Composable
private fun DashboardModule(
    dashboard: AndroidDashboard,
    readArticleIds: Set<Long>,
    viewModel: YuqingViewModel,
) {
    val latestArticles = dashboard.articles.items.take(5)
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        item {
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier.fillMaxWidth(),
            ) {
                MetricCard("文章", dashboard.overview.articleCount.toString(), Modifier.weight(1f))
                MetricCard("任务", dashboard.overview.crawlRunCount.toString(), Modifier.weight(1f))
                MetricCard(
                    title = "清除",
                    value = "缓存",
                    onClick = { viewModel.clearCache() },
                    icon = Icons.Default.Refresh,
                    modifier = Modifier.weight(1f),
                )
            }
        }
        item { SectionTitle("最新文章") }
        items(latestArticles, key = { articleStableKey(it) }) {
            SwipeHiddenArticleRow(
                item = it,
                read = it.id in readArticleIds,
                onClick = { viewModel.openArticleDetail(it) },
                onSwipeRight = { article -> viewModel.hideArticle(article) },
                onSwipeLeft = { article -> viewModel.hideArticle(article) },
            )
        }
    }
}

@Composable
private fun ProjectsModule(projects: List<Project>, rules: List<com.jiansutech.yuqing.data.MonitorRule>) {
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        item { SectionTitle("项目") }
        items(projects) { SimpleRow(it.name, "${it.status} ${it.keywords}") }
        item { SectionTitle("监测规则") }
        items(rules) { SimpleRow(it.name, "${it.status} ${it.includeKeywords}") }
    }
}

@Composable
private fun ArticlesModule(
    result: ItemListResult?,
    loading: Boolean,
    error: String,
    readArticleIds: Set<Long>,
    viewModel: YuqingViewModel,
) {
    if (result == null || result.items.isEmpty()) {
        LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            item {
                EmptyState(
                    when {
                        loading -> "新闻获取中"
                        error.isNotBlank() -> "新闻获取中，请检查网络连接"
                        else -> "暂无最新新闻"
                    },
                )
            }
            item {
                Row(modifier = Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.Center) {
                    TextButton(onClick = { viewModel.loadArticles(1) }, enabled = !loading) {
                        Text(if (loading) "获取中..." else "重新获取")
                    }
                }
            }
        }
        return
    }
    val pageSize = result.pageSize.coerceAtLeast(1)
    val canGoPrevious = result.page > 1
    val canGoNext = result.page * pageSize < result.total
    val totalPages = if (result.total <= 0) 1 else ((result.total + pageSize - 1) / pageSize).coerceAtLeast(1)
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        if (error.isNotBlank()) {
            item {
                Text(error, color = MaterialTheme.colorScheme.primary, style = MaterialTheme.typography.bodySmall)
            }
        }
        items(result.items, key = { articleStableKey(it) }) {
            SwipeHiddenArticleRow(
                item = it,
                read = it.id in readArticleIds,
                onClick = { viewModel.openArticleDetail(it) },
                onSwipeRight = { article -> viewModel.hideArticle(article) },
                onSwipeLeft = { article -> viewModel.clearArticleAction(article) },
            )
        }
        item {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Button(onClick = { viewModel.loadArticles(result.page - 1) }, enabled = canGoPrevious) {
                    Text("上一页")
                }
                Text("第 ${result.page} / $totalPages 页", style = MaterialTheme.typography.bodySmall)
                Button(onClick = { viewModel.loadArticles(result.page + 1) }, enabled = canGoNext) {
                    Text("下一页")
                }
            }
        }
    }
}

@Composable
private fun SearchModule(state: YuqingUiState, viewModel: YuqingViewModel) {
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        item {
            Row(verticalAlignment = Alignment.CenterVertically) {
                OutlinedTextField(
                    value = state.searchKeyword,
                    onValueChange = viewModel::updateSearchKeyword,
                    label = { Text("全文搜索") },
                    modifier = Modifier.weight(1f),
                    singleLine = true,
                )
                Spacer(Modifier.width(8.dp))
                IconButton(onClick = viewModel::search) { Icon(Icons.Default.Search, contentDescription = "搜索") }
            }
        }
        state.searchResult?.let { result ->
            item { SectionTitle("搜索结果 ${result.total}") }
            items(result.items) { ArticleRow(it, onClick = { viewModel.openArticleDetail(it) }) }
        }
    }
}

@Composable
private fun AnalysisModule(dashboard: AndroidDashboard) {
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        item { SectionTitle("舆情概览") }
        item { SimpleRow("系统状态", if (dashboard.operations.ready) "ready" else "needs attention") }
        item { SimpleRow("服务数量", dashboard.operations.services.size.toString()) }
        item { SimpleRow("调度任务", dashboard.operations.schedulerJobs.size.toString()) }
        items(dashboard.notices) { SimpleRow(it.title, it.content) }
    }
}

@Composable
private fun ReportsModule(reports: List<Report>, viewModel: YuqingViewModel) {
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        item {
            Button(onClick = { viewModel.requestAction("refresh_analysis", "刷新分析") }) {
                Text("刷新分析")
            }
        }
        items(reports) { SimpleRow(it.title, "${it.status} ${it.summary}") }
    }
}

@Composable
private fun AStockModule(
    state: YuqingUiState,
    viewModel: YuqingViewModel,
    onOpenAStockBacktest: (AStockBacktestDetailState) -> Unit,
) {
    val window = state.aStockRecommendationWindow
    val morningSnapshot = state.morningAStockRecommendation
    val morningRecommendations = state.morningAStockRecommendations
    val afternoonSnapshot = state.afternoonAStockRecommendation
    val afternoonRecommendations = state.afternoonAStockRecommendations
    val morningBacktests = remember(morningSnapshot?.backtestsJson) { parseAStockBacktests(morningSnapshot?.backtestsJson) }
    val afternoonBacktests = remember(afternoonSnapshot?.backtestsJson) { parseAStockBacktests(afternoonSnapshot?.backtestsJson) }
    val isLatestDate = isLatestSelectableAStockDate(window.date)
    val dateLabel = formatAStockDateLabelParts(window.date)
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        item {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Box(
                    modifier = Modifier.weight(1f),
                    contentAlignment = Alignment.CenterStart,
                ) {
                    TextButton(onClick = { viewModel.shiftAStockRecommendationDate(-1) }) {
                        Text("前一日")
                    }
                }
                Column(
                    modifier = Modifier.weight(1f),
                    horizontalAlignment = Alignment.CenterHorizontally,
                ) {
                    Text(
                        dateLabel.date,
                        fontWeight = FontWeight.SemiBold,
                        textAlign = TextAlign.Center,
                        maxLines = 1,
                    )
                    if (dateLabel.weekday.isNotBlank()) {
                        Text(
                            dateLabel.weekday,
                            fontWeight = FontWeight.SemiBold,
                            textAlign = TextAlign.Center,
                            maxLines = 1,
                            style = MaterialTheme.typography.bodyMedium,
                        )
                    }
                }
                Box(
                    modifier = Modifier.weight(1f),
                    contentAlignment = Alignment.CenterEnd,
                ) {
                    if (!isLatestDate) {
                        TextButton(onClick = { viewModel.shiftAStockRecommendationDate(1) }) {
                            Text("后一日")
                        }
                    }
                }
            }
        }
        item {
            SimpleRow(
                "当日全部推荐 ${window.date}",
                listOf(
                    "上午 ${morningRecommendations.size}",
                    "下午 ${afternoonRecommendations.size}",
                    "合计 ${morningRecommendations.size + afternoonRecommendations.size}",
                ).joinToString("  "),
            )
        }
        item { SectionTitle("上午推荐") }
        if (morningRecommendations.isEmpty()) {
            item { SimpleRow("暂无上午推荐", morningSnapshot?.emptyReason.ifNullOrBlank("08:00-09:30 暂无推荐股票")) }
        }
        items(morningRecommendations) { item ->
            AStockRecommendationRow(item) {
                onOpenAStockBacktest(
                    AStockBacktestDetailState(
                        recommendation = item,
                        row = findAStockBacktest(morningBacktests, item),
                        strategyDate = window.date,
                        period = "morning",
                        sectionLabel = "上午推荐",
                        recommendations = morningRecommendations,
                        backtests = morningBacktests,
                    ),
                )
            }
        }
        item { RecommendationSeparator() }
        item { SectionTitle("下午推荐") }
        if (afternoonRecommendations.isEmpty()) {
            item { SimpleRow("暂无下午推荐", afternoonSnapshot?.emptyReason.ifNullOrBlank("09:30-13:00 暂无推荐股票")) }
        }
        items(afternoonRecommendations) { item ->
            AStockRecommendationRow(item) {
                onOpenAStockBacktest(
                    AStockBacktestDetailState(
                        recommendation = item,
                        row = findAStockBacktest(afternoonBacktests, item),
                        strategyDate = window.date,
                        period = "afternoon",
                        sectionLabel = "下午推荐",
                        recommendations = afternoonRecommendations,
                        backtests = afternoonBacktests,
                    ),
                )
            }
        }
    }
}

@Composable
private fun AStockAuctionModule(result: AStockAuctionListResult, trendDays: Int, viewModel: YuqingViewModel) {
    val storedCount = result.summaryCount.takeIf { it > 0 } ?: result.total
    val completenessText = if (storedCount in 1 until 4000) "数据可能不全" else ""
    val shenzhenLeaders = result.items
        .filter { it.code.startsWith("0") || it.code.startsWith("3") }
        .sortedByDescending { it.auctionAmount }
        .take(3)
    val shanghaiLeaders = result.items
        .filter { it.code.startsWith("6") }
        .sortedByDescending { it.auctionAmount }
        .take(3)
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        item {
            SimpleRow(
                "今日集合竞价 ${result.date.ifBlank { result.latestDate.ifBlank { "--" } }}",
                listOf(
                    "入库 $storedCount",
                    "本页 ${result.items.size}",
                    "总额 ${formatAuctionAmount(result.totalAmount)}",
                    completenessText,
                    result.fetchedAt,
                ).filter { it.isNotBlank() }.joinToString("  "),
            )
        }
        if (completenessText.isNotBlank()) {
            item { SimpleRow("集合竞价数据可能不全", "当前交易日仅入库 $storedCount 只股票，正常全市场应为数千只；请重新抓取最新交易日或回补当日集合竞价。") }
        }
        item {
            TextButton(onClick = { viewModel.loadAStockAuction() }) {
                Text("刷新集合竞价")
            }
        }
        item { AuctionTrendHeader(trendDays, onPeriodSelected = viewModel::selectAStockAuctionTrendDays) }
        item { AStockAuctionTrendChart(result, trendDays) }
        if (result.trend.isEmpty()) {
            item { SimpleRow("暂无历史走势", "接口暂未返回历史集合竞价金额") }
        }
        item { SectionTitle("沪市金额最高") }
        if (shanghaiLeaders.isEmpty()) {
            item { SimpleRow("暂无沪市集合竞价数据", "请刷新或等待交易日数据写入") }
        }
        items(shanghaiLeaders) { AStockAuctionRow(it) }
        item { SectionTitle("深市金额最高") }
        if (shenzhenLeaders.isEmpty()) {
            item { SimpleRow("暂无深市集合竞价数据", "请刷新或等待交易日数据写入") }
        }
        items(shenzhenLeaders) { AStockAuctionRow(it) }
    }
}

@Composable
private fun StockResearchModule(state: YuqingUiState, viewModel: YuqingViewModel) {
    val items = state.stockResearch.items
    val page = state.stockResearch.page.coerceAtLeast(1)
    val pageSize = state.stockResearch.pageSize.takeIf { it > 0 } ?: STOCK_RESEARCH_PAGE_SIZE
    val totalPages = stockResearchTotalPages(state.stockResearch.total, pageSize)
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        if (state.stockResearchLoading) {
            item {
                Row(
                    modifier = Modifier.fillMaxWidth().padding(vertical = 12.dp),
                    horizontalArrangement = Arrangement.Center,
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
                    Spacer(Modifier.width(8.dp))
                    Text("加载研报信息")
                }
            }
        }
        if (!state.stockResearchLoading && items.isEmpty()) {
            item { EmptyState("暂无研报信息") }
        }
        items(items) { item ->
            StockResearchRow(item, onOpenDetail = { viewModel.openStockResearchDetail(item) })
        }
        if (totalPages > 1 || state.stockResearch.total > 0) {
            item {
                StockResearchPagination(
                    page = page,
                    totalPages = totalPages,
                    loading = state.stockResearchLoading,
                    onPrevious = { viewModel.loadStockResearch(page - 1) },
                    onNext = { viewModel.loadStockResearch(page + 1) },
                )
            }
        }
    }
}

@Composable
private fun StockResearchRow(item: StockResearch, onOpenDetail: () -> Unit) {
    val date = stockResearchDisplayDate(item)
    Card(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onOpenDetail),
    ) {
        Column(Modifier.fillMaxWidth().padding(12.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    stockResearchStockLabel(item),
                    modifier = Modifier.weight(1f),
                    color = MaterialTheme.colorScheme.primary,
                    fontWeight = FontWeight.SemiBold,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
                if (date.isNotBlank()) {
                    Text(
                        date,
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1,
                    )
                }
            }
            val subtitle = stockResearchListSubtitle(item)
            if (subtitle.isNotBlank()) {
                Spacer(Modifier.height(4.dp))
                Text(subtitle, style = MaterialTheme.typography.bodySmall, maxLines = 3, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

@Composable
private fun StockResearchPagination(
    page: Int,
    totalPages: Int,
    loading: Boolean,
    onPrevious: () -> Unit,
    onNext: () -> Unit,
) {
    Row(
        modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        TextButton(onClick = onPrevious, enabled = !loading && page > 1) {
            Text("上一页")
        }
        Text(
            "第 $page / ${totalPages.coerceAtLeast(1)} 页",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        TextButton(onClick = onNext, enabled = !loading && page < totalPages) {
            Text("下一页")
        }
    }
}

@Composable
private fun StockResearchDetailScreen(
    item: StockResearch,
    loading: Boolean,
    error: String,
    pdfState: StockResearchPdfState,
    onDownloadPdf: () -> Unit,
    onRedownloadPdf: () -> Unit,
    onPdfError: (String) -> Unit,
) {
    val uriHandler = LocalUriHandler.current
    val context = LocalContext.current
    val body = stockResearchDetailBody(item)
    val sourceUrl = stockResearchOpenableUrl(item.sourceUrl)
    var sourceDialogUrl by remember(item.id, item.sourceUrl) { mutableStateOf<String?>(null) }
    LazyColumn(contentPadding = PaddingValues(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        item {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    item.title.trim().ifBlank { stockResearchStockLabel(item) },
                    style = MaterialTheme.typography.headlineSmall,
                    fontWeight = FontWeight.Bold,
                )
                Text(
                    stockResearchStockLabel(item),
                    style = MaterialTheme.typography.titleMedium,
                    color = MaterialTheme.colorScheme.primary,
                    fontWeight = FontWeight.SemiBold,
                )
                Row(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(stockResearchSourceLabel(item), style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.primary)
                    val date = stockResearchDisplayDate(item)
                    if (date.isNotBlank()) {
                        Text(date, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
            }
        }
        if (loading) {
            item {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
                    Text("正在加载详情", style = MaterialTheme.typography.bodySmall)
                }
            }
        }
        if (error.isNotBlank()) {
            item { Text(error, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall) }
        }
        val meta = stockResearchDetailMeta(item)
        if (meta.isNotBlank()) {
            item { Text(meta, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
        }
        if (item.nlpScoredAt.isNotBlank() || item.nlpRating.isNotBlank() || item.nlpReason.isNotBlank()) {
            item {
                Card {
                    Column(Modifier.fillMaxWidth().padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                        Text("NLP 评分", fontWeight = FontWeight.SemiBold)
                        val scoreText = if (item.nlpScoredAt.isNotBlank() || item.nlpScore != 0.0) {
                            String.format("%.2f", item.nlpScore)
                        } else {
                            ""
                        }
                        Text(
                            listOf(scoreText, item.nlpRating.trim()).filter { it.isNotBlank() }.joinToString(" "),
                            style = MaterialTheme.typography.bodySmall,
                        )
                        if (item.nlpReason.isNotBlank()) {
                            Text(item.nlpReason.trim(), style = MaterialTheme.typography.bodySmall)
                        }
                    }
                }
            }
        }
        item {
            if (body.isBlank()) {
                EmptyState("暂无详情内容")
            } else {
                Text(body, style = MaterialTheme.typography.bodyMedium, fontFamily = FontFamily.Monospace)
            }
        }
        item {
            StockResearchPdfSection(
                item = item,
                pdfState = pdfState,
                onDownloadPdf = onDownloadPdf,
                onRedownloadPdf = onRedownloadPdf,
                onOpenLocalPdf = {
                    openLocalStockResearchPdf(
                        context = context,
                        localUri = pdfState.localUri,
                        onError = onPdfError,
                    )
                },
            )
        }
        if (sourceUrl.isNotBlank() || item.pdfUrl.isNotBlank()) {
            item {
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (sourceUrl.isNotBlank()) {
                        TextButton(onClick = { sourceDialogUrl = sourceUrl }) {
                            Icon(Icons.AutoMirrored.Filled.OpenInNew, contentDescription = null)
                            Spacer(Modifier.width(6.dp))
                            Text("打开原文")
                        }
                    }
                    if (item.pdfUrl.isNotBlank()) {
                        TextButton(onClick = { runCatching { uriHandler.openUri(item.pdfUrl) } }) {
                            Icon(Icons.AutoMirrored.Filled.OpenInNew, contentDescription = null)
                            Spacer(Modifier.width(6.dp))
                            Text("在线打开PDF")
                        }
                    }
                }
            }
        }
    }
    val dialogUrl = sourceDialogUrl
    if (dialogUrl != null) {
        StockResearchSourceDialog(
            url = dialogUrl,
            onDismiss = { sourceDialogUrl = null },
        )
    }
}

@Composable
private fun StockResearchSourceDialog(
    url: String,
    onDismiss: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("原文") },
        text = {
            AndroidView(
                modifier = Modifier
                    .fillMaxWidth()
                    .height(520.dp),
                factory = { context ->
                    WebView(context).apply {
                        webViewClient = WebViewClient()
                        settings.javaScriptEnabled = true
                        settings.domStorageEnabled = true
                        loadUrl(url)
                    }
                },
                update = { webView ->
                    if (webView.url != url) {
                        webView.loadUrl(url)
                    }
                },
            )
        },
        confirmButton = {
            TextButton(onClick = onDismiss) {
                Text("关闭")
            }
        },
    )
}

@Composable
private fun StockResearchPdfSection(
    item: StockResearch,
    pdfState: StockResearchPdfState,
    onDownloadPdf: () -> Unit,
    onRedownloadPdf: () -> Unit,
    onOpenLocalPdf: () -> Unit,
) {
    Card {
        Column(Modifier.fillMaxWidth().padding(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Text("PDF", fontWeight = FontWeight.SemiBold)
            val status = listOf(
                item.pdfStatus.trim(),
                item.pdfFetchedAt.trim().takeIf { it.isNotBlank() }?.let { "抓取 $it" }.orEmpty(),
            ).filter { it.isNotBlank() }.joinToString("  ")
            if (status.isNotBlank()) {
                Text(status, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            if (item.pdfError.isNotBlank()) {
                Text(item.pdfError.trim(), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
            }
            if (pdfState.message.isNotBlank()) {
                Text(pdfState.message, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.primary)
            }
            if (pdfState.error.isNotBlank()) {
                Text(pdfState.error, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
            }
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Button(
                    onClick = onDownloadPdf,
                    enabled = !pdfState.loading && !pdfState.hasLocalFile,
                    modifier = Modifier.fillMaxWidth(),
                ) {
                    if (pdfState.loading) {
                        CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
                        Spacer(Modifier.width(6.dp))
                    }
                    Text(if (pdfState.hasLocalFile) "已下载" else "下载PDF")
                }
                if (pdfState.hasLocalFile) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        TextButton(onClick = onRedownloadPdf, enabled = !pdfState.loading) {
                            Text("重新下载PDF")
                        }
                        TextButton(onClick = onOpenLocalPdf, enabled = !pdfState.loading) {
                            Icon(Icons.AutoMirrored.Filled.OpenInNew, contentDescription = null)
                            Spacer(Modifier.width(6.dp))
                            Text("离线打开PDF")
                        }
                    }
                }
            }
            if (pdfState.hasLocalFile) {
                StockResearchPdfPreview(
                    localPath = pdfState.localPath,
                )
            } else {
                Text("下载后可在本页预览，并可离线打开。", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
    }
}

@Composable
private fun StockResearchPdfPreview(
    localPath: String,
) {
    var renderState by remember(localPath) { mutableStateOf(PdfRenderState(loading = true)) }
    LaunchedEffect(localPath) {
        renderState = PdfRenderState(loading = true)
        renderState = withContext(Dispatchers.IO) {
            renderPdfPages(localPath)
        }
    }
    if (renderState.loading) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
            Text("正在生成PDF预览", style = MaterialTheme.typography.bodySmall)
        }
        return
    }
    if (renderState.error.isNotBlank()) {
        Text(renderState.error, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
        return
    }
    val bitmaps = renderState.bitmaps
    if (bitmaps.isEmpty()) {
        return
    }
    val pageCount = renderState.pageCount.coerceAtLeast(1)
    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        bitmaps.forEachIndexed { index, bitmap ->
            Text("第 ${index + 1} / $pageCount 页", style = MaterialTheme.typography.bodySmall)
            Image(
                bitmap = bitmap.asImageBitmap(),
                contentDescription = "PDF预览第${index + 1}页",
                modifier = Modifier
                    .fillMaxWidth()
                    .aspectRatio(bitmap.width.toFloat() / bitmap.height.toFloat()),
                contentScale = ContentScale.FillWidth,
            )
        }
    }
}

private data class PdfRenderState(
    val loading: Boolean = false,
    val bitmaps: List<Bitmap> = emptyList(),
    val pageCount: Int = 0,
    val error: String = "",
)

private fun renderPdfPages(localPath: String): PdfRenderState {
    val file = File(localPath)
    if (!file.isFile || file.length() <= 0L) {
        return PdfRenderState(error = "本地PDF文件不存在")
    }
    return runCatching {
        ParcelFileDescriptor.open(file, ParcelFileDescriptor.MODE_READ_ONLY).use { descriptor ->
            PdfRenderer(descriptor).use { renderer ->
                if (renderer.pageCount <= 0) {
                    return PdfRenderState(error = "PDF没有可预览页面")
                }
                val bitmaps = mutableListOf<Bitmap>()
                for (pageIndex in 0 until renderer.pageCount) {
                    renderer.openPage(pageIndex).use { page ->
                        val width = 900
                        val height = (width.toFloat() / page.width.toFloat() * page.height.toFloat()).toInt().coerceAtLeast(1)
                        val bitmap = Bitmap.createBitmap(width, height, Bitmap.Config.ARGB_8888)
                        bitmap.eraseColor(android.graphics.Color.WHITE)
                        page.render(bitmap, null, null, PdfRenderer.Page.RENDER_MODE_FOR_DISPLAY)
                        bitmaps += bitmap
                    }
                }
                PdfRenderState(bitmaps = bitmaps, pageCount = renderer.pageCount)
            }
        }
    }.getOrElse { throwable ->
        PdfRenderState(error = throwable.message ?: "PDF预览失败")
    }
}

private fun openLocalStockResearchPdf(
    context: Context,
    localUri: String,
    onError: (String) -> Unit,
) {
    if (localUri.isBlank()) {
        onError("本地PDF不存在，请先下载")
        return
    }
    val intent = Intent(Intent.ACTION_VIEW).apply {
        setDataAndType(Uri.parse(localUri), "application/pdf")
        addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
    }
    try {
        context.startActivity(Intent.createChooser(intent, "打开PDF"))
    } catch (_: ActivityNotFoundException) {
        onError("手机未安装PDF阅读器")
    } catch (throwable: RuntimeException) {
        onError(throwable.message ?: "无法打开本地PDF")
    }
}

@Composable
private fun HoldingsModule(items: List<StockHolding>) {
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        items(items) { SimpleRow("${it.stockCode} ${it.stockName}", "${it.holderName} ${it.holderType} ${it.floatRatio}%") }
    }
}

@Composable
private fun SystemModule(
    dashboard: AndroidDashboard,
    state: YuqingUiState,
    viewModel: YuqingViewModel,
    versionUpgradeState: VersionUpgradeUiState,
    onCheckUpgrade: () -> Unit,
) {
    val recentTasks = dashboard.recentTaskRunsForDisplay()
    val database = dashboard.operations.database
    val servicesByName = dashboard.operations.services.associateBy { it.name }
    val endpoints = state.networkEndpoints
    val webUrl = endpoints.webBaseUrl
    val contentUrl = endpoints.contentBaseUrl
    val authUrl = endpoints.authBaseUrl
    val releaseUrl = endpoints.releaseBaseUrl
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        item { SectionTitle("版本") }
        item {
            VersionInfoRow(
                state = versionUpgradeState,
                onCheckUpgrade = onCheckUpgrade,
            )
        }
        item { SectionTitle("连接") }
        item {
            SimpleRow(
                "网络环境",
                listOfNotNull(
                    endpoints.label,
                    "heartbeat 120秒",
                    if (state.networkHeartbeatLoading) "检测中" else state.networkHeartbeatMessage.ifBlank { null },
                ).joinToString("  "),
            )
        }
        item {
            ConnectionRow(
                title = "网页地址",
                value = webUrl,
                service = servicesByName["gateway-web"],
                testResult = state.connectionTests["网页地址"],
                onTest = { viewModel.testConnection("网页地址", webUrl) },
            )
        }
        item {
            ConnectionRow(
                title = "内容服务地址",
                value = contentUrl,
                service = servicesByName["content-service"],
                testResult = state.connectionTests["内容服务地址"],
                onTest = { viewModel.testConnection("内容服务地址", contentUrl) },
            )
        }
        item {
            ConnectionRow(
                title = "API 地址",
                value = authUrl,
                service = servicesByName["auth-service"],
                testResult = state.connectionTests["API 地址"],
                onTest = { viewModel.testConnection("API 地址", authUrl) },
            )
        }
        item {
            ConnectionRow(
                title = "发布服务地址",
                value = releaseUrl,
                service = servicesByName["release-service"],
                testResult = state.connectionTests["发布服务地址"],
                onTest = { viewModel.testConnection("发布服务地址", releaseUrl) },
            )
        }
        item {
            SimpleRow(
                "数据库",
                listOf(
                    "driver ${database.driver.ifBlank { "--" }}",
                    "runtime ${database.runtimeDriver.ifBlank { "--" }}",
                    "status ${database.status.ifBlank { "--" }}",
                ).joinToString("  "),
            )
        }
        if (database.sqlitePath.isNotBlank()) {
            item { SimpleRow("SQLite", database.sqlitePath) }
        }
        if (database.postgresHost.isNotBlank() || database.postgresDsn.isNotBlank()) {
            item {
                SimpleRow(
                    "PostgreSQL",
                    database.postgresDsn.ifBlank {
                        listOf(database.postgresHost, database.postgresPort, database.postgresDatabase, database.postgresUser)
                            .filter { it.isNotBlank() }
                            .joinToString("  ")
                    },
                )
            }
        }
        item { SectionTitle("服务") }
        items(dashboard.operations.services) { ServiceRow(it, viewModel) }
        item { SectionTitle("调度任务") }
        items(dashboard.operations.schedulerJobs) { SchedulerJobRow(it, viewModel) }
        item { SectionTitle("最近任务") }
        if (recentTasks.isEmpty()) {
            item { SimpleRow("暂无任务记录", "") }
        }
        items(recentTasks) { SimpleRow(it.taskName, "${it.status} ${it.message}") }
    }
}

@Composable
private fun GenericModule(key: String, dashboard: AndroidDashboard) {
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        item { SectionTitle(key) }
        item { SimpleRow("入口", "该模块已纳入原生导航，后续按现有 BFF/API 数据扩展详情页。") }
        item { SimpleRow("当前数据", "文章 ${dashboard.overview.articleCount} / 项目 ${dashboard.overview.projectCount}") }
    }
}

@Composable
private fun ServiceRow(service: ServiceStatus, viewModel: YuqingViewModel) {
    val statusKey = connectionStatusKey(service)
    val statusText = buildString {
        append(connectionStatusLabel(statusKey))
        val detail = service.message.trim()
        if (detail.isNotBlank() && !detail.equals("ok", ignoreCase = true) && !detail.equals("working", ignoreCase = true)) {
            append(" ")
            append(detail)
        }
    }
    Card {
        Row(Modifier.fillMaxWidth().padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text(service.name, fontWeight = FontWeight.SemiBold)
                Text(statusText, style = MaterialTheme.typography.bodySmall)
            }
            ConnectionStatusDot(statusKey)
            Spacer(Modifier.width(12.dp))
            TextButton(onClick = { viewModel.requestAction("service_restart", "重启 ${service.name}", mapOf("service" to service.name)) }) {
                Text("重启")
            }
        }
    }
}

@Composable
private fun SchedulerJobRow(job: SchedulerJob, viewModel: YuqingViewModel) {
    Card {
        Row(Modifier.fillMaxWidth().padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text(job.name, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text("${job.group} ${job.lastStatus}", style = MaterialTheme.typography.bodySmall)
            }
            TextButton(onClick = { viewModel.requestAction("run_scheduler_job", "运行 ${job.name}", mapOf("name" to job.name)) }) {
                Text("运行")
            }
        }
    }
}

@Composable
private fun MetricCard(
    title: String,
    value: String,
    modifier: Modifier = Modifier,
    onClick: (() -> Unit)? = null,
    icon: ImageVector? = null,
) {
    val cardModifier = if (onClick == null) {
        modifier
    } else {
        modifier.clickable(onClick = onClick)
    }
    Card(modifier = cardModifier.height(86.dp)) {
        Column(
            Modifier.fillMaxSize().padding(14.dp),
            verticalArrangement = Arrangement.Center,
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                if (icon != null) {
                    Icon(icon, contentDescription = null, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(6.dp))
                }
                Text(title, style = MaterialTheme.typography.labelMedium, maxLines = 1)
            }
            Text(
                value,
                style = MaterialTheme.typography.headlineSmall,
                fontWeight = FontWeight.Bold,
                maxLines = 1,
            )
        }
    }
}

@Composable
private fun SwipeHiddenArticleRow(
    item: ArticleItem,
    read: Boolean,
    onClick: () -> Unit,
    onSwipeRight: (ArticleItem) -> Unit,
    onSwipeLeft: (ArticleItem) -> Unit,
) {
    val key = articleStableKey(item)
    var actionRequested by remember(key) { mutableStateOf(false) }
    val dismissState = rememberSwipeToDismissBoxState(
        confirmValueChange = { value ->
            when (value) {
                SwipeToDismissBoxValue.StartToEnd -> {
                    if (!actionRequested) {
                        actionRequested = true
                        onSwipeRight(item)
                    }
                }
                SwipeToDismissBoxValue.EndToStart -> onSwipeLeft(item)
                SwipeToDismissBoxValue.Settled -> Unit
            }
            false
        },
    )
    SwipeToDismissBox(
        state = dismissState,
        backgroundContent = {},
        enableDismissFromStartToEnd = true,
        enableDismissFromEndToStart = true,
    ) {
        ArticleRow(item = item, read = read, onClick = onClick)
    }
}

enum class VersionUpgradeStatus {
    Idle,
    Checking,
    Latest,
    UpdateFound,
    Downloading,
    InstallerOpened,
    Failed,
}

data class VersionUpgradeUiState(
    val status: VersionUpgradeStatus = VersionUpgradeStatus.Idle,
    val sourceLabel: String = "",
    val latestVersionName: String = "",
    val latestFileName: String = "",
    val message: String = "",
)

@Composable
private fun VersionInfoRow(
    state: VersionUpgradeUiState,
    onCheckUpgrade: () -> Unit,
) {
    val loading = state.status == VersionUpgradeStatus.Checking || state.status == VersionUpgradeStatus.Downloading
    val messageColor = when (state.status) {
        VersionUpgradeStatus.Failed -> MaterialTheme.colorScheme.error
        VersionUpgradeStatus.Latest,
        VersionUpgradeStatus.UpdateFound,
        VersionUpgradeStatus.InstallerOpened -> MaterialTheme.colorScheme.primary
        VersionUpgradeStatus.Idle,
        VersionUpgradeStatus.Checking,
        VersionUpgradeStatus.Downloading -> MaterialTheme.colorScheme.onSurfaceVariant
    }
    Card {
        Row(
            modifier = Modifier.fillMaxWidth().padding(12.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(Modifier.weight(1f)) {
                Text("当前版本", fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Spacer(Modifier.height(4.dp))
                Text(
                    "${BuildConfig.VERSION_NAME} (${BuildConfig.VERSION_CODE})",
                    style = MaterialTheme.typography.bodySmall,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                )
                if (state.sourceLabel.isNotBlank() || state.latestVersionName.isNotBlank()) {
                    Spacer(Modifier.height(4.dp))
                    Text(
                        listOfNotNull(
                            state.sourceLabel.takeIf { it.isNotBlank() }?.let { "来源 $it" },
                            state.latestVersionName.takeIf { it.isNotBlank() }?.let { "最新 $it" },
                        ).joinToString("  "),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
                if (state.message.isNotBlank()) {
                    Spacer(Modifier.height(4.dp))
                    Text(
                        state.message,
                        style = MaterialTheme.typography.bodySmall,
                        color = messageColor,
                        maxLines = 3,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
            TextButton(onClick = onCheckUpgrade, enabled = !loading) {
                Text(versionUpgradeButtonText(state.status))
            }
        }
    }
}

internal fun versionUpgradeButtonText(status: VersionUpgradeStatus): String {
    return when (status) {
        VersionUpgradeStatus.Checking -> "检测中"
        VersionUpgradeStatus.Downloading -> "下载中"
        else -> "检测升级"
    }
}

internal fun articleStableKey(item: ArticleItem): String {
    if (item.id > 0) {
        return "id:${item.id}"
    }
    return listOf(item.sourceUrl, item.capturedAt, item.title)
        .joinToString("|")
        .ifBlank { "title:${item.title}" }
}

internal enum class ArticleListSwipeAction {
    Hide,
    Clear,
    None,
}

internal fun articleListSwipeAction(value: SwipeToDismissBoxValue): ArticleListSwipeAction {
    return when (value) {
        SwipeToDismissBoxValue.StartToEnd -> ArticleListSwipeAction.Hide
        SwipeToDismissBoxValue.EndToStart -> ArticleListSwipeAction.Clear
        SwipeToDismissBoxValue.Settled -> ArticleListSwipeAction.None
    }
}

@Composable
private fun ArticleRow(
    item: ArticleItem,
    modifier: Modifier = Modifier,
    read: Boolean = false,
    onClick: (() -> Unit)? = null,
) {
    val displayTime = remember(item.capturedAt, item.publishTime, item.publishTimeText) {
        formatArticleRelativeTime(item.publishTimeText)
            .ifBlank { formatArticleRelativeTime(item.publishTime) }
            .ifBlank { formatArticleRelativeTime(item.capturedAt) }
    }
    val title = remember(item.title) { item.title.trim().ifBlank { "--" } }
    val relativeTimeStyle = SpanStyle(
        fontWeight = FontWeight.Normal,
        fontSize = MaterialTheme.typography.bodySmall.fontSize,
    )
    val readStyle = SpanStyle(
        fontWeight = FontWeight.Normal,
        fontSize = MaterialTheme.typography.bodySmall.fontSize,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    val titleText = remember(title, displayTime, read, relativeTimeStyle, readStyle) {
        buildAnnotatedString {
            append(title)
            if (read) {
                append(" ")
                withStyle(readStyle) {
                    append("已读")
                }
            }
            if (displayTime.isNotBlank()) {
                append(" ")
                withStyle(relativeTimeStyle) {
                    append(displayTime)
                }
            }
        }
    }
    val cardModifier = if (onClick == null) modifier else modifier.clickable(onClick = onClick)
    Card(cardModifier) {
        Column(Modifier.fillMaxWidth().padding(12.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(
                titleText,
                modifier = Modifier.fillMaxWidth(),
                fontWeight = if (read) FontWeight.Normal else FontWeight.SemiBold,
                maxLines = 3,
                overflow = TextOverflow.Ellipsis,
            )
            if (shouldShowMobileArticleSummary(item)) {
                Text(
                    item.summary.trim(),
                    style = MaterialTheme.typography.bodySmall,
                    maxLines = 3,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
    }
}

@Composable
private fun ArticleDetailScreen(
    item: ArticleItem,
    loading: Boolean,
    error: String,
    onSwipeLeft: () -> Unit,
    onSwipeDown: () -> Unit,
    onSwipeUp: () -> Unit,
) {
    val uriHandler = LocalUriHandler.current
    val swipeThreshold = with(LocalDensity.current) { 96.dp.toPx() }
    var dragOffset by remember(articleStableKey(item)) { mutableStateOf(Offset.Zero) }
    val displayTime = remember(item.capturedAt, item.publishTime, item.publishTimeText) {
        formatArticleRelativeTime(item.publishTimeText)
            .ifBlank { formatArticleRelativeTime(item.publishTime) }
            .ifBlank { formatArticleRelativeTime(item.capturedAt) }
    }
    val body = remember(item.content, item.summary) {
        item.content.trim().ifBlank { item.summary.trim() }
    }
    LazyColumn(
        modifier = Modifier.pointerInput(articleStableKey(item)) {
            detectDragGestures(
                onDragStart = { dragOffset = Offset.Zero },
                onDrag = { _, dragAmount ->
                    dragOffset += dragAmount
                },
                onDragEnd = {
                    val horizontal = dragOffset.x
                    val vertical = dragOffset.y
                    when {
                        horizontal < -swipeThreshold && abs(horizontal) > abs(vertical) -> onSwipeLeft()
                        vertical > swipeThreshold && vertical > abs(horizontal) -> onSwipeDown()
                        vertical < -swipeThreshold && abs(vertical) > abs(horizontal) -> onSwipeUp()
                    }
                    dragOffset = Offset.Zero
                },
                onDragCancel = { dragOffset = Offset.Zero },
            )
        },
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    item.title.trim().ifBlank { "--" },
                    style = MaterialTheme.typography.headlineSmall,
                    fontWeight = FontWeight.Bold,
                )
                Row(horizontalArrangement = Arrangement.spacedBy(10.dp), verticalAlignment = Alignment.CenterVertically) {
                    if (item.sourceType.isNotBlank()) {
                        Text(item.sourceType, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.primary)
                    }
                    if (displayTime.isNotBlank()) {
                        Text(displayTime, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
            }
        }
        if (loading) {
            item {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
                    Text("正在加载详情", style = MaterialTheme.typography.bodySmall)
                }
            }
        }
        if (error.isNotBlank()) {
            item {
                Text(error, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
            }
        }
        if (shouldShowMobileArticleSummary(item) && item.summary.trim() != body) {
            item {
                Card {
                    Text(
                        item.summary.trim(),
                        modifier = Modifier.fillMaxWidth().padding(12.dp),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                }
            }
        }
        item {
            if (body.isBlank()) {
                EmptyState("暂无正文内容")
            } else {
                Text(
                    body,
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        }
        if (item.sourceUrl.isNotBlank()) {
            item {
                TextButton(onClick = { runCatching { uriHandler.openUri(item.sourceUrl) } }) {
                    Icon(Icons.AutoMirrored.Filled.OpenInNew, contentDescription = null)
                    Spacer(Modifier.width(6.dp))
                    Text("打开原文")
                }
            }
        }
    }
}

internal fun shouldShowMobileArticleSummary(item: ArticleItem): Boolean {
    if (item.summary.isBlank()) {
        return false
    }
    return !isPanewsNewsflashSource(item.sourceType)
}

private fun isPanewsNewsflashSource(sourceType: String): Boolean {
    return sourceType.trim().equals("panews_newsflash", ignoreCase = true)
}

private data class AStockBacktestDetailState(
    val recommendation: AStockRecommendation,
    val row: AStockBacktestRow?,
    val strategyDate: String,
    val period: String,
    val sectionLabel: String,
    val recommendations: List<AStockRecommendation> = emptyList(),
    val backtests: List<AStockBacktestRow> = emptyList(),
)

private fun AStockBacktestRow.displayEntryOpen(): String {
    val afternoon = afternoonOpen.trim()
    if (afternoon.isNotBlank() && afternoon != "--") {
        return afternoon
    }
    val morning = entryOpen.trim()
    if (morning.isNotBlank()) {
        return morning
    }
    return "--"
}

@Composable
private fun AStockBacktestDetailScreen(
    state: AStockBacktestDetailState,
    onSwipeDown: () -> Unit,
    onSwipeUp: () -> Unit,
    onPreviousStock: () -> Unit,
    onNextStock: () -> Unit,
    refreshingPrice: Boolean,
    onRefreshPrice: () -> Unit,
) {
    val row = state.row
    val currentClosePrice = aStockBacktestDetailCurrentPrice(row, state.recommendation)
    val currentMarketPct = aStockBacktestDetailCurrentMarketPct(row)
    val adjacentLabels = aStockBacktestAdjacentLabels(state.recommendations, state.recommendation)
    val swipeThreshold = with(LocalDensity.current) { 96.dp.toPx() }
    var dragOffset by remember(state.recommendation.code, state.sectionLabel) { mutableStateOf(Offset.Zero) }
    LazyColumn(
        modifier = Modifier.pointerInput(state.recommendation.code, state.sectionLabel) {
            detectDragGestures(
                onDragStart = { dragOffset = Offset.Zero },
                onDrag = { _, dragAmount ->
                    dragOffset += dragAmount
                },
                onDragEnd = {
                    val horizontal = dragOffset.x
                    val vertical = dragOffset.y
                    when {
                        vertical > swipeThreshold && vertical > abs(horizontal) -> onSwipeDown()
                        vertical < -swipeThreshold && abs(vertical) > abs(horizontal) -> onSwipeUp()
                    }
                    dragOffset = Offset.Zero
                },
                onDragCancel = { dragOffset = Offset.Zero },
            )
        },
        contentPadding = PaddingValues(12.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        item {
            SimpleRow(
                "${state.recommendation.code} ${state.recommendation.name}",
                listOf(
                    state.strategyDate,
                    state.sectionLabel,
                    "现价 $currentClosePrice",
                    "今日 $currentMarketPct",
                ).joinToString("  "),
            )
        }
        if (row == null) {
            item { SimpleRow("暂无回测结果", "请先在 A股页面刷新当前回测。") }
        } else {
            item {
                AStockBacktestEntryRow(
                    row = row,
                    currentPrice = currentClosePrice,
                    currentMarketPct = currentMarketPct,
                    refreshingPrice = refreshingPrice,
                    onRefreshPrice = onRefreshPrice,
                )
            }
            item { SectionTitle("回测数据") }
            item {
                Card {
                    Column(Modifier.fillMaxWidth().padding(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                        AStockBacktestMetricHeader()
                        AStockBacktestMetricRow("T+0", row.t0Return.ifBlank { "--" }, row.t0Close.ifBlank { "--" })
                        (0 until 5).forEach { index ->
                            val cell = row.days.getOrNull(index)
                            AStockBacktestMetricRow(
                                label = "T+${index + 1}",
                                returnValue = cell?.returnPct?.ifBlank { "--" } ?: "--",
                                closeValue = cell?.close?.ifBlank { "--" } ?: "--",
                            )
                        }
                    }
                }
            }
            item {
                SimpleRow(
                    "五日内最高涨幅 ${row.bestReturn.ifBlank { "--" }}",
                    cleanAStockRecommendationReason(state.recommendation.reason).ifBlank { "暂无推荐说明" },
                )
            }
            if (adjacentLabels.hasAnyTarget) {
                item {
                    AStockBacktestAdjacentNavigation(
                        previousLabel = adjacentLabels.previous,
                        nextLabel = adjacentLabels.next,
                        onPreviousStock = onPreviousStock,
                        onNextStock = onNextStock,
                    )
                }
            }
        }
    }
}

@Composable
private fun AStockBacktestEntryRow(
    row: AStockBacktestRow,
    currentPrice: String,
    currentMarketPct: String,
    refreshingPrice: Boolean,
    onRefreshPrice: () -> Unit,
) {
    Card {
        Row(
            modifier = Modifier.fillMaxWidth().padding(12.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            Row(
                modifier = Modifier.weight(1f),
                verticalAlignment = Alignment.Top,
                horizontalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                Column(Modifier.weight(1f)) {
                    Text("推荐价 ${row.displayEntryOpen()}", fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    Spacer(Modifier.height(4.dp))
                    Text("状态 ${row.status.ifBlank { "--" }}", style = MaterialTheme.typography.bodySmall, maxLines = 2, overflow = TextOverflow.Ellipsis)
                }
                Column(
                    modifier = Modifier.widthIn(min = 96.dp, max = 128.dp),
                    horizontalAlignment = Alignment.End,
                    verticalArrangement = Arrangement.spacedBy(4.dp),
                ) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.End,
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        Text(
                            "现价 ",
                            fontSize = 14.sp,
                            fontWeight = FontWeight.SemiBold,
                            maxLines = 1,
                        )
                        Text(
                            currentPrice,
                            color = backtestValueColor(currentMarketPct),
                            fontSize = 16.sp,
                            fontWeight = FontWeight.SemiBold,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                            textAlign = TextAlign.End,
                        )
                    }
                    Text(
                        "今日涨跌 $currentMarketPct",
                        color = backtestValueColor(currentMarketPct),
                        fontSize = 12.sp,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        textAlign = TextAlign.End,
                    )
                }
            }
            Button(
                onClick = onRefreshPrice,
                enabled = !refreshingPrice,
                modifier = Modifier.height(40.dp),
                contentPadding = PaddingValues(horizontal = 10.dp, vertical = 0.dp),
            ) {
                if (refreshingPrice) {
                    CircularProgressIndicator(
                        modifier = Modifier.size(16.dp),
                        strokeWidth = 2.dp,
                    )
                } else {
                    Icon(Icons.Default.Refresh, contentDescription = null, modifier = Modifier.size(14.dp))
                    Spacer(Modifier.width(4.dp))
                    Text("刷新", fontSize = 13.sp)
                }
            }
        }
    }
}

internal val AStockBacktestAdjacentButtonSpacing = 0.dp
internal val AStockBacktestAdjacentButtonFontSize = 14.sp

@Composable
private fun AStockBacktestAdjacentNavigation(
    previousLabel: String?,
    nextLabel: String?,
    onPreviousStock: () -> Unit,
    onNextStock: () -> Unit,
) {
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(AStockBacktestAdjacentButtonSpacing)) {
        AStockBacktestAdjacentButton(
            label = previousLabel,
            onClick = onPreviousStock,
            modifier = Modifier.weight(1f),
        )
        AStockBacktestAdjacentButton(
            label = nextLabel,
            onClick = onNextStock,
            modifier = Modifier.weight(1f),
        )
    }
}

@Composable
private fun AStockBacktestAdjacentButton(
    label: String?,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    if (label == null) {
        Spacer(modifier.height(44.dp))
        return
    }
    OutlinedButton(
        onClick = onClick,
        modifier = modifier.height(44.dp),
        contentPadding = PaddingValues(horizontal = 8.dp, vertical = 0.dp),
    ) {
        Text(
            label,
            fontSize = AStockBacktestAdjacentButtonFontSize,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            textAlign = TextAlign.Center,
        )
    }
}

@Composable
private fun AStockBacktestMetricHeader() {
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        Text("阶段", modifier = Modifier.weight(1f), style = MaterialTheme.typography.labelMedium)
        Text("收盘价", modifier = Modifier.weight(1f), style = MaterialTheme.typography.labelMedium)
        Text("收益", modifier = Modifier.weight(1f), style = MaterialTheme.typography.labelMedium)
    }
}

@Composable
private fun AStockBacktestMetricRow(label: String, returnValue: String, closeValue: String) {
    Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
        Text(label, modifier = Modifier.weight(1f), fontWeight = FontWeight.SemiBold)
        Text(closeValue, modifier = Modifier.weight(1f))
        Text(
            returnValue,
            modifier = Modifier.weight(1f),
            color = backtestValueColor(returnValue),
            fontWeight = FontWeight.Medium,
        )
    }
}

@Composable
private fun backtestValueColor(value: String) = when (backtestValueTone(value)) {
    BacktestValueTone.Up -> AStockPriceUpColor
    BacktestValueTone.Down -> AStockPriceDownColor
    BacktestValueTone.Flat -> MaterialTheme.colorScheme.onSurface
}

private val AStockPriceUpColor = Color(0xFFC62828)
private val AStockPriceDownColor = Color(0xFF2E7D32)

internal enum class BacktestValueTone {
    Up,
    Down,
    Flat,
}

private val SignedPercentPrefixPattern = Regex("""^[+-]\s*\d""")

internal fun backtestValueTone(value: String): BacktestValueTone = when {
    value.trimStart().startsWith("+") && SignedPercentPrefixPattern.containsMatchIn(value.trimStart()) -> BacktestValueTone.Up
    value.trimStart().startsWith("-") && SignedPercentPrefixPattern.containsMatchIn(value.trimStart()) -> BacktestValueTone.Down
    else -> BacktestValueTone.Flat
}

@Composable
private fun AStockRecommendationRow(item: AStockRecommendation, onClick: () -> Unit = {}) {
    SimpleRow(
        "#${item.rank.coerceAtLeast(1)} ${item.code} ${item.name}",
        listOf(
            "热点 ${item.hotspot.ifBlank { "--" }}",
            "综合分 ${item.marketScore}",
            "现价 ${item.currentPrice.ifBlank { "--" }}",
            "今日 ${item.todayPct.ifBlank { "--" }}",
            "机构 ${item.holdingSummary.ifBlank { "--" }}",
            cleanAStockRecommendationReason(item.reason),
        ).filter { it.isNotBlank() }.joinToString("  "),
        Modifier.clickable(onClick = onClick),
    )
}

@Composable
private fun AStockAuctionRow(item: AStockAuctionAmount) {
    SimpleRow(
        "${item.code} ${item.name}",
        listOf(
            "价格 ${item.auctionPrice}",
            "成交量 ${formatAuctionAmount(item.auctionVolume)}",
            "成交额 ${formatAuctionAmount(item.auctionAmount)}",
            item.status,
        ).filter { it.isNotBlank() }.joinToString("  "),
    )
}

@Composable
private fun AuctionTrendHeader(selectedDays: Int, onPeriodSelected: (Int) -> Unit) {
    Row(
        Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
    ) {
        SectionTitle("历史金额走势")
        Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            AuctionTrendPeriodChip("近7天", 7, selectedDays, onPeriodSelected)
            AuctionTrendPeriodChip("近两周", 14, selectedDays, onPeriodSelected)
            AuctionTrendPeriodChip("近30天", 30, selectedDays, onPeriodSelected)
        }
    }
}

@Composable
private fun AuctionTrendPeriodChip(
    label: String,
    days: Int,
    selectedDays: Int,
    onPeriodSelected: (Int) -> Unit,
) {
    FilterChip(
        selected = selectedDays == days,
        onClick = { onPeriodSelected(days) },
        label = { Text(label, maxLines = 1) },
    )
}

@Composable
private fun AStockAuctionTrendChart(result: AStockAuctionListResult, days: Int) {
    val trend = result.trend
        .filter { it.totalAmount > 0 }
        .takeLast(normalizeAStockAuctionTrendDays(days))
    if (trend.isEmpty()) {
        return
    }
    val lineColor = MaterialTheme.colorScheme.primary
    val gridColor = MaterialTheme.colorScheme.outlineVariant
    val maxAmount = trend.maxOf { it.totalAmount }.coerceAtLeast(1.0)
    Card {
        Column(Modifier.fillMaxWidth().padding(12.dp)) {
            Text("金额最高 ${formatAuctionAmount(maxAmount)}", style = MaterialTheme.typography.bodySmall)
            Spacer(Modifier.height(8.dp))
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Column(
                    modifier = Modifier.width(64.dp).height(150.dp),
                    verticalArrangement = Arrangement.SpaceBetween,
                    horizontalAlignment = Alignment.End,
                ) {
                    listOf(maxAmount, maxAmount * 2 / 3, maxAmount / 3, 0.0).forEach { amount ->
                        Text(
                            formatAuctionAmount(amount),
                            style = MaterialTheme.typography.labelSmall,
                            textAlign = TextAlign.End,
                            maxLines = 1,
                        )
                    }
                }
                Spacer(Modifier.width(8.dp))
                Canvas(Modifier.weight(1f).height(150.dp)) {
                    val left = 0f
                    val right = size.width
                    val top = 10f
                    val bottom = size.height - 18f
                    repeat(4) { index ->
                        val y = top + (bottom - top) * index / 3f
                        drawLine(gridColor, Offset(left, y), Offset(right, y), strokeWidth = 1f)
                    }
                    val points = trend.mapIndexed { index, item ->
                        val x = if (trend.size == 1) {
                            (left + right) / 2f
                        } else {
                            left + (right - left) * index / (trend.size - 1).toFloat()
                        }
                        val y = bottom - ((item.totalAmount / maxAmount).toFloat() * (bottom - top))
                        Offset(x, y)
                    }
                    points.forEach { point ->
                        drawLine(gridColor, Offset(point.x, top), Offset(point.x, bottom), strokeWidth = 1f)
                    }
                    drawLine(gridColor, Offset(left, top), Offset(left, bottom), strokeWidth = 1f)
                    drawLine(gridColor, Offset(left, bottom), Offset(right, bottom), strokeWidth = 1f)
                    points.zipWithNext().forEach { (start, end) ->
                        drawLine(lineColor, start, end, strokeWidth = 4f, cap = StrokeCap.Round)
                    }
                    points.forEach { point ->
                        drawCircle(lineColor, radius = 4f, center = point)
                    }
                }
            }
            Spacer(Modifier.height(6.dp))
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                Text(trend.first().date, style = MaterialTheme.typography.labelSmall)
                Text(trend.last().date, style = MaterialTheme.typography.labelSmall)
            }
        }
    }
}

private fun AndroidDashboard.recentTaskRunsForDisplay(): List<TaskRun> {
    return if (taskRuns.isNotEmpty()) taskRuns else operations.recentTaskRuns
}

private fun cleanAStockRecommendationReason(value: String): String {
    return value
        .replace("；集合竞价候选为空，", "；")
        .replace("集合竞价候选为空，", "")
        .replace("集合竞价", "行情")
        .trim { it.isWhitespace() || it == '；' || it == '，' }
}

internal data class AStockDateLabelParts(
    val date: String,
    val weekday: String = "",
)

internal fun formatAStockDateLabelParts(value: String): AStockDateLabelParts {
    val date = runCatching { LocalDate.parse(value) }.getOrNull() ?: return AStockDateLabelParts(date = value)
    val weekday = when (date.dayOfWeek) {
        DayOfWeek.MONDAY -> "周一"
        DayOfWeek.TUESDAY -> "周二"
        DayOfWeek.WEDNESDAY -> "周三"
        DayOfWeek.THURSDAY -> "周四"
        DayOfWeek.FRIDAY -> "周五"
        DayOfWeek.SATURDAY -> "周六"
        DayOfWeek.SUNDAY -> "周日"
    }
    return AStockDateLabelParts(date = value, weekday = weekday)
}

private fun isLatestSelectableAStockDate(value: String): Boolean {
    val date = runCatching { LocalDate.parse(value) }.getOrNull() ?: return false
    val latest = AStockTradingCalendar.latestSelectableTradingDay(LocalDate.now(ZoneId.of("Asia/Shanghai")))
    return !date.isBefore(latest)
}

private fun parseAStockBacktests(raw: String?): List<AStockBacktestRow> {
    val payload = raw.orEmpty().trim().ifBlank { "[]" }
    return runCatching {
        ApiFactory.json.decodeFromString<List<AStockBacktestRow>>(payload)
    }.getOrDefault(emptyList())
}

private fun applyAStockBacktestDetailSnapshot(
    state: AStockBacktestDetailState,
    snapshot: AStockRecommendationSnapshot,
): AStockBacktestDetailState {
    val recommendations = parseAStockRecommendations(snapshot.recommendationsJson)
    val backtests = parseAStockBacktests(snapshot.backtestsJson)
    val recommendation = recommendations.firstOrNull { sameAStockRecommendation(it, state.recommendation) }
        ?: state.recommendation
    return state.copy(
        recommendation = recommendation,
        row = findAStockBacktest(backtests, recommendation) ?: findAStockBacktest(backtests, state.recommendation) ?: state.row,
        strategyDate = snapshot.strategyDate.ifBlank { state.strategyDate },
        period = snapshot.period.ifBlank { state.period },
        recommendations = recommendations.ifEmpty { state.recommendations },
        backtests = backtests.ifEmpty { state.backtests },
    )
}

private fun sameAStockBacktestDetailTarget(
    current: AStockBacktestDetailState,
    target: AStockBacktestDetailState,
): Boolean {
    return current.strategyDate == target.strategyDate &&
        current.period == target.period &&
        current.recommendation.code == target.recommendation.code
}

private fun findAStockBacktest(rows: List<AStockBacktestRow>, item: AStockRecommendation): AStockBacktestRow? {
    val code = item.code.trim()
    if (code.isBlank()) {
        return null
    }
    return rows.firstOrNull { row ->
        val stock = row.stock.trim()
        stock == code || stock.startsWith("$code ")
    }
}

internal fun aStockBacktestDetailCurrentPrice(row: AStockBacktestRow?, recommendation: AStockRecommendation): String {
    return aStockUsableDisplayValue(row?.currentPrice)
        ?: latestAStockBacktestClose(row)
        ?: aStockUsableDisplayValue(recommendation.currentPrice)
        ?: "--"
}

internal fun aStockBacktestDetailCurrentMarketPct(row: AStockBacktestRow?): String {
    return aStockUsableDisplayValue(row?.currentMarketPct)
        ?: latestAStockBacktestMarketPct(row)
        ?: "--"
}

private fun latestAStockBacktestClose(row: AStockBacktestRow?): String? {
    row ?: return null
    return row.days.asReversed().firstNotNullOfOrNull { aStockUsableDisplayValue(it.close) }
        ?: aStockUsableDisplayValue(row.t0Close)
}

private fun latestAStockBacktestMarketPct(row: AStockBacktestRow?): String? {
    row ?: return null
    return row.days.asReversed().firstNotNullOfOrNull { aStockUsableDisplayValue(it.marketPct) }
}

private fun aStockUsableDisplayValue(value: String?): String? {
    val trimmed = value?.trim().orEmpty()
    if (trimmed.isBlank() || trimmed == "--") {
        return null
    }
    return trimmed
}

private fun adjacentAStockBacktestDetail(state: AStockBacktestDetailState, offset: Int): AStockBacktestDetailState? {
    val currentIndex = state.recommendations.indexOfFirst { sameAStockRecommendation(it, state.recommendation) }
    if (currentIndex < 0) {
        return null
    }
    val nextRecommendation = state.recommendations.getOrNull(currentIndex + offset) ?: return null
    return state.copy(
        recommendation = nextRecommendation,
        row = findAStockBacktest(state.backtests, nextRecommendation),
    )
}

internal data class AStockBacktestAdjacentLabels(
    val previous: String? = null,
    val next: String? = null,
) {
    val hasAnyTarget: Boolean
        get() = previous != null || next != null
}

internal fun aStockBacktestAdjacentLabels(
    recommendations: List<AStockRecommendation>,
    current: AStockRecommendation,
): AStockBacktestAdjacentLabels {
    if (recommendations.size <= 1) {
        return AStockBacktestAdjacentLabels()
    }
    val currentIndex = recommendations.indexOfFirst { sameAStockRecommendation(it, current) }
    if (currentIndex < 0) {
        return AStockBacktestAdjacentLabels()
    }
    return AStockBacktestAdjacentLabels(
        previous = recommendations.getOrNull(currentIndex - 1)?.let(::aStockRecommendationCodeNameLabel)?.ifBlank { null },
        next = recommendations.getOrNull(currentIndex + 1)?.let(::aStockRecommendationCodeNameLabel)?.ifBlank { null },
    )
}

internal fun aStockRecommendationCodeNameLabel(item: AStockRecommendation): String {
    return listOf(item.code.trim(), item.name.trim()).filter { it.isNotBlank() }.joinToString(" ")
}

private fun sameAStockRecommendation(left: AStockRecommendation, right: AStockRecommendation): Boolean {
    val leftCode = left.code.trim()
    val rightCode = right.code.trim()
    if (leftCode.isNotBlank() && rightCode.isNotBlank()) {
        return leftCode == rightCode
    }
    return left.name == right.name && left.rank == right.rank
}

private fun formatAuctionAmount(value: Double): String {
    return when {
        value >= 100_000_000 -> String.format("%.2f亿", value / 100_000_000)
        value >= 10_000 -> String.format("%.2f万", value / 10_000)
        value > 0 -> String.format("%.0f", value)
        else -> "0"
    }
}

internal fun stockResearchDisplayDate(item: StockResearch): String {
    val researchDate = item.researchDate.trim()
    if (researchDate.isNotBlank()) {
        return researchDate
    }
    val publishTime = item.publishTime.trim()
    if (publishTime.length >= 10) {
        return publishTime.take(10)
    }
    return publishTime
}

internal fun stockResearchDetailBody(item: StockResearch): String =
    item.sourceText.trim().ifBlank { item.pdfText.trim().ifBlank { item.summary.trim() } }

internal fun stockResearchTotalPages(total: Int, pageSize: Int): Int {
    if (total <= 0) {
        return 1
    }
    val safePageSize = pageSize.coerceAtLeast(1)
    return ((total + safePageSize - 1) / safePageSize).coerceAtLeast(1)
}

internal fun stockResearchOpenableUrl(value: String): String {
    val trimmed = value.trim()
    if (trimmed.isBlank()) {
        return ""
    }
    return when {
        trimmed.startsWith("http://", ignoreCase = true) -> trimmed
        trimmed.startsWith("https://", ignoreCase = true) -> trimmed
        trimmed.startsWith("//") -> "https:$trimmed"
        trimmed.startsWith("www.", ignoreCase = true) -> "https://$trimmed"
        else -> ""
    }
}

internal fun stockResearchSourceLabel(item: StockResearch): String {
    val sourceType = item.sourceType.trim()
    if (sourceType.equals("cninfo_investor_relation", ignoreCase = true)) {
        return "投资者关系"
    }
    return when (item.kind.trim().lowercase()) {
        "survey", "调研" -> "调研"
        "report", "研报" -> "研报"
        else -> {
            if (sourceType.contains("investor", ignoreCase = true)) {
                "投资者关系"
            } else {
                "研报调研"
            }
        }
    }
}

private fun stockResearchStockLabel(item: StockResearch): String =
    listOf(item.code.trim(), item.name.trim()).filter { it.isNotBlank() }.joinToString(" ").ifBlank { "--" }

private fun stockResearchListSubtitle(item: StockResearch): String =
    listOf(item.title.trim(), item.institution.trim()).filter { it.isNotBlank() }.joinToString(" ")

private fun stockResearchDetailMeta(item: StockResearch): String {
    val meta = mutableListOf<String>()
    item.institution.trim().takeIf { it.isNotBlank() }?.let { meta += "机构：$it" }
    item.analyst.trim().takeIf { it.isNotBlank() }?.let { meta += "分析师：$it" }
    item.rating.trim().takeIf { it.isNotBlank() }?.let { meta += "评级：$it" }
    item.targetPrice.trim().takeIf { it.isNotBlank() }?.let { meta += "目标价：$it" }
    item.pdfStatus.trim().takeIf { it.isNotBlank() }?.let { meta += "PDF：$it" }
    return meta.joinToString("  ")
}

@Composable
private fun SimpleRow(title: String, subtitle: String, modifier: Modifier = Modifier) {
    Card(modifier = modifier) {
        Column(Modifier.fillMaxWidth().padding(12.dp)) {
            Text(title.ifBlank { "--" }, fontWeight = FontWeight.SemiBold, maxLines = 2, overflow = TextOverflow.Ellipsis)
            if (subtitle.isNotBlank()) {
                Spacer(Modifier.height(4.dp))
                Text(subtitle, style = MaterialTheme.typography.bodySmall, maxLines = 3, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

@Composable
private fun ConnectionRow(
    title: String,
    value: String,
    service: ServiceStatus?,
    testResult: ConnectionTestResult?,
    onTest: () -> Unit,
) {
    val statusKey = connectionStatusKey(service)
    val testColor = when (testResult?.ok) {
        true -> MaterialTheme.colorScheme.primary
        false -> MaterialTheme.colorScheme.error
        null -> MaterialTheme.colorScheme.onSurfaceVariant
    }
    Card {
        Row(
            modifier = Modifier.fillMaxWidth().padding(12.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(Modifier.weight(1f)) {
                Text(title.ifBlank { "--" }, fontWeight = FontWeight.SemiBold, maxLines = 2, overflow = TextOverflow.Ellipsis)
                if (value.isNotBlank()) {
                    Spacer(Modifier.height(4.dp))
                    Text(value, style = MaterialTheme.typography.bodySmall, maxLines = 3, overflow = TextOverflow.Ellipsis)
                }
                if (testResult != null) {
                    Spacer(Modifier.height(4.dp))
                    Text(
                        text = when {
                            testResult.loading -> "测试中"
                            testResult.ok == true -> "测试通过 ${testResult.message}"
                            else -> "测试失败 ${testResult.message.ifBlank { "--" }}"
                        },
                        style = MaterialTheme.typography.bodySmall,
                        color = testColor,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
            }
            ConnectionStatusDot(statusKey)
            TextButton(onClick = onTest, enabled = testResult?.loading != true) {
                Text(if (testResult?.loading == true) "测试中" else "测试")
            }
        }
    }
}

@Composable
private fun ConnectionStatusDot(statusKey: String) {
    Box(
        modifier = Modifier
            .size(12.dp)
            .background(connectionStatusColor(statusKey), CircleShape),
    )
}

private fun connectionStatusKey(service: ServiceStatus?): String {
    if (service == null) {
        return "failed"
    }
    val normalized = when (service.status.trim().lowercase()) {
        "", "ok", "healthy", "normal", "ready" -> "ok"
        "working", "busy", "running", "starting", "warming" -> "working"
        else -> "failed"
    }
    return if (normalized == "ok" && !service.healthy) "failed" else normalized
}

private fun connectionStatusLabel(statusKey: String): String {
    return when (statusKey) {
        "ok" -> "正常"
        "working" -> "工作中"
        else -> "异常"
    }
}

private fun connectionStatusColor(statusKey: String): Color {
    return when (statusKey) {
        "ok" -> Color(0xFF2E7D32)
        "working" -> Color(0xFF1565C0)
        else -> Color(0xFFC62828)
    }
}

@Composable
private fun SectionTitle(title: String) {
    Text(title, style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
}

@Composable
private fun RecommendationSeparator() {
    val lineColor = MaterialTheme.colorScheme.outline
    Canvas(Modifier.fillMaxWidth().height(20.dp)) {
        val y = size.height / 2f
        drawLine(lineColor, Offset(0f, y), Offset(size.width, y), strokeWidth = 2f, cap = StrokeCap.Round)
    }
}

@Composable
private fun EmptyState(text: String) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.Center, horizontalAlignment = Alignment.CenterHorizontally) {
        Text(text)
    }
}

private fun String?.ifNullOrBlank(fallback: String): String {
    return if (isNullOrBlank()) fallback else this
}

private fun fallbackModules(): List<AndroidModule> = listOf(
    AndroidModule("dashboard", "总览", "", "dashboard", "workbench"),
    AndroidModule("projects", "项目", "", "projects", "public_opinion"),
    AndroidModule("articles", "文章", "", "articles", "public_opinion"),
    AndroidModule("search", "搜索", "", "search", "public_opinion"),
    AndroidModule("analysis", "分析", "", "analysis", "public_opinion"),
    AndroidModule("reports", "报告", "", "reports", "public_opinion"),
    AndroidModule("a_stock", "A股", "", "a-stock", "finance"),
    AndroidModule("auction", "集合", "", "a-stock/auction", "finance"),
    AndroidModule("stock_research", "研报", "", "stock-research", "finance"),
    AndroidModule("holdings", "持仓", "", "holdings", "finance"),
    AndroidModule("system", "系统", "", "system", "admin"),
)

private fun moduleIcon(key: String): ImageVector = when (key) {
    "dashboard" -> Icons.Default.Dashboard
    "projects" -> Icons.Default.Business
    "articles" -> Icons.Default.Article
    "search" -> Icons.Default.Search
    "analysis" -> Icons.Default.Assessment
    "reports" -> Icons.Default.Description
    "a_stock" -> Icons.Default.ShowChart
    "auction" -> Icons.Default.Assessment
    "stock_research" -> StockResearchIcon
    "holdings" -> Icons.Default.Groups
    else -> Icons.Default.Settings
}

private val StockResearchIcon: ImageVector by lazy {
    ImageVector.Builder(
        name = "StockResearchIcon",
        defaultWidth = 24.dp,
        defaultHeight = 24.dp,
        viewportWidth = 24f,
        viewportHeight = 24f,
    ).apply {
        path(
            fill = null,
            stroke = SolidColor(Color.Black),
            strokeLineWidth = 1.7f,
            strokeLineCap = StrokeCap.Round,
            strokeLineJoin = StrokeJoin.Round,
        ) {
            moveTo(4.2f,3.2f)
            lineTo(14.1f,3.2f)
            lineTo(14.1f,5.4f)
            moveTo(4.2f,3.2f)
            lineTo(4.2f,17.4f)
            lineTo(6.4f,17.4f)

            moveTo(6.4f,5.5f)
            lineTo(15.0f,5.5f)
            lineTo(18.8f,9.3f)
            lineTo(18.8f,18.8f)
            lineTo(6.4f,18.8f)
            close()

            moveTo(15.0f,5.5f)
            lineTo(15.0f,9.3f)
            lineTo(18.8f,9.3f)
        }
        path(
            fill = null,
            stroke = SolidColor(Color.Black),
            strokeLineWidth = 1.45f,
            strokeLineCap = StrokeCap.Round,
            strokeLineJoin = StrokeJoin.Round,
        ) {
            moveTo(8.4f,10.1f)
            lineTo(12.9f,10.1f)
            moveTo(8.4f,12.4f)
            lineTo(11.0f,12.4f)
            moveTo(8.4f,14.7f)
            lineTo(11.0f,14.7f)
            moveTo(8.4f,17.0f)
            lineTo(13.6f,17.0f)
        }
        path(
            fill = null,
            stroke = SolidColor(Color.Black),
            strokeLineWidth = 1.95f,
            strokeLineCap = StrokeCap.Round,
            strokeLineJoin = StrokeJoin.Round,
        ) {
            moveTo(19.2f,13.5f)
            curveTo(19.2f,15.5f,17.6f,17.1f,15.6f,17.1f)
            curveTo(13.6f,17.1f,12.0f,15.5f,12.0f,13.5f)
            curveTo(12.0f,11.5f,13.6f,9.9f,15.6f,9.9f)
            curveTo(17.6f,9.9f,19.2f,11.5f,19.2f,13.5f)
            close()

            moveTo(18.1f,16.0f)
            lineTo(21.4f,19.3f)
        }
    }.build()
}
