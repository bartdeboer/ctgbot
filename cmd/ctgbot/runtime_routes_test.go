package main

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	systempkg "github.com/bartdeboer/ctgbot/internal/system"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func TestWorkspaceSetAndComponentRegister(t *testing.T) {
	withTempCwd(t, func(root string) {
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewCwd: %v", err)
		}

		router := clir.New()
		registerRuntimeRoutes(router, store, nil)

		workspaceOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"workspace", "set", "work", "--path", "workspaces/work-root"}); err != nil {
				t.Fatalf("workspace set: %v", err)
			}
		})
		if !strings.Contains(workspaceOutput, "workspace saved") || !strings.Contains(workspaceOutput, "workspaces/work-root") {
			t.Fatalf("unexpected workspace output: %q", workspaceOutput)
		}

		registerOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"component", "register", "codex/work", "--runtime", "local"}); err != nil {
				t.Fatalf("component register: %v", err)
			}
		})
		if !strings.Contains(registerOutput, "component registered") || !strings.Contains(registerOutput, "runtime: local") {
			t.Fatalf("unexpected register output: %q", registerOutput)
		}

		listOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"component", "list"}); err != nil {
				t.Fatalf("component list: %v", err)
			}
		})
		if !strings.Contains(listOutput, "codex/work") || !strings.Contains(listOutput, "runtime=local") || !strings.Contains(listOutput, "host_home=") {
			t.Fatalf("unexpected component list output: %q", listOutput)
		}

		expectedHome := filepath.Join(root, ".ctgbot", "components", "codex", "work")
		assertDirExists(t, expectedHome)

		system, err := systempkg.Open(context.Background(), "", "", store, log.New(io.Discard, "", 0))
		if err != nil {
			t.Fatalf("open runtime: %v", err)
		}
		componentRow, err := system.Storage.Components().GetByTypeAndName(context.Background(), "codex", "work")
		if err != nil {
			t.Fatalf("GetByTypeAndName: %v", err)
		}
		if componentRow == nil {
			t.Fatal("expected registered component")
		}
		if componentRow.Runtime != "local" {
			t.Fatalf("Runtime = %q, want local", componentRow.Runtime)
		}
	})
}

func TestRuntimeOverrideFlagsAreRejected(t *testing.T) {
	withTempCwd(t, func(root string) {
		_ = root
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewCwd: %v", err)
		}

		router := clir.New()
		registerRuntimeRoutes(router, store, nil)

		tests := [][]string{
			{"run", "--telegram-token", "token"},
			{"component", "register", "codex/work", "--state-root", "other"},
			{"component", "codex/work", "--image", "codex:test"},
			{"chat", "create", "team", "--db-path", "other.db"},
		}
		for _, argv := range tests {
			if err := router.Run(context.Background(), argv); err == nil {
				t.Fatalf("router.Run(%v) error = nil, want removed flag error", argv)
			}
		}
	})
}

