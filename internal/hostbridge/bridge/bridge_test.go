package bridge

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	hostbridgetls "github.com/bartdeboer/ctgbot/internal/hostbridge/tls"
	"github.com/bartdeboer/ctgbot/internal/hostbridge/transport"
	hostbridgev2 "github.com/bartdeboer/ctgbot/internal/hostbridge/v2"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
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

func TestHostbridgeHTTPListenAddressUsesEphemeralPort(t *testing.T) {
	if got, want := hostbridgeHTTPListenAddress("127.0.0.1:4567"), "127.0.0.1:0"; got != want {
		t.Fatalf("hostbridgeHTTPListenAddress() = %q, want %q", got, want)
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
	if bridge.containerHTTPURL == "" || bridge.hostHTTPAddress == "" {
		t.Fatalf("Start() did not initialize v2 addresses container=%q host=%q", bridge.containerHTTPURL, bridge.hostHTTPAddress)
	}

	container2, host2, err := bridge.Start()
	if err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	if container2 != container1 || host2 != host1 {
		t.Fatalf("second Start() = (%q, %q), want (%q, %q)", container2, host2, container1, host1)
	}
}

func TestBridgeServesHostbridgeV2HTTPCommands(t *testing.T) {
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

	router, err := commandengine.NewRouter([]commandengine.Definition{
		{
			Pattern: "echo",
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleAgent),
			Build: func(req *clir.Request) (any, error) {
				return testEchoCommand{}, nil
			},
		},
	}, commandengine.SourceHostbridge)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	registry := commandengine.NewRegistry()
	if err := commandengine.Register[testEchoCommand](registry, func(ctx context.Context, req commandengine.Request, cmd testEchoCommand) (commandengine.Result, error) {
		if req.Context.ThreadID != thread.ID || req.Context.SandboxID != thread.ID || req.Context.ChatID != chat.ID {
			t.Fatalf("context = chat:%s thread:%s sandbox:%s", req.Context.ChatID, req.Context.ThreadID, req.Context.SandboxID)
		}
		return commandengine.Result{Text: "v2:" + req.Stdin}, nil
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	engine := commandengine.NewEngine(router, registry)
	bridge := NewBridge(t.TempDir(), storage, log.New(io.Discard, "", 0)).WithListenAddress("127.0.0.1:0")
	t.Cleanup(func() {
		_ = bridge.Close()
	})

	_, _, _, tlsDir, unregister, err := bridge.bindThread(thread.ID, engine)
	if err != nil {
		skipIfListenerUnavailable(t, err)
		t.Fatalf("bindThread() error = %v", err)
	}
	defer unregister()

	tlsConfig, err := hostbridgetls.LoadClientTLSConfig(tlsDir)
	if err != nil {
		t.Fatalf("LoadClientTLSConfig() error = %v", err)
	}
	client := &hostbridgev2.Client{
		BaseURL:    "https://" + bridge.hostHTTPAddress,
		HTTPClient: hostbridgev2HTTPClient(tlsConfig),
	}
	resp, err := client.Run(ctx, hostbridgev2.RunRequest{
		Command:   []string{"echo"},
		Stdin:     "hello",
		SandboxID: thread.ID,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if resp.Text != "v2:hello" {
		t.Fatalf("response text = %q, want v2:hello", resp.Text)
	}
}

type testEchoCommand struct {
	Text string
}

func hostbridgev2HTTPClient(tlsConfig *tls.Config) *http.Client {
	return &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig}}
}

func TestActiveCommandComponentsFromEngine(t *testing.T) {
	engine := commandengine.NewEngine(nil, nil).WithActiveComponentRefs([]string{"codex/codex", "gmail/work"})
	if got, want := activeCommandComponents(engine), "codex/codex,gmail/work"; got != want {
		t.Fatalf("activeCommandComponents() = %q, want %q", got, want)
	}
}

func TestActiveCommandComponentsFromWrappedExecutor(t *testing.T) {
	commands := activeComponentExecutor{refs: []string{"codex/codex", "gmailv2/personal"}}
	if got, want := activeCommandComponents(commands), "codex/codex,gmailv2/personal"; got != want {
		t.Fatalf("activeCommandComponents() = %q, want %q", got, want)
	}
}

type activeComponentExecutor struct {
	refs []string
}

func (e activeComponentExecutor) ActiveComponents() []string {
	return append([]string(nil), e.refs...)
}

func (e activeComponentExecutor) Execute(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	_, _ = ctx, req
	return commandengine.Result{}, nil
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

	req, err := bridge.prepareRequest(ctx, transport.PeerIdentity{CommonName: thread.ID.String()}, commandengine.Request{})
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

	_, err := bridge.prepareRequest(ctx, transport.PeerIdentity{CommonName: authenticated.String()}, commandengine.Request{
		Context: commandengine.Context{SandboxID: claimed},
	})
	if err == nil || !strings.Contains(err.Error(), "client identity does not match claimed sandbox id") {
		t.Fatalf("prepareRequest() error = %v, want sandbox mismatch", err)
	}
}
