package schedule

import (
	"fmt"
	"time"
)

// NextAnchoredInterval returns the next fixed-cadence due time after after.
//
// The cadence is anchored to previousDue, not to completion time. If the caller
// was delayed or missed several slots, the result coalesces forward to the
// first slot after after. This is the right shape for scheduled jobs where
// "every 24h" means a stable cadence, not a cooldown after completion.
func NextAnchoredInterval(previousDue time.Time, every time.Duration, after time.Time) (time.Time, error) {
	if every <= 0 {
		return time.Time{}, fmt.Errorf("interval must be positive")
	}
	if after.IsZero() {
		after = time.Now().UTC()
	}
	after = after.UTC()
	base := previousDue.UTC()
	if base.IsZero() {
		base = after
	}
	next := base.Add(every)
	for !next.After(after) {
		next = next.Add(every)
	}
	return next.UTC(), nil
}
