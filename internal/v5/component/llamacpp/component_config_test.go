package llamacpp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRuntimeConfigDefaultsWhenMissing(t *testing.T) {
	t.Parallel()

	config, err := loadRuntimeConfig(t.TempDir())
	if err != nil {
		t.Fatalf("loadRuntimeConfig() error = %v", err)
	}
	if got, want := config.Image, DefaultImage; got != want {
		t.Fatalf("Image = %q, want %q", got, want)
	}
	if got, want := config.GPUs, "all"; got != want {
		t.Fatalf("GPUs = %q, want %q", got, want)
	}
	if len(config.Env) != 0 {
		t.Fatalf("Env = %#v, want nil/empty", config.Env)
	}
}

func TestLoadRuntimeConfigReadsRuntimeJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "runtime.json")
	data := []byte(`{"image":"llama:test","gpus":"device=0","env":["FOO=bar"," BAR=baz "]}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(runtime.json) error = %v", err)
	}

	config, err := loadRuntimeConfig(dir)
	if err != nil {
		t.Fatalf("loadRuntimeConfig() error = %v", err)
	}
	if got, want := config.Image, "llama:test"; got != want {
		t.Fatalf("Image = %q, want %q", got, want)
	}
	if got, want := config.GPUs, "device=0"; got != want {
		t.Fatalf("GPUs = %q, want %q", got, want)
	}
	if got, want := len(config.Env), 2; got != want {
		t.Fatalf("len(Env) = %d, want %d: %#v", got, want, config.Env)
	}
}

func TestLoadComponentConfigDefaultsWhenMissing(t *testing.T) {
	t.Parallel()

	config, err := loadComponentConfig(t.TempDir(), "gemma4-e4b")
	if err != nil {
		t.Fatalf("loadComponentConfig() error = %v", err)
	}
	if config.ModelPath == "" {
		t.Fatalf("ModelPath = empty, want default model path")
	}
	if config.MMProjPath == "" {
		t.Fatalf("MMProjPath = empty, want default mmproj path")
	}
	if got, want := config.HostPort, 19082; got != want {
		t.Fatalf("HostPort = %d, want %d", got, want)
	}
}

func TestLoadComponentConfigReadsComponentJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ComponentConfigFilename)
	data := []byte(`{"model_path":"/models/foo.gguf","host_port":18080,"temperature":0.7}`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(component.json) error = %v", err)
	}

	config, err := loadComponentConfig(dir, "custom")
	if err != nil {
		t.Fatalf("loadComponentConfig() error = %v", err)
	}
	if got, want := config.ModelPath, "/models/foo.gguf"; got != want {
		t.Fatalf("ModelPath = %q, want %q", got, want)
	}
	if got, want := config.HostPort, 18080; got != want {
		t.Fatalf("HostPort = %d, want %d", got, want)
	}
	if got, want := config.Temperature, 0.7; got != want {
		t.Fatalf("Temperature = %v, want %v", got, want)
	}
}
