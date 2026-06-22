package com.jiansutech.yuqing.data

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class ApiEnvelope<T>(
    val code: Int = 0,
    val message: String = "",
    val data: T? = null,
)

@Serializable
data class User(
    val id: Long = 0,
    val username: String = "",
    @SerialName("display_name") val displayName: String = "",
    val role: String = "",
)

@Serializable
data class LoginRequest(val username: String, val password: String)

@Serializable
data class LoginData(
    val user: User = User(),
    @SerialName("session_token") val sessionToken: String = "",
    @SerialName("expires_at") val expiresAt: String = "",
)

@Serializable
data class AndroidBootstrap(
    @SerialName("package_name") val packageName: String = "",
    @SerialName("app_name") val appName: String = "",
    @SerialName("api_version") val apiVersion: String = "",
    val user: AndroidUser = AndroidUser(),
    val modules: List<AndroidModule> = emptyList(),
    val actions: List<String> = emptyList(),
)

@Serializable
data class AndroidUser(
    val id: Long = 0,
    val username: String = "",
    val role: String = "",
)

@Serializable
data class AndroidModule(
    val key: String = "",
    val title: String = "",
    val description: String = "",
    val path: String = "",
    val category: String = "",
    @SerialName("requires_operation") val requiresOperation: Boolean = false,
)

@Serializable
data class AndroidDashboard(
    @SerialName("generated_at") val generatedAt: String = "",
    val overview: Overview = Overview(),
    val projects: List<Project> = emptyList(),
    val rules: List<MonitorRule> = emptyList(),
    val articles: ItemListResult = ItemListResult(),
    val reports: List<Report> = emptyList(),
    val notices: List<SystemNotice> = emptyList(),
    @SerialName("task_runs") val taskRuns: List<TaskRun> = emptyList(),
    @SerialName("crawl_runs") val crawlRuns: List<CrawlRun> = emptyList(),
    val operations: OperationsSummary = OperationsSummary(),
    @SerialName("a_stock") val aStock: AStockDashboard = AStockDashboard(),
    @SerialName("stock_research") val stockResearch: StockResearchListResult = StockResearchListResult(),
    val holdings: StockInstitutionHoldingListResult = StockInstitutionHoldingListResult(),
    @SerialName("partial_errors") val partialErrors: Map<String, String> = emptyMap(),
)

@Serializable
data class Overview(
    @SerialName("article_count") val articleCount: Int = 0,
    @SerialName("project_count") val projectCount: Int = 0,
    @SerialName("report_count") val reportCount: Int = 0,
    @SerialName("crawl_run_count") val crawlRunCount: Int = 0,
    @SerialName("alert_rule_count") val alertRuleCount: Int = 0,
)

@Serializable
data class Project(
    val id: Long = 0,
    val name: String = "",
    val keywords: String = "",
    val description: String = "",
    val status: String = "",
)

@Serializable
data class MonitorRule(
    val id: Long = 0,
    @SerialName("project_id") val projectId: Long = 0,
    val name: String = "",
    @SerialName("include_keywords") val includeKeywords: String = "",
    val severity: String = "",
    val status: String = "",
)

@Serializable
data class ItemListResult(
    val items: List<ArticleItem> = emptyList(),
    val page: Int = 1,
    @SerialName("page_size") val pageSize: Int = 20,
    val total: Int = 0,
)

@Serializable
data class SearchResult(
    val items: List<ArticleItem> = emptyList(),
    val keyword: String = "",
    val total: Int = 0,
    val page: Int = 1,
    @SerialName("page_size") val pageSize: Int = 20,
)

@Serializable
data class ArticleItem(
    val id: Long = 0,
    @SerialName("source_type") val sourceType: String = "",
    val title: String = "",
    val content: String = "",
    val summary: String = "",
    @SerialName("publish_time_text") val publishTimeText: String = "",
    @SerialName("source_url") val sourceUrl: String = "",
    val favorited: Boolean = false,
    val read: Boolean = false,
)

@Serializable
data class Report(
    val id: Long = 0,
    @SerialName("project_id") val projectId: Long = 0,
    val title: String = "",
    val summary: String = "",
    val content: String = "",
    val status: String = "",
)

