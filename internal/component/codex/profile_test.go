package codex

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/go-clistate"
)

type captureBindFactory struct {
	home   runtimepkg.Home
	config runtimepkg.BindConfig
}

func (f *captureBindFactory) Kind() string { return "docker" }
func (f *captureBindFactory) ComponentHome(registration coremodel.Component) runtimepkg.Home {
	_ = registration
	return f.home
}
func (f *captureBindFactory) RuntimeComponentHomePath(registration coremodel.Component, home runtimepkg.Home) string {
	_, _ = registration, home
	return "/profile/components/codex/gpu"
}
func (f *captureBindFactory) RuntimeWorkspacePath(workspacePath string) string { return workspacePath }
func (f *captureBindFactory) Bind(registration coremodel.Component, home runtimepkg.Home, config runtimepkg.BindConfig) runtimepkg.Runtime {
	_, _ = registration, home
	f.config = config
	return &capturedRuntime{home: f.home}
}

type capturedRuntime struct{ home runtimepkg.Home }

func (r *capturedRuntime) Kind() string                                          { return "docker" }
func (r *capturedRuntime) ComponentHome() runtimepkg.Home                        { return r.home }
func (r *capturedRuntime) RuntimeComponentHomePath() string                      { return "/profile/components/codex/gpu" }
func (r *capturedRuntime) RuntimeWorkspacePath(workspacePath string) string      { return workspacePath }
func (r *capturedRuntime) Refresh(context.Context, string, modeluuid.UUID) error { return nil }
func (r *capturedRuntime) Start(context.Context, string, modeluuid.UUID) (runtimepkg.Status, error) {
	return runtimepkg.Status{}, nil
}
func (r *capturedRuntime) Stop(context.Context, string, modeluuid.UUID) error { return nil }
func (r *capturedRuntime) Interrupt(context.Context, string, modeluuid.UUID) (bool, error) {
	return false, nil
}
func (r *capturedRuntime) Status(context.Context, string, modeluuid.UUID) (runtimepkg.Status, error) {
	return runtimepkg.Status{}, nil
}
func (r *capturedRuntime) Exec(context.Context, string, modeluuid.UUID, commandengine.CommandExecutor, io.Writer, io.Writer, string, ...string) error {
	return nil
}
func (r *capturedRuntime) CombinedOutput(context.Context, string, modeluuid.UUID, commandengine.CommandExecutor, string, ...string) ([]byte, error) {
	return nil, nil
}
func (r *capturedRuntime) OpenHTTPRelayPort(context.Context, string, modeluuid.UUID, commandengine.CommandExecutor, int, time.Duration) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}

