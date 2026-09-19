package agentcommon

import "math"

// SumUsage preserves unknowns and refuses negative/overflowed provider counts.
func SumUsage(values ...*int64) *int64 {
	var total int64
	if len(values) == 0 {
		return nil
	}
	for _, value := range values {
		if value == nil || *value < 0 || *value > math.MaxInt64-total {
			return nil
		}
		total += *value
	}
	return &total
}
