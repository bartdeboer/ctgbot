package bridge

import (
	"context"
	"io"
	"log"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

func skipIfListenerUnavailable(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	text := err.Error()
	if strings.Contains(text, "bind: operation not permitted") || (strings.Contains(text, "listen tcp") && strings.Contains(text, "operation not permitted")) {
		t.Skipf("listener unavailable in this environment: %v", err)
	}
}

func TestDefaultListenAddressMatchesEstablishedNetworkContract(t *testing.T) {
	if got, want := DefaultListenAddress, "127.0.0.1:4568"; got != want {
		t.Fatalf("DefaultListenAddress = %q, want %q", got, want)
	}
}

func TestBridgeStartIsIdempotent(t *testing.T) {
	bridge := NewBridge(t.TempDir(), nil, log.New(io.Discard, "", 0)).WithListenAddress("127.0.0.1:0")
	t.Cleanup(func() {
		_ = bridge.Close()
	})

	container1, host1, err := bridge.Start()
	if err != nil {
		skipIfListenerUnavailable(t, err)
		t.Fatalf("Start() error = %v", err)
	}
	if container1 == "" || host1 == "" {
		t.Fatalf("Start() returned empty addresses container=%q host=%q", container1, host1)
	}

	container2, host2, err := bridge.Start()
	if err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	if container2 != container1 || host2 != host1 {
		t.Fatalf("second Start() = (%q, %q), want (%q, %q)", container2, host2, container1, host1)
	}
}

func TestActiveCommandComponentsFromEngine(t *testing.T) {
	engine := commandengine.NewEngine(nil, nil).WithActiveComponentRefs([]string{"codex/codex", "gmail/work"})
	if got, want := activeCommandComponents(engine), "codex/codex,gmail/work"; got != want {
		t.Fatalf("activeCommandComponents() = %q, want %q", got, want)
	}
}

func TestPrepareRequestUsesTLSClientIdentityAsThread(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	chat := &coremodel.Chat{Label: "chat", Enabled: true}
	if err := storage.Chats().Save(ctx, chat); err != nil {
		t.Fatalf("Save(chat) error = %v", err)
	}
	thread := &coremodel.Thread{ChatID: chat.ID, Label: "thread"}
	if err := storage.Threads().Save(ctx, thread); err != nil {
		t.Fatalf("Save(thread) error = %v", err)
	}
	bridge := NewBridge(t.TempDir(), storage, log.New(io.Discard, "", 0))

	req, err := bridge.prepareRequest(ctx, thread.ID.String(), commandengine.Request{})
	if err != nil {
		t.Fatalf("prepareRequest() error = %v", err)
	}
	if req.Context.ThreadID != thread.ID || req.Context.SandboxID != thread.ID {
		t.Fatalf("thread ids = (%s, %s), want %s", req.Context.ThreadID, req.Context.SandboxID, thread.ID)
	}
	if req.Context.ChatID != chat.ID {
		t.Fatalf("ChatID = %s, want %s", req.Context.ChatID, chat.ID)
	}
	if req.Context.Actor.ID != thread.ID.String() {
		t.Fatalf("Actor.ID = %q, want client identity", req.Context.Actor.ID)
	}
}

func TestPrepareRequestRejectsClaimedThreadMismatch(t *testing.T) {
	ctx := context.Background()
	bridge := NewBridge(t.TempDir(), repository.NewMemory(), log.New(io.Discard, "", 0))
	authenticated := modeluuid.New()
	claimed := modeluuid.New()

	_, err := bridge.prepareRequest(ctx, authenticated.String(), commandengine.Request{
		Context: commandengine.Context{SandboxID: claimed},
	})
	if err == nil || !strings.Contains(err.Error(), "client identity does not match claimed sandbox id") {
		t.Fatalf("prepareRequest() error = %v, want sandbox mismatch", err)
	}
}
