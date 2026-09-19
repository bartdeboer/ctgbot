package appstate

import (
	"testing"
	"time"
)

func TestCodexSessionTimeout(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		want    time.Duration
		invalid bool
	}{
		{"", 0, false}, {"0", 0, false}, {"0s", 0, false}, {" 0m ", 0, false},
		{"10", 10 * time.Minute, false}, {"2h", 2 * time.Hour, false},
		{"48h", 48 * time.Hour, false}, {"1.5h", 90 * time.Minute, false},
		{"-1", 0, true}, {"-1s", 0, true}, {"junk", 0, true},
		{"9223372036854775807", 0, true}, {"153722868", 0, true},
		{"99999999999999999999999999", 0, true}, {"999999999h", 0, true},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			cfg, store := newTestConfig(t)
			// Read existing config directly too: invalid legacy values must not
			// silently become unlimited or fall back to ten minutes.
			if err := store.PersistString("session.timeout_min", tc.raw); err != nil {
				t.Fatal(err)
			}
			got, err := cfg.Codex().SessionTimeout()
			if (err != nil) != tc.invalid || (!tc.invalid && got != tc.want) {
				t.Fatalf("read = %v, %v; want %v, invalid=%v", got, err, tc.want, tc.invalid)
			}
			if err := cfg.Codex().SetSessionTimeout("2h"); err != nil {
				t.Fatal(err)
			}
			err = cfg.Codex().SetSessionTimeout(tc.raw)
			if (err != nil) != tc.invalid {
				t.Fatalf("set error = %v", err)
			}
			if tc.invalid {
				got, err = cfg.Codex().SessionTimeout()
				if err != nil || got != 2*time.Hour {
					t.Fatalf("invalid set changed value: %v, %v", got, err)
				}
			}
		})
	}
	cfg, _ := newTestConfig(t)
	if got, err := cfg.Codex().SessionTimeout(); err != nil || got != 0 {
		t.Fatalf("unset timeout = %v, %v", got, err)
	}
}