func TestChatCreateListAndWorkspaceRoutes(t *testing.T) {
	withTempCwd(t, func(root string) {
		_ = root
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewCwd: %v", err)
		}

		router := clir.New()
		registerRuntimeRoutes(router, store, nil)

		if err := router.Run(context.Background(), []string{"workspace", "set", "work", "--path", "workspaces/work-root"}); err != nil {
			t.Fatalf("workspace set: %v", err)
		}

		createOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"chat", "create", "team"}); err != nil {
				t.Fatalf("chat create: %v", err)
			}
		})
		if !strings.Contains(createOutput, "chat created") || !strings.Contains(createOutput, "label: team") {
			t.Fatalf("unexpected chat create output: %q", createOutput)
		}

		system, err := systempkg.Open(context.Background(), "", "", store, log.New(io.Discard, "", 0))
		if err != nil {
			t.Fatalf("open runtime: %v", err)
		}
		chats, err := system.Storage.Chats().List(context.Background())
		if err != nil {
			t.Fatalf("list chats: %v", err)
		}
		if got, want := len(chats), 1; got != want {
			t.Fatalf("chat count = %d, want %d", got, want)
		}
		chat := chats[0]
		shortChatID := shortChatIDForTest(t, system.Storage, chat.ID)

		listOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"chat", "list"}); err != nil {
				t.Fatalf("chat list: %v", err)
			}
		})
		if !strings.Contains(listOutput, chat.ID.String()) || !strings.Contains(listOutput, "short_id="+shortChatID) || !strings.Contains(listOutput, "team") || !strings.Contains(listOutput, "enabled=true") {
			t.Fatalf("unexpected chat list output: %q", listOutput)
		}

		setOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"chat", shortChatID, "workspace", "set", "work"}); err != nil {
				t.Fatalf("chat workspace set: %v", err)
			}
		})
		if !strings.Contains(setOutput, "chat workspace updated") || !strings.Contains(setOutput, "workspace: work") {
			t.Fatalf("unexpected workspace set output: %q", setOutput)
		}
		updated, err := system.Storage.Chats().GetByID(context.Background(), chat.ID)
		if err != nil {
			t.Fatalf("get chat: %v", err)
		}
		if updated == nil || updated.Workspace != "work" {
			t.Fatalf("chat after workspace set = %#v, want workspace work", updated)
		}

		clearOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"chat", chat.ID.String(), "workspace", "clear"}); err != nil {
				t.Fatalf("chat workspace clear: %v", err)
			}
		})
		if !strings.Contains(clearOutput, "chat workspace cleared") || !strings.Contains(clearOutput, "chat_id: "+chat.ID.String()) {
			t.Fatalf("unexpected workspace clear output: %q", clearOutput)
		}
		updated, err = system.Storage.Chats().GetByID(context.Background(), chat.ID)
		if err != nil {
			t.Fatalf("get chat after clear: %v", err)
		}
		if updated == nil || updated.Workspace != "" {
			t.Fatalf("chat after workspace clear = %#v, want empty workspace", updated)
		}
	})
}

func TestChatComponentAddBindsExternalChatID(t *testing.T) {
	withTempCwd(t, func(root string) {
		_ = root
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewCwd: %v", err)
		}
		globalStore, err := clistate.NewGlobal("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewGlobal: %v", err)
		}

		router := clir.New()
		registerRuntimeRoutes(router, store, globalStore)

		if err := router.Run(context.Background(), []string{"component", "register", "telegram", "--runtime", "local"}); err != nil {
			t.Fatalf("component register: %v", err)
		}
		if err := router.Run(context.Background(), []string{"chat", "create", "team"}); err != nil {
			t.Fatalf("chat create: %v", err)
		}

		system, err := systempkg.Open(context.Background(), "", "", store, log.New(io.Discard, "", 0))
		if err != nil {
			t.Fatalf("open runtime: %v", err)
		}
		chats, err := system.Storage.Chats().List(context.Background())
		if err != nil {
			t.Fatalf("list chats: %v", err)
		}
		if len(chats) != 1 {
			t.Fatalf("chat count = %d, want 1", len(chats))
		}
		shortChatID := shortChatIDForTest(t, system.Storage, chats[0].ID)

		bindOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"chat", chats[0].ID.String(), "component", "add", "source", "telegram", "--external-chat-id", "chat-1"}); err != nil {
				t.Fatalf("component add: %v", err)
			}
		})
		if !strings.Contains(bindOutput, "chat component bound") || !strings.Contains(bindOutput, "external_chat_id: chat-1") {
			t.Fatalf("unexpected bind output: %q", bindOutput)
		}

		bindings, err := system.Storage.ChatComponents().ListEnabledByChatID(context.Background(), chats[0].ID)
		if err != nil {
			t.Fatalf("list chat components: %v", err)
		}
		if len(bindings) != 1 {
			t.Fatalf("binding count = %d, want 1", len(bindings))
		}
		if bindings[0].ExternalChatID != "chat-1" {
			t.Fatalf("ExternalChatID = %q, want chat-1", bindings[0].ExternalChatID)
		}

		listOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"chat", shortChatID, "component", "list"}); err != nil {
				t.Fatalf("component list: %v", err)
			}
		})
		if !strings.Contains(listOutput, "telegram") || !strings.Contains(listOutput, "role=source") || !strings.Contains(listOutput, "external_chat_id=chat-1") {
			t.Fatalf("unexpected component list output: %q", listOutput)
		}
	})
}

