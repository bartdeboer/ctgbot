package routers

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	v2commands "github.com/bartdeboer/ctgbot/internal/v2/schema/commands"
)

func TestRegisterProcessHandlersDispatchesCommands(t *testing.T) {
	t.Parallel()

	handlers := &fakeProcessHandlers{}
	registry := commandengine.NewRegistry()
	if err := RegisterProcessHandlers(registry, handlers); err != nil {
		t.Fatalf("register handlers: %v", err)
	}
	router, err := commandengine.NewRouter(v2commands.ProcessCommands(), commandengine.SourceMessage)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	engine := commandengine.NewEngine(router, registry)
	base := commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleRoot}},
		},
	}

	result, err := engine.Run(context.Background(), base, []string{"install"})
	if err != nil {
		t.Fatalf("run install: %v", err)
	}
	if handlers.calls != "install" || result.Text != "install" {
		t.Fatalf("unexpected install dispatch: calls=%q result=%#v", handlers.calls, result)
	}

	result, err = engine.Run(context.Background(), base, []string{"quit"})
	if err != nil {
		t.Fatalf("run quit: %v", err)
	}
	if handlers.calls != "quit" || result.Text != "quit" {
		t.Fatalf("unexpected quit dispatch: calls=%q result=%#v", handlers.calls, result)
	}
}

type fakeProcessHandlers struct {
	calls string
}

var _ ProcessHandlers = (*fakeProcessHandlers)(nil)

func (f *fakeProcessHandlers) Install(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	f.calls = "install"
	return commandengine.Result{Text: "install"}, nil
}

func (f *fakeProcessHandlers) Quit(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	f.calls = "quit"
	return commandengine.Result{Text: "quit"}, nil
}
