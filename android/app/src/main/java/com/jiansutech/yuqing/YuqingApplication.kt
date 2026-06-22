package com.jiansutech.yuqing

import android.app.Application
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
        super.onCreate()
        sessionStore = SessionStore(this)
        database = Room.databaseBuilder(this, AppDatabase::class.java, "yuqing-mobile.db").build()
        AStockRecommendationScheduler.scheduleDailyRecommendations(this)
    }
}
