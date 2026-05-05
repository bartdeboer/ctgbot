package system

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
)

type fakeRuntime struct {
	home v5runtime.Home
	kind string
}

func (r fakeRuntime) Kind() string { return r.kind }
func (r fakeRuntime) ComponentHome() v5runtime.Home {
	return r.home
}
func (r fakeRuntime) RuntimeComponentHomePath() string {
	return r.home.Path
}
func (r fakeRuntime) RuntimeWorkspacePath(workspacePath string) string {
	return workspacePath
}
func (r fakeRuntime) Exec(ctx context.Context, workspacePath string, threadID modeluuid.UUID, commands commandengine.CommandExecutor, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	_, _, _, _, _, _, _, _, _ = ctx, workspacePath, threadID, commands, stdout, stderr, name, args, r.kind
	return fmt.Errorf("not implemented")
}
func (r fakeRuntime) CombinedOutput(ctx context.Context, workspacePath string, threadID modeluuid.UUID, commands commandengine.CommandExecutor, name string, args ...string) ([]byte, error) {
	_, _, _, _, _, _, _ = ctx, workspacePath, threadID, commands, name, args, r.kind
	return nil, fmt.Errorf("not implemented")
}
func (r fakeRuntime) OpenHTTPRelayPort(ctx context.Context, workspacePath string, threadID modeluuid.UUID, commands commandengine.CommandExecutor, callbackPort int, callbackTimeout time.Duration) (func(context.Context) error, error) {
	_, _, _, _, _, _, _ = ctx, workspacePath, threadID, commands, callbackPort, callbackTimeout, r.kind
	return nil, fmt.Errorf("not implemented")
}

type fakeFactory struct {
	kind           string
	componentsRoot string
}

func (f fakeFactory) Kind() string { return f.kind }
func (f fakeFactory) ComponentHome(registration coremodel.Component) v5runtime.Home {
	hostPath := registration.HomePath
	if hostPath == "" {
		hostPath = filepath.Join(f.componentsRoot, registration.Type, registration.Name)
	}
	return v5runtime.Home{Path: hostPath}
}
func (f fakeFactory) RuntimeComponentHomePath(registration coremodel.Component, home v5runtime.Home) string {
	_, _ = registration, home
	return home.Path
}
func (f fakeFactory) RuntimeWorkspacePath(workspacePath string) string {
	return workspacePath
}
func (f fakeFactory) Bind(registration coremodel.Component, home v5runtime.Home, image string, env []string) v5runtime.Runtime {
	_, _, _ = registration, image, env
	return fakeRuntime{home: home, kind: f.kind}
}

type fakeResolved struct {
	componentType string
	runtimeKind   string
}

func (f fakeResolved) Type() string { return f.componentType }

type fakeAuthenticator struct {
	calls int
	last  struct {
		registration coremodel.Component
		home         v5runtime.Home
		callbackPort int
		timeout      time.Duration
	}
}

func (f *fakeAuthenticator) Type() string { return "gmail" }
func (f *fakeAuthenticator) Auth(ctx context.Context, callbackPort int, callbackTimeout time.Duration, stdout io.Writer, stderr io.Writer) error {
	_, _, _ = ctx, stdout, stderr
	f.calls++
	f.last.callbackPort = callbackPort
	f.last.timeout = callbackTimeout
	return nil
}

func newTestSystem(t *testing.T, root string, storage repository.Storage) *System {
	t.Helper()

	registry := component.NewRegistry()
	if err := registry.Add("telegram", func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
		_, _, _ = ctx, home, storage
		return fakeResolved{componentType: registration.Type, runtimeKind: runtime.Kind()}, nil
	}); err != nil {
		t.Fatal(err)
	}
	auth := &fakeAuthenticator{}
	if err := registry.Add("gmail", func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
		_, _, _, _ = ctx, runtime, home, storage
		auth.last.registration = registration
		auth.last.home = home
		return auth, nil
	}); err != nil {
		t.Fatal(err)
	}

	workspaces := map[string]Workspace{
		"work": {Name: "work", Path: filepath.Join(root, "worktree")},
	}
	runtimes := map[string]v5runtime.Factory{
		"docker": fakeFactory{kind: "docker", componentsRoot: filepath.Join(root, ".ctgbot", "components")},
		"local":  fakeFactory{kind: "local", componentsRoot: filepath.Join(root, ".ctgbot", "components")},
	}
	system := New(storage, workspaces, runtimes, registry)
	system.StateRoot = filepath.Join(root, ".ctgbot")
	system.loaded = map[string]*component.Loaded{}
	return system
}

