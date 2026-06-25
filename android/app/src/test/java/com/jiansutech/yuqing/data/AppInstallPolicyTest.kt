package com.jiansutech.yuqing.data

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.LocalDateTime
import java.time.ZoneId

class AppInstallPolicyTest {
    private val zone = ZoneId.of("Asia/Shanghai")

    @Test
    fun firstRunCreatesCurrentVersionInstallRecord() {
        val now = millis("2026-06-25T10:00:00")

        val record = resolveCurrentInstallRecord(
            stored = AppInstallRecord(versionCode = 0, versionName = "", installedAtMillis = 0),
            currentVersionCode = 12,
            currentVersionName = "20260625-120000-abcdef12",
            nowMillis = now,
        )

        assertEquals(12, record.versionCode)
        assertEquals("20260625-120000-abcdef12", record.versionName)
        assertEquals(now, record.installedAtMillis)
    }

    @Test
    fun sameVersionKeepsOriginalInstallTime() {
        val installedAt = millis("2026-01-01T09:00:00")
        val now = millis("2026-02-01T09:00:00")

        val record = resolveCurrentInstallRecord(
            stored = AppInstallRecord(12, "same", installedAt),
            currentVersionCode = 12,
            currentVersionName = "same",
            nowMillis = now,
        )

        assertEquals(installedAt, record.installedAtMillis)
    }

    @Test
    fun versionChangeResetsInstallTime() {
        val now = millis("2026-06-25T10:00:00")

        val record = resolveCurrentInstallRecord(
            stored = AppInstallRecord(11, "old", millis("2026-01-01T09:00:00")),
            currentVersionCode = 12,
            currentVersionName = "new",
            nowMillis = now,
        )

        assertEquals(12, record.versionCode)
        assertEquals("new", record.versionName)
        assertEquals(now, record.installedAtMillis)
    }

    @Test
    fun installDatePlusSixMonthsIsExpiredOnThatMoment() {
        val installedAt = millis("2026-01-01T09:00:00")

        assertFalse(isInstallRecordExpired(installedAt, millis("2026-07-01T08:59:59"), zone))
        assertTrue(isInstallRecordExpired(installedAt, millis("2026-07-01T09:00:00"), zone))
    }

    private fun millis(value: String): Long {
        return LocalDateTime.parse(value).atZone(zone).toInstant().toEpochMilli()
    }
}
