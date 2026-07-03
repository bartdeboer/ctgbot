package schedule

import (
	"testing"
	"time"
)

func TestNextAnchoredIntervalAvoidsCompletionDrift(t *testing.T) {
	due := time.Date(2026, 6, 30, 1, 0, 0, 0, time.UTC)
	finished := due.Add(17 * time.Minute)
	next := NextAnchoredInterval(due, 24*time.Hour, finished)
	if got, want := next, due.Add(24*time.Hour); !got.Equal(want) {
		t.Fatalf("next = %v, want %v", got, want)
	}
}

func TestNextAnchoredIntervalCoalescesMissedSlots(t *testing.T) {
	due := time.Date(2026, 6, 30, 1, 0, 0, 0, time.UTC)
	finished := due.Add(49 * time.Hour)
	next := NextAnchoredInterval(due, 24*time.Hour, finished)
	if got, want := next, due.Add(72*time.Hour); !got.Equal(want) {
		t.Fatalf("next = %v, want %v", got, want)
	}
}
