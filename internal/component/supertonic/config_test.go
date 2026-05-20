package supertonic

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadComponentConfigDefaults(t *testing.T) {
	config, err := loadComponentConfig(t.TempDir())
	if err != nil {
		t.Fatalf("loadComponentConfig() error = %v", err)
	}
	if config.ModelStore != "model" || config.DefaultVoice != "F5" || config.Language != "en" || config.PythonCommand != "python" || config.MaxConcurrent != 1 {
		t.Fatalf("config = %#v", config)
	}
}

func TestLoadRuntimeConfigDefaults(t *testing.T) {
	config, err := loadRuntimeConfig(t.TempDir())
	if err != nil {
		t.Fatalf("loadRuntimeConfig() error = %v", err)
	}
	if config.Image != DefaultImage {
		t.Fatalf("image = %q", config.Image)
	}
	if len(config.Cmd) != 3 || config.Cmd[0] != "tail" {
		t.Fatalf("cmd = %#v", config.Cmd)
	}
	if config.IdleTimeout != "30s" {
		t.Fatalf("idle timeout = %q", config.IdleTimeout)
	}
}

func TestLoadComponentConfigOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ComponentConfigFilename)
	if err := os.WriteFile(path, []byte(`{"model_store":"models","default_model":"supertonic3-f1","default_voice":"M1","language":"nl","python_command":"python3","max_concurrent":2}`), 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := loadComponentConfig(dir)
	if err != nil {
		t.Fatalf("loadComponentConfig() error = %v", err)
	}
	if config.ModelStore != "models" || config.DefaultModel != "supertonic3-f1" || config.DefaultVoice != "M1" || config.Language != "nl" || config.PythonCommand != "python3" || config.MaxConcurrent != 2 {
		t.Fatalf("config = %#v", config)
	}
}

func TestModelHostPathKeepsAbsoluteModelPath(t *testing.T) {
	got := modelHostPath("/workspace/models/supertonic3")
	want := filepath.Join("/workspace", "models", "supertonic3")
	if got != want {
		t.Fatalf("modelHostPath() = %q, want %q", got, want)
	}
}

func TestModelHostPathResolvesRelativeModelPath(t *testing.T) {
	got := modelHostPath("models/supertonic3")
	want, err := filepath.Abs(filepath.Join("models", "supertonic3"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("modelHostPath() = %q, want %q", got, want)
	}
}
