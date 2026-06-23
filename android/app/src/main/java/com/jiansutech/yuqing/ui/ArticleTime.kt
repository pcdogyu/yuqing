package com.jiansutech.yuqing.ui

import java.time.Duration
import java.time.Instant
import java.time.LocalDateTime
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter

private val articleTimeFormats = listOf(
    DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm:ss"),
    DateTimeFormatter.ofPattern("yyyy-MM-dd HH:mm"),
    DateTimeFormatter.ofPattern("yyyy/MM/dd HH:mm:ss"),
    DateTimeFormatter.ofPattern("yyyy/MM/dd HH:mm"),
)

internal fun formatArticleRelativeTime(
    raw: String,
    now: LocalDateTime = LocalDateTime.now(ZoneId.of("Asia/Shanghai")),
): String {
    val value = raw.trim()
    if (value.isBlank()) {
        return ""
    }
    relativeMinutes(value)?.let { minutes ->
        return minutesToRelativeLabel(minutes)
    }
    if (value == "刚刚") {
        return "刚刚"
    }
    parseArticleTime(value)?.let { publishTime ->
        val minutes = Duration.between(publishTime, now).toMinutes()
        return minutesToRelativeLabel(minutes)
    }
    return value
}

internal fun formatArticleCapturedTime(raw: String): String {
    val value = raw.trim()
    if (value.isBlank()) {
        return ""
    }
    val withoutZone = value
        .replace('T', ' ')
        .replace(Regex("""(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$"""), "")
        .trim()
    return when {
        withoutZone.length >= 16 -> withoutZone.substring(0, 16)
        else -> withoutZone
    }
}

private fun relativeMinutes(value: String): Long? {
    val minuteMatch = Regex("""^(\d+)\s*分钟前$""").matchEntire(value)
    if (minuteMatch != null) {
        return minuteMatch.groupValues[1].toLongOrNull()
    }
    val hourMatch = Regex("""^(\d+)\s*小时前$""").matchEntire(value)
    if (hourMatch != null) {
        return hourMatch.groupValues[1].toLongOrNull()?.times(60)
    }
    return null
}

private fun parseArticleTime(value: String): LocalDateTime? {
    runCatching {
        OffsetDateTime.parse(value).atZoneSameInstant(ZoneId.of("Asia/Shanghai")).toLocalDateTime()
    }.getOrNull()?.let { return it }
    runCatching {
        Instant.parse(value).atZone(ZoneId.of("Asia/Shanghai")).toLocalDateTime()
    }.getOrNull()?.let { return it }
    for (formatter in articleTimeFormats) {
        val parsed = runCatching { LocalDateTime.parse(value, formatter) }.getOrNull()
        if (parsed != null) {
            return parsed
        }
    }
    return null
}

private fun minutesToRelativeLabel(minutes: Long): String {
    if (minutes <= 0) {
        return "刚刚"
    }
    if (minutes < 60) {
        return "${minutes}分钟之前"
    }
    return "${minutes / 60}小时之前"
}