@Serializable
data class SystemNotice(val id: Long = 0, val title: String = "", val content: String = "")

@Serializable
data class TaskRun(
    val id: Long = 0,
    @SerialName("task_name") val taskName: String = "",
    val status: String = "",
    val message: String = "",
    @SerialName("started_at") val startedAt: String = "",
)

@Serializable
data class CrawlRun(
    val id: Long = 0,
    @SerialName("source_type") val sourceType: String = "",
    val status: String = "",
    @SerialName("fetched_count") val fetchedCount: Int = 0,
    @SerialName("inserted_count") val insertedCount: Int = 0,
    @SerialName("error_text") val errorText: String = "",
)

@Serializable
data class OperationsSummary(
    val ready: Boolean = false,
    val services: List<ServiceStatus> = emptyList(),
    @SerialName("scheduler_jobs") val schedulerJobs: List<SchedulerJob> = emptyList(),
    @SerialName("recent_task_runs") val recentTaskRuns: List<TaskRun> = emptyList(),
    val database: DatabaseConfigStatus = DatabaseConfigStatus(),
)

@Serializable
data class DatabaseConfigStatus(
    val driver: String = "",
    @SerialName("configured_driver") val configuredDriver: String = "",
    @SerialName("runtime_driver") val runtimeDriver: String = "",
    val status: String = "",
    val message: String = "",
    @SerialName("config_path") val configPath: String = "",
    @SerialName("restart_required") val restartRequired: Boolean = false,
    @SerialName("sqlite_path") val sqlitePath: String = "",
    @SerialName("postgres_host") val postgresHost: String = "",
    @SerialName("postgres_port") val postgresPort: String = "",
    @SerialName("postgres_database") val postgresDatabase: String = "",
    @SerialName("postgres_user") val postgresUser: String = "",
    @SerialName("postgres_sslmode") val postgresSslMode: String = "",
    @SerialName("postgres_configured") val postgresConfigured: Boolean = false,
    @SerialName("postgres_dsn") val postgresDsn: String = "",
)

@Serializable
data class ServiceStatus(
    val name: String = "",
    val url: String = "",
    val healthy: Boolean = false,
    val message: String = "",
)

@Serializable
data class SchedulerJob(
    val name: String = "",
    val group: String = "",
    val description: String = "",
    val enabled: Boolean = false,
    @SerialName("last_status") val lastStatus: String = "",
)

@Serializable
data class AStockDashboard(
    val auction: AStockAuctionListResult = AStockAuctionListResult(),
    val recommendation: AStockRecommendationSnapshot = AStockRecommendationSnapshot(),
)

@Serializable
data class AStockRecommendationSnapshot(
    val found: Boolean = false,
    @SerialName("strategy_date") val strategyDate: String = "",
    val period: String = "",
    @SerialName("ignore_recent") val ignoreRecent: Boolean = false,
    @SerialName("recommendations_json") val recommendationsJson: String = "[]",
    @SerialName("backtests_json") val backtestsJson: String = "[]",
    @SerialName("backtest_status") val backtestStatus: String = "",
    @SerialName("generated_count") val generatedCount: Int = 0,
    @SerialName("recent_filtered") val recentFiltered: Int = 0,
    @SerialName("same_day_morning_filtered") val sameDayMorningFiltered: Int = 0,
    @SerialName("limit_up_filter_enabled") val limitUpFilterEnabled: Boolean = false,
    @SerialName("limit_up_filtered") val limitUpFiltered: Int = 0,
    @SerialName("market_candidate_status") val marketCandidateStatus: String = "",
    @SerialName("market_candidate_count") val marketCandidateCount: Int = 0,
    @SerialName("empty_reason") val emptyReason: String = "",
    @SerialName("updated_at") val updatedAt: String = "",
)

