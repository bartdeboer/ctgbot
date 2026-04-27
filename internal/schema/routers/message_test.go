package routers_test

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/schema/routers"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestMessageCommandEngineRunsConfigAndThreadCommands(t *testing.T) {
	cfg := newTestConfig(t)
	manager := newConfigManager(t, cfg)
	threads := &fakeThreadHandlers{}
	engine, err := routers.NewMessageCommandEngine(manager, threads)
	if err != nil {
		t.Fatalf("NewMessageCommandEngine() error = %v", err)
	}
	req := commandengine.Request{Context: commandengine.Context{
		ChatID:   modeluuid.New(),
		ThreadID: modeluuid.New(),
		Actor:    commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleUser, simplerbac.RoleElevated}},
	}}

	configResult, err := engine.Run(context.Background(), req, []string{"config", "set", "chat.enabled", "true"})
	if err != nil {
		t.Fatalf("config set: %v", err)
	}
	if configResult.Text != "chat.enabled=true" {
		t.Fatalf("config result = %q, want chat.enabled=true", configResult.Text)
	}

	threadResult, err := engine.Run(context.Background(), req, []string{"container", "refresh"})
	if err != nil {
		t.Fatalf("container refresh: %v", err)
	}
	if threadResult.Text != "refresh" {
		t.Fatalf("thread result = %q, want refresh", threadResult.Text)
	}
	if threads.calls != "refresh" {
		t.Fatalf("thread calls = %q, want refresh", threads.calls)
	}
}

type fakeThreadHandlers struct {
	calls string
}

func (f *fakeThreadHandlers) RefreshContainer(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	f.calls = "refresh"
	return commandengine.Result{Text: "refresh"}, nil
}

func (f *fakeThreadHandlers) StartContainer(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	f.calls = "container start"
	return commandengine.Result{Text: "container start"}, nil
}

func (f *fakeThreadHandlers) StopContainer(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	f.calls = "container stop"
	return commandengine.Result{Text: "container stop"}, nil
}

func (f *fakeThreadHandlers) PurgeChat(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	f.calls = "purge"
	return commandengine.Result{Text: "purge"}, nil
}

func (f *fakeThreadHandlers) InterruptTurn(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	f.calls = "interrupt"
	return commandengine.Result{Text: "interrupt"}, nil
}

func (f *fakeThreadHandlers) Upgrade(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	f.calls = "upgrade"
	return commandengine.Result{Text: "upgrade"}, nil
}

func (f *fakeThreadHandlers) Quit(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	f.calls = "quit"
	return commandengine.Result{Text: "quit"}, nil
}

func (f *fakeThreadHandlers) Stop(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	f.calls = "stop"
	return commandengine.Result{Text: "stop"}, nil
}

func (f *fakeThreadHandlers) Status(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	f.calls = "status"
	return commandengine.Result{Text: "status"}, nil
}
