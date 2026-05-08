package llamacpp

import (
	"slices"
	"testing"

	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

func TestComponentManagedFiles(t *testing.T) {
	t.Parallel()

	component := &Component{}
	got := component.ManagedFiles()
	if len(got) != 2 {
		t.Fatalf("len(ManagedFiles) = %d, want 2", len(got))
	}
	if got[0].RelativePath != runtimepkg.ConfigFilename {
		t.Fatalf("ManagedFiles[0] = %q, want %q", got[0].RelativePath, runtimepkg.ConfigFilename)
	}
	if got[1].RelativePath != ComponentConfigFilename {
		t.Fatalf("ManagedFiles[1] = %q, want %q", got[1].RelativePath, ComponentConfigFilename)
	}
}

func TestServiceSpecUsesComponentConfig(t *testing.T) {
	t.Parallel()

	spec := serviceSpec(ComponentConfig{
		ModelPath:   "/srv/models/qwen/model.gguf",
		MMProjPath:  "/srv/mmproj/mmproj.gguf",
		HostPort:    18080,
		ContextSize: 4096,
		GPULayers:   48,
	})
	if got, want := spec.BaseURL, "http://127.0.0.1:18080"; got != want {
		t.Fatalf("BaseURL = %q, want %q", got, want)
	}
	if !slices.Equal(spec.Ports, []string{"127.0.0.1:18080:8080"}) {
		t.Fatalf("Ports = %#v", spec.Ports)
	}
	if len(spec.Mounts) != 2 {
		t.Fatalf("len(Mounts) = %d, want 2: %#v", len(spec.Mounts), spec.Mounts)
	}
	if !slices.Contains(spec.Cmd, "--mmproj") {
		t.Fatalf("Cmd missing --mmproj: %#v", spec.Cmd)
	}
	if !slices.Contains(spec.Cmd, "/mmproj/mmproj.gguf") {
		t.Fatalf("Cmd missing /mmproj/mmproj.gguf: %#v", spec.Cmd)
	}
}
