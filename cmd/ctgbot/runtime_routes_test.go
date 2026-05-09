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
