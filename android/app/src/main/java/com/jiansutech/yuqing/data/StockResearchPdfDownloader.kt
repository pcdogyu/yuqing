package com.jiansutech.yuqing.data

import android.content.Context
import android.net.Uri
import android.os.Environment
import androidx.core.content.FileProvider
import com.jiansutech.yuqing.BuildConfig
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.File
import java.util.concurrent.TimeUnit

data class StockResearchPdfFile(
    val file: File,
    val uri: Uri,
)

class StockResearchPdfDownloader(
    private val context: Context,
    private val client: OkHttpClient = OkHttpClient.Builder()
        .connectTimeout(20, TimeUnit.SECONDS)
        .readTimeout(60, TimeUnit.SECONDS)
        .build(),
) {
    suspend fun existingPdf(item: StockResearch): StockResearchPdfFile? = withContext(Dispatchers.IO) {
        localPdfFile(item)
            .takeIf { it.isFile && it.length() > 0L }
            ?.let(::asPdfFile)
    }

    suspend fun downloadPdf(
        item: StockResearch,
        contentBaseUrl: String,
        token: String,
        force: Boolean = false,
    ): StockResearchPdfFile = withContext(Dispatchers.IO) {
        if (!force) {
            existingPdf(item)?.let { return@withContext it }
        }

        val target = localPdfFile(item)
        target.parentFile?.mkdirs()
        val temp = File(target.parentFile, "${target.name}.download")
        if (temp.exists()) {
            temp.delete()
        }

        val errors = mutableListOf<String>()
        if (item.id > 0) {
            runCatching {
                val requestBuilder = Request.Builder()
                    .url(stockResearchBackendPdfUrl(contentBaseUrl, item.id))
                    .get()
                if (token.isNotBlank()) {
                    requestBuilder.header("Authorization", "Bearer $token")
                }
                downloadToFile(requestBuilder.build(), temp)
            }.onSuccess {
                replaceDownloadedFile(temp, target)
                return@withContext asPdfFile(target)
            }.onFailure { throwable ->
                errors += throwable.message ?: "后端PDF下载失败"
            }
        }

        val externalPdfUrl = item.pdfUrl.trim()
        if (externalPdfUrl.isNotBlank()) {
            runCatching {
                downloadToFile(Request.Builder().url(externalPdfUrl).get().build(), temp)
            }.onSuccess {
                replaceDownloadedFile(temp, target)
                return@withContext asPdfFile(target)
            }.onFailure { throwable ->
                errors += throwable.message ?: "外部PDF下载失败"
            }
        }

        if (temp.exists()) {
            temp.delete()
        }
        error(errors.filter { it.isNotBlank() }.joinToString("；").ifBlank { "没有可下载的PDF" })
    }

    private fun downloadToFile(request: Request, target: File) {
        client.newCall(request).execute().use { response ->
            if (!response.isSuccessful) {
                error("PDF下载失败 HTTP ${response.code}")
            }
            val body = response.body ?: error("PDF响应为空")
            target.outputStream().use { output ->
                body.byteStream().use { input ->
                    input.copyTo(output)
                }
            }
        }
        if (!target.isFile || target.length() <= 0L) {
            error("PDF文件为空")
        }
    }

    private fun replaceDownloadedFile(temp: File, target: File) {
        if (target.exists() && !target.delete()) {
            error("无法覆盖本地PDF")
        }
        if (!temp.renameTo(target)) {
            temp.copyTo(target, overwrite = true)
            temp.delete()
        }
    }

    private fun localPdfFile(item: StockResearch): File {
        val root = context.getExternalFilesDir(Environment.DIRECTORY_DOCUMENTS)
            ?: context.filesDir
        return File(File(root, PDF_DIRECTORY), stockResearchPdfFileName(item))
    }

    private fun asPdfFile(file: File): StockResearchPdfFile {
        val uri = FileProvider.getUriForFile(
            context,
            "${BuildConfig.APPLICATION_ID}.fileprovider",
            file,
        )
        return StockResearchPdfFile(file = file, uri = uri)
    }

    private fun stockResearchBackendPdfUrl(contentBaseUrl: String, id: Long) =
        ApiFactory.normalizeApiBaseUrl(contentBaseUrl)
            .toHttpUrl()
            .newBuilder()
            .encodedPath("/")
            .addPathSegments("api/v1/stock-research")
            .addPathSegment(id.toString())
            .addPathSegment("pdf")
            .build()

    companion object {
        private const val PDF_DIRECTORY = "stock-research"
    }
}

internal fun stockResearchPdfFileName(item: StockResearch): String {
    val id = item.id.takeIf { it > 0 }?.toString() ?: "unknown"
    val label = listOf(item.code, item.name)
        .joinToString("-")
        .trim('-')
        .ifBlank { item.title }
        .ifBlank { "stock-research" }
    val safeLabel = label
        .replace(Regex("[\\\\/:*?\"<>|\\p{Cntrl}]+"), "-")
        .replace(Regex("\\s+"), "-")
        .trim('-', '.', ' ')
        .take(80)
        .ifBlank { "stock-research" }
    return "$id-$safeLabel.pdf"
}
