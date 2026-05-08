package main

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"strings"
	"testing"

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
