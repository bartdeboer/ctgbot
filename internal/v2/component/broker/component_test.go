package broker

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	v2codex "github.com/bartdeboer/ctgbot/internal/v2/component/codex"
	v2runtimecomponent "github.com/bartdeboer/ctgbot/internal/v2/component/runtime"
	v2telegram "github.com/bartdeboer/ctgbot/internal/v2/component/telegram"
	"github.com/bartdeboer/ctgbot/internal/v2/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestComponentListsUnregisteredChats(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	chat, err := store.Chats().EnsureProviderChat(context.Background(), "telegram", "-10042")
	if err != nil {
		t.Fatalf("ensure chat: %v", err)
	}

	result, err := runCommand(t, New(store, Config{CodexProfile: "v2test"}), []string{"chat", "list-unregistered"})
	if err != nil {
		t.Fatalf("run command: %v", err)
	}
	if !strings.Contains(result.Text, chat.ID.String()) || !strings.Contains(result.Text, "telegram:-10042") {
		t.Fatalf("unexpected list output: %q", result.Text)
	}
}

func TestComponentAppliesTelegramCodexPreset(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	chat, err := store.Chats().EnsureProviderChat(context.Background(), "telegram", "-10042")
	if err != nil {
		t.Fatalf("ensure chat: %v", err)
	}

	result, err := runCommand(t, New(store, Config{CodexProfile: "v2test"}), []string{"chat", chat.ID.String(), "preset", PresetTelegramCodex})
	if err != nil {
		t.Fatalf("run command: %v", err)
	}
	if !strings.Contains(result.Text, "enabled") {
		t.Fatalf("unexpected result: %q", result.Text)
	}

	chat, err = store.Chats().GetByID(context.Background(), chat.ID)
	if err != nil {
		t.Fatalf("get chat: %v", err)
	}
	if chat == nil || !chat.Enabled {
		t.Fatalf("chat not enabled: %#v", chat)
	}

	bindings, err := store.ChatComponents().ListEnabledByChatID(context.Background(), chat.ID)
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	want := map[string]string{
		v2telegram.ComponentType:         v2telegram.DefaultProfileName,
		v2codex.ComponentType:            "v2test",
		v2runtimecomponent.ComponentType: "",
	}
	if len(bindings) != len(want) {
		t.Fatalf("bindings = %#v, want %d", bindings, len(want))
	}
	for _, binding := range bindings {
		if want[binding.ComponentType] != binding.ProfileName {
			t.Fatalf("unexpected binding: %#v", binding)
		}
	}
}

func TestComponentRequiresCodexProfileBeforeEnablingChat(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	chat, err := store.Chats().EnsureProviderChat(context.Background(), "telegram", "-10042")
	if err != nil {
		t.Fatalf("ensure chat: %v", err)
	}

	_, err = runCommand(t, New(store, Config{}), []string{"chat", chat.ID.String(), "preset", PresetTelegramCodex})
	if err == nil || !strings.Contains(err.Error(), "missing codex profile") {
		t.Fatalf("run command error = %v, want missing codex profile", err)
	}
	chat, err = store.Chats().GetByID(context.Background(), chat.ID)
	if err != nil {
		t.Fatalf("get chat: %v", err)
	}
	if chat == nil || chat.Enabled {
		t.Fatalf("chat should remain disabled: %#v", chat)
	}
}

func runCommand(t *testing.T, component *Component, argv []string) (commandengine.Result, error) {
	t.Helper()

	registry := commandengine.NewRegistry()
	if err := component.RegisterCommandHandlers(registry); err != nil {
		t.Fatalf("register handlers: %v", err)
	}
	router, err := commandengine.NewRouter(component.CommandDefinitions(), commandengine.SourceCLI)
	if err != nil {
		t.Fatalf("new router: %v", err)
	}
	engine := commandengine.NewEngine(router, registry)
	return engine.Run(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Source: commandengine.SourceCLI,
			Actor:  commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleRoot}},
		},
	}, argv)
}

func newTestStore(t *testing.T) repository.Storage {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "broker-component-v2.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	store := repository.NewGORM(db)
	if err := store.AutoMigrate(context.Background()); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return store
}
