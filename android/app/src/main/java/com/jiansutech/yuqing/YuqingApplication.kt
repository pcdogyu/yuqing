package com.jiansutech.yuqing

import android.app.Application
import android.os.SystemClock
import android.util.Log
import androidx.room.Room
import com.jiansutech.yuqing.data.AppDatabase
import com.jiansutech.yuqing.data.SessionStore
import com.jiansutech.yuqing.notifications.AStockRecommendationScheduler

class YuqingApplication : Application() {
    lateinit var sessionStore: SessionStore
        private set
    lateinit var database: AppDatabase
        private set

    override fun onCreate() {
        val startedAt = SystemClock.elapsedRealtime()
        Log.i(STARTUP_TAG, "YuqingApplication.onCreate start")
        super.onCreate()
        sessionStore = SessionStore(this)
        Log.i(STARTUP_TAG, "YuqingApplication.sessionStore ready")
        database = Room.databaseBuilder(this, AppDatabase::class.java, "yuqing-mobile.db").build()
        Log.i(STARTUP_TAG, "YuqingApplication.database ready")
        AStockRecommendationScheduler.scheduleDailyRecommendations(this)
        Log.i(STARTUP_TAG, "YuqingApplication.notifications scheduled")
        Log.i(STARTUP_TAG, "YuqingApplication.onCreate end elapsedMs=${SystemClock.elapsedRealtime() - startedAt}")
    }
}

private const val STARTUP_TAG = "YuqingStartup"
