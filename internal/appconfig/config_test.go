package appconfig

import "testing"

func TestNormalizeContainerPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      string
		fallback string
		want     string
	}{
		{name: "empty uses fallback", raw: "", fallback: "/codex-home", want: "/codex-home"},
		{name: "windows absolute becomes posix", raw: `\codex-home\ctgbot-bootstrap.md`, fallback: "/codex-home", want: "/codex-home/ctgbot-bootstrap.md"},
		{name: "mixed separators cleaned", raw: `/etc\ctgbot/hostbridge-tls`, fallback: "/etc/ctgbot/hostbridge-tls", want: "/etc/ctgbot/hostbridge-tls"},
		{name: "missing leading slash", raw: `workspace`, fallback: "/workspace", want: "/workspace"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeContainerPath(tt.raw, tt.fallback); got != tt.want {
				t.Fatalf("normalizeContainerPath(%q, %q) = %q, want %q", tt.raw, tt.fallback, got, tt.want)
			}
		})
	}
}
