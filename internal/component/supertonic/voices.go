package supertonic

import "strings"

// supertonicVoiceName accepts both the SDK's compact voice style ids and the
// friendly names shown by the public Supertonic demo. The model files only
// contain F1-F5/M1-M5, so the demo names are aliases at the ctgbot boundary.
func supertonicVoiceName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "alex":
		return "M1"
	case "james":
		return "M2"
	case "robert":
		return "M3"
	case "sam":
		return "M4"
	case "daniel":
		return "M5"
	case "sarah":
		return "F1"
	case "lily":
		return "F2"
	case "jessica":
		return "F3"
	case "olivia":
		return "F4"
	case "emily":
		return "F5"
	default:
		return strings.TrimSpace(name)
	}
}
