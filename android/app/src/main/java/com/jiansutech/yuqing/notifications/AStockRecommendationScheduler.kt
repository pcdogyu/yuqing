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
    const val notificationChannelId = "a_stock_recommendations"
    const val extraSlotId = "slot_id"
    const val extraPeriod = "period"
    const val extraTitle = "title"
    const val extraHour = "hour"
    const val extraMinute = "minute"

    private val zone: ZoneId = ZoneId.of("Asia/Shanghai")

    val slots: List<AStockRecommendationSlot> = listOf(
        AStockRecommendationSlot("morning_0925", "morning", "上午热门股票推荐", "08:00-09:30", 9, 25),
        AStockRecommendationSlot("morning_0930", "morning", "上午热门股票推荐", "08:00-09:30", 9, 30),
        AStockRecommendationSlot("afternoon_1255", "afternoon", "下午热门股票推荐", "09:30-13:00", 12, 55),
        AStockRecommendationSlot("afternoon_1300", "afternoon", "下午热门股票推荐", "09:30-13:00", 13, 0),
    )

    fun ensureNotificationChannel(context: Context) {
        val manager = context.getSystemService(NotificationManager::class.java)
        val channel = NotificationChannel(
            notificationChannelId,
            "A股热门股票推荐",
            NotificationManager.IMPORTANCE_DEFAULT,
        ).apply {
            description = "每日 A股 上午和下午热门股票推荐提醒"
        }
        manager.createNotificationChannel(channel)
    }

    fun scheduleDailyRecommendations(context: Context) {
        ensureNotificationChannel(context)
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
            putExtra(extraPeriod, slot.period)
            putExtra(extraTitle, slot.title)
            putExtra(extraHour, slot.hour)
            putExtra(extraMinute, slot.minute)
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
}

data class AStockRecommendationSlot(
    val id: String,
    val period: String,
    val title: String,
    val windowLabel: String,
    val hour: Int,
    val minute: Int,
)
