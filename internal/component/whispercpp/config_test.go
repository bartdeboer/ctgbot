package whispercpp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRuntimeConfigUsesWhisperCppImageAndLibraryPath(t *testing.T) {
	dir := t.TempDir()
	config, err := loadRuntimeConfig(dir)
	if err != nil {
		t.Fatalf("loadRuntimeConfig() error = %v", err)
	}
	if config.Image != DefaultImage {
		t.Fatalf("Image = %q, want %q", config.Image, DefaultImage)
	}
	if config.GPUs != "all" {
		t.Fatalf("GPUs = %q, want all", config.GPUs)
	}
	if got := envValue(config.Env, "LD_LIBRARY_PATH"); got != DefaultLDLibraryPath {
		t.Fatalf("LD_LIBRARY_PATH = %q, want %q in %#v", got, DefaultLDLibraryPath, config.Env)
	}
	if config.Entrypoint != "tail" {
		t.Fatalf("Entrypoint = %q, want tail", config.Entrypoint)
	}
	if len(config.Cmd) != 2 || config.Cmd[0] != "-f" || config.Cmd[1] != "/dev/null" {
		t.Fatalf("Cmd = %#v, want whisper.cpp idle command", config.Cmd)
	}
	if config.IdleTimeout != "30s" {
		t.Fatalf("IdleTimeout = %q, want 30s", config.IdleTimeout)
	}
}

func TestLoadRuntimeConfigAllowsLibraryPathOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "runtime.json"), []byte(`{
  "env": ["LD_LIBRARY_PATH=/custom/libs"]
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := loadRuntimeConfig(dir)
	if err != nil {
		t.Fatalf("loadRuntimeConfig() error = %v", err)
	}
	if got := envValue(config.Env, "LD_LIBRARY_PATH"); got != "/custom/libs" {
		t.Fatalf("LD_LIBRARY_PATH = %q, want override in %#v", got, config.Env)
	}
}

func TestComponentConfigDefaultsToOfficialWhisperCommand(t *testing.T) {
	config := ComponentConfig{}.withDefaults()
	if config.WhisperCommand != DefaultWhisperCommand {
		t.Fatalf("WhisperCommand = %q, want %q", config.WhisperCommand, DefaultWhisperCommand)
	}
	if config.FFMpegCommand != "ffmpeg" {
		t.Fatalf("FFMpegCommand = %q", config.FFMpegCommand)
	}
	if config.MaxConcurrent != 1 {
		t.Fatalf("MaxConcurrent = %d, want 1", config.MaxConcurrent)
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
