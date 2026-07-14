package astockcalendar

import "time"

var marketHolidays = map[string]struct{}{
	"2026-01-01": {},
	"2026-01-02": {},
	"2026-01-03": {},
	"2026-02-15": {},
	"2026-02-16": {},
	"2026-02-17": {},
	"2026-02-18": {},
	"2026-02-19": {},
	"2026-02-20": {},
	"2026-02-21": {},
	"2026-02-22": {},
	"2026-02-23": {},
	"2026-04-04": {},
	"2026-04-05": {},
	"2026-04-06": {},
	"2026-05-01": {},
	"2026-05-02": {},
	"2026-05-03": {},
	"2026-05-04": {},
	"2026-05-05": {},
	"2026-06-19": {},
	"2026-06-20": {},
	"2026-06-21": {},
	"2026-09-25": {},
	"2026-09-26": {},
	"2026-09-27": {},
	"2026-10-01": {},
	"2026-10-02": {},
	"2026-10-03": {},
	"2026-10-04": {},
	"2026-10-05": {},
	"2026-10-06": {},
	"2026-10-07": {},
}

func Location() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("UTC+8", 8*60*60)
	}
	return location
}

func IsTradingDay(date string) bool {
	day, err := time.ParseInLocation("2006-01-02", date, Location())
	if err != nil {
		return false
	}
	if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
		return false
	}
	_, holiday := marketHolidays[date]
	return !holiday
}

func AdjacentTradingDay(date string, direction int) string {
	day, err := time.ParseInLocation("2006-01-02", date, Location())
	if err != nil {
		return ""
	}
	if direction == 0 {
		if IsTradingDay(date) {
			return date
		}
		return AdjacentTradingDay(date, -1)
	}
	step := 1
	if direction < 0 {
		step = -1
	}
	for offset := step; offset >= -14 && offset <= 14; offset += step {
		candidate := day.AddDate(0, 0, offset).Format("2006-01-02")
		if IsTradingDay(candidate) {
			return candidate
		}
	}
	return ""
}
