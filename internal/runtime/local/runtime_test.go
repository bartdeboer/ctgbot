package local

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

func TestBindPropagatesBaseEnvOverRuntimeEnv(t *testing.T) {
	t.Parallel()

	factory := New("/state", "/state/components").WithEnv("GIT_AUTHOR_NAME=Human")
	runtime := factory.Bind(
		coremodel.Component{Type: "mock", Name: "default"},
		runtimepkg.Profile{Path: "/state/components/mock/default"},
		runtimepkg.BindConfig{Env: []string{"GIT_AUTHOR_NAME=Bot"}},
	).(*Runtime)

	if got, want := findEnv(runtime.env, "GIT_AUTHOR_NAME"), "Human"; got != want {
		t.Fatalf("GIT_AUTHOR_NAME = %q, want %q in %#v", got, want, runtime.env)
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
