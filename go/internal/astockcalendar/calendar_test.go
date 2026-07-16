package astockcalendar

import "testing"

func TestIsTradingDay(t *testing.T) {
	tests := map[string]bool{
		"2026-07-16": true,
		"2026-07-18": false,
		"2026-10-01": false,
		"bad-date":   false,
	}
	for date, want := range tests {
		if got := IsTradingDay(date); got != want {
			t.Fatalf("IsTradingDay(%q) = %v, want %v", date, got, want)
		}
	}
}

func TestAdjacentTradingDay(t *testing.T) {
	tests := map[string]struct {
		date      string
		direction int
		want      string
	}{
		"same trading day":       {date: "2026-07-16", direction: 0, want: "2026-07-16"},
		"weekend previous":       {date: "2026-07-18", direction: 0, want: "2026-07-17"},
		"previous skips holiday": {date: "2026-10-08", direction: -1, want: "2026-09-30"},
		"next skips weekend":     {date: "2026-07-17", direction: 1, want: "2026-07-20"},
		"invalid date":           {date: "bad-date", direction: 1, want: ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := AdjacentTradingDay(tt.date, tt.direction); got != tt.want {
				t.Fatalf("AdjacentTradingDay(%q, %d) = %q, want %q", tt.date, tt.direction, got, tt.want)
			}
		})
	}
}
