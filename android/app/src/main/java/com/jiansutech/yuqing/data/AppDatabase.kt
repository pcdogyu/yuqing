package com.jiansutech.yuqing.data

import androidx.room.Dao
import androidx.room.Database
import androidx.room.Entity
import androidx.room.PrimaryKey
import androidx.room.Query
import androidx.room.RoomDatabase
import androidx.room.Upsert

@Entity(tableName = "dashboard_cache")
data class DashboardCacheEntity(
    @PrimaryKey val key: String = "dashboard",
    val payload: String,
    val savedAt: Long,
)

@Dao
interface DashboardCacheDao {
    @Query("SELECT * FROM dashboard_cache WHERE key = :key")
    suspend fun get(key: String = "dashboard"): DashboardCacheEntity?

    @Upsert
    suspend fun upsert(entity: DashboardCacheEntity)
}

@Database(entities = [DashboardCacheEntity::class], version = 1)
abstract class AppDatabase : RoomDatabase() {
    abstract fun dashboardCacheDao(): DashboardCacheDao
}
