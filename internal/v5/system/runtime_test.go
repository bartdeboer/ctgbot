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
	home    v5runtime.Home
	profile v5runtime.Profile
	rootDir string
}

func (r fakeRuntime) Kind() string               { return r.profile.Runtime }
func (r fakeRuntime) Profile() v5runtime.Profile { return r.profile }
func (r fakeRuntime) ComponentHome() v5runtime.Home {
	return r.home
}
func (r fakeRuntime) ThreadWorkspace(threadID modeluuid.UUID) (string, string, error) {
	return filepath.Join(r.rootDir, ".ctgbot", "threads", threadID.String(), "workspace"), "/workspace", nil
}

func (r fakeRuntime) Exec(ctx context.Context, threadID modeluuid.UUID, commands commandengine.CommandExecutor, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	_, _, _, _, _, _, _ = ctx, threadID, commands, stdout, stderr, name, args
	return fmt.Errorf("not implemented")
}

func (r fakeRuntime) CombinedOutput(ctx context.Context, threadID modeluuid.UUID, commands commandengine.CommandExecutor, name string, args ...string) ([]byte, error) {
	_, _, _, _, _ = ctx, threadID, commands, name, args
	return nil, fmt.Errorf("not implemented")
}

func (r fakeRuntime) OpenHTTPRelayPort(ctx context.Context, threadID modeluuid.UUID, commands commandengine.CommandExecutor, callbackPort int, callbackTimeout time.Duration) (func(context.Context) error, error) {
	_, _, _, _, _ = ctx, threadID, commands, callbackPort, callbackTimeout
	return nil, fmt.Errorf("not implemented")
}

type fakeFactory struct {
	profile v5runtime.Profile
	rootDir string
}

func (f fakeFactory) Kind() string               { return f.profile.Runtime }
func (f fakeFactory) Profile() v5runtime.Profile { return f.profile }
func (f fakeFactory) ComponentHome(registration coremodel.Component) v5runtime.Home {
	return v5runtime.Home{
		HostPath:      filepath.Join(f.profile.Root, "components", registration.Type, registration.Name),
		ContainerPath: "/profile/components/" + registration.Type + "/" + registration.Name,
	}
}
func (f fakeFactory) Bind(registration coremodel.Component, home v5runtime.Home, image string, env []string) v5runtime.Runtime {
	_, _, _ = registration, image, env
	return fakeRuntime{
		home:    home,
		profile: f.profile,
		rootDir: f.rootDir,
	}
}

type fakeResolved struct {
	componentType string
	profileName   string
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
		return fakeResolved{componentType: registration.Type, profileName: runtime.Profile().Name, runtimeKind: runtime.Kind()}, nil
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

	profiles := map[string]v5runtime.Profile{
		"default": {Name: "default", Runtime: "local", Root: filepath.Join(root, ".ctgbot", "profiles", "default")},
		"work":    {Name: "work", Runtime: "local", Root: filepath.Join(root, ".ctgbot", "profiles", "work")},
	}
	runtimes := map[string]v5runtime.Factory{
		"default": fakeFactory{profile: profiles["default"], rootDir: root},
		"work":    fakeFactory{profile: profiles["work"], rootDir: root},
	}
	system := New(storage, profiles, runtimes, registry)
	system.loaded = map[string]*component.Loaded{}
	return system
}

func TestResolveComponentUsesProfileRuntimeAndHome(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	system := newTestSystem(t, root, storage)

	registration := &coremodel.Component{
		Type:      "telegram",
		Name:      "telegram",
		Profile:   "default",
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
	if resolved.profileName != "default" {
		t.Fatalf("profile name = %q, want default", resolved.profileName)
	}
	if resolved.runtimeKind != "local" {
		t.Fatalf("runtime kind = %q, want local", resolved.runtimeKind)
	}
	if got, want := loaded.Home.HostPath, filepath.Join(root, ".ctgbot", "profiles", "default", "components", "telegram", "telegram"); got != want {
		t.Fatalf("HostPath = %q, want %q", got, want)
	}
}

func TestEnsureComponentPreservesExistingProfileWhenBlank(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	system := newTestSystem(t, root, storage)

	registration := &coremodel.Component{
		Type:    "telegram",
		Name:    "telegram",
		Profile: "work",
		Enabled: true,
	}
	if err := storage.Components().Save(context.Background(), registration); err != nil {
		t.Fatal(err)
	}

	updated, err := system.EnsureComponent(context.Background(), "telegram", "")
	if err != nil {
		t.Fatalf("EnsureComponent() error = %v", err)
	}
	if updated.Profile != "work" {
		t.Fatalf("Profile = %q, want work", updated.Profile)
	}
}

func TestAuthComponentUsesResolvedHomeAndRegistration(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	system := newTestSystem(t, root, storage)

	if _, err := system.EnsureComponent(context.Background(), "gmail/work", "work"); err != nil {
		t.Fatalf("EnsureComponent() error = %v", err)
	}
	if err := system.AuthComponent(context.Background(), "gmail/work", "", "busybox", 9090, 2*time.Minute, io.Discard, io.Discard); err != nil {
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
	if got, want := auth.last.home.HostPath, filepath.Join(root, ".ctgbot", "profiles", "work", "components", "gmail", "work"); got != want {
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
