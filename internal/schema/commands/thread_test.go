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
	install, err := router.Parse(context.Background(), base, []string{"install"})
	if err != nil {
		t.Fatalf("parse install: %v", err)
	}
	modelStatus, err := router.Parse(context.Background(), base, []string{"model"})
	if err != nil {
		t.Fatalf("parse model: %v", err)
	}
	modelList, err := router.Parse(context.Background(), base, []string{"model", "list"})
	if err != nil {
		t.Fatalf("parse model list: %v", err)
	}
	modelSet, err := router.Parse(context.Background(), base, []string{"model", "set", "gpt-test"})
	if err != nil {
		t.Fatalf("parse model set: %v", err)
	}
	modelClear, err := router.Parse(context.Background(), base, []string{"model", "clear"})
	if err != nil {
		t.Fatalf("parse model clear: %v", err)
	}
	effortStatus, err := router.Parse(context.Background(), base, []string{"model", "effort"})
	if err != nil {
		t.Fatalf("parse model effort: %v", err)
	}
	effortList, err := router.Parse(context.Background(), base, []string{"model", "effort", "list"})
	if err != nil {
		t.Fatalf("parse model effort list: %v", err)
	}
	effortSet, err := router.Parse(context.Background(), base, []string{"model", "effort", "set", "high"})
	if err != nil {
		t.Fatalf("parse model effort set: %v", err)
	}
	effortClear, err := router.Parse(context.Background(), base, []string{"model", "effort", "clear"})
	if err != nil {
		t.Fatalf("parse model effort clear: %v", err)
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
	if _, ok := install.Command.(Install); !ok {
		t.Fatalf("install command = %T, want Install", install.Command)
	}
	if _, ok := modelStatus.Command.(ModelStatus); !ok {
		t.Fatalf("model command = %T, want ModelStatus", modelStatus.Command)
	}
	if _, ok := modelList.Command.(ModelList); !ok {
		t.Fatalf("model list command = %T, want ModelList", modelList.Command)
	}
	if cmd, ok := modelSet.Command.(ModelSet); !ok || cmd.Model != "gpt-test" {
		t.Fatalf("model set command = %#v, want ModelSet{Model:gpt-test}", modelSet.Command)
	}
	if _, ok := modelClear.Command.(ModelClear); !ok {
		t.Fatalf("model clear command = %T, want ModelClear", modelClear.Command)
	}
	if _, ok := effortStatus.Command.(ModelEffortStatus); !ok {
		t.Fatalf("model effort command = %T, want ModelEffortStatus", effortStatus.Command)
	}
	if _, ok := effortList.Command.(ModelEffortList); !ok {
		t.Fatalf("model effort list command = %T, want ModelEffortList", effortList.Command)
	}
	if cmd, ok := effortSet.Command.(ModelEffortSet); !ok || cmd.Effort != "high" {
		t.Fatalf("model effort set command = %#v, want ModelEffortSet{Effort:high}", effortSet.Command)
	}
	if _, ok := effortClear.Command.(ModelEffortClear); !ok {
		t.Fatalf("model effort clear command = %T, want ModelEffortClear", effortClear.Command)
	}
	if _, err := router.Parse(context.Background(), base, []string{"stop"}); err == nil {
		t.Fatal("expected stop command to be unregistered")
	}
}