func TestResolveComponentUsesRuntimeAndHome(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	system := newTestSystem(t, root, storage)

	registration := &coremodel.Component{
		Type:      "telegram",
		Name:      "telegram",
		Runtime:   "local",
		Enabled:   true,
		IsDefault: true,
	}
	if err := storage.Components().Save(context.Background(), registration); err != nil {
		t.Fatal(err)
	}

	loaded, err := system.ResolveComponent(context.Background(), registration.ID)
	if err != nil {
		t.Fatalf("ResolveComponent() error = %v", err)
	}
	resolved, ok := loaded.Component.(fakeResolved)
	if !ok {
		t.Fatalf("resolved component = %#v", loaded.Component)
	}
	if resolved.runtimeKind != "local" {
		t.Fatalf("runtime kind = %q, want local", resolved.runtimeKind)
	}
	if got, want := loaded.Home.Path, filepath.Join(root, ".ctgbot", "components", "telegram", "telegram"); got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestEnsureComponentPreservesExistingRuntimeWhenBlank(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	system := newTestSystem(t, root, storage)

	registration := &coremodel.Component{
		Type:    "telegram",
		Name:    "telegram",
		Runtime: "local",
		Enabled: true,
	}
	if err := storage.Components().Save(context.Background(), registration); err != nil {
		t.Fatal(err)
	}

	updated, err := system.EnsureComponent(context.Background(), "telegram", "", "")
	if err != nil {
		t.Fatalf("EnsureComponent() error = %v", err)
	}
	if updated.Runtime != "local" {
		t.Fatalf("Runtime = %q, want local", updated.Runtime)
	}
}

func TestResolveChatWorkspaceUsesConfiguredWorkspace(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	system := newTestSystem(t, root, storage)
	chat := coremodel.Chat{ID: modeluuid.New(), Label: "work", Workspace: "work", Enabled: true}

	workspace, err := system.ResolveChatWorkspace(context.Background(), chat)
	if err != nil {
		t.Fatalf("ResolveChatWorkspace() error = %v", err)
	}
	if got, want := workspace, filepath.Join(root, "worktree"); got != want {
		t.Fatalf("workspace = %q, want %q", got, want)
	}
}

func TestResolveChatWorkspaceFallsBackToChatLocalWorkspace(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	system := newTestSystem(t, root, storage)
	chat := coremodel.Chat{ID: modeluuid.New(), Label: "scratch", Enabled: true}

	workspace, err := system.ResolveChatWorkspace(context.Background(), chat)
	if err != nil {
		t.Fatalf("ResolveChatWorkspace() error = %v", err)
	}
	if got, want := workspace, filepath.Join(root, ".ctgbot", "chats", chat.ID.String(), "workspace"); got != want {
		t.Fatalf("workspace = %q, want %q", got, want)
	}
}

func TestAuthComponentUsesResolvedHomeAndRegistration(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	system := newTestSystem(t, root, storage)

	if _, err := system.EnsureComponent(context.Background(), "gmail/work", "local", ""); err != nil {
		t.Fatalf("EnsureComponent() error = %v", err)
	}
	if err := system.AuthComponent(context.Background(), "gmail/work", "", "", 9090, 2*time.Minute, io.Discard, io.Discard); err != nil {
		t.Fatalf("AuthComponent() error = %v", err)
	}

	loaded, err := system.ResolveComponent(context.Background(), mustResolveComponentID(t, storage, "gmail", "work"))
	if err != nil {
		t.Fatalf("ResolveComponent() error = %v", err)
	}
	auth, ok := loaded.Component.(*fakeAuthenticator)
	if !ok {
		t.Fatalf("resolved component = %#v", loaded.Component)
	}
	if auth.calls != 1 {
		t.Fatalf("auth calls = %d, want 1", auth.calls)
	}
	if auth.last.registration.Ref() != "gmail/work" {
		t.Fatalf("auth registration = %q", auth.last.registration.Ref())
	}
	if got, want := auth.last.home.Path, filepath.Join(root, ".ctgbot", "components", "gmail", "work"); got != want {
		t.Fatalf("auth home = %q, want %q", got, want)
	}
}

func mustResolveComponentID(t *testing.T, storage repository.Storage, componentType string, name string) modeluuid.UUID {
	t.Helper()
	registration, err := storage.Components().GetByTypeAndName(context.Background(), componentType, name)
	if err != nil {
		t.Fatalf("GetByTypeAndName() error = %v", err)
	}
	if registration == nil {
		t.Fatalf("component not found: %s/%s", componentType, name)
	}
	return registration.ID
}