func shortChatIDForTest(t *testing.T, storage repository.Storage, chatID modeluuid.UUID) string {
	t.Helper()
	ids, err := storage.Chats().ListIDs(context.Background())
	if err != nil {
		t.Fatalf("Chats().ListIDs() error = %v", err)
	}
	shortID, err := repository.NewShortIDResolver(ids).ShortIDFor(chatID, 6)
	if err != nil {
		t.Fatalf("ShortIDFor(%s) error = %v", chatID, err)
	}
	return shortID
}

func TestComponentGuardSetStatusReplacesAndClearDisables(t *testing.T) {
	withTempCwd(t, func(root string) {
		_ = root
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewCwd: %v", err)
		}

		router := clir.New()
		registerRuntimeRoutes(router, store, nil)

		if err := router.Run(context.Background(), []string{"component", "register", "gmail/work", "--runtime", "local"}); err != nil {
			t.Fatalf("register source: %v", err)
		}
		if err := router.Run(context.Background(), []string{"component", "register", "llamacpp/qwen3-q5", "--runtime", "backend"}); err != nil {
			t.Fatalf("register first guard: %v", err)
		}
		if err := router.Run(context.Background(), []string{"component", "register", "llamacpp/gemma4-e4b", "--runtime", "backend"}); err != nil {
			t.Fatalf("register second guard: %v", err)
		}

		output := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"component", "gmail/work", "guard", "set", "llamacpp/qwen3-q5"}); err != nil {
				t.Fatalf("component guard set: %v", err)
			}
		})
		if !strings.Contains(output, "component guard set") || !strings.Contains(output, "source: gmail/work") || !strings.Contains(output, "guard: llamacpp/qwen3-q5") {
			t.Fatalf("unexpected guard set output: %q", output)
		}

		system, source, firstGuard, secondGuard := openSourceGuardTestSystem(t, store)
		bindings, err := system.Storage.ComponentBindings().ListEnabledBySourceAndRole(context.Background(), source.ID, coremodel.ComponentBindingRoleGuard)
		if err != nil {
			t.Fatalf("list guard bindings: %v", err)
		}
		if got, want := len(bindings), 1; got != want {
			t.Fatalf("enabled guard bindings = %d, want %d", got, want)
		}
		if bindings[0].TargetComponentID != firstGuard.ID {
			t.Fatalf("guard target = %s, want %s", bindings[0].TargetComponentID, firstGuard.ID)
		}

		statusOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"component", "gmail/work", "guard", "status"}); err != nil {
				t.Fatalf("component guard status: %v", err)
			}
		})
		if !strings.Contains(statusOutput, "component guard status") || !strings.Contains(statusOutput, "guard: llamacpp/qwen3-q5") {
			t.Fatalf("unexpected guard status output: %q", statusOutput)
		}
		if err := router.Run(context.Background(), []string{"component", "gmail/work", "guard", "status", "--state-root", "other"}); err == nil {
			t.Fatal("component guard status accepted removed state-root flag")
		}

		if err := router.Run(context.Background(), []string{"component", "gmail/work", "guard", "set", "llamacpp/gemma4-e4b"}); err != nil {
			t.Fatalf("replace component guard: %v", err)
		}
		system, source, _, secondGuard = openSourceGuardTestSystem(t, store)
		bindings, err = system.Storage.ComponentBindings().ListEnabledBySourceAndRole(context.Background(), source.ID, coremodel.ComponentBindingRoleGuard)
		if err != nil {
			t.Fatalf("list replaced guard bindings: %v", err)
		}
		if got, want := len(bindings), 1; got != want {
			t.Fatalf("enabled guard bindings after replace = %d, want %d", got, want)
		}
		if bindings[0].TargetComponentID != secondGuard.ID {
			t.Fatalf("replaced guard target = %s, want %s", bindings[0].TargetComponentID, secondGuard.ID)
		}

		clearOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"component", "gmail/work", "guard", "clear"}); err != nil {
				t.Fatalf("component guard clear: %v", err)
			}
		})
		if !strings.Contains(clearOutput, "component guard cleared") || !strings.Contains(clearOutput, "disabled: 1") {
			t.Fatalf("unexpected guard clear output: %q", clearOutput)
		}
		system, source, _, _ = openSourceGuardTestSystem(t, store)
		bindings, err = system.Storage.ComponentBindings().ListEnabledBySourceAndRole(context.Background(), source.ID, coremodel.ComponentBindingRoleGuard)
		if err != nil {
			t.Fatalf("list cleared guard bindings: %v", err)
		}
		if got := len(bindings); got != 0 {
			t.Fatalf("enabled guard bindings after clear = %d, want 0", got)
		}
	})
}

