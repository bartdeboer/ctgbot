package backend

import (
	"slices"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/containerengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

func TestFactoryComponentProfileDefaultsToComponentsRoot(t *testing.T) {
	t.Parallel()

	factory := New("/state/components", nil)
	profile := factory.ComponentProfile(coremodel.Component{Type: "llamacpp", Name: "qwen3-q5"})
	if got, want := profile.Path, "/state/components/llamacpp/qwen3-q5"; got != want {
		t.Fatalf("profile.Path = %q, want %q", got, want)
	}
}

func TestBindBackendBuildsContainerSpecFromRuntimeAndServiceConfig(t *testing.T) {
	t.Parallel()

	factory := New("/state/components", nil)
	runtime := factory.BindBackend(
		coremodel.Component{Type: "llamacpp", Name: "qwen3-q5"},
		runtimepkg.Profile{Path: "/state/components/llamacpp/qwen3-q5"},
		runtimepkg.BindConfig{
			Image:   "llama:test",
			GPUs:    "all",
			Env:     []string{"RUNTIME=1"},
			Seccomp: "unconfined",
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

	spec, err := runtime.containerSpec()
	if err != nil {
		t.Fatalf("containerSpec() error = %v", err)
	}
	if got, want := spec.Name, "ctgbot-backend-llamacpp-qwen3-q5"; got != want {
		t.Fatalf("Name = %q, want %q", got, want)
	}
	if got, want := spec.Image, "llama:test"; got != want {
		t.Fatalf("Image = %q, want %q", got, want)
	}
	if got, want := spec.GPUs, "all"; got != want {
		t.Fatalf("GPUs = %q, want %q", got, want)
	}
	for _, want := range []string{"RUNTIME=1", "SERVICE=1"} {
		if !slices.Contains(spec.Env, want) {
			t.Fatalf("Env missing %q in %#v", want, spec.Env)
		}
	}
	if !slices.Equal(spec.SecurityOpts, []string{"seccomp=unconfined"}) {
		t.Fatalf("SecurityOpts = %#v", spec.SecurityOpts)
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

func TestBindBackendPropagatesBaseEnvOverRuntimeEnv(t *testing.T) {
	t.Parallel()

	factory := New("/state/components", nil).WithEnv("GIT_AUTHOR_NAME=Human")
	runtime := factory.BindBackend(
		coremodel.Component{Type: "llamacpp", Name: "qwen3-q5"},
		runtimepkg.Profile{Path: "/state/components/llamacpp/qwen3-q5"},
		runtimepkg.BindConfig{Env: []string{"GIT_AUTHOR_NAME=Bot"}},
		ServiceSpec{Env: []string{"GIT_AUTHOR_NAME=Service"}},
	)

	spec, err := runtime.containerSpec()
	if err != nil {
		t.Fatalf("containerSpec() error = %v", err)
	}
	if got, want := findEnv(spec.Env, "GIT_AUTHOR_NAME"), "Human"; got != want {
		t.Fatalf("GIT_AUTHOR_NAME = %q, want %q in %#v", got, want, spec.Env)
	}
}

func findEnv(env []string, key string) string {
	prefix := key + "="
	for _, value := range env {
		if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
			return value[len(prefix):]
		}
	}
	return ""
}
