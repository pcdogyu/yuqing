package com.jiansutech.yuqing.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
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
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.jiansutech.yuqing.data.AndroidDashboard
import com.jiansutech.yuqing.data.AndroidModule
import com.jiansutech.yuqing.data.AStockAuctionAmount
import com.jiansutech.yuqing.data.AStockAuctionListResult
import com.jiansutech.yuqing.data.AStockRecommendation
import com.jiansutech.yuqing.data.ArticleItem
import com.jiansutech.yuqing.data.ItemListResult
import com.jiansutech.yuqing.data.Project
import com.jiansutech.yuqing.data.Report
import com.jiansutech.yuqing.data.SchedulerJob
import com.jiansutech.yuqing.data.ServiceStatus
import com.jiansutech.yuqing.data.StockHolding
import com.jiansutech.yuqing.data.StockResearch
import com.jiansutech.yuqing.data.TaskRun

@Composable
fun YuqingApp(viewModel: YuqingViewModel) {
    val state by viewModel.uiState.collectAsState()
    YuqingTheme {
        PortalScreen(state, viewModel)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun PortalScreen(state: YuqingUiState, viewModel: YuqingViewModel) {
    val modules = state.modules.ifEmpty { fallbackModules() }
    val fallback = fallbackModules()
    val selected = modules.firstOrNull { it.key == state.selectedModuleKey }
        ?: fallback.firstOrNull { it.key == state.selectedModuleKey }
        ?: modules.first()
    val hideHeaderContent = selected.key == "dashboard" || selected.key == "search" || selected.key == "a_stock" || selected.key == "auction"
    Scaffold(
        topBar = if (hideHeaderContent) {
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
            NavigationBar {
                listOf("dashboard", "articles", "a_stock", "auction", "system").forEach { key ->
                    val module = modules.firstOrNull { it.key == key }
                        ?: fallback.firstOrNull { it.key == key }
                        ?: AndroidModule(key = key, title = key)
                    NavigationBarItem(
                        selected = selected.key == key,
                        onClick = { viewModel.selectModule(key) },
                        icon = { Icon(moduleIcon(key), contentDescription = module.title) },
                        label = { Text(module.title, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                    )
                }
            }
        },
    ) { padding ->
        val contentModifier = if (hideHeaderContent) {
            Modifier.padding(padding).fillMaxSize().statusBarsPadding()
        } else {
            Modifier.padding(padding).fillMaxSize()
        }
        Column(contentModifier) {
            if (!hideHeaderContent) {
                StatusMessages(state)
                ModuleStrip(modules, selected.key, viewModel)
            }
            if (state.loading) {
                Row(Modifier.fillMaxWidth().padding(12.dp), horizontalArrangement = Arrangement.Center) {
                    CircularProgressIndicator()
                }
            }
            ModuleContent(selected.key, state.dashboard, state, viewModel)
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
private fun ModuleContent(key: String, dashboard: AndroidDashboard?, state: YuqingUiState, viewModel: YuqingViewModel) {
    if (dashboard == null) {
        EmptyState("暂无缓存数据，请刷新")
        return
    }
    when (key) {
        "dashboard" -> DashboardModule(dashboard)
        "projects" -> ProjectsModule(dashboard.projects, dashboard.rules)
        "articles" -> ArticlesModule(state.articleList ?: ItemListResult(), viewModel)
        "search" -> SearchModule(state, viewModel)
        "auction" -> AStockAuctionModule(state.aStockAuction, viewModel)
        "analysis" -> AnalysisModule(dashboard)
        "reports" -> ReportsModule(dashboard.reports, viewModel)
        "a_stock" -> AStockModule(state, viewModel)
        "stock_research" -> StockResearchModule(dashboard.stockResearch.items, viewModel)
        "holdings" -> HoldingsModule(dashboard.holdings.items)
        "system" -> SystemModule(dashboard, state, viewModel)
        else -> GenericModule(key, dashboard)
    }
}

@Composable
private fun DashboardModule(dashboard: AndroidDashboard) {
    val recentTasks = dashboard.recentTaskRunsForDisplay()
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        item {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
                MetricCard("文章", dashboard.overview.articleCount.toString(), Modifier.weight(1f))
                MetricCard("项目", dashboard.overview.projectCount.toString(), Modifier.weight(1f))
            }
        }
        item {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
                MetricCard("报告", dashboard.overview.reportCount.toString(), Modifier.weight(1f))
                MetricCard("任务", dashboard.overview.crawlRunCount.toString(), Modifier.weight(1f))
            }
        }
        item { SectionTitle("最新文章") }
        items(dashboard.articles.items) { ArticleRow(it) }
        item { SectionTitle("最近任务") }
        if (recentTasks.isEmpty()) {
            item { SimpleRow("暂无任务记录", "") }
        }
        items(recentTasks) { SimpleRow(it.taskName, "${it.status} ${it.message}") }
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
private fun ArticlesModule(result: ItemListResult, viewModel: YuqingViewModel) {
    val pageSize = result.pageSize.coerceAtLeast(1)
    val canGoPrevious = result.page > 1
    val canGoNext = result.page * pageSize < result.total
    val totalPages = if (result.total <= 0) 1 else ((result.total + pageSize - 1) / pageSize).coerceAtLeast(1)
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        items(result.items) { ArticleRow(it) }
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
            items(result.items) { ArticleRow(it) }
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
private fun AStockModule(state: YuqingUiState, viewModel: YuqingViewModel) {
    val window = state.aStockRecommendationWindow
    val morningSnapshot = state.morningAStockRecommendation
    val morningRecommendations = state.morningAStockRecommendations
    val afternoonSnapshot = state.afternoonAStockRecommendation
    val afternoonRecommendations = state.afternoonAStockRecommendations
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        item {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text("日期选择", style = MaterialTheme.typography.labelMedium)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp), verticalAlignment = Alignment.CenterVertically) {
                    TextButton(onClick = { viewModel.shiftAStockRecommendationDate(-1) }) {
                        Text("前一交易日")
                    }
                    Text(window.date, modifier = Modifier.weight(1f), fontWeight = FontWeight.SemiBold)
                    TextButton(onClick = viewModel::resetAStockRecommendationDate) {
                        Text("今日")
                    }
                    TextButton(onClick = { viewModel.shiftAStockRecommendationDate(1) }) {
                        Text("后一交易日")
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
        items(morningRecommendations) { AStockRecommendationRow(it) }
        item { RecommendationSeparator() }
        item { SectionTitle("下午推荐") }
        if (afternoonRecommendations.isEmpty()) {
            item { SimpleRow("暂无下午推荐", afternoonSnapshot?.emptyReason.ifNullOrBlank("09:30-13:00 暂无推荐股票")) }
        }
        items(afternoonRecommendations) { AStockRecommendationRow(it) }
    }
}

@Composable
private fun AStockAuctionModule(result: AStockAuctionListResult, viewModel: YuqingViewModel) {
    var trendDays by remember { mutableStateOf(7) }
    val storedCount = result.summaryCount.takeIf { it > 0 } ?: result.total
    val completenessText = if (storedCount in 1 until 4000) "数据可能不全" else ""
    val shenzhenLeaders = result.items
        .filter { it.code.startsWith("0") || it.code.startsWith("3") }
        .sortedByDescending { it.auctionAmount }
        .take(2)
    val shanghaiLeader = result.items
        .filter { it.code.startsWith("6") }
        .maxByOrNull { it.auctionAmount }
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
        item { SectionTitle("深市金额最高") }
        if (shenzhenLeaders.isEmpty()) {
            item { SimpleRow("暂无深市集合竞价数据", "请刷新或等待交易日数据写入") }
        }
        items(shenzhenLeaders) { AStockAuctionRow(it) }
        item { SectionTitle("沪市金额最高") }
        if (shanghaiLeader == null) {
            item { SimpleRow("暂无沪市集合竞价数据", "请刷新或等待交易日数据写入") }
        } else {
            item { AStockAuctionRow(shanghaiLeader) }
        }
        item { AuctionTrendHeader(trendDays, onPeriodSelected = { trendDays = it }) }
        item { AStockAuctionTrendChart(result, trendDays) }
        if (result.trend.isEmpty()) {
            item { SimpleRow("暂无历史走势", "接口暂未返回历史集合竞价金额") }
        }
    }
}

@Composable
private fun StockResearchModule(items: List<StockResearch>, viewModel: YuqingViewModel) {
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        item {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Button(onClick = { viewModel.requestAction("stock_research_backfill", "回补研报调研") }) { Text("回补") }
                Button(onClick = { viewModel.requestAction("stock_research_pdf_parse", "解析研报 PDF") }) { Text("解析PDF") }
            }
        }
        items(items) { SimpleRow("${it.code} ${it.name}", "${it.title} ${it.institution} ${it.pdfStatus}") }
    }
}

@Composable
private fun HoldingsModule(items: List<StockHolding>) {
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        items(items) { SimpleRow("${it.stockCode} ${it.stockName}", "${it.holderName} ${it.holderType} ${it.floatRatio}%") }
    }
}

@Composable
private fun SystemModule(dashboard: AndroidDashboard, state: YuqingUiState, viewModel: YuqingViewModel) {
    val recentTasks = dashboard.recentTaskRunsForDisplay()
    val database = dashboard.operations.database
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        item { SectionTitle("连接") }
        item { SimpleRow("API 地址", state.session.apiBaseUrl) }
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
    Card {
        Row(Modifier.fillMaxWidth().padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text(service.name, fontWeight = FontWeight.SemiBold)
                Text(if (service.healthy) "healthy" else service.message, style = MaterialTheme.typography.bodySmall)
            }
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
private fun MetricCard(title: String, value: String, modifier: Modifier = Modifier) {
    Card(modifier = modifier) {
        Column(Modifier.padding(14.dp)) {
            Text(title, style = MaterialTheme.typography.labelMedium)
            Text(value, style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        }
    }
}

@Composable
private fun ArticleRow(item: ArticleItem) {
    SimpleRow(item.title, listOf(item.sourceType, item.publishTimeText, item.summary).filter { it.isNotBlank() }.joinToString("  "))
}

@Composable
private fun AStockRecommendationRow(item: AStockRecommendation) {
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
        .takeLast(days)
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
            Canvas(Modifier.fillMaxWidth().height(150.dp)) {
                val left = 8f
                val right = size.width - 8f
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
                points.zipWithNext().forEach { (start, end) ->
                    drawLine(lineColor, start, end, strokeWidth = 5f, cap = StrokeCap.Round)
                }
                points.forEach { point ->
                    drawCircle(lineColor, radius = 5f, center = point)
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

private fun formatAuctionAmount(value: Double): String {
    return when {
        value >= 100_000_000 -> String.format("%.2f亿", value / 100_000_000)
        value >= 10_000 -> String.format("%.2f万", value / 10_000)
        value > 0 -> String.format("%.0f", value)
        else -> "0"
    }
}

@Composable
private fun SimpleRow(title: String, subtitle: String) {
    Card {
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
    "stock_research" -> Icons.Default.ShowChart
    "holdings" -> Icons.Default.Groups
    else -> Icons.Default.Settings
}
