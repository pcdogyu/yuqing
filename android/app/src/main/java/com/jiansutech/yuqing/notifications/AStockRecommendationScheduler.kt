package com.jiansutech.yuqing.notifications

import android.annotation.SuppressLint
import android.app.AlarmManager
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build
import com.jiansutech.yuqing.astock.AStockTradingCalendar
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.LocalTime
import java.time.ZoneId

object AStockRecommendationScheduler {
    const val alarmAction = "com.jiansutech.yuqing.A_STOCK_RECOMMENDATION_ALARM"
    const val recommendationChannelId = "a_stock_recommendations"
    const val newsCountChannelId = "a_stock_news_counts"
    const val extraSlotId = "slot_id"

    private val zone: ZoneId = ZoneId.of("Asia/Shanghai")
    private val obsoleteSlotIds = listOf("morning_0930", "afternoon_1300")

    val slots: List<AStockRecommendationSlot> = listOf(
        AStockRecommendationSlot(
            id = "morning_0920",
            period = "morning",
            kind = AStockNotificationKind.NewsCount,
            title = "上午财经新闻抓取进度",
            windowLabel = "08:00-09:20",
            hour = 9,
            minute = 20,
            windowStartHour = 8,
            windowStartMinute = 0,
            windowEndHour = 9,
            windowEndMinute = 20,
        ),
        AStockRecommendationSlot(
            id = "morning_0925",
            period = "morning",
            kind = AStockNotificationKind.Recommendation,
            title = "09:25 上午热门股票推荐",
            windowLabel = "08:00-09:25",
            hour = 9,
            minute = 25,
            windowStartHour = 8,
            windowStartMinute = 0,
            windowEndHour = 9,
            windowEndMinute = 25,
        ),
        AStockRecommendationSlot(
            id = "afternoon_1250",
            period = "afternoon",
            kind = AStockNotificationKind.NewsCount,
            title = "下午财经新闻抓取进度",
            windowLabel = "09:30-12:50",
            hour = 12,
            minute = 50,
            windowStartHour = 9,
            windowStartMinute = 30,
            windowEndHour = 12,
            windowEndMinute = 50,
        ),
        AStockRecommendationSlot(
            id = "afternoon_1255",
            period = "afternoon",
            kind = AStockNotificationKind.Recommendation,
            title = "12:55 下午热门股票推荐",
            windowLabel = "09:30-12:55",
            hour = 12,
            minute = 55,
            windowStartHour = 9,
            windowStartMinute = 30,
            windowEndHour = 12,
            windowEndMinute = 55,
        ),
    )

    fun ensureNotificationChannels(context: Context) {
        val manager = context.getSystemService(NotificationManager::class.java)
        val recommendationChannel = NotificationChannel(
            recommendationChannelId,
            "A股热门股票推荐",
            NotificationManager.IMPORTANCE_DEFAULT,
        ).apply {
            description = "每日 A股 上午和下午热门股票推荐提醒"
        }
        val newsCountChannel = NotificationChannel(
            newsCountChannelId,
            "A股财经新闻抓取进度",
            NotificationManager.IMPORTANCE_HIGH,
        ).apply {
            description = "每日 09:20 和 12:50 的财经新闻抓取数量提醒"
        }
        manager.createNotificationChannel(recommendationChannel)
        manager.createNotificationChannel(newsCountChannel)
    }

    fun scheduleDailyRecommendations(context: Context) {
        ensureNotificationChannels(context)
        cancelObsoleteSlots(context)
        slots.forEach { slot -> scheduleNext(context, slot) }
    }

    fun scheduleNext(context: Context, slot: AStockRecommendationSlot) {
        val triggerAtMillis = nextTriggerMillis(slot)
        val alarmManager = context.getSystemService(AlarmManager::class.java)
        val pendingIntent = pendingIntent(context, slot)
        scheduleAlarm(alarmManager, triggerAtMillis, pendingIntent)
    }

    @SuppressLint("ScheduleExactAlarm")
    private fun scheduleAlarm(alarmManager: AlarmManager, triggerAtMillis: Long, pendingIntent: PendingIntent) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S && !alarmManager.canScheduleExactAlarms()) {
            alarmManager.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, triggerAtMillis, pendingIntent)
            return
        }
        alarmManager.setExactAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, triggerAtMillis, pendingIntent)
    }

    private fun pendingIntent(context: Context, slot: AStockRecommendationSlot): PendingIntent {
        val intent = Intent(context, AStockRecommendationReceiver::class.java).apply {
            action = alarmAction
            putExtra(extraSlotId, slot.id)
        }
        return PendingIntent.getBroadcast(
            context,
            slot.id.hashCode(),
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
    }

    private fun nextTriggerMillis(slot: AStockRecommendationSlot): Long {
        val now = LocalDateTime.now(zone)
        var targetDate = AStockTradingCalendar.nextOrSameTradingDay(LocalDate.now(zone))
        var target = LocalDateTime.of(targetDate, LocalTime.of(slot.hour, slot.minute))
        if (!target.isAfter(now)) {
            targetDate = AStockTradingCalendar.nextTradingDay(targetDate)
            target = LocalDateTime.of(targetDate, LocalTime.of(slot.hour, slot.minute))
        }
        return target.atZone(zone).toInstant().toEpochMilli()
    }

    fun slotById(slotID: String?): AStockRecommendationSlot? {
        if (slotID.isNullOrBlank()) {
            return null
        }
        return slots.firstOrNull { it.id == slotID }
    }

    private fun cancelObsoleteSlots(context: Context) {
        val alarmManager = context.getSystemService(AlarmManager::class.java)
        obsoleteSlotIds.forEach { slotID ->
            val intent = Intent(context, AStockRecommendationReceiver::class.java).apply {
                action = alarmAction
                putExtra(extraSlotId, slotID)
            }
            val pendingIntent = PendingIntent.getBroadcast(
                context,
                slotID.hashCode(),
                intent,
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
            )
            alarmManager.cancel(pendingIntent)
            pendingIntent.cancel()
        }
    }
}

enum class AStockNotificationKind {
    NewsCount,
    Recommendation,
}

data class AStockRecommendationSlot(
    val id: String,
    val period: String,
    val kind: AStockNotificationKind,
    val title: String,
    val windowLabel: String,
    val hour: Int,
    val minute: Int,
    val windowStartHour: Int,
    val windowStartMinute: Int,
    val windowEndHour: Int,
    val windowEndMinute: Int,
)
