package runtime

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/v3/component"
	"github.com/bartdeboer/ctgbot/internal/v3/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v3/homes"
	"github.com/bartdeboer/ctgbot/internal/v3/repository"
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

type fakeSandboxManager struct{}

func (fakeSandboxManager) CreateSandbox(spec *sandboxengine.SandboxSpec) *sandboxengine.Sandbox {
	_ = spec
	return nil
}

func TestEnsureComponentCreatesDefaultRegistrationAndHome(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	registry := component.NewRegistry(fakeFactory{})
	rt := New(storage, registry, homes.New(root))

	componentRow, err := rt.EnsureComponent(context.Background(), "telegram")
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
	if got, want := home.HostPath, filepath.Join(root, ".ctgbot", "components", "telegram", "telegram"); got != want {
		t.Fatalf("HostPath = %q, want %q", got, want)
	}
}

func TestResolveComponentRefPrefersDefaultRegistration(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	registry := component.NewRegistry(fakeFactory{})
	rt := New(storage, registry, homes.New(root))

	defaultComp := &coremodel.Component{Type: "telegram", Name: "telegram", Enabled: true, IsDefault: true}
	otherComp := &coremodel.Component{Type: "telegram", Name: "bot2", Enabled: true}
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
	rt := New(storage, registry, homes.New(root))

	chat := &coremodel.Chat{Label: "team", Enabled: true}
	if err := storage.Chats().Save(context.Background(), chat); err != nil {
		t.Fatal(err)
	}
	componentRow, err := rt.EnsureComponent(context.Background(), "telegram")
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
	rt := New(storage, registry, homes.New(root))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := rt.AuthComponent(context.Background(), "gmail/work", AuthOptions{
		Image:           "busybox",
		CallbackPort:    9090,
		CallbackTimeout: 2 * time.Minute,
		SandboxManager:  fakeSandboxManager{},
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
	if got, want := recorder.last.Home.HostPath, filepath.Join(root, ".ctgbot", "components", "gmail", "work"); got != want {
		t.Fatalf("auth host path = %q, want %q", got, want)
	}
	if recorder.last.Image != "busybox" || recorder.last.CallbackPort != 9090 || recorder.last.CallbackTimeout != 2*time.Minute {
		t.Fatalf("auth request = %#v", recorder.last)
	}
}
