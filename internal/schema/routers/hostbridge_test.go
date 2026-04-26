package routers_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
	"github.com/bartdeboer/ctgbot/internal/schema/routers"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestHostbridgeRouterIncludesConfigAndHostbridgeCommands(t *testing.T) {
	router, err := routers.NewHostbridgeRouter()
	if err != nil {
		t.Fatalf("NewHostbridgeRouter() error = %v", err)
	}
	base := commandengine.Request{Context: commandengine.Context{
		Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
	}}

	configReq, err := router.Parse(context.Background(), base, []string{"config", "get", "docker.image"})
	if err != nil {
		t.Fatalf("parse config get: %v", err)
	}
	if _, ok := configReq.Command.(schemacommands.ConfigGet); !ok {
		t.Fatalf("config command = %T, want ConfigGet", configReq.Command)
	}
	if _, err := router.Parse(context.Background(), base, []string{"config", "hostbridge", "scaffold", "deploy"}); err == nil {
		t.Fatal("parse hostbridge scaffold succeeded from hostbridge source, want rejection")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	mediaReq, err := router.Parse(context.Background(), base, []string{"sendfile", path})
	if err != nil {
		t.Fatalf("parse sendfile: %v", err)
	}
	if _, ok := mediaReq.Command.(schemacommands.SendMedia); !ok {
		t.Fatalf("media command = %T, want SendMedia", mediaReq.Command)
	}

	statusReq, err := router.Parse(context.Background(), base, []string{"status"})
	if err != nil {
		t.Fatalf("parse status: %v", err)
	}
	if _, ok := statusReq.Command.(schemacommands.Status); !ok {
		t.Fatalf("status command = %T, want Status", statusReq.Command)
	}
}

func TestHostbridgeRunRouterOnlyIncludesRunCommand(t *testing.T) {
	router, err := routers.NewHostbridgeRunRouter()
	if err != nil {
		t.Fatalf("NewHostbridgeRunRouter() error = %v", err)
	}
	base := commandengine.Request{Context: commandengine.Context{
		Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
	}}

	runReq, err := router.Parse(context.Background(), base, []string{"run", "echo", "hello"})
	if err != nil {
		t.Fatalf("parse run: %v", err)
	}
	if _, ok := runReq.Command.(schemacommands.RunCommand); !ok {
		t.Fatalf("run command = %T, want RunCommand", runReq.Command)
	}
	for _, argv := range [][]string{
		{"config", "get", "docker.image"},
		{"status"},
		{"sendstdin"},
	} {
		if _, err := router.Parse(context.Background(), base, argv); err == nil {
			t.Fatalf("Parse(%v) succeeded, want run-only rejection", argv)
		}
	}
}

func TestHostbridgeCommandEngineRunsConfigThreadAndHostbridgeCommands(t *testing.T) {
	cfg := newTestConfig(t)
	manager := newConfigManager(t, cfg)
	threads := &fakeThreadHandlers{}
	host := &fakeHostbridgeHandlers{}
	engine, err := routers.NewHostbridgeCommandEngine(manager, threads, host)
	if err != nil {
		t.Fatalf("NewHostbridgeCommandEngine() error = %v", err)
	}
	base := commandengine.Request{Context: commandengine.Context{
		ChatID: modeluuid.New(),
		Actor:  commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
	}}

	runResult, err := engine.Run(context.Background(), base, []string{"run", "echo", "hello"})
	if err != nil {
		t.Fatalf("run command: %v", err)
	}
	if runResult.Text != "run:echo" {
		t.Fatalf("run result = %q, want run:echo", runResult.Text)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	mediaResult, err := engine.Run(context.Background(), base, []string{"sendfile", path})
	if err != nil {
		t.Fatalf("sendfile command: %v", err)
	}
	if mediaResult.Text != "media:note.txt" {
		t.Fatalf("media result = %q, want media:note.txt", mediaResult.Text)
	}

	statusResult, err := engine.Run(context.Background(), base, []string{"status"})
	if err != nil {
		t.Fatalf("status command: %v", err)
	}
	if statusResult.Text != "status" {
		t.Fatalf("status result = %q, want status", statusResult.Text)
	}
}

type fakeHostbridgeHandlers struct {
	runCommand string
}

func (f *fakeHostbridgeHandlers) RunCommand(ctx context.Context, req commandengine.Request, cmd schemacommands.RunCommand) (commandengine.Result, error) {
	f.runCommand = cmd.Command
	return commandengine.Result{Text: "run:" + cmd.Command}, nil
}

func (f *fakeHostbridgeHandlers) SendMedia(ctx context.Context, req commandengine.Request, cmd schemacommands.SendMedia) (commandengine.Result, error) {
	return commandengine.Result{Text: "media:" + cmd.Filename}, nil
}
