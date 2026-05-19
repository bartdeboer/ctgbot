package languagedetect

import "testing"

func TestDetectUsesCandidateWhitelist(t *testing.T) {
	got, ok := Detect("Hallo, dit is een Nederlandse test. Kijken of het nu wel goed werkt.", []string{"nl", "en"})
	if !ok || got != "nl" {
		t.Fatalf("Detect() = %q, %v; want nl, true", got, ok)
	}
}

func TestDetectCanChooseEnglishOverInputLanguage(t *testing.T) {
	got, ok := Detect("This is an English reply that should be spoken as English.", []string{"nl", "en"})
	if !ok || got != "en" {
		t.Fatalf("Detect() = %q, %v; want en, true", got, ok)
	}
}

func TestDetectFallsBackWhenUnreliable(t *testing.T) {
	got, ok := Detect("ok", []string{"nl", "en"})
	if ok || got != "" {
		t.Fatalf("Detect() = %q, %v; want empty, false", got, ok)
	}
}
