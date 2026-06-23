package com.jiansutech.yuqing.data

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.jiansutech.yuqing.BuildConfig
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map

private val Context.sessionDataStore by preferencesDataStore("yuqing_session")

data class SessionState(
    val token: String = "",
    val username: String = "admin",
    val authBaseUrl: String = BuildConfig.DEFAULT_AUTH_BASE_URL,
    val apiBaseUrl: String = BuildConfig.DEFAULT_API_BASE_URL,
) {
    val loggedIn: Boolean get() = true
}

class SessionStore(private val context: Context) {
    val state: Flow<SessionState> = context.sessionDataStore.data.map { preferences ->
        SessionState(
            token = preferences[tokenKey].orEmpty(),
            username = preferences[usernameKey].orEmpty().ifBlank { "admin" },
            authBaseUrl = ApiFactory.normalizeAuthBaseUrl(preferences[authBaseUrlKey].orEmpty(), BuildConfig.DEFAULT_AUTH_BASE_URL),
            apiBaseUrl = ApiFactory.normalizeApiBaseUrl(preferences[apiBaseUrlKey].orEmpty(), BuildConfig.DEFAULT_API_BASE_URL),
        )
    }

    suspend fun saveLogin(token: String, username: String, authBaseUrl: String, apiBaseUrl: String) {
        context.sessionDataStore.edit { preferences ->
            preferences[tokenKey] = token
            preferences[usernameKey] = username
            preferences[authBaseUrlKey] = ApiFactory.normalizeAuthBaseUrl(authBaseUrl, BuildConfig.DEFAULT_AUTH_BASE_URL)
            preferences[apiBaseUrlKey] = ApiFactory.normalizeApiBaseUrl(apiBaseUrl, BuildConfig.DEFAULT_API_BASE_URL)
        }
    }

    suspend fun saveServers(authBaseUrl: String, apiBaseUrl: String) {
        context.sessionDataStore.edit { preferences ->
            preferences[authBaseUrlKey] = ApiFactory.normalizeAuthBaseUrl(authBaseUrl, BuildConfig.DEFAULT_AUTH_BASE_URL)
            preferences[apiBaseUrlKey] = ApiFactory.normalizeApiBaseUrl(apiBaseUrl, BuildConfig.DEFAULT_API_BASE_URL)
        }
    }

    suspend fun clear() {
        context.sessionDataStore.edit { preferences ->
            preferences.remove(tokenKey)
            preferences.remove(usernameKey)
            preferences.remove(authBaseUrlKey)
            preferences.remove(apiBaseUrlKey)
        }
    }

    private companion object {
        val tokenKey = stringPreferencesKey("session_token")
        val usernameKey = stringPreferencesKey("username")
        val authBaseUrlKey = stringPreferencesKey("auth_base_url")
        val apiBaseUrlKey = stringPreferencesKey("api_base_url")
    }
}
