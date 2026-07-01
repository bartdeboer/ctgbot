package schedule

import (
	"testing"
	"time"
)

func TestNextCronUsesTimezone(t *testing.T) {
	after := time.Date(2026, 7, 1, 21, 0, 0, 0, time.UTC)
	next, err := NextCron("30 3 * * *", "Europe/Amsterdam", after)
	if err != nil {
		t.Fatalf("NextCron error = %v", err)
	}
	want := time.Date(2026, 7, 2, 1, 30, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next = %s, want %s", next, want)
	}
}

func TestParseCronRejectsDoubleTimezone(t *testing.T) {
	if _, err := ParseCron("CRON_TZ=UTC 30 3 * * *", "Europe/Amsterdam"); err == nil {
		t.Fatal("ParseCron error = nil, want error")
	}
}
