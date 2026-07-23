package com.jiansutech.yuqing.notifications

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Test

class AStockRecommendationSchedulerTest {
    @Test
    fun slotsIncludeEveningRecommendationAlarm() {
        val slot = AStockRecommendationScheduler.slotById("evening_1830")

        assertNotNull(slot)
        assertEquals("evening", slot?.period)
        assertEquals(AStockNotificationKind.Recommendation, slot?.kind)
        assertEquals("18:30 晚间热门股票推荐", slot?.title)
        assertEquals("15:00-18:30", slot?.windowLabel)
        assertEquals(18, slot?.hour)
        assertEquals(30, slot?.minute)
    }
}
