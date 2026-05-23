package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

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

func TestBindConfigCleanNormalizesIdleTimeout(t *testing.T) {
	config := BindConfig{IdleTimeout: " 45s "}.Clean()

	if config.IdleTimeout != "45s" {
		t.Fatalf("IdleTimeout = %q, want 45s", config.IdleTimeout)
	}
}

func TestBindConfigUIDGIDDefaultsToCtgbotUser(t *testing.T) {
	uid, gid := (BindConfig{}).UIDGID()
	if uid != 1000 || gid != 1000 {
		t.Fatalf("UIDGID() = %d:%d, want 1000:1000", uid, gid)
	}
	if got := (BindConfig{}).UserString(); got != "1000:1000" {
		t.Fatalf("UserString() = %q, want 1000:1000", got)
	}
}

func TestBindConfigUIDGIDAllowsRoot(t *testing.T) {
	uidValue := 0
	gidValue := 0
	config := BindConfig{UID: &uidValue, GID: &gidValue}
	if got := config.UserString(); got != "0:0" {
		t.Fatalf("UserString() = %q, want 0:0", got)
	}
}

func TestBindConfigUIDDefaultsGIDToUID(t *testing.T) {
	uidValue := 1001
	config := BindConfig{UID: &uidValue}
	if got := config.UserString(); got != "1001:1001" {
		t.Fatalf("UserString() = %q, want 1001:1001", got)
	}
}

func TestLoadBindConfigReadsRuntimeImageUses(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ConfigFilename), []byte(`{
		"image": "ctgbot-codex:gpu",
		"dockerfile": "cuda.Dockerfile",
		"no_cache": true,
		"uid": 0,
		"gid": 0,
		"uses": {
			"name": "codex-cuda-base",
			"image": "ctgbot-codex-cuda-base:latest",
			"dockerfile": "cuda.base.Dockerfile"
		}
	}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	config, err := LoadBindConfig(home)
	if err != nil {
		t.Fatalf("LoadBindConfig() error = %v", err)
	}
	if config.Image != "ctgbot-codex:gpu" || config.Dockerfile != "cuda.Dockerfile" || !config.NoCache {
		t.Fatalf("config = %#v", config)
	}
	if got := config.UserString(); got != "0:0" {
		t.Fatalf("UserString() = %q, want 0:0", got)
	}
	if config.Uses == nil || config.Uses.Name != "codex-cuda-base" || config.Uses.Image != "ctgbot-codex-cuda-base:latest" || config.Uses.Dockerfile != "cuda.base.Dockerfile" {
		t.Fatalf("uses = %#v", config.Uses)
	}
}
