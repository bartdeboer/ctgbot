package routers

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	v2commands "github.com/bartdeboer/ctgbot/internal/v2/schema/commands"
)

func TestRegisterChatHandlersDispatchesCommands(t *testing.T) {
	t.Parallel()

	handlers := &fakeChatHandlers{}
	registry := commandengine.NewRegistry()
	if err := RegisterChatHandlers(registry, handlers); err != nil {
		t.Fatalf("register handlers: %v", err)
	}
	router, err := commandengine.NewRouter(v2commands.ChatCommands(), commandengine.SourceCLI)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	engine := commandengine.NewEngine(router, registry)
	base := commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleRoot}},
		},
	}

	result, err := engine.Run(context.Background(), base, []string{"chat", "list-unregistered"})
	if err != nil {
		t.Fatalf("run list: %v", err)
	}
	if handlers.calls != "list" || result.Text != "list" {
		t.Fatalf("unexpected list dispatch: calls=%q result=%#v", handlers.calls, result)
	}

	result, err = engine.Run(context.Background(), base, []string{"chat", "00abc", "preset", "telegram-codex"})
	if err != nil {
		t.Fatalf("run preset: %v", err)
	}
	if handlers.calls != "preset:00abc:telegram-codex" || result.Text != "preset" {
		t.Fatalf("unexpected preset dispatch: calls=%q result=%#v", handlers.calls, result)
	}
}

type fakeChatHandlers struct {
	calls string
}

var _ ChatHandlers = (*fakeChatHandlers)(nil)

func (f *fakeChatHandlers) ListUnregisteredChats(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	f.calls = "list"
	return commandengine.Result{Text: "list"}, nil
}

func (f *fakeChatHandlers) ApplyChatPreset(ctx context.Context, req commandengine.Request, cmd v2commands.ChatApplyPreset) (commandengine.Result, error) {
	f.calls = "preset:" + cmd.ChatID + ":" + cmd.Preset
	return commandengine.Result{Text: "preset"}, nil
}
