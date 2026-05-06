package broker

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/v5/commandset"
	brokercomponent "github.com/bartdeboer/ctgbot/internal/v5/component/broker"
	codexcomponent "github.com/bartdeboer/ctgbot/internal/v5/component/codex"
	configcomponent "github.com/bartdeboer/ctgbot/internal/v5/component/config"
)

func TestHostbridgeControlCommandsUsesCanonicalAgentSurface(t *testing.T) {
	router, err := commandset.NewRouterForSource(
		commandengine.SourceHostbridge,
		brokercomponent.New(nil),
		(*configcomponent.Component)(nil),
		(*codexcomponent.Component)(nil),
	)
	if err != nil {
		t.Fatalf("NewRouterForSource() error = %v", err)
	}

	got := hostbridgeControlCommands(&ChatRuntime{AgentCommands: commandengine.NewEngine(router, nil)})
	wantContains := []string{
		"hostbridge codex status",
		"hostbridge codex refresh",
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
