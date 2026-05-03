package runtime

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/v4/component"
	"github.com/bartdeboer/ctgbot/internal/v4/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v4/execution"
	"github.com/bartdeboer/ctgbot/internal/v4/homes"
	"github.com/bartdeboer/ctgbot/internal/v4/profiles"
	"github.com/bartdeboer/ctgbot/internal/v4/repository"
	"github.com/bartdeboer/ctgbot/internal/v4/workspaces"
	"github.com/bartdeboer/go-clistate"
)

type fakeAuthFactory struct {
	recorder *fakeAuthRecorder
}

func (f fakeAuthFactory) Type() string { return "gmail" }

func (f fakeAuthFactory) Create(ctx context.Context, req component.CreateRequest) (component.Component, error) {
	_ = ctx
	return fakeAuthComponent{req: req, recorder: f.recorder}, nil
}

type fakeAuthComponent struct {
	req      component.CreateRequest
	recorder *fakeAuthRecorder
}

func (c fakeAuthComponent) Type() string { return c.req.Registration.Type }

func (c fakeAuthComponent) Auth(ctx context.Context, req component.AuthRequest) error {
	_ = ctx
	c.recorder.calls++
	c.recorder.last = req
	return nil
}

type fakeAuthRecorder struct {
	calls int
	last  component.AuthRequest
}

func newTestProfileManager(t *testing.T, root string) *profiles.Manager {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore Chdir() error = %v", err)
		}
	})
	if err := os.MkdirAll(".ctgbot", 0o755); err != nil {
		t.Fatalf("MkdirAll(.ctgbot) error = %v", err)
	}
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("NewCwd() error = %v", err)
	}

	manager := profiles.New(root, store)
	for _, name := range []string{"default", "work"} {
		if err := manager.Set(name, profiles.Settings{Runtime: "local"}); err != nil {
			t.Fatalf("Set(%s) error = %v", name, err)
		}
	}
	return manager
}

func newTestRuntime(t *testing.T, root string, storage repository.Storage, registry *component.Registry) *Runtime {
	t.Helper()

	profileManager := newTestProfileManager(t, root)
	return New(
		storage,
		registry,
		profileManager,
		homes.New(profileManager),
		execution.NewResolver(execution.CreateRequest{
			Workspaces: workspaces.New(root),
		}, execution.LocalFactory{}),
	)
}

func TestEnsureComponentCreatesDefaultRegistrationAndHome(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	registry := component.NewRegistry(fakeFactory{})
	rt := newTestRuntime(t, root, storage, registry)

	componentRow, err := rt.EnsureComponent(context.Background(), "telegram", "work")
	if err != nil {
		t.Fatalf("EnsureComponent() error = %v", err)
	}
	if componentRow.Ref() != "telegram" {
		t.Fatalf("Ref() = %q", componentRow.Ref())
	}
	if !componentRow.IsDefault {
		t.Fatal("expected default component registration")
	}
	home, err := rt.Homes.Home(*componentRow)
	if err != nil {
		t.Fatalf("Home() error = %v", err)
	}
	if componentRow.Profile != "work" {
		t.Fatalf("Profile = %q, want work", componentRow.Profile)
	}
	if got, want := home.HostPath, filepath.Join(root, ".ctgbot", "profiles", "work", "components", "telegram", "telegram"); got != want {
		t.Fatalf("HostPath = %q, want %q", got, want)
	}
}

func TestResolveComponentRefPrefersDefaultRegistration(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	registry := component.NewRegistry(fakeFactory{})
	rt := newTestRuntime(t, root, storage, registry)

	defaultComp := &coremodel.Component{Type: "telegram", Name: "telegram", Profile: "default", Enabled: true, IsDefault: true}
	otherComp := &coremodel.Component{Type: "telegram", Name: "bot2", Profile: "work", Enabled: true}
	if err := storage.Components().Save(context.Background(), defaultComp); err != nil {
		t.Fatal(err)
	}
	if err := storage.Components().Save(context.Background(), otherComp); err != nil {
		t.Fatal(err)
	}

	componentRow, err := rt.ResolveComponentRef(context.Background(), "telegram")
	if err != nil {
		t.Fatalf("ResolveComponentRef() error = %v", err)
	}
	if componentRow.ID != defaultComp.ID {
		t.Fatalf("ResolveComponentRef() id = %s, want %s", componentRow.ID, defaultComp.ID)
	}
}

func TestBindChatComponentUpdatesExistingBinding(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	registry := component.NewRegistry(fakeFactory{})
	rt := newTestRuntime(t, root, storage, registry)

	chat := &coremodel.Chat{Label: "team", Enabled: true}
	if err := storage.Chats().Save(context.Background(), chat); err != nil {
		t.Fatal(err)
	}
	componentRow, err := rt.EnsureComponent(context.Background(), "telegram", "default")
	if err != nil {
		t.Fatal(err)
	}
	binding := &coremodel.ChatComponent{
		ChatID:         chat.ID,
		ComponentID:    componentRow.ID,
		Role:           coremodel.ChatComponentRoleSource,
		ExternalChatID: "old-chat",
		Enabled:        false,
	}
	if err := storage.ChatComponents().Save(context.Background(), binding); err != nil {
		t.Fatal(err)
	}

	got, err := rt.BindChatComponent(context.Background(), chat.ID, coremodel.ChatComponentRoleSource, "telegram", "new-chat")
	if err != nil {
		t.Fatalf("BindChatComponent() error = %v", err)
	}
	if got.ID != binding.ID {
		t.Fatalf("binding id = %s, want %s", got.ID, binding.ID)
	}
	if !got.Enabled || got.ExternalChatID != "new-chat" {
		t.Fatalf("binding = %#v", got)
	}
}

func TestAuthComponentEnsuresRegistrationAndCallsAuthenticator(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	recorder := &fakeAuthRecorder{}
	registry := component.NewRegistry(fakeAuthFactory{recorder: recorder})
	rt := newTestRuntime(t, root, storage, registry)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := rt.AuthComponent(context.Background(), "gmail/work", AuthOptions{
		Profile:         "work",
		Image:           "busybox",
		CallbackPort:    9090,
		CallbackTimeout: 2 * time.Minute,
		Stdout:          &stdout,
		Stderr:          &stderr,
	})
	if err != nil {
		t.Fatalf("AuthComponent() error = %v", err)
	}
	if recorder.calls != 1 {
		t.Fatalf("auth calls = %d, want 1", recorder.calls)
	}
	if got := recorder.last.Registration.Ref(); got != "gmail/work" {
		t.Fatalf("auth registration = %q", got)
	}
	if got, want := recorder.last.Home.HostPath, filepath.Join(root, ".ctgbot", "profiles", "work", "components", "gmail", "work"); got != want {
		t.Fatalf("auth host path = %q, want %q", got, want)
	}
	if recorder.last.Image != "busybox" || recorder.last.CallbackPort != 9090 || recorder.last.CallbackTimeout != 2*time.Minute {
		t.Fatalf("auth request = %#v", recorder.last)
	}
}
