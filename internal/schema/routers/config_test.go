package routers_test

import (
	"bytes"
	"context"
	"encoding/gob"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configengine"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	configschema "github.com/bartdeboer/ctgbot/internal/schema/config"
	"github.com/bartdeboer/ctgbot/internal/schema/routers"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clistate"
)

func TestConfigCommandsUseConfigItemRBAC(t *testing.T) {
	cfg := newTestConfig(t)
	manager := newConfigManager(t, cfg)
	engine, err := routers.NewConfigCommandEngine(manager, commandengine.SourceMessage)
	if err != nil {
		t.Fatalf("NewConfigCommandEngine() error = %v", err)
	}
	chatID := modeluuid.New()

	userReq := commandengine.Request{Context: commandengine.Context{
		ChatID: chatID,
		Actor:  commandengine.Actor{ID: "user", Roles: []simplerbac.Role{simplerbac.RoleUser}},
	}}
	list, err := engine.Run(context.Background(), userReq, []string{"config", "list"})
	if err != nil {
		t.Fatalf("config list as user: %v", err)
	}
	if !containsLine(list.Text, "chat.enabled") {
		t.Fatalf("list = %q, want chat.enabled", list.Text)
	}
	if containsLine(list.Text, "docker.image") {
		t.Fatalf("list = %q, did not expect docker.image for user", list.Text)
	}
	if containsLine(list.Text, "git.user-name") {
		t.Fatalf("list = %q, did not expect git.user-name for user", list.Text)
	}
	if _, err := engine.Run(context.Background(), userReq, []string{"config", "set", "chat.enabled", "true"}); err == nil || !strings.Contains(err.Error(), "set chat.enabled denied") {
		t.Fatalf("user set error = %v, want item RBAC denial", err)
	}

	elevatedReq := commandengine.Request{Context: commandengine.Context{
		ChatID: chatID,
		Actor:  commandengine.Actor{ID: "elevated", Roles: []simplerbac.Role{simplerbac.RoleUser, simplerbac.RoleElevated}},
	}}
	result, err := engine.Run(context.Background(), elevatedReq, []string{"config", "set", "chat.enabled", "true"})
	if err != nil {
		t.Fatalf("config set as elevated: %v", err)
	}
	if result.Text != "chat.enabled=true" {
		t.Fatalf("result = %q, want chat.enabled=true", result.Text)
	}
	if !cfg.Chat(chatID).Enabled() {
		t.Fatal("chat enabled was not persisted")
	}
}

