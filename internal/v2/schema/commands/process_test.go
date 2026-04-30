package commands

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestProcessCommandsParseRootMessageCommands(t *testing.T) {
	t.Parallel()

	router := newProcessRouter(t, commandengine.SourceMessage)
	req, err := router.Parse(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleRoot}},
		},
	}, []string{"install"})
	if err != nil {
		t.Fatalf("parse install: %v", err)
	}
	if _, ok := req.Command.(Install); !ok {
		t.Fatalf("command = %T, want Install", req.Command)
	}
	if req.DefinitionID != "process.install" || req.Route != "install" {
		t.Fatalf("unexpected command request: %#v", req)
	}

	req, err = router.Parse(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleRoot}},
		},
	}, []string{"quit"})
	if err != nil {
		t.Fatalf("parse quit: %v", err)
	}
	if _, ok := req.Command.(Quit); !ok {
		t.Fatalf("command = %T, want Quit", req.Command)
	}
}

func TestProcessCommandsAreAvailableToHostbridge(t *testing.T) {
	t.Parallel()

	router := newProcessRouter(t, commandengine.SourceHostbridge)
	req, err := router.Parse(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleRoot}},
		},
	}, []string{"quit"})
	if err != nil {
		t.Fatalf("parse hostbridge quit: %v", err)
	}
	if _, ok := req.Command.(Quit); !ok {
		t.Fatalf("command = %T, want Quit", req.Command)
	}
}

func TestProcessCommandsRequireRoot(t *testing.T) {
	t.Parallel()

	router := newProcessRouter(t, commandengine.SourceMessage)
	_, err := router.Parse(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleUser}},
		},
	}, []string{"quit"})
	if err == nil {
		t.Fatal("expected non-root actor to be denied")
	}
}

func newProcessRouter(t *testing.T, source commandengine.Source) *commandengine.Router {
	t.Helper()

	router, err := commandengine.NewRouter(ProcessCommands(), source)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	return router
}