func TestNewUsesProfileRuntimeConfigAndProfileDefaults(t *testing.T) {
	withTempCwd(t, func(root string) {
		homePath := filepath.Join(root, ".ctgbot", "components", "codex", "gpu")
		if err := os.MkdirAll(homePath, 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(homePath, runtimepkg.ConfigFilename), []byte(`{
  "image": "ctgbot-codex:gpu",
  "gpus": "all",
  "env": ["FOO=bar"]
}`), 0o644); err != nil {
			t.Fatalf("WriteFile(runtime.json) error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(homePath, ComponentConfigFilename), []byte(`{
  "model": "gpt-5.5",
  "reasoning_effort": "high",
  "sandbox_mode": "workspace-write"
}`), 0o644); err != nil {
			t.Fatalf("WriteFile(component.json) error = %v", err)
		}

		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewCwd() error = %v", err)
		}
		cfg := appstate.New(root, store)
		if err := cfg.Docker().SetImage("ctgbot-codex:cpu"); err != nil {
			t.Fatalf("SetImage() error = %v", err)
		}
		registration := coremodel.Component{ID: modeluuid.New(), Type: Type, Name: "gpu"}
		factory := &captureBindFactory{home: runtimepkg.Home{Path: homePath}}

		value, err := New(context.Background(), registration, factory, factory.home, repository.NewMemory(), cfg, func(context.Context, coremodel.Chat) (string, error) {
			return filepath.Join(root, "workspace"), nil
		}, log.New(io.Discard, "", 0), "")
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}
		componentValue, ok := value.(*Component)
		if !ok {
			t.Fatalf("New() component = %#v", value)
		}
		if got, want := factory.config.Image, "ctgbot-codex:gpu"; got != want {
			t.Fatalf("bind image = %q, want %q", got, want)
		}
		if got, want := factory.config.GPUs, "all"; got != want {
			t.Fatalf("bind gpus = %q, want %q", got, want)
		}
		settings, err := componentValue.resolveThreadSettings(context.Background(), &coremodel.Thread{})
		if err != nil {
			t.Fatalf("resolveThreadSettings() error = %v", err)
		}
		if got, want := settings.Model, "gpt-5.5"; got != want {
			t.Fatalf("settings.Model = %q, want %q", got, want)
		}
		if got, want := settings.ReasoningEffort, "high"; got != want {
			t.Fatalf("settings.ReasoningEffort = %q, want %q", got, want)
		}
		if got, want := settings.SandboxMode, "workspace-write"; got != want {
			t.Fatalf("settings.SandboxMode = %q, want %q", got, want)
		}
		if got, want := componentValue.componentConfig.SandboxMode, "workspace-write"; got != want {
			t.Fatalf("componentConfig.SandboxMode = %q, want %q", got, want)
		}
		if got, want := factory.config.Env[0], "FOO=bar"; got != want {
			t.Fatalf("first env = %q, want %q", got, want)
		}
	})
}

func TestNewFallsBackToGlobalDockerImageWhenProfileImageUnset(t *testing.T) {
	withTempCwd(t, func(root string) {
		homePath := filepath.Join(root, ".ctgbot", "components", "codex", "work")
		if err := os.MkdirAll(homePath, 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(homePath, runtimepkg.ConfigFilename), []byte(`{"gpus":"all"}`), 0o644); err != nil {
			t.Fatalf("WriteFile(runtime.json) error = %v", err)
		}

		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewCwd() error = %v", err)
		}
		cfg := appstate.New(root, store)
		if err := cfg.Docker().SetImage("ctgbot-codex:gpu"); err != nil {
			t.Fatalf("SetImage() error = %v", err)
		}
		registration := coremodel.Component{ID: modeluuid.New(), Type: Type, Name: "work"}
		factory := &captureBindFactory{home: runtimepkg.Home{Path: homePath}}

		if _, err := New(context.Background(), registration, factory, factory.home, repository.NewMemory(), cfg, func(context.Context, coremodel.Chat) (string, error) {
			return filepath.Join(root, "workspace"), nil
		}, log.New(io.Discard, "", 0), ""); err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if got, want := factory.config.Image, "ctgbot-codex:gpu"; got != want {
			t.Fatalf("bind image = %q, want %q", got, want)
		}
		if got, want := factory.config.GPUs, "all"; got != want {
			t.Fatalf("bind gpus = %q, want %q", got, want)
		}
	})
}

func TestNewPrefersExplicitImageOverrideOverProfileAndGlobal(t *testing.T) {
	withTempCwd(t, func(root string) {
		homePath := filepath.Join(root, ".ctgbot", "components", "codex", "override")
		if err := os.MkdirAll(homePath, 0o755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		if err := os.WriteFile(filepath.Join(homePath, runtimepkg.ConfigFilename), []byte(`{"image":"ctgbot-codex:gpu"}`), 0o644); err != nil {
			t.Fatalf("WriteFile(runtime.json) error = %v", err)
		}

		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewCwd() error = %v", err)
		}
		cfg := appstate.New(root, store)
		if err := cfg.Docker().SetImage("ctgbot-codex:cpu"); err != nil {
			t.Fatalf("SetImage() error = %v", err)
		}
		registration := coremodel.Component{ID: modeluuid.New(), Type: Type, Name: "override"}
		factory := &captureBindFactory{home: runtimepkg.Home{Path: homePath}}

		if _, err := New(context.Background(), registration, factory, factory.home, repository.NewMemory(), cfg, func(context.Context, coremodel.Chat) (string, error) {
			return filepath.Join(root, "workspace"), nil
		}, log.New(io.Discard, "", 0), "explicit:image"); err != nil {
			t.Fatalf("New() error = %v", err)
		}
		if got, want := factory.config.Image, "explicit:image"; got != want {
			t.Fatalf("bind image = %q, want %q", got, want)
		}
	})
}
