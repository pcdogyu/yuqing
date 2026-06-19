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
data class AStockDashboard(val auction: AStockAuctionListResult = AStockAuctionListResult())

@Serializable
data class AStockAuctionListResult(
    val items: List<AStockAuctionAmount> = emptyList(),
    val total: Int = 0,
    val date: String = "",
    @SerialName("latest_date") val latestDate: String = "",
    @SerialName("total_amount") val totalAmount: Double = 0.0,
)

@Serializable
data class AStockAuctionAmount(
    val code: String = "",
    val name: String = "",
    @SerialName("auction_price") val auctionPrice: Double = 0.0,
    @SerialName("auction_amount") val auctionAmount: Double = 0.0,
    val status: String = "",
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
