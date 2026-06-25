package com.jiansutech.yuqing.data

import androidx.room.Dao
import androidx.room.Database
import androidx.room.Entity
import androidx.room.PrimaryKey
import androidx.room.ColumnInfo
import androidx.room.Query
import androidx.room.RoomDatabase
import androidx.room.Upsert
import androidx.room.migration.Migration
import androidx.sqlite.db.SupportSQLiteDatabase

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

    @Query("DELETE FROM dashboard_cache")
    suspend fun clear()
}

@Entity(tableName = "article_user_actions")
data class ArticleUserActionEntity(
    @PrimaryKey @ColumnInfo(name = "article_id") val articleId: Long,
    val read: Boolean,
    val hidden: Boolean,
    @ColumnInfo(name = "updated_at") val updatedAt: Long,
)

@Dao
interface ArticleUserActionDao {
    @Query("SELECT article_id FROM article_user_actions WHERE hidden = 1")
    suspend fun hiddenArticleIds(): List<Long>

    @Upsert
    suspend fun upsert(entity: ArticleUserActionEntity)

    @Query("DELETE FROM article_user_actions")
    suspend fun clearAll()
}

@Database(entities = [DashboardCacheEntity::class, ArticleUserActionEntity::class], version = 2, exportSchema = false)
abstract class AppDatabase : RoomDatabase() {
    abstract fun dashboardCacheDao(): DashboardCacheDao
    abstract fun articleUserActionDao(): ArticleUserActionDao

    companion object {
        val MIGRATION_1_2 = object : Migration(1, 2) {
            override fun migrate(db: SupportSQLiteDatabase) {
                db.execSQL(
                    """
                    CREATE TABLE IF NOT EXISTS `article_user_actions` (
                        `article_id` INTEGER NOT NULL,
                        `read` INTEGER NOT NULL,
                        `hidden` INTEGER NOT NULL,
                        `updated_at` INTEGER NOT NULL,
                        PRIMARY KEY(`article_id`)
                    )
                    """.trimIndent(),
                )
            }
        }
    }
}
