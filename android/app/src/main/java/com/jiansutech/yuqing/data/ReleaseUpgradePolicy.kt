package com.jiansutech.yuqing.data

import okhttp3.HttpUrl.Companion.toHttpUrl

data class ReleaseSource(
    val baseUrl: String,
    val label: String,
)

class ReleaseSourceSelector(
    private val networkEnvironmentSelector: NetworkEnvironmentSelector,
) {
    suspend fun select(
        checkedAtMillis: Long = System.currentTimeMillis(),
    ): ReleaseSource {
        val endpoints = networkEnvironmentSelector.detect(checkedAtMillis)
        return ReleaseSource(endpoints.releaseBaseUrl, endpoints.label)
    }
}

object ReleaseUpgradePolicy {
    fun isCurrentVersion(
        latest: ReleasePackage,
        currentVersionName: String,
        currentVersionCode: Int,
    ): Boolean {
        val latestVersionName = latest.versionName.trim()
        if (latestVersionName.isNotBlank()) {
            return latestVersionName == currentVersionName.trim()
        }
        return latest.versionCode > 0 && latest.versionCode <= currentVersionCode
    }

    fun downloadUrlFor(baseUrl: String, fileName: String): String {
        val normalizedBaseUrl = ApiFactory.normalizeReleaseBaseUrl(baseUrl)
        return normalizedBaseUrl.toHttpUrl()
            .newBuilder()
            .encodedPath("/")
            .addPathSegment("release")
            .addPathSegment(fileName)
            .build()
            .toString()
    }
}
