package main

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"strings"
	"testing"

	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func TestV5ProfileSetAndComponentRegister(t *testing.T) {
	withTempCwd(t, func(root string) {
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewCwd: %v", err)
		}

		router := clir.New()
		registerV5Routes(router, store)

		profileOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"v5", "profile", "set", "work", "--runtime", "local", "--home-path", "profiles/work-root"}); err != nil {
				t.Fatalf("profile set: %v", err)
			}
		})
		if !strings.Contains(profileOutput, "profile saved") || !strings.Contains(profileOutput, "runtime: local") {
			t.Fatalf("unexpected profile output: %q", profileOutput)
		}

		registerOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"v5", "component", "register", "codex/work", "--profile", "work"}); err != nil {
				t.Fatalf("component register: %v", err)
			}
		})
		if !strings.Contains(registerOutput, "component registered") || !strings.Contains(registerOutput, "profile: work") || !strings.Contains(registerOutput, "runtime: local") {
			t.Fatalf("unexpected register output: %q", registerOutput)
		}

		expectedHome := filepath.Join(root, "profiles", "work-root", "components", "codex", "work")
		assertDirExists(t, expectedHome)

		system, err := v5runtime.Open(context.Background(), "", "", store, log.New(io.Discard, "", 0))
		if err != nil {
			t.Fatalf("open v5 runtime: %v", err)
		}
		componentRow, err := system.Storage.Components().GetByTypeAndName(context.Background(), "codex", "work")
		if err != nil {
			t.Fatalf("GetByTypeAndName: %v", err)
		}
		if componentRow == nil {
			t.Fatal("expected registered component")
		}
		if componentRow.Profile != "work" {
			t.Fatalf("Profile = %q, want work", componentRow.Profile)
		}
	})
}

func TestV5ChatComponentAddBindsExternalChatID(t *testing.T) {
	withTempCwd(t, func(root string) {
		_ = root
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewCwd: %v", err)
		}

		router := clir.New()
		registerV5Routes(router, store)

		if err := router.Run(context.Background(), []string{"v5", "component", "register", "telegram", "--profile", "default"}); err != nil {
			t.Fatalf("component register: %v", err)
		}
		if err := router.Run(context.Background(), []string{"v5", "chat", "create", "team"}); err != nil {
			t.Fatalf("chat create: %v", err)
		}

		system, err := v5runtime.Open(context.Background(), "", "", store, log.New(io.Discard, "", 0))
		if err != nil {
			t.Fatalf("open v5 runtime: %v", err)
		}
		chats, err := system.Storage.Chats().List(context.Background())
		if err != nil {
			t.Fatalf("list chats: %v", err)
		}
		if len(chats) != 1 {
			t.Fatalf("chat count = %d, want 1", len(chats))
		}

		bindOutput := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"v5", "chat", chats[0].ID.String(), "component", "add", "source", "telegram", "--external-chat-id", "chat-1"}); err != nil {
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
