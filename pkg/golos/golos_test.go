package golos

import (
	"testing"
	"time"
)

func TestLunchIsMoscowWeekdays(t *testing.T) {
	g := New(false)
	for at, want := range map[string]bool{
		"2026-10-26T10:30:00Z": true,  // 13:30 in Moscow, Monday
		"2026-10-26T09:30:00Z": false, // 12:30
		"2026-10-26T11:00:00Z": false, // 14:00
		"2026-10-25T10:30:00Z": false, // Sunday
	} {
		now, _ := time.Parse(time.RFC3339, at)
		g.Now = func() time.Time { return now }
		if got := g.Lunch(); got != want {
			t.Errorf("%s: lunch %v, want %v", at, got, want)
		}
	}
}
