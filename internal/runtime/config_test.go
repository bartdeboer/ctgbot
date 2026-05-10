package runtime

import "testing"

func TestBindConfigWithEnvOverrideUpserts(t *testing.T) {
	config := BindConfig{
		Env: []string{
			"KEEP=1",
			"GIT_AUTHOR_NAME=Bot",
			"BAD",
		},
	}.WithEnvOverride(
		"GIT_AUTHOR_NAME=Human",
		"GIT_AUTHOR_EMAIL=human@example.com",
	)

	if got, want := envValue(config.Env, "KEEP"), "1"; got != want {
		t.Fatalf("KEEP = %q, want %q in %#v", got, want, config.Env)
	}
	if got, want := envValue(config.Env, "GIT_AUTHOR_NAME"), "Human"; got != want {
		t.Fatalf("GIT_AUTHOR_NAME = %q, want %q in %#v", got, want, config.Env)
	}
	if got, want := envValue(config.Env, "GIT_AUTHOR_EMAIL"), "human@example.com"; got != want {
		t.Fatalf("GIT_AUTHOR_EMAIL = %q, want %q in %#v", got, want, config.Env)
	}
	if envValue(config.Env, "BAD") != "" {
		t.Fatalf("invalid env entry survived in %#v", config.Env)
	}
	if countEnv(config.Env, "GIT_AUTHOR_NAME") != 1 {
		t.Fatalf("GIT_AUTHOR_NAME duplicated in %#v", config.Env)
	}
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, value := range env {
		if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
			return value[len(prefix):]
		}
	}
	return ""
}

func countEnv(env []string, key string) int {
	count := 0
	prefix := key + "="
	for _, value := range env {
		if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
			count++
		}
	}
	return count
}
