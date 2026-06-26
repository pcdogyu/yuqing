package com.jiansutech.yuqing.data

import com.jiansutech.yuqing.BuildConfig

const val INTRANET_WEB_BASE_URL = "http://10.15.0.7:8079/"
const val INTRANET_AUTH_BASE_URL = "http://10.15.0.7:8081/"
const val INTRANET_CONTENT_BASE_URL = "http://10.15.0.7:8082/"
const val INTRANET_RELEASE_BASE_URL = "http://10.15.0.7:8099/"
const val NETWORK_HEARTBEAT_INTERVAL_MILLIS = 120_000L

enum class NetworkEnvironment(val label: String) {
    Intranet("内网"),
    External("外网"),
}

data class NetworkEndpoints(
    val environment: NetworkEnvironment,
    val webBaseUrl: String,
    val contentBaseUrl: String,
    val authBaseUrl: String,
    val releaseBaseUrl: String,
    val checkedAtMillis: Long = 0L,
    val heartbeatOk: Boolean = environment == NetworkEnvironment.Intranet,
) {
    val label: String get() = environment.label
}

class NetworkEnvironmentSelector(
    private val intranetContentReachable: suspend (String) -> Boolean,
) {
    suspend fun detect(checkedAtMillis: Long = System.currentTimeMillis()): NetworkEndpoints {
        val intranetContentBaseUrl = ApiFactory.normalizeApiBaseUrl(INTRANET_CONTENT_BASE_URL)
        return NetworkEndpointPolicy.endpointsFor(
            environment = if (intranetContentReachable(intranetContentBaseUrl)) {
                NetworkEnvironment.Intranet
            } else {
                NetworkEnvironment.External
            },
            checkedAtMillis = checkedAtMillis,
        )
    }
}

object NetworkEndpointPolicy {
    fun endpointsFor(
        environment: NetworkEnvironment,
        checkedAtMillis: Long = 0L,
    ): NetworkEndpoints {
        return when (environment) {
            NetworkEnvironment.Intranet -> NetworkEndpoints(
                environment = NetworkEnvironment.Intranet,
                webBaseUrl = ApiFactory.normalizeBaseUrl(INTRANET_WEB_BASE_URL, INTRANET_WEB_BASE_URL),
                contentBaseUrl = ApiFactory.normalizeApiBaseUrl(INTRANET_CONTENT_BASE_URL, INTRANET_CONTENT_BASE_URL),
                authBaseUrl = ApiFactory.normalizeAuthBaseUrl(INTRANET_AUTH_BASE_URL, INTRANET_AUTH_BASE_URL),
                releaseBaseUrl = ApiFactory.normalizeReleaseBaseUrl(INTRANET_RELEASE_BASE_URL, INTRANET_RELEASE_BASE_URL),
                checkedAtMillis = checkedAtMillis,
                heartbeatOk = true,
            )
            NetworkEnvironment.External -> NetworkEndpoints(
                environment = NetworkEnvironment.External,
                webBaseUrl = ApiFactory.normalizeBaseUrl(BuildConfig.DEFAULT_WEB_BASE_URL, BuildConfig.DEFAULT_WEB_BASE_URL),
                contentBaseUrl = ApiFactory.normalizeApiBaseUrl(BuildConfig.DEFAULT_API_BASE_URL),
                authBaseUrl = ApiFactory.normalizeAuthBaseUrl(BuildConfig.DEFAULT_AUTH_BASE_URL),
                releaseBaseUrl = ApiFactory.normalizeReleaseBaseUrl(BuildConfig.DEFAULT_RELEASE_BASE_URL),
                checkedAtMillis = checkedAtMillis,
                heartbeatOk = false,
            )
        }
    }
}
