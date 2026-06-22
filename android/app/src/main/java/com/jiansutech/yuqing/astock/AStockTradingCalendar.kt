package com.jiansutech.yuqing.astock

import java.time.DayOfWeek
import java.time.LocalDate

object AStockTradingCalendar {
    private val chinaPublicHolidayRanges2026 = listOf(
        LocalDate.of(2026, 1, 1) to LocalDate.of(2026, 1, 3),
        LocalDate.of(2026, 2, 15) to LocalDate.of(2026, 2, 23),
        LocalDate.of(2026, 4, 4) to LocalDate.of(2026, 4, 6),
        LocalDate.of(2026, 5, 1) to LocalDate.of(2026, 5, 5),
        LocalDate.of(2026, 6, 19) to LocalDate.of(2026, 6, 21),
        LocalDate.of(2026, 9, 25) to LocalDate.of(2026, 9, 27),
        LocalDate.of(2026, 10, 1) to LocalDate.of(2026, 10, 7),
    )

    private val chinaPublicHolidays2026: Set<LocalDate> = chinaPublicHolidayRanges2026
        .flatMap { (start, end) -> datesBetween(start, end) }
        .toSet()

    fun isTradingDay(date: LocalDate): Boolean {
        if (date.dayOfWeek == DayOfWeek.SATURDAY || date.dayOfWeek == DayOfWeek.SUNDAY) {
            return false
        }
        return date !in chinaPublicHolidays2026
    }

    fun previousOrSameTradingDay(date: LocalDate): LocalDate {
        var cursor = date
        while (!isTradingDay(cursor)) {
            cursor = cursor.minusDays(1)
        }
        return cursor
    }

    fun previousTradingDay(date: LocalDate): LocalDate {
        var cursor = date.minusDays(1)
        while (!isTradingDay(cursor)) {
            cursor = cursor.minusDays(1)
        }
        return cursor
    }

    fun nextTradingDay(date: LocalDate): LocalDate {
        var cursor = date.plusDays(1)
        while (!isTradingDay(cursor)) {
            cursor = cursor.plusDays(1)
        }
        return cursor
    }

    fun nextOrSameTradingDay(date: LocalDate): LocalDate {
        var cursor = date
        while (!isTradingDay(cursor)) {
            cursor = cursor.plusDays(1)
        }
        return cursor
    }

    fun latestSelectableTradingDay(today: LocalDate): LocalDate = previousOrSameTradingDay(today)

    private fun datesBetween(start: LocalDate, end: LocalDate): List<LocalDate> {
        val dates = mutableListOf<LocalDate>()
        var cursor = start
        while (!cursor.isAfter(end)) {
            dates.add(cursor)
            cursor = cursor.plusDays(1)
        }
        return dates
    }
}
