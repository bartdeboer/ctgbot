package supertonic

import "testing"

func TestSupertonicVoiceNameAcceptsDemoAliases(t *testing.T) {
	tests := map[string]string{
		"Alex":     "M1",
		"Daniel":   "M5",
		"Sarah":    "F1",
		"Emily":    "F5",
		"F3":       "F3",
		" custom ": "custom",
	}
	for input, want := range tests {
		if got := supertonicVoiceName(input); got != want {
			t.Fatalf("supertonicVoiceName(%q) = %q, want %q", input, got, want)
		}
	}
}