func TestConfigCommandCanRoundTripThroughHostbridgeProtocol(t *testing.T) {
	cfg := newTestConfig(t)
	manager := newConfigManager(t, cfg)
	engine, err := routers.NewConfigCommandEngine(manager, commandengine.SourceHostbridge)
	if err != nil {
		t.Fatalf("NewConfigCommandEngine() error = %v", err)
	}
	base := commandengine.Request{Context: commandengine.Context{
		Actor: commandengine.Actor{ID: "root", Roles: []simplerbac.Role{simplerbac.RoleRoot}},
	}}

	parsed, err := engine.Parse(context.Background(), base, []string{"config", "set", "docker.image", "ctgbot:test"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	var wire bytes.Buffer
	if err := gob.NewEncoder(&wire).Encode(hostbridge.CommandRequest{Request: parsed}); err != nil {
		t.Fatalf("encode command request: %v", err)
	}
	var decoded hostbridge.CommandRequest
	if err := gob.NewDecoder(&wire).Decode(&decoded); err != nil {
		t.Fatalf("decode command request: %v", err)
	}

	result, err := engine.Execute(context.Background(), decoded.Request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Text != "docker.image=ctgbot:test" {
		t.Fatalf("result = %q, want docker.image=ctgbot:test", result.Text)
	}
	if got := cfg.Docker().Image(); got != "ctgbot:test" {
		t.Fatalf("docker image = %q, want ctgbot:test", got)
	}
}

func TestConfigRegistryCoversFormerScalarSetters(t *testing.T) {
	cfg := newTestConfig(t)
	manager := newConfigManager(t, cfg)
	engine, err := routers.NewConfigCommandEngine(manager, commandengine.SourceMessage)
	if err != nil {
		t.Fatalf("NewConfigCommandEngine() error = %v", err)
	}
	chatID := modeluuid.New()
	rootReq := commandengine.Request{Context: commandengine.Context{
		ChatID: chatID,
		Actor:  commandengine.Actor{ID: "root", Roles: []simplerbac.Role{simplerbac.RoleRoot}},
	}}

	list, err := engine.Run(context.Background(), rootReq, []string{"config", "list"})
	if err != nil {
		t.Fatalf("config list: %v", err)
	}
	for _, want := range []string{
		"build.compiler-path",
		"chat.codex-profile-host-path",
		"chat.enabled",
		"chat.interactive-interrupt-enabled",
		"chat.process-tools-enabled",
		"chat.skills",
		"chat.workspace-host-path",
		"codex.login-callback-port",
		"codex.model",
		"codex.profile-host-path",
		"codex.session-timeout",
		"docker.container-hostbridge-tcp-addr",
		"docker.image",
		"docker.workspace-host-path",
		"git.user-email",
		"git.user-name",
		"hostbridge.tcp-listen-addr",
	} {
		if !containsLine(list.Text, want) {
			t.Fatalf("config list missing %q:\n%s", want, list.Text)
		}
	}

	workspace := t.TempDir()
	skill := filepath.Join(t.TempDir(), "skill")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	for _, tc := range []struct {
		key       string
		value     string
		wantReply string
	}{
		{key: "codex.model", value: "gpt-test", wantReply: "codex.model=gpt-test"},
		{key: "git.user_name", value: "Registry User", wantReply: "git.user-name=Registry User"},
		{key: "git.user_email", value: "registry@example.com", wantReply: "git.user-email=registry@example.com"},
		{key: "hostbridge.tcp-listen-addr", value: "127.0.0.1:9999", wantReply: "hostbridge.tcp-listen-addr=127.0.0.1:9999"},
		{key: "chat.process-tools-enabled", value: "true", wantReply: "chat.process-tools-enabled=true"},
		{key: "chat.interactive-interrupt-enabled", value: "false", wantReply: "chat.interactive-interrupt-enabled=false"},
		{key: "chat.workspace-host-path", value: workspace, wantReply: "chat.workspace-host-path=" + workspace},
		{key: "chat.skills", value: skill, wantReply: "chat.skills=" + skill},
	} {
		result, err := engine.Run(context.Background(), rootReq, []string{"config", "set", tc.key, tc.value})
		if err != nil {
			t.Fatalf("config set %s: %v", tc.key, err)
		}
		if result.Text != tc.wantReply {
			t.Fatalf("config set %s reply = %q, want %q", tc.key, result.Text, tc.wantReply)
		}
	}
	for _, tc := range []struct {
		key       string
		wantReply string
	}{
		{key: "git.user_name", wantReply: "git.user-name=Registry User"},
		{key: "git.user_email", wantReply: "git.user-email=registry@example.com"},
	} {
		result, err := engine.Run(context.Background(), rootReq, []string{"config", "get", tc.key})
		if err != nil {
			t.Fatalf("config get %s: %v", tc.key, err)
		}
		if !strings.Contains(result.Text, tc.wantReply) {
			t.Fatalf("config get %s reply = %q, want to contain %q", tc.key, result.Text, tc.wantReply)
		}
	}
	if got := cfg.Git().UserName(); got != "Registry User" {
		t.Fatalf("git user name = %q, want Registry User", got)
	}
	if got := cfg.Git().UserEmail(); got != "registry@example.com" {
		t.Fatalf("git user email = %q, want registry@example.com", got)
	}
	if got := cfg.Chat(chatID).Skills(); len(got) != 1 || got[0] != skill {
		t.Fatalf("chat skills = %#v, want %q", got, skill)
	}
}

func newConfigManager(t *testing.T, cfg *appstate.Config) *configengine.Manager {
	t.Helper()
	registry, err := configschema.Registry(cfg)
	if err != nil {
		t.Fatalf("config registry: %v", err)
	}
	return configengine.New(registry)
}

func newTestConfig(t *testing.T) *appstate.Config {
	t.Helper()
	root := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("new cwd store: %v", err)
	}
	return appstate.New(filepath.Join(root, ".ctgbot"), store)
}

func containsLine(text string, want string) bool {
	for _, line := range strings.Split(text, "\n") {
		line := strings.TrimSpace(line)
		if line == want || strings.HasPrefix(line, want+"=") {
			return true
		}
	}
	return false
}
