package commandset

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	componentadmin "github.com/bartdeboer/ctgbot/internal/component/admin"
	brokercomponent "github.com/bartdeboer/ctgbot/internal/component/broker"
	codexcomponent "github.com/bartdeboer/ctgbot/internal/component/codex"
	configcomponent "github.com/bartdeboer/ctgbot/internal/component/config"
	heartbeatcomponent "github.com/bartdeboer/ctgbot/internal/component/heartbeat"
	indexingcomponent "github.com/bartdeboer/ctgbot/internal/component/indexing"
	messagingcomponent "github.com/bartdeboer/ctgbot/internal/component/messaging"
	processcomponent "github.com/bartdeboer/ctgbot/internal/component/process"
	theatercomponent "github.com/bartdeboer/ctgbot/internal/component/theater"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clistate"
)

func TestHelpFixtureAgentMain(t *testing.T) {
	router := agentMainHelpRouter(t)

	var buf bytes.Buffer
	err := router.FPrintHelp(context.Background(), &buf, nil, commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}})
	if err != nil {
		t.Fatalf("FPrintHelp() error = %v", err)
	}

	assertTextFixture(t, "help-agent-main.txt", normalizeGoldenText(buf.String()))
}

func agentMainHelpRouter(t *testing.T) *commandengine.Router {
	t.Helper()
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("NewCwd() error = %v", err)
	}
	configSurface, err := configcomponent.New(appstate.New(t.TempDir(), store))
	if err != nil {
		t.Fatalf("configcomponent.New() error = %v", err)
	}
	router, err := NewBoundRouterForSource(
		commandengine.SourceHostbridge,
		[]BoundSurface{
			{Surface: (*codexcomponent.Component)(nil), ComponentRef: "codex/work", ComponentType: "codex"},
			{Surface: processcomponent.New(nil), ComponentRef: "process", ComponentType: "process"},
			{Surface: &indexingcomponent.SearchComponent{}, ComponentRef: "search", ComponentType: "search"},
			{Surface: &heartbeatcomponent.Component{}, ComponentRef: "heartbeat", ComponentType: "heartbeat"},
			{Surface: &theatercomponent.Component{}, ComponentRef: "theater", ComponentType: "theater"},
		},
		componentadmin.New(nil, nil),
		brokercomponent.New(nil),
		messagingcomponent.New(nil, nil),
		configSurface,
	)
	if err != nil {
		t.Fatalf("NewBoundRouterForSource() error = %v", err)
	}
	return router
}

func assertTextFixture(t *testing.T, name string, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	got = normalizeGoldenText(got)
	if os.Getenv("CTGBOT_UPDATE_TESTDATA") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(got+"\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", path, err)
		}
	}
	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	want := normalizeGoldenText(string(wantBytes))
	if got != want {
		t.Fatalf("fixture %s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func normalizeGoldenText(text string) string {
	return strings.TrimSuffix(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
}
