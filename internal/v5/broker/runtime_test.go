package broker

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/v5/commandset"
	brokercomponent "github.com/bartdeboer/ctgbot/internal/v5/component/broker"
	codexcomponent "github.com/bartdeboer/ctgbot/internal/v5/component/codex"
	configcomponent "github.com/bartdeboer/ctgbot/internal/v5/component/config"
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
		brokercomponent.New(nil),
		configSurface,
	)
	if err != nil {
		t.Fatalf("NewBoundEngineForSource() error = %v", err)
	}

	got := hostbridgeControlCommands(&ChatRuntime{AgentCommands: engine})
	wantContains := []string{
		"hostbridge codex status",
		"hostbridge codex container refresh",
		"hostbridge codex interrupt",
		"hostbridge codex model",
		"hostbridge config list",
		"hostbridge config get <key>",
		"hostbridge config set <key> <value>",
	}
	for _, want := range wantContains {
		if !containsString(got, want) {
			t.Fatalf("hostbridgeControlCommands() missing %q in %v", want, got)
		}
	}
	for _, notWant := range []string{
		"hostbridge run <command>",
		"hostbridge sendfile <path>",
		"hostbridge sendstdin",
	} {
		if containsString(got, notWant) {
			t.Fatalf("hostbridgeControlCommands() unexpectedly contains %q in %v", notWant, got)
		}
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
