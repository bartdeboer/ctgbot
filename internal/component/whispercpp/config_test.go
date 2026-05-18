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
	if got := envValue(config.Env, "LD_LIBRARY_PATH"); got != DefaultLDLibraryPath {
		t.Fatalf("LD_LIBRARY_PATH = %q, want %q in %#v", got, DefaultLDLibraryPath, config.Env)
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
