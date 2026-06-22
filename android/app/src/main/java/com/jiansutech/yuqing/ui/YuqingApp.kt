package com.jiansutech.yuqing.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
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
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.jiansutech.yuqing.data.AndroidDashboard
import com.jiansutech.yuqing.data.AndroidModule
import com.jiansutech.yuqing.data.AStockAuctionAmount
import com.jiansutech.yuqing.data.AStockAuctionListResult
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
    val selected = modules.firstOrNull { it.key == state.selectedModuleKey } ?: modules.first()
    Scaffold(
        topBar = {
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
        },
        bottomBar = {
            NavigationBar {
                listOf("dashboard", "articles", "search", "a_stock", "system").forEach { key ->
                    val module = modules.firstOrNull { it.key == key } ?: AndroidModule(key = key, title = key)
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
        Column(Modifier.padding(padding).fillMaxSize()) {
            StatusMessages(state)
            ModuleStrip(modules, selected.key, viewModel)
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
        "analysis" -> AnalysisModule(dashboard)
        "reports" -> ReportsModule(dashboard.reports, viewModel)
        "a_stock" -> AStockModule(state.aStockAuction ?: dashboard.aStock.auction, state, viewModel)
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
private fun AStockModule(auction: AStockAuctionListResult, state: YuqingUiState, viewModel: YuqingViewModel) {
    val dates = auction.dates
    val currentDate = state.aStockAuctionDate.ifBlank { auction.date.ifBlank { auction.latestDate } }
    val currentIndex = dates.indexOf(currentDate)
    val canGoNewer = currentIndex > 0
    val canGoOlder = currentIndex >= 0 && currentIndex < dates.lastIndex
    LazyColumn(contentPadding = PaddingValues(12.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        item {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
                Button(onClick = { viewModel.requestAction("a_stock_auction_latest", "抓取最新集合竞价") }) { Text("抓取最新") }
                Button(onClick = { viewModel.requestAction("a_stock_auction_backfill", "回补集合竞价", mapOf("days" to "30")) }) { Text("回补") }
            }
        }
        item {
            Row(verticalAlignment = Alignment.CenterVertically) {
                OutlinedTextField(
                    value = currentDate,
                    onValueChange = viewModel::updateAStockAuctionDate,
                    label = { Text("交易日期") },
                    modifier = Modifier.weight(1f),
                    singleLine = true,
                )
                Spacer(Modifier.width(8.dp))
                Button(onClick = { viewModel.loadAStockAuction(currentDate) }) { Text("查询") }
            }
        }
        item {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.fillMaxWidth()) {
                Button(
                    onClick = { viewModel.selectAdjacentAStockAuctionDate(1) },
                    enabled = canGoOlder,
                    modifier = Modifier.weight(1f),
                ) { Text("前一日") }
                Button(
                    onClick = { viewModel.selectAdjacentAStockAuctionDate(-1) },
                    enabled = canGoNewer,
                    modifier = Modifier.weight(1f),
                ) { Text("后一日") }
            }
        }
        item {
            SimpleRow(
                "集合竞价 ${auction.date.ifBlank { currentDate }}",
                listOf(
                    "股票 ${auction.summaryCount.ifZero(auction.total)}",
                    "总金额 ${formatAStockMoney(auction.totalAmount)}",
                    "最新 ${auction.latestDate.ifBlank { "--" }}",
                ).joinToString("  "),
            )
        }
        auction.maxItem?.let { maxItem ->
            item {
                SimpleRow(
                    "最大成交额 ${maxItem.code} ${maxItem.name}",
                    "金额 ${formatAStockMoney(maxItem.auctionAmount)}  价格 ${formatAStockNumber(maxItem.auctionPrice)}  成交量 ${formatAStockVolume(maxItem.auctionVolume)}",
                )
            }
        }
        item {
            SimpleRow(
                "更新时间",
                auction.fetchedAt.ifBlank { auction.items.firstOrNull()?.fetchedAt.orEmpty() }.ifBlank { "--" },
            )
        }
        if (auction.items.isEmpty()) {
            item { SimpleRow("暂无集合竞价数据", "可切换日期或先执行抓取/回补") }
        }
        items(auction.items) { AStockAuctionRow(it) }
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
private fun AStockAuctionRow(item: AStockAuctionAmount) {
    SimpleRow(
        "${item.code} ${item.name}",
        listOf(
            "价格 ${formatAStockNumber(item.auctionPrice)}",
            "成交量 ${formatAStockVolume(item.auctionVolume)}",
            "金额 ${formatAStockMoney(item.auctionAmount)}",
            item.status,
            item.source,
        ).filter { it.isNotBlank() }.joinToString("  "),
    )
}

private fun AndroidDashboard.recentTaskRunsForDisplay(): List<TaskRun> {
    return if (taskRuns.isNotEmpty()) taskRuns else operations.recentTaskRuns
}

private fun Int.ifZero(fallback: Int): Int {
    return if (this == 0) fallback else this
}

private fun formatAStockNumber(value: Double): String {
    return if (value == 0.0) "--" else "%.2f".format(value)
}

private fun formatAStockVolume(value: Double): String {
    return when {
        value >= 100000000 -> "%.2f亿".format(value / 100000000)
        value >= 10000 -> "%.2f万".format(value / 10000)
        value > 0 -> "%.0f".format(value)
        else -> "--"
    }
}

private fun formatAStockMoney(value: Double): String {
    return when {
        value >= 100000000 -> "%.2f亿".format(value / 100000000)
        value >= 10000 -> "%.2f万".format(value / 10000)
        value > 0 -> "%.2f".format(value)
        else -> "--"
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
private fun EmptyState(text: String) {
    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.Center, horizontalAlignment = Alignment.CenterHorizontally) {
        Text(text)
    }
}

private fun fallbackModules(): List<AndroidModule> = listOf(
    AndroidModule("dashboard", "总览", "", "dashboard", "workbench"),
    AndroidModule("projects", "项目", "", "projects", "public_opinion"),
    AndroidModule("articles", "文章", "", "articles", "public_opinion"),
    AndroidModule("search", "搜索", "", "search", "public_opinion"),
    AndroidModule("analysis", "分析", "", "analysis", "public_opinion"),
    AndroidModule("reports", "报告", "", "reports", "public_opinion"),
    AndroidModule("a_stock", "A股", "", "a-stock", "finance"),
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
    "stock_research" -> Icons.Default.ShowChart
    "holdings" -> Icons.Default.Groups
    else -> Icons.Default.Settings
}
