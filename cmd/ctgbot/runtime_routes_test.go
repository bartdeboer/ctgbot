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

		unregisterOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"component", "unregister", "codex/work"}); err != nil {
				t.Fatalf("component unregister: %v", err)
			}
		})
		if !strings.Contains(unregisterOutput, "component unregistered") || !strings.Contains(unregisterOutput, "ref: codex/work") || !strings.Contains(unregisterOutput, "component_removed: true") {
			t.Fatalf("unexpected unregister output: %q", unregisterOutput)
		}
		componentRow, err = system.Storage.Components().GetByTypeAndName(context.Background(), "codex", "work")
		if err != nil {
			t.Fatalf("GetByTypeAndName after unregister: %v", err)
		}
		if componentRow != nil {
			t.Fatalf("component after unregister = %#v, want nil", componentRow)
		}
		assertDirExists(t, expectedHome)
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

func TestChatComponentAddBindsExternalChannelID(t *testing.T) {
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
			if err := router.Run(context.Background(), []string{"chat", chats[0].ID.String(), "component", "add", "source", "telegram", "--external-channel-id", "chat-1"}); err != nil {
				t.Fatalf("component add: %v", err)
			}
		})
		if !strings.Contains(bindOutput, "chat component bound") || !strings.Contains(bindOutput, "external_channel_id: chat-1") {
			t.Fatalf("unexpected bind output: %q", bindOutput)
		}

		bindings, err := system.Storage.ChatComponents().ListEnabledByChatID(context.Background(), chats[0].ID)
		if err != nil {
			t.Fatalf("list chat components: %v", err)
		}
		if len(bindings) != 1 {
			t.Fatalf("binding count = %d, want 1", len(bindings))
		}
		if bindings[0].ExternalChannelID != "chat-1" {
			t.Fatalf("ExternalChannelID = %q, want chat-1", bindings[0].ExternalChannelID)
		}

		aliasOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"chat", chats[0].ID.String(), "component", "add", "relay", "telegram", "--external-chat-id", "chat-2"}); err != nil {
				t.Fatalf("component add with deprecated external-chat-id alias: %v", err)
			}
		})
		if !strings.Contains(aliasOutput, "chat component bound") || !strings.Contains(aliasOutput, "external_channel_id: chat-2") {
			t.Fatalf("unexpected alias bind output: %q", aliasOutput)
		}

		listOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"chat", shortChatID, "component", "list"}); err != nil {
				t.Fatalf("component list: %v", err)
			}
		})
		if !strings.Contains(listOutput, "telegram") ||
			!strings.Contains(listOutput, "role=source") ||
			!strings.Contains(listOutput, "external_channel_id=chat-1") ||
			!strings.Contains(listOutput, "role=relay") ||
			!strings.Contains(listOutput, "external_channel_id=chat-2") {
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

func TestChatComponentFilterRoutesBindSourceBinding(t *testing.T) {
	withTempCwd(t, func(root string) {
		_ = root
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewCwd: %v", err)
		}

		router := clir.New()
		registerRuntimeRoutes(router, store, nil)

		for _, argv := range [][]string{
			{"component", "register", "gmail/work", "--runtime", "local"},
			{"component", "register", "filters/allowlist", "--runtime", "local"},
			{"component", "register", "guard/qwen", "--runtime", "local"},
			{"chat", "create", "team"},
		} {
			if err := router.Run(context.Background(), argv); err != nil {
				t.Fatalf("router.Run(%v): %v", argv, err)
			}
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
		chatID := chats[0].ID.String()
		if err := router.Run(context.Background(), []string{"chat", chatID, "component", "add", "source", "gmail/work", "--external-channel-id", "work@example.com"}); err != nil {
			t.Fatalf("component add source: %v", err)
		}

		setOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"chat", chatID, "component", "gmail/work", "filter", "add", "filters/allowlist"}); err != nil {
				t.Fatalf("filter add: %v", err)
			}
		})
		if !strings.Contains(setOutput, "chat component filter add") || !strings.Contains(setOutput, "external_channel_id: work@example.com") || !strings.Contains(setOutput, "filter: filters/allowlist") {
			t.Fatalf("unexpected filter add output: %q", setOutput)
		}

		system, err = systempkg.Open(context.Background(), "", "", store, log.New(io.Discard, "", 0))
		if err != nil {
			t.Fatalf("reopen runtime: %v", err)
		}
		source, err := system.Storage.Components().GetByTypeAndName(context.Background(), "gmail", "work")
		if err != nil || source == nil {
			t.Fatalf("source = %#v err=%v", source, err)
		}
		sourceBinding, err := system.Storage.ChatComponents().FindByComponentRoleAndExternalChannelID(context.Background(), source.ID, coremodel.ChatComponentRoleSource, "work@example.com")
		if err != nil || sourceBinding == nil {
			t.Fatalf("source binding = %#v err=%v", sourceBinding, err)
		}
		bindings, err := system.Storage.InboundFilterBindings().ListEnabledBySourceBindingID(context.Background(), sourceBinding.ID)
		if err != nil {
			t.Fatalf("list inbound filter bindings: %v", err)
		}
		if got, want := len(bindings), 1; got != want {
			t.Fatalf("enabled filter bindings = %d, want %d", got, want)
		}

		statusOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"chat", chatID, "component", "gmail/work", "filter", "list"}); err != nil {
				t.Fatalf("filter list: %v", err)
			}
		})
		if !strings.Contains(statusOutput, "chat component filter list") || !strings.Contains(statusOutput, "filter: filters/allowlist") {
			t.Fatalf("unexpected filter list output: %q", statusOutput)
		}

		clearOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"chat", chatID, "component", "gmail/work", "filter", "clear"}); err != nil {
				t.Fatalf("filter clear: %v", err)
			}
		})
		if !strings.Contains(clearOutput, "chat component filter cleared") || !strings.Contains(clearOutput, "disabled: 1") {
			t.Fatalf("unexpected filter clear output: %q", clearOutput)
		}

		guardOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"chat", chatID, "component", "gmail/work", "filter", "add", "guard/qwen"}); err != nil {
				t.Fatalf("guard filter add: %v", err)
			}
		})
		if !strings.Contains(guardOutput, "chat component filter add") || !strings.Contains(guardOutput, "filter: guard/qwen") {
			t.Fatalf("unexpected guard filter add output: %q", guardOutput)
		}
	})
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
			ComponentID:       registration.ID,
			ExternalChannelID: "me",
			ChatLabel:         "Inbox",
			ActorLabel:        "Email",
			LastTextPreview:   "hello",
			MessageCount:      1,
			FirstSeenAt:       time.Now().Add(-time.Minute),
			LastSeenAt:        time.Now(),
		}); err != nil {
			t.Fatalf("Save drop: %v", err)
		}

		output := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"chat", "dropped"}); err != nil {
				t.Fatalf("chat dropped: %v", err)
			}
		})
		if !strings.Contains(output, "gmail") || !strings.Contains(output, "external_channel_id=me") || !strings.Contains(output, "preview=hello") {
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
			ComponentID:       registration.ID,
			ExternalChannelID: "chat-1",
			ChatLabel:         "Team room",
			LastTextPreview:   "hello",
			MessageCount:      1,
			FirstSeenAt:       time.Now().Add(-time.Minute),
			LastSeenAt:        time.Now(),
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
