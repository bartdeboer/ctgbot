package runtime

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/v4/component"
	"github.com/bartdeboer/ctgbot/internal/v4/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v4/execution"
	"github.com/bartdeboer/ctgbot/internal/v4/homes"
	"github.com/bartdeboer/ctgbot/internal/v4/repository"
	"github.com/bartdeboer/ctgbot/internal/v4/workspaces"
)

type fakeFactory struct{}

func (fakeFactory) Type() string { return "telegram" }

func (fakeFactory) Create(ctx context.Context, req component.CreateRequest) (component.Component, error) {
	_ = ctx
	driver := ""
	profileName := ""
	if req.Runtime != nil {
		driver = req.Runtime.Driver()
	}
	profileName = req.Profile.Name
	return fakeResolved{componentType: req.Registration.Type, runtimeDriver: driver, profileName: profileName}, nil
}

type fakeResolved struct {
	componentType string
	runtimeDriver string
	profileName   string
}

func (f fakeResolved) Type() string { return f.componentType }

func TestResolveComponentUsesRegisteredHomeConvention(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	registry := component.NewRegistry(fakeFactory{})
	profileManager := newTestProfileManager(t, root)
	rt := New(storage, registry, profileManager, homes.New(profileManager), execution.NewResolver(execution.CreateRequest{
		Workspaces: workspaces.New(root),
	}, execution.LocalFactory{}))

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

	instance, err := rt.ResolveComponent(context.Background(), registration.ID)
	if err != nil {
		t.Fatalf("ResolveComponent() error = %v", err)
	}
	if instance.Registration.Ref() != "telegram" {
		t.Fatalf("Ref() = %q", instance.Registration.Ref())
	}
	resolved, ok := instance.Implementation.(fakeResolved)
	if !ok {
		t.Fatalf("implementation = %#v", instance.Implementation)
	}
	if resolved.runtimeDriver != "local" {
		t.Fatalf("runtime driver = %q, want local", resolved.runtimeDriver)
	}
	if resolved.profileName != "default" {
		t.Fatalf("profile name = %q, want default", resolved.profileName)
	}
	if instance.Home.HostPath == "" || instance.Home.ContainerPath == "" {
		t.Fatalf("home = %#v", instance.Home)
	}
}