func openSourceGuardTestSystem(t *testing.T, store *clistate.Store) (*systempkg.System, *coremodel.Component, *coremodel.Component, *coremodel.Component) {
	t.Helper()

	system, err := systempkg.Open(context.Background(), "", "", store, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("open runtime: %v", err)
	}
	source, err := system.Storage.Components().GetByTypeAndName(context.Background(), "gmail", "work")
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	if source == nil {
		t.Fatal("expected source component")
	}
	firstGuard, err := system.Storage.Components().GetByTypeAndName(context.Background(), "llamacpp", "qwen3-q5")
	if err != nil {
		t.Fatalf("get first guard: %v", err)
	}
	if firstGuard == nil {
		t.Fatal("expected first guard component")
	}
	secondGuard, err := system.Storage.Components().GetByTypeAndName(context.Background(), "llamacpp", "gemma4-e4b")
	if err != nil {
		t.Fatalf("get second guard: %v", err)
	}
	if secondGuard == nil {
		t.Fatal("expected second guard component")
	}
	return system, source, firstGuard, secondGuard
}

func TestComponentCommandRouteUsesBoundCLISurface(t *testing.T) {
	withTempCwd(t, func(root string) {
		_ = root
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewCwd: %v", err)
		}

		router := clir.New()
		registerRuntimeRoutes(router, store, nil)

		if err := router.Run(context.Background(), []string{"component", "register", "process", "--runtime", "local"}); err != nil {
			t.Fatalf("component register: %v", err)
		}

		helpOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"component", "process"}); err != nil {
				t.Fatalf("component help: %v", err)
			}
		})
		if !strings.Contains(helpOutput, "available component commands:") || !strings.Contains(helpOutput, "process quit") {
			t.Fatalf("unexpected component help output: %q", helpOutput)
		}

		output := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"component", "process", "quit"}); err != nil {
				t.Fatalf("component quit: %v", err)
			}
		})
		if !strings.Contains(output, "quit requested") {
			t.Fatalf("unexpected component command output: %q", output)
		}
	})
}

