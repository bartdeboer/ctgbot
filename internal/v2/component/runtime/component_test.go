package runtime

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/ctgbot/internal/v2/component"
)

func TestComponentExposesRuntimeCommandSurface(t *testing.T) {
	t.Parallel()

	actions := &fakeActions{}
	runtimeComponent := New(actions)
	engine := newTestEngine(t, runtimeComponent, commandengine.SourceMessage)
	base := commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleRoot}},
		},
	}

	result, err := engine.Run(context.Background(), base, []string{"install"})
	if err != nil {
		t.Fatalf("run install: %v", err)
	}
	if actions.installs != 1 || result.Text != "install completed\ntype /quit to restart" {
		t.Fatalf("unexpected install result: actions=%#v result=%#v", actions, result)
	}

	result, err = engine.Run(context.Background(), base, []string{"quit"})
	if err != nil {
		t.Fatalf("run quit: %v", err)
	}
	if actions.quits != 1 || result.Text != "quit requested" {
		t.Fatalf("unexpected quit result: actions=%#v result=%#v", actions, result)
	}
}

func TestComponentRequiresRuntimeActions(t *testing.T) {
	t.Parallel()

	engine := newTestEngine(t, New(nil), commandengine.SourceMessage)
	_, err := engine.Run(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleRoot}},
		},
	}, []string{"quit"})
	if err == nil {
		t.Fatal("expected missing runtime actions error")
	}
}

func TestComponentCommandsRequireRoot(t *testing.T) {
	t.Parallel()

	engine := newTestEngine(t, New(&fakeActions{}), commandengine.SourceMessage)
	_, err := engine.Run(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleUser}},
		},
	}, []string{"quit"})
	if err == nil {
		t.Fatal("expected non-root actor to be denied")
	}
}

func TestComponentCommandsAreAvailableToHostbridge(t *testing.T) {
	t.Parallel()

	actions := &fakeActions{}
	engine := newTestEngine(t, New(actions), commandengine.SourceHostbridge)
	_, err := engine.Run(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleRoot}},
		},
	}, []string{"quit"})
	if err != nil {
		t.Fatalf("run hostbridge quit: %v", err)
	}
	if actions.quits != 1 {
		t.Fatalf("expected quit action, got %#v", actions)
	}
}

func newTestEngine(t *testing.T, surface component.CommandSurface, source commandengine.Source) *commandengine.Engine {
	t.Helper()

	router, err := commandengine.NewRouter(surface.CommandDefinitions(), source)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	registry := commandengine.NewRegistry()
	if err := surface.RegisterCommandHandlers(registry); err != nil {
		t.Fatalf("register handlers: %v", err)
	}
	return commandengine.NewEngine(router, registry)
}

type fakeActions struct {
	installs int
	quits    int
}

var _ Actions = (*fakeActions)(nil)

func (f *fakeActions) Install(ctx context.Context) error {
	f.installs++
	return nil
}

func (f *fakeActions) Quit(ctx context.Context) error {
	f.quits++
	return nil
}
