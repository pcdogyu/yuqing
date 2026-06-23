package com.jiansutech.yuqing.data

import com.jiansutech.yuqing.BuildConfig
import com.jakewharton.retrofit2.converter.kotlinx.serialization.asConverterFactory
import kotlinx.serialization.json.Json
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.logging.HttpLoggingInterceptor
import retrofit2.Retrofit
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.Query
import java.util.concurrent.TimeUnit

interface AuthApi {
    @POST("api/v1/auth/login")
    suspend fun login(@Body request: LoginRequest): ApiEnvelope<LoginData>
}

interface YuqingApi {
    @GET("api/v1/android/bootstrap")
    suspend fun bootstrap(): ApiEnvelope<AndroidBootstrap>

    @GET("api/v1/android/dashboard")
    suspend fun dashboard(): ApiEnvelope<AndroidDashboard>

    @GET("api/v1/articles")
    suspend fun articles(
        @Query("page") page: Int = 1,
        @Query("page_size") pageSize: Int = 10,
        @Query("sort") sort: String = "publish_time_desc",
    ): ApiEnvelope<ItemListResult>

    @GET("api/v1/a-stock/auction")
    suspend fun aStockAuction(
        @Query("date") date: String = "",
        @Query("page") page: Int = 1,
        @Query("page_size") pageSize: Int = 20,
    ): ApiEnvelope<AStockAuctionListResult>

    @GET("api/v1/a-stock/recommendations")
    suspend fun aStockRecommendations(
        @Query("date") date: String,
        @Query("period") period: String,
        @Query("ignore_recent") ignoreRecent: Boolean = false,
    ): ApiEnvelope<AStockRecommendationSnapshot>

    @GET("api/v1/search/full")
    suspend fun searchFull(
        @Query("keyword") keyword: String,
        @Query("page") page: Int = 1,
        @Query("page_size") pageSize: Int = 20,
    ): ApiEnvelope<SearchResult>

    @POST("api/v1/android/actions/{action}")
    suspend fun runAction(
        @Path("action") action: String,
        @Body request: AndroidActionRequest,
    ): ApiEnvelope<AndroidActionResponse>
}

object ApiFactory {
    val json: Json = Json {
        ignoreUnknownKeys = true
        explicitNulls = false
        coerceInputValues = true
    }

    fun auth(baseUrl: String = BuildConfig.DEFAULT_AUTH_BASE_URL): AuthApi {
        return retrofit(normalizeAuthBaseUrl(baseUrl), null).create(AuthApi::class.java)
    }

    fun yuqing(baseUrl: String = BuildConfig.DEFAULT_API_BASE_URL, token: String? = null): YuqingApi {
        return retrofit(normalizeApiBaseUrl(baseUrl), token).create(YuqingApi::class.java)
    }

    private fun retrofit(baseUrl: String, token: String?): Retrofit {
        val logging = HttpLoggingInterceptor().apply {
            level = if (BuildConfig.DEBUG) HttpLoggingInterceptor.Level.BASIC else HttpLoggingInterceptor.Level.NONE
        }
        val client = OkHttpClient.Builder()
            .connectTimeout(20, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .addInterceptor { chain ->
                val builder = chain.request().newBuilder()
                if (!token.isNullOrBlank()) {
                    builder.header("Authorization", "Bearer $token")
                }
                chain.proceed(builder.build())
            }
            .addInterceptor(logging)
            .build()
        return Retrofit.Builder()
            .baseUrl(baseUrl)
            .client(client)
            .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
            .build()
    }

    fun normalizeAuthBaseUrl(value: String, fallback: String = BuildConfig.DEFAULT_AUTH_BASE_URL): String {
        return normalizeBaseUrl(value, fallback)
    }

    fun normalizeApiBaseUrl(value: String, fallback: String = BuildConfig.DEFAULT_API_BASE_URL): String {
        return normalizeBaseUrl(value, fallback)
    }

    fun normalizeBaseUrl(value: String, fallback: String = BuildConfig.DEFAULT_API_BASE_URL): String {
        val fallbackUrl = ensureHttpUrl(fallback)
        val raw = value.trim()
        if (raw.isBlank()) {
            return fallbackUrl.toString()
        }

        val candidate = if (raw.contains("://")) raw else "${fallbackUrl.scheme}://$raw"
        val parsed = candidate.toHttpUrlOrNull() ?: return fallbackUrl.toString()
        val normalizedPath = parsed.encodedPath
            .takeIf { it.isNotBlank() && it != "/" }
            ?.let { if (it.endsWith("/")) it else "$it/" }
            ?: "/"

        return parsed.newBuilder()
            .apply {
                if (!hasExplicitPort(raw)) {
                    port(fallbackUrl.port)
                }
                encodedPath(normalizedPath)
                query(null)
                fragment(null)
            }
            .build()
            .toString()
    }

    private fun ensureHttpUrl(value: String) = value.trim()
        .let { if (it.endsWith("/")) it else "$it/" }
        .toHttpUrl()

    private fun hasExplicitPort(raw: String): Boolean {
        val authority = raw.substringAfter("://", raw)
            .substringBefore('/')
            .substringBefore('?')
            .substringBefore('#')
            .trim()
        if (authority.isBlank()) {
            return false
        }
        return if (authority.startsWith("[")) {
            authority.contains("]:")
        } else {
            authority.contains(':')
        }
    }
}
