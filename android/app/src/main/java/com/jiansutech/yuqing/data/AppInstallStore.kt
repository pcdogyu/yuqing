package com.jiansutech.yuqing.data

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.longPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import java.time.Instant
import java.time.ZoneId

private val Context.installDataStore by preferencesDataStore("yuqing_install")

data class AppInstallRecord(
    val versionCode: Int,
    val versionName: String,
    val installedAtMillis: Long,
)

class AppInstallStore(private val context: Context) {
    suspend fun ensureCurrentVersionInstallRecord(
        currentVersionCode: Int,
        currentVersionName: String,
        nowMillis: Long = System.currentTimeMillis(),
    ): AppInstallRecord {
        var nextRecord: AppInstallRecord? = null
        context.installDataStore.edit { preferences ->
            val stored = AppInstallRecord(
                versionCode = preferences[installedVersionCodeKey] ?: 0,
                versionName = preferences[installedVersionNameKey].orEmpty(),
                installedAtMillis = preferences[installedAtMillisKey] ?: 0L,
            )
            nextRecord = resolveCurrentInstallRecord(
                stored = stored,
                currentVersionCode = currentVersionCode,
                currentVersionName = currentVersionName,
                nowMillis = nowMillis,
            )
            val record = nextRecord ?: return@edit
            preferences[installedVersionCodeKey] = record.versionCode
            preferences[installedVersionNameKey] = record.versionName
            preferences[installedAtMillisKey] = record.installedAtMillis
        }
        return nextRecord ?: AppInstallRecord(currentVersionCode, currentVersionName, nowMillis)
    }

    private companion object {
        val installedVersionCodeKey = intPreferencesKey("installed_version_code")
        val installedVersionNameKey = stringPreferencesKey("installed_version_name")
        val installedAtMillisKey = longPreferencesKey("installed_at_millis")
    }
}

fun resolveCurrentInstallRecord(
    stored: AppInstallRecord,
    currentVersionCode: Int,
    currentVersionName: String,
    nowMillis: Long,
): AppInstallRecord {
    val versionChanged = stored.versionCode != currentVersionCode || stored.versionName != currentVersionName
    return if (versionChanged || stored.installedAtMillis <= 0L) {
        AppInstallRecord(currentVersionCode, currentVersionName, nowMillis)
    } else {
        stored
    }
}

fun isInstallRecordExpired(
    installedAtMillis: Long,
    nowMillis: Long,
    zoneId: ZoneId = ZoneId.of("Asia/Shanghai"),
): Boolean {
    if (installedAtMillis <= 0L) {
        return false
    }
    val expiresAt = Instant.ofEpochMilli(installedAtMillis)
        .atZone(zoneId)
        .plusMonths(6)
        .toInstant()
        .toEpochMilli()
    return nowMillis >= expiresAt
}