@Serializable
data class AStockRecommendation(
    @SerialName("Rank") val rank: Int = 0,
    @SerialName("Hotspot") val hotspot: String = "",
    @SerialName("Code") val code: String = "",
    @SerialName("Name") val name: String = "",
    @SerialName("HotspotScore") val hotspotScore: Int = 0,
    @SerialName("MarketScore") val marketScore: Int = 0,
    @SerialName("PrevClose") val prevClose: String = "",
    @SerialName("PrevPct") val prevPct: String = "",
    @SerialName("Change30") val change30: String = "",
    @SerialName("Change60") val change60: String = "",
    @SerialName("CurrentPrice") val currentPrice: String = "",
    @SerialName("TodayPct") val todayPct: String = "",
    @SerialName("HoldingSummary") val holdingSummary: String = "",
    @SerialName("HoldingRatio") val holdingRatio: String = "",
    @SerialName("Reason") val reason: String = "",
)

@Serializable
data class AStockBacktestRow(
    @SerialName("Stock") val stock: String = "",
    @SerialName("EntryOpen") val entryOpen: String = "",
    @SerialName("T0Return") val t0Return: String = "",
    @SerialName("Days") val days: List<AStockBacktestCell> = emptyList(),
    @SerialName("BestReturn") val bestReturn: String = "",
    @SerialName("Status") val status: String = "",
)

@Serializable
data class AStockBacktestCell(
    @SerialName("Close") val close: String = "",
    @SerialName("Return") val returnPct: String = "",
)

@Serializable
data class AStockAuctionListResult(
    val items: List<AStockAuctionAmount> = emptyList(),
    val page: Int = 1,
    @SerialName("page_size") val pageSize: Int = 6000,
    val total: Int = 0,
    val date: String = "",
    val keyword: String = "",
    @SerialName("latest_date") val latestDate: String = "",
    val dates: List<String> = emptyList(),
    @SerialName("summary_count") val summaryCount: Int = 0,
    @SerialName("total_amount") val totalAmount: Double = 0.0,
    @SerialName("max_item") val maxItem: AStockAuctionAmount? = null,
    @SerialName("fetched_at") val fetchedAt: String = "",
    val trend: List<AStockAuctionTrend> = emptyList(),
)

@Serializable
data class AStockAuctionAmount(
    @SerialName("trade_date") val tradeDate: String = "",
    val code: String = "",
    val name: String = "",
    @SerialName("auction_price") val auctionPrice: Double = 0.0,
    @SerialName("auction_volume") val auctionVolume: Double = 0.0,
    @SerialName("auction_amount") val auctionAmount: Double = 0.0,
    val source: String = "",
    val status: String = "",
    @SerialName("fetched_at") val fetchedAt: String = "",
)

@Serializable
data class AStockAuctionTrend(
    val date: String = "",
    @SerialName("stock_count") val stockCount: Int = 0,
    @SerialName("total_volume") val totalVolume: Double = 0.0,
    @SerialName("total_amount") val totalAmount: Double = 0.0,
    @SerialName("max_stock_code") val maxStockCode: String = "",
    @SerialName("max_stock_name") val maxStockName: String = "",
)

@Serializable
data class StockResearchListResult(
    val items: List<StockResearch> = emptyList(),
    val total: Int = 0,
    val page: Int = 1,
    @SerialName("page_size") val pageSize: Int = 20,
)

@Serializable
data class StockResearch(
    val id: Long = 0,
    val code: String = "",
    val name: String = "",
    val kind: String = "",
    val title: String = "",
    val institution: String = "",
    val analyst: String = "",
    val rating: String = "",
    @SerialName("research_date") val researchDate: String = "",
    @SerialName("pdf_status") val pdfStatus: String = "",
)

@Serializable
data class StockInstitutionHoldingListResult(
    val items: List<StockHolding> = emptyList(),
    val total: Int = 0,
    val page: Int = 1,
    @SerialName("page_size") val pageSize: Int = 20,
)

@Serializable
data class StockHolding(
    val id: Long = 0,
    @SerialName("stock_code") val stockCode: String = "",
    @SerialName("stock_name") val stockName: String = "",
    @SerialName("holder_name") val holderName: String = "",
    @SerialName("holder_type") val holderType: String = "",
    @SerialName("report_period") val reportPeriod: String = "",
    @SerialName("float_ratio") val floatRatio: Double = 0.0,
)

@Serializable
data class AndroidActionRequest(val params: Map<String, String> = emptyMap())

@Serializable
data class AndroidActionResponse(
    val action: String = "",
    val status: String = "",
    @SerialName("target_url") val targetUrl: String = "",
    val message: String = "",
)
