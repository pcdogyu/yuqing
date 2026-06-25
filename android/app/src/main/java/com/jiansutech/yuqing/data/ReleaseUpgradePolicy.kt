package com.jiansutech.yuqing.data

import com.jiansutech.yuqing.BuildConfig
import okhttp3.HttpUrl.Companion.toHttpUrl

const val INTRANET_RELEASE_BASE_URL = "http://10.15.0.7:8099/"

data class ReleaseSource(
    val baseUrl: String,
    val label: String,
)

class ReleaseSourceSelector(
    private val releaseDirectoryReachable: suspend (String) -> Boolean,
) {
    suspend fun select(
        intranetBaseUrl: String = INTRANET_RELEASE_BASE_URL,
        externalBaseUrl: String = BuildConfig.DEFAULT_RELEASE_BASE_URL,
    ): ReleaseSource {
        val normalizedIntranet = ApiFactory.normalizeReleaseBaseUrl(intranetBaseUrl)
        return if (releaseDirectoryReachable(normalizedIntranet)) {
            ReleaseSource(normalizedIntranet, "内网")
        } else {
            ReleaseSource(ApiFactory.normalizeReleaseBaseUrl(externalBaseUrl), "外网")
        }
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
