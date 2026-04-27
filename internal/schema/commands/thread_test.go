package commands

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestThreadCommandAliasesParseToSameCommandType(t *testing.T) {
	router, err := commandengine.NewRouter(ThreadCommands(), commandengine.SourceMessage)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	base := commandengine.Request{Context: commandengine.Context{
		Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleUser}},
	}}

	refresh, err := router.Parse(context.Background(), base, []string{"refresh"})
	if err != nil {
		t.Fatalf("parse refresh: %v", err)
	}
	containerRefresh, err := router.Parse(context.Background(), base, []string{"container", "refresh"})
	if err != nil {
		t.Fatalf("parse container refresh: %v", err)
	}
	containerStart, err := router.Parse(context.Background(), base, []string{"container", "start"})
	if err != nil {
		t.Fatalf("parse container start: %v", err)
	}
	containerStop, err := router.Parse(context.Background(), base, []string{"container", "stop"})
	if err != nil {
		t.Fatalf("parse container stop: %v", err)
	}
	if _, ok := refresh.Command.(RefreshContainer); !ok {
		t.Fatalf("refresh command = %T, want RefreshContainer", refresh.Command)
	}
	if _, ok := containerRefresh.Command.(RefreshContainer); !ok {
		t.Fatalf("container refresh command = %T, want RefreshContainer", containerRefresh.Command)
	}
	if _, ok := containerStart.Command.(StartContainer); !ok {
		t.Fatalf("container start command = %T, want StartContainer", containerStart.Command)
	}
	if _, ok := containerStop.Command.(StopContainer); !ok {
		t.Fatalf("container stop command = %T, want StopContainer", containerStop.Command)
	}
}
