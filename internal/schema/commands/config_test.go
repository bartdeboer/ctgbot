package commands

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestConfigHostbridgeScaffoldIsCLIOnly(t *testing.T) {
	base := commandengine.Request{Context: commandengine.Context{
		Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleRoot}},
	}}
	cli, err := commandengine.NewRouter(ConfigCommands(), commandengine.SourceCLI)
	if err != nil {
		t.Fatalf("NewRouter(cli) error = %v", err)
	}
	req, err := cli.Parse(context.Background(), base, []string{"config", "hostbridge", "scaffold", "deploy"})
	if err != nil {
		t.Fatalf("Parse(cli scaffold) error = %v", err)
	}
	if _, ok := req.Command.(ConfigHostbridgeScaffold); !ok {
		t.Fatalf("command = %T, want ConfigHostbridgeScaffold", req.Command)
	}

	for _, source := range []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge} {
		router, err := commandengine.NewRouter(ConfigCommands(), source)
		if err != nil {
			t.Fatalf("NewRouter(%s) error = %v", source, err)
		}
		if _, err := router.Parse(context.Background(), base, []string{"config", "hostbridge", "scaffold", "deploy"}); err == nil {
			t.Fatalf("Parse(%s scaffold) succeeded, want source rejection", source)
		}
	}
}
