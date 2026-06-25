package com.jiansutech.yuqing.data

import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ReleaseUpgradePolicyTest {
    @Test
    fun selectorUsesIntranetWhenReachable() = runTest {
        val selector = ReleaseSourceSelector { baseUrl -> baseUrl.contains("10.15.0.7") }

        val source = selector.select(
            intranetBaseUrl = "http://10.15.0.7:8099/",
            externalBaseUrl = "http://yuqin.jiansutech.com:8099/",
        )

        assertEquals("内网", source.label)
        assertEquals("http://10.15.0.7:8099/", source.baseUrl)
    }

    @Test
    fun selectorFallsBackToExternalWhenIntranetUnavailable() = runTest {
        val selector = ReleaseSourceSelector { false }

        val source = selector.select(
            intranetBaseUrl = "http://10.15.0.7:8099/",
            externalBaseUrl = "http://yuqin.jiansutech.com:8099/",
        )

        assertEquals("外网", source.label)
        assertEquals("http://yuqin.jiansutech.com:8099/", source.baseUrl)
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
