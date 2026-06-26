package com.jiansutech.yuqing.data

import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ReleaseUpgradePolicyTest {
    @Test
    fun selectorUsesIntranetWhenReachable() = runTest {
        val selector = ReleaseSourceSelector(
            NetworkEnvironmentSelector { baseUrl -> baseUrl.contains("10.15.0.7") },
        )

        val source = selector.select()

        assertEquals("内网", source.label)
        assertEquals("http://10.15.0.7:8099/", source.baseUrl)
    }

    @Test
    fun selectorFallsBackToExternalWhenIntranetUnavailable() = runTest {
        val selector = ReleaseSourceSelector(
            NetworkEnvironmentSelector { false },
        )

        val source = selector.select()

        assertEquals("外网", source.label)
        assertEquals("http://yuqin.jiansutech.com:8099/", source.baseUrl)
    }

    @Test
    fun networkEndpointsUseIntranetHostWhenHeartbeatSucceeds() = runTest {
        val endpoints = NetworkEnvironmentSelector { true }.detect(checkedAtMillis = 123)

        assertEquals(NetworkEnvironment.Intranet, endpoints.environment)
        assertEquals("http://10.15.0.7:8079/", endpoints.webBaseUrl)
        assertEquals("http://10.15.0.7:8082/", endpoints.contentBaseUrl)
        assertEquals("http://10.15.0.7:8081/", endpoints.authBaseUrl)
        assertEquals("http://10.15.0.7:8099/", endpoints.releaseBaseUrl)
        assertEquals(123, endpoints.checkedAtMillis)
        assertTrue(endpoints.heartbeatOk)
    }

    @Test
    fun networkEndpointsUseExternalDomainWhenHeartbeatFails() = runTest {
        val endpoints = NetworkEnvironmentSelector { false }.detect()

        assertEquals(NetworkEnvironment.External, endpoints.environment)
        assertEquals("http://yuqin.jiansutech.com:8079/", endpoints.webBaseUrl)
        assertEquals("http://yuqin.jiansutech.com:8082/", endpoints.contentBaseUrl)
        assertEquals("http://yuqin.jiansutech.com:8081/", endpoints.authBaseUrl)
        assertEquals("http://yuqin.jiansutech.com:8099/", endpoints.releaseBaseUrl)
        assertFalse(endpoints.heartbeatOk)
    }

    @Test
    fun heartbeatIntervalIsTwoMinutes() {
        assertEquals(120_000L, NETWORK_HEARTBEAT_INTERVAL_MILLIS)
    }

    @Test
    fun sameVersionNameMeansCurrentVersion() {
        val latest = ReleasePackage(versionName = "20260625-120000-abcdef12", versionCode = 200)

        assertTrue(
            ReleaseUpgradePolicy.isCurrentVersion(
                latest = latest,
                currentVersionName = "20260625-120000-abcdef12",
                currentVersionCode = 100,
            ),
        )
    }

    @Test
    fun differentVersionNameMeansUpdateAvailable() {
        val latest = ReleasePackage(versionName = "20260625-120000-abcdef12", versionCode = 0)

        assertFalse(
            ReleaseUpgradePolicy.isCurrentVersion(
                latest = latest,
                currentVersionName = "20260624-120000-aaaaaaaa",
                currentVersionCode = 100,
            ),
        )
    }

    @Test
    fun intranetDownloadUrlUsesSelectedBaseAndFileName() {
        val url = ReleaseUpgradePolicy.downloadUrlFor(
            baseUrl = "http://10.15.0.7:8099/",
            fileName = "yuqing-20260625-120000-abcdef12-release.apk",
        )

        assertEquals(
            "http://10.15.0.7:8099/release/yuqing-20260625-120000-abcdef12-release.apk",
            url,
        )
    }
}
