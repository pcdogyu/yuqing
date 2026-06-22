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
import com.jiansutech.yuqing.data.AStockRecommendation
import com.jiansutech.yuqing.data.ApiFactory
import com.jiansutech.yuqing.data.SessionStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.serialization.decodeFromString
import java.time.LocalDate
import java.time.ZoneId

class AStockRecommendationReceiver : BroadcastReceiver() {
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
                    showRecommendationNotification(appContext, slot, content)
                }
            } finally {
                pendingResult.finish()
            }
        }
    }

    private fun slotFromIntent(intent: Intent): AStockRecommendationSlot? {
        val id = intent.getStringExtra(AStockRecommendationScheduler.extraSlotId).orEmpty()
        val period = intent.getStringExtra(AStockRecommendationScheduler.extraPeriod).orEmpty()
        val title = intent.getStringExtra(AStockRecommendationScheduler.extraTitle).orEmpty()
        val hour = intent.getIntExtra(AStockRecommendationScheduler.extraHour, -1)
        val minute = intent.getIntExtra(AStockRecommendationScheduler.extraMinute, -1)
        if (id.isBlank() || period.isBlank() || title.isBlank() || hour < 0 || minute < 0) {
            return null
        }
        val windowLabel = if (period == "afternoon") "09:30-13:00" else "08:00-09:30"
        return AStockRecommendationSlot(id, period, title, windowLabel, hour, minute)
    }

    private suspend fun loadNotificationContent(context: Context, slot: AStockRecommendationSlot): String {
        val session = SessionStore(context).state.first()
        val date = LocalDate.now(ZoneId.of("Asia/Shanghai")).toString()
        return runCatching {
            val snapshot = ApiFactory.yuqing(session.apiBaseUrl, session.token)
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

    private fun showRecommendationNotification(context: Context, slot: AStockRecommendationSlot, content: String) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
            context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            return
        }
        AStockRecommendationScheduler.ensureNotificationChannel(context)
        val openAppIntent = PendingIntent.getActivity(
            context,
            0,
            Intent(context, MainActivity::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        val notification = Notification.Builder(context, AStockRecommendationScheduler.notificationChannelId)
            .setSmallIcon(android.R.drawable.ic_dialog_info)
            .setContentTitle(slot.title)
            .setContentText(content)
            .setStyle(Notification.BigTextStyle().bigText(content))
            .setContentIntent(openAppIntent)
            .setAutoCancel(true)
            .build()
        context.getSystemService(NotificationManager::class.java)
            .notify(notificationId(slot), notification)
    }

    private fun notificationId(slot: AStockRecommendationSlot): Int {
        val date = LocalDate.now(ZoneId.of("Asia/Shanghai")).toString()
        return "$date-${slot.id}".hashCode()
    }

    private fun AStockRecommendation.displayName(): String? {
        val label = listOf(code, name).filter { it.isNotBlank() }.joinToString(" ")
        return label.ifBlank { null }
    }
}
