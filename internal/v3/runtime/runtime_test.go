package runtime

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/v3/component"
	"github.com/bartdeboer/ctgbot/internal/v3/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v3/homes"
	"github.com/bartdeboer/ctgbot/internal/v3/repository"
)

type fakeFactory struct{}

func (fakeFactory) Type() string { return "telegram" }

func (fakeFactory) Create(ctx context.Context, req component.CreateRequest) (component.Component, error) {
	_ = ctx
	return fakeResolved{componentType: req.Registration.Type}, nil
}

type fakeResolved struct{ componentType string }

func (f fakeResolved) Type() string { return f.componentType }

func TestResolveComponentUsesRegisteredHomeConvention(t *testing.T) {
	root := t.TempDir()
	storage := repository.NewMemory()
	registry := component.NewRegistry(fakeFactory{})
	rt := New(storage, registry, homes.New(root))

	registration := &coremodel.Component{
		Type:      "telegram",
		Name:      "telegram",
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
	if instance.Home.HostPath == "" || instance.Home.ContainerPath == "" {
		t.Fatalf("home = %#v", instance.Home)
	}
}
