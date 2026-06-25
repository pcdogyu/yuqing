package com.jiansutech.yuqing.data

import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.core.content.FileProvider
import com.jiansutech.yuqing.BuildConfig
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.File
import java.util.concurrent.TimeUnit

class ReleaseUpdater(private val context: Context) {
    private val client = OkHttpClient.Builder()
        .connectTimeout(20, TimeUnit.SECONDS)
        .readTimeout(60, TimeUnit.SECONDS)
        .build()
    private val sourceSelector = ReleaseSourceSelector { baseUrl ->
        ApiFactory.testUrl(baseUrl, "release/").ok
    }

    suspend fun selectReleaseSource(
        intranetBaseUrl: String = INTRANET_RELEASE_BASE_URL,
        externalBaseUrl: String = BuildConfig.DEFAULT_RELEASE_BASE_URL,
    ): ReleaseSource {
        return sourceSelector.select(intranetBaseUrl, externalBaseUrl)
    }

    suspend fun fetchLatest(baseUrl: String = BuildConfig.DEFAULT_RELEASE_BASE_URL): ReleasePackage {
        return ApiFactory.release(baseUrl).latest().data ?: error("未找到可用安装包")
    }

    suspend fun downloadLatestApk(baseUrl: String = BuildConfig.DEFAULT_RELEASE_BASE_URL): File = withContext(Dispatchers.IO) {
        val latest = fetchLatest(baseUrl)
        downloadApk(latest, baseUrl, preferBaseDownloadUrl = false)
    }

    suspend fun downloadApk(
        latest: ReleasePackage,
        baseUrl: String,
        preferBaseDownloadUrl: Boolean,
    ): File = withContext(Dispatchers.IO) {
        val fileName = latest.fileName.ifBlank { "yuqing-latest-release.apk" }
        val downloadUrl = if (preferBaseDownloadUrl && latest.fileName.isNotBlank()) {
            ReleaseUpgradePolicy.downloadUrlFor(baseUrl, latest.fileName)
        } else {
            latest.downloadUrl.ifBlank {
                if (latest.fileName.isBlank()) {
                    error("安装包下载地址为空")
                }
                ReleaseUpgradePolicy.downloadUrlFor(baseUrl, latest.fileName)
            }
        }
        val target = File(context.cacheDir, "updates/$fileName")
        target.parentFile?.mkdirs()
        client.newCall(Request.Builder().url(downloadUrl).get().build()).execute().use { response ->
            if (!response.isSuccessful) {
                error("下载安装包失败 HTTP ${response.code}")
            }
            val body = response.body ?: error("安装包响应为空")
            target.outputStream().use { output ->
                body.byteStream().use { input ->
                    input.copyTo(output)
                }
            }
        }
        target
    }

    fun installApk(file: File): Intent {
        val uri = FileProvider.getUriForFile(
            context,
            "${BuildConfig.APPLICATION_ID}.fileprovider",
            file,
        )
        return Intent(Intent.ACTION_VIEW).apply {
            setDataAndType(uri, "application/vnd.android.package-archive")
            addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        }
    }

    fun unknownSourcesSettingsIntent(): Intent {
        return Intent(
            android.provider.Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,
            Uri.parse("package:${context.packageName}"),
        )
    }
}