func TestChatDroppedListsUnresolvedInboundChats(t *testing.T) {
	withTempCwd(t, func(root string) {
		_ = root
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewCwd: %v", err)
		}

		router := clir.New()
		registerRuntimeRoutes(router, store, nil)

		if err := router.Run(context.Background(), []string{"component", "register", "gmail", "--runtime", "local"}); err != nil {
			t.Fatalf("component register: %v", err)
		}

		system, err := systempkg.Open(context.Background(), "", "", store, log.New(io.Discard, "", 0))
		if err != nil {
			t.Fatalf("open runtime: %v", err)
		}
		registration, err := system.Storage.Components().GetByTypeAndName(context.Background(), "gmail", "gmail")
		if err != nil {
			t.Fatalf("GetByTypeAndName: %v", err)
		}
		if registration == nil {
			t.Fatal("expected gmail registration")
		}
		if err := system.Storage.InboundDrops().Save(context.Background(), &coremodel.InboundDrop{
			ComponentID:     registration.ID,
			ExternalChatID:  "me",
			ChatLabel:       "Inbox",
			ActorLabel:      "Email",
			LastTextPreview: "hello",
			MessageCount:    1,
			FirstSeenAt:     time.Now().Add(-time.Minute),
			LastSeenAt:      time.Now(),
		}); err != nil {
			t.Fatalf("Save drop: %v", err)
		}

		output := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"chat", "dropped"}); err != nil {
				t.Fatalf("chat dropped: %v", err)
			}
		})
		if !strings.Contains(output, "gmail") || !strings.Contains(output, "external_chat_id=me") || !strings.Contains(output, "preview=hello") {
			t.Fatalf("unexpected dropped output: %q", output)
		}
	})
}

func TestChatBindCreatesEnabledChatAndAutoBindsSupportedRoles(t *testing.T) {
	withTempCwd(t, func(root string) {
		_ = root
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewCwd: %v", err)
		}
		if err := store.PersistString("telegram.token", "test-token"); err != nil {
			t.Fatalf("PersistString: %v", err)
		}

		router := clir.New()
		registerRuntimeRoutes(router, store, nil)

		if err := router.Run(context.Background(), []string{"component", "register", "telegram", "--runtime", "local"}); err != nil {
			t.Fatalf("component register: %v", err)
		}

		system, err := systempkg.Open(context.Background(), "", "", store, log.New(io.Discard, "", 0))
		if err != nil {
			t.Fatalf("open runtime: %v", err)
		}
		registration, err := system.Storage.Components().GetByTypeAndName(context.Background(), "telegram", "telegram")
		if err != nil {
			t.Fatalf("GetByTypeAndName: %v", err)
		}
		if registration == nil {
			t.Fatal("expected telegram registration")
		}
		if err := system.Storage.InboundDrops().Save(context.Background(), &coremodel.InboundDrop{
			ComponentID:     registration.ID,
			ExternalChatID:  "chat-1",
			ChatLabel:       "Team room",
			LastTextPreview: "hello",
			MessageCount:    1,
			FirstSeenAt:     time.Now().Add(-time.Minute),
			LastSeenAt:      time.Now(),
		}); err != nil {
			t.Fatalf("Save drop: %v", err)
		}

		output := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"chat", "bind", "telegram", "chat-1"}); err != nil {
				t.Fatalf("chat bind: %v", err)
			}
		})
		if !strings.Contains(output, "chat bound") || !strings.Contains(output, "role=source") || !strings.Contains(output, "role=relay") {
			t.Fatalf("unexpected bind output: %q", output)
		}

		chats, err := system.Storage.Chats().List(context.Background())
		if err != nil {
			t.Fatalf("list chats: %v", err)
		}
		if len(chats) != 1 {
			t.Fatalf("chat count = %d, want 1", len(chats))
		}
		if got, want := chats[0].Label, "Team room"; got != want {
			t.Fatalf("chat label = %q, want %q", got, want)
		}
		bindings, err := system.Storage.ChatComponents().ListEnabledByChatID(context.Background(), chats[0].ID)
		if err != nil {
			t.Fatalf("list bindings: %v", err)
		}
		if len(bindings) != 2 {
			t.Fatalf("binding count = %d, want 2", len(bindings))
		}
		drops, err := system.Storage.InboundDrops().List(context.Background())
		if err != nil {
			t.Fatalf("list drops: %v", err)
		}
		if len(drops) != 0 {
			t.Fatalf("drop count = %d, want 0", len(drops))
		}
	})
}
