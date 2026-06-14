package broker

import (
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	componentadmin "github.com/bartdeboer/ctgbot/internal/component/admin"
	brokercomponent "github.com/bartdeboer/ctgbot/internal/component/broker"
	codexcomponent "github.com/bartdeboer/ctgbot/internal/component/codex"
	configcomponent "github.com/bartdeboer/ctgbot/internal/component/config"
	messagingcomponent "github.com/bartdeboer/ctgbot/internal/component/messaging"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
	"github.com/bartdeboer/go-clistate"
)

func TestHostbridgeControlCommandsUsesCanonicalAgentSurface(t *testing.T) {
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("NewCwd() error = %v", err)
	}
	configSurface, err := configcomponent.New(appstate.New(t.TempDir(), store))
	if err != nil {
		t.Fatalf("configcomponent.New() error = %v", err)
	}
	engine, err := commandset.NewBoundEngineForSource(
		commandengine.SourceHostbridge,
		[]commandset.BoundSurface{{
			Surface:       (*codexcomponent.Component)(nil),
			ComponentRef:  "codex/work",
			ComponentType: "codex",
		}},
		componentadmin.New(nil, nil),
		brokercomponent.New(nil),
		messagingcomponent.New(nil, nil),
		configSurface,
	)
	if err != nil {
		t.Fatalf("NewBoundEngineForSource() error = %v", err)
	}

	got := hostbridgeControlCommands(&ChatRuntime{AgentCommands: engine})
	wantContains := []string{
		"hostbridge sendfile <path>",
		"hostbridge send <text>",
		"hostbridge status",
		"hostbridge component help",
		"hostbridge component list",
		"hostbridge component <component> help",
		"hostbridge thread help",
		"hostbridge thread list",
		"hostbridge thread <thread> message send <message>",
		"hostbridge codex help",
		"hostbridge codex status",
		"hostbridge codex interrupt",
		"hostbridge codex chat purge",
		"hostbridge config help",
		"hostbridge config list",
		"hostbridge config get <key>",
	}
	for _, want := range wantContains {
		if !containsString(got, want) {
			t.Fatalf("hostbridgeControlCommands() missing %q in %v", want, got)
		}
	}
	for _, notWant := range []string{
		"hostbridge run <command>",
		"hostbridge codex model effort list",
		"hostbridge codex model effort set <effort>",
		"hostbridge component <component> managed-file status",
		"hostbridge thread <thread> message list",
		"hostbridge config set <key> <value>",
	} {
		if containsString(got, notWant) {
			t.Fatalf("hostbridgeControlCommands() unexpectedly contains %q in %v", notWant, got)
		}
	}
}

func TestChatRuntimeRunHostbridgeCommandUsesRuntimeAliases(t *testing.T) {
	runtime := &ChatRuntime{RunCommands: map[string]hostbridgeserver.AllowedCommand{
		"echo-runtime": {
			Name: "/bin/echo",
			Args: []string{"runtime-ok"},
		},
	}}

	result, err := runtime.RunHostbridgeCommand(context.Background(), commandengine.Request{}, schemacommands.RunCommand{Command: "echo-runtime"})
	if err != nil {
		t.Fatalf("RunHostbridgeCommand() error = %v", err)
	}
	if got, want := strings.TrimSpace(result.Text), "runtime-ok"; got != want {
		t.Fatalf("RunHostbridgeCommand() text = %q, want %q", got, want)
	}

	if _, err := runtime.RunHostbridgeCommand(context.Background(), commandengine.Request{}, schemacommands.RunCommand{Command: "not-allowed"}); err == nil {
		t.Fatalf("RunHostbridgeCommand(not-allowed) error = nil")
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
