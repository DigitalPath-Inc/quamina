package quamina

import (
	"fmt"
	"testing"
	"time"
)

func TestTimeWindowDays(t *testing.T) {
	// 2026-11-01 is a Sunday.
	sunday := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		days    string
		matches []time.Weekday
	}{
		{`["sun"]`, []time.Weekday{time.Sunday}},
		{`["Sunday"]`, []time.Weekday{time.Sunday}},
		{`[0]`, []time.Weekday{time.Sunday}},
		{`[7]`, []time.Weekday{time.Sunday}},
		{`[1]`, []time.Weekday{time.Monday}},
		{`[6]`, []time.Weekday{time.Saturday}},
		{`["mon", 3, "fri"]`, []time.Weekday{time.Monday, time.Wednesday, time.Friday}},
		{`[0, 7]`, []time.Weekday{time.Sunday}},
	}
	for _, c := range cases {
		q, _ := New()
		pattern := fmt.Sprintf(`{"ts": [{"time-window": {"tz": "UTC", "days": %s, "start": "09:00", "end": "17:00"}}]}`, c.days)
		if err := q.AddPattern("p", pattern); err != nil {
			t.Fatalf("days %s: %v", c.days, err)
		}
		want := make(map[time.Weekday]bool)
		for _, d := range c.matches {
			want[d] = true
		}
		for i := range 7 {
			ts := sunday.AddDate(0, 0, i)
			got, err := q.MatchesForEvent(fmt.Appendf(nil, `{"ts": %d}`, ts.UnixMilli()))
			if err != nil {
				t.Fatal(err)
			}
			if (len(got) == 1) != want[ts.Weekday()] {
				t.Errorf("days %s on %s: matched %v", c.days, ts.Weekday(), got)
			}
		}
	}
}

func TestTimeWindowBadDays(t *testing.T) {
	for _, days := range []string{`[-1]`, `[8]`, `[1.5]`, `[1e0]`, `[true]`, `["funday"]`, `["1"]`} {
		q, _ := New()
		pattern := fmt.Sprintf(`{"ts": [{"time-window": {"tz": "UTC", "days": %s, "start": "09:00", "end": "17:00"}}]}`, days)
		if err := q.AddPattern("p", pattern); err == nil {
			t.Errorf("days %s: expected error", days)
		}
	}
}
