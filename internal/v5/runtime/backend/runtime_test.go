package backend

import (
	"slices"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/containerengine"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
)

func TestFactoryComponentHomeDefaultsToComponentsRoot(t *testing.T) {
	t.Parallel()

	factory := New("/state/components", nil)
	home := factory.ComponentHome(coremodel.Component{Type: "llamacpp", Name: "qwen3-q5"})
	if got, want := home.Path, "/state/components/llamacpp/qwen3-q5"; got != want {
		t.Fatalf("home.Path = %q, want %q", got, want)
	}
}

func TestBindBackendBuildsContainerSpecFromRuntimeAndServiceConfig(t *testing.T) {
	t.Parallel()

	factory := New("/state/components", nil)
	runtime := factory.BindBackend(
		coremodel.Component{Type: "llamacpp", Name: "qwen3-q5"},
		v5runtime.Home{Path: "/state/components/llamacpp/qwen3-q5"},
		v5runtime.BindConfig{
			Image: "llama:test",
			GPUs:  "all",
			Env:   []string{"RUNTIME=1"},
		},
		ServiceSpec{
			BaseURL:   "http://127.0.0.1:18080",
			HealthURL: "http://127.0.0.1:18080/health",
			Ports:     []string{"127.0.0.1:18080:8080"},
			Env:       []string{"SERVICE=1"},
			Mounts: []containerengine.Mount{
				{Source: "/models", Target: "/models", ReadOnly: true},
			},
			Cmd: []string{"--server"},
		},
	)

	spec := runtime.containerSpec()
	if got, want := spec.Name, "ctgbot-v5-backend-llamacpp-qwen3-q5"; got != want {
		t.Fatalf("Name = %q, want %q", got, want)
	}
	if got, want := spec.Image, "llama:test"; got != want {
		t.Fatalf("Image = %q, want %q", got, want)
	}
	if got, want := spec.GPUs, "all"; got != want {
		t.Fatalf("GPUs = %q, want %q", got, want)
	}
	if !slices.Equal(spec.Env, []string{"RUNTIME=1", "SERVICE=1"}) {
		t.Fatalf("Env = %#v", spec.Env)
	}
	if !slices.Equal(spec.Ports, []string{"127.0.0.1:18080:8080"}) {
		t.Fatalf("Ports = %#v", spec.Ports)
	}
	if !slices.Equal(spec.Cmd, []string{"--server"}) {
		t.Fatalf("Cmd = %#v", spec.Cmd)
	}
	if got, want := runtime.BaseURL(), "http://127.0.0.1:18080"; got != want {
		t.Fatalf("BaseURL = %q, want %q", got, want)
	}
}
