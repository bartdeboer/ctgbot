package runtime

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
)

type fakeRuntime struct {
	profile component.Profile
	rootDir string
}

func (r fakeRuntime) Kind() string               { return r.profile.Runtime }
func (r fakeRuntime) Profile() component.Profile { return r.profile }
func (r fakeRuntime) ComponentHome(registration coremodel.Component) component.Home {
	return component.Home{
		HostPath:      filepath.Join(r.profile.Root, "components", registration.Type, registration.Name),
		ContainerPath: "/profile/components/" + registration.Type + "/" + registration.Name,
	}
}
func (r fakeRuntime) ThreadWorkspace(threadID modeluuid.UUID) (string, string, error) {
	return filepath.Join(r.rootDir, ".ctgbot", "threads", threadID.String(), "workspace"), "/workspace", nil
}
func (r fakeRuntime) StartAuth(ctx context.Context, registration coremodel.Component, home component.Home, image string, workdir string, env []string) (*sandboxengine.Sandbox, error) {
	_, _, _, _, _, _ = ctx, registration, home, image, workdir, env
	return nil, fmt.Errorf("not implemented")
}
func (r fakeRuntime) StartTurn(ctx context.Context, registration coremodel.Component, thread coremodel.Thread, home component.Home, image string, workdir string, env []string, developerInstructions string, commands commandengine.CommandExecutor) (*sandboxengine.SandboxRuntime, error) {
	_, _, _, _, _, _, _, _, _ = ctx, registration, thread, home, image, workdir, env, developerInstructions, commands
	return nil, fmt.Errorf("not implemented")
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
		home         component.Home
		image        string
		callbackPort int
		timeout      time.Duration
	}
}

func (f *fakeAuthenticator) Type() string { return "gmail" }
func (f *fakeAuthenticator) Auth(ctx context.Context, registration coremodel.Component, home component.Home, image string, callbackPort int, callbackTimeout time.Duration, stdout io.Writer, stderr io.Writer) error {
	_, _, _ = ctx, stdout, stderr
	f.calls++
	f.last.registration = registration
	f.last.home = home
	f.last.image = image
	f.last.callbackPort = callbackPort
	f.last.timeout = callbackTimeout
	return nil
}

func newTestSystem(t *testing.T, root string, storage repository.Storage) *System {
	t.Helper()

	registry := component.NewRegistry()
	if err := registry.Add("telegram", func(ctx context.Context, registration coremodel.Component, profile component.Profile, runtime component.Runtime, home component.Home, storage repository.Storage) (component.Component, error) {
		_, _, _, _ = ctx, home, runtime, storage
		return fakeResolved{componentType: registration.Type, profileName: profile.Name, runtimeKind: runtime.Kind()}, nil
	}); err != nil {
		t.Fatal(err)
	}
	auth := &fakeAuthenticator{}
	if err := registry.Add("gmail", func(ctx context.Context, registration coremodel.Component, profile component.Profile, runtime component.Runtime, home component.Home, storage repository.Storage) (component.Component, error) {
		_, _, _, _, _, _ = ctx, registration, profile, runtime, home, storage
		return auth, nil
	}); err != nil {
		t.Fatal(err)
	}

	profiles := map[string]component.Profile{
		"default": {Name: "default", Runtime: "local", Root: filepath.Join(root, ".ctgbot", "profiles", "default")},
		"work":    {Name: "work", Runtime: "local", Root: filepath.Join(root, ".ctgbot", "profiles", "work")},
	}
	runtimes := map[string]component.Runtime{
		"default": fakeRuntime{profile: profiles["default"], rootDir: root},
		"work":    fakeRuntime{profile: profiles["work"], rootDir: root},
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
