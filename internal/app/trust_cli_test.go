package app

import "testing"

func TestValidTrustedControllerFingerprint(t *testing.T) {
	valid := "SHA256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if !validTrustedControllerFingerprint(valid) {
		t.Fatalf("validTrustedControllerFingerprint(%q) = false", valid)
	}
	for _, value := range []string{
		"",
		"SHA255:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"SHA256:0123",
		"SHA256:0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef",
	} {
		if validTrustedControllerFingerprint(value) {
			t.Fatalf("validTrustedControllerFingerprint(%q) = true", value)
		}
	}
}
