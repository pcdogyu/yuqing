package com.jiansutech.yuqing.notifications

import android.Manifest
import android.app.Notification
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import com.jiansutech.yuqing.MainActivity
import com.jiansutech.yuqing.astock.AStockTradingCalendar
import com.jiansutech.yuqing.data.AStockRecommendation
import com.jiansutech.yuqing.data.ApiFactory
import com.jiansutech.yuqing.data.NetworkEnvironmentSelector
import com.jiansutech.yuqing.data.SessionStore
import com.jiansutech.yuqing.data.YuqingApi
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.serialization.decodeFromString
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.LocalTime
import java.time.ZoneId

class AStockRecommendationReceiver : BroadcastReceiver() {
    private val zone: ZoneId = ZoneId.of("Asia/Shanghai")
    private val aStockNewsSourceTypes = listOf(
        "flash",
        "headline",
        "jin10_full",
        "eastmoney_kuaixun",
        "wallstreetcn_a_stock",
        "cls_telegraph",
        "sina_finance_7x24",
    )

    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action == Intent.ACTION_BOOT_COMPLETED) {
            AStockRecommendationScheduler.scheduleDailyRecommendations(context.applicationContext)
            return
        }
        if (intent.action != AStockRecommendationScheduler.alarmAction) {
            return
        }

        val appContext = context.applicationContext
        val slot = slotFromIntent(intent) ?: return
        AStockRecommendationScheduler.scheduleNext(appContext, slot)
        val pendingResult = goAsync()
        CoroutineScope(Dispatchers.IO).launch {
            try {
                val content = loadNotificationContent(appContext, slot)
                withContext(Dispatchers.Main) {
                    showNotification(appContext, slot, content)
                }
            } finally {
                pendingResult.finish()
            }
        }
    }

    private fun slotFromIntent(intent: Intent): AStockRecommendationSlot? {
        return AStockRecommendationScheduler.slotById(intent.getStringExtra(AStockRecommendationScheduler.extraSlotId))
    }

    private suspend fun loadNotificationContent(context: Context, slot: AStockRecommendationSlot): String {
        return when (slot.kind) {
            AStockNotificationKind.NewsCount -> loadNewsCountContent(context, slot)
            AStockNotificationKind.Recommendation -> loadRecommendationContent(context, slot)
        }
    }

    private suspend fun loadNewsCountContent(context: Context, slot: AStockRecommendationSlot): String {
        val session = SessionStore(context).state.first()
        val date = latestTradingDate()
        val api = currentApi(session.token)
        val (start, end) = slotWindowBounds(date, slot)
        return runCatching {
            var total = 0
            var okCount = 0
            for (sourceType in aStockNewsSourceTypes) {
                runCatching {
                    api.articles(
                        page = 1,
                        pageSize = 1,
                        start = start,
                        end = end,
                        sourceType = sourceType,
                    ).data?.total ?: 0
                }.onSuccess { count ->
                    total += count
                    okCount++
                }
            }
            if (okCount == 0) {
                return@runCatching "财经新闻抓取数量暂时不可用，请打开 App 刷新后重试。"
            }
            buildString {
                append(date)
                append(' ')
                append(slot.windowLabel)
                append(" 已抓取 ")
                append(total)
                append(" 条财经新闻")
                if (okCount < aStockNewsSourceTypes.size) {
                    append("，部分来源待同步")
                }
            }
        }.getOrDefault("财经新闻抓取数量暂时不可用，请打开 App 刷新后重试。")
    }

    private suspend fun loadRecommendationContent(context: Context, slot: AStockRecommendationSlot): String {
        val session = SessionStore(context).state.first()
        val date = latestTradingDate()
        return runCatching {
            val snapshot = currentApi(session.token)
                .aStockRecommendations(date = date, period = slot.period)
                .data ?: return@runCatching "$date ${slot.windowLabel} 暂无推荐股票"
            val recommendations = parseRecommendations(snapshot.recommendationsJson)
            if (!snapshot.found || recommendations.isEmpty()) {
                snapshot.emptyReason.ifBlank { "$date ${slot.windowLabel} 暂无推荐股票" }
            } else {
                "$date ${slot.windowLabel}：" + recommendations
                    .take(5)
                    .mapNotNull { it.displayName() }
                    .joinToString("、")
            }
        }.getOrDefault("推荐数据暂时不可用，请打开 App 刷新 A股。")
    }

    private fun parseRecommendations(raw: String): List<AStockRecommendation> {
        val payload = raw.trim().ifBlank { "[]" }
        return runCatching {
            ApiFactory.json.decodeFromString<List<AStockRecommendation>>(payload)
        }.getOrDefault(emptyList())
    }

    private suspend fun currentApi(token: String): YuqingApi {
        val selector = NetworkEnvironmentSelector { baseUrl ->
            ApiFactory.testUrl(baseUrl, "healthz").ok
        }
        return ApiFactory.yuqing(selector.detect().contentBaseUrl, token)
    }

    private fun showNotification(context: Context, slot: AStockRecommendationSlot, content: String) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            return
        }
        AStockRecommendationScheduler.ensureNotificationChannels(context)
        val openAppIntent = PendingIntent.getActivity(
            context,
            0,
            Intent(context, MainActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        val channelID = when (slot.kind) {
            AStockNotificationKind.NewsCount -> AStockRecommendationScheduler.newsCountChannelId
            AStockNotificationKind.Recommendation -> AStockRecommendationScheduler.recommendationChannelId
        }
        val notification = Notification.Builder(context, channelID)
            .setSmallIcon(android.R.drawable.ic_dialog_info)
            .setContentTitle(slot.title)
            .setContentText(content)
            .setStyle(Notification.BigTextStyle().bigText(content))
            .setContentIntent(openAppIntent)
            .setCategory(
                when (slot.kind) {
                    AStockNotificationKind.NewsCount -> Notification.CATEGORY_STATUS
                    AStockNotificationKind.Recommendation -> Notification.CATEGORY_REMINDER
                },
            )
            .setAutoCancel(true)
            .build()
        context.getSystemService(NotificationManager::class.java)
            .notify(notificationId(slot), notification)
    }

    private fun notificationId(slot: AStockRecommendationSlot): Int {
        val date = LocalDate.now(zone).toString()
        return "$date-${slot.id}".hashCode()
    }

    private fun latestTradingDate(): String {
        return AStockTradingCalendar.latestSelectableTradingDay(LocalDate.now(zone)).toString()
    }

    private fun slotWindowBounds(date: String, slot: AStockRecommendationSlot): Pair<String, String> {
        val day = runCatching { LocalDate.parse(date) }.getOrDefault(LocalDate.now(zone))
        val start = LocalDateTime.of(day, LocalTime.of(slot.windowStartHour, slot.windowStartMinute))
            .atZone(zone)
            .toInstant()
            .toString()
        val end = LocalDateTime.of(day, LocalTime.of(slot.windowEndHour, slot.windowEndMinute, 59))
            .atZone(zone)
            .toInstant()
            .toString()
        return start to end
    }

    private fun AStockRecommendation.displayName(): String? {
        val label = listOf(code, name).filter { it.isNotBlank() }.joinToString(" ")
        return label.ifBlank { null }
    }
}
