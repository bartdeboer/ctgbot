package integration

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type runtimeState struct {
	mu           sync.Mutex
	execCalls    int
	lastThreadID modeluuid.UUID
	lastName     string
	lastArgs     []string
}

type fakeRuntimeFactory struct {
	profile v5runtime.Profile
	rootDir string
	state   *runtimeState
}

func (f fakeRuntimeFactory) Kind() string {
	return f.profile.Runtime
}

func (f fakeRuntimeFactory) Profile() v5runtime.Profile {
	return f.profile
}

func (f fakeRuntimeFactory) ComponentHome(registration coremodel.Component) v5runtime.Home {
	return v5runtime.Home{
		HostPath:      filepath.Join(f.profile.Root, "components", registration.Type, registration.Name),
		ContainerPath: "/profile/components/" + registration.Type + "/" + registration.Name,
	}
}

func (f fakeRuntimeFactory) Bind(registration coremodel.Component, home v5runtime.Home, image string, env []string) v5runtime.Runtime {
	_, _, _ = registration, image, env
	return &fakeRuntime{
		rootDir: f.rootDir,
		profile: f.profile,
		home:    home,
		state:   f.state,
	}
}

type fakeRuntime struct {
	rootDir string
	profile v5runtime.Profile
	home    v5runtime.Home
	state   *runtimeState
}

func (r *fakeRuntime) Kind() string {
	return r.profile.Runtime
}

func (r *fakeRuntime) Profile() v5runtime.Profile {
	return r.profile
}

func (r *fakeRuntime) ComponentHome() v5runtime.Home {
	return r.home
}

func (r *fakeRuntime) ThreadWorkspace(threadID modeluuid.UUID) (string, string, error) {
	if threadID.IsNull() {
		return "", "", fmt.Errorf("missing thread id")
	}
	hostPath := filepath.Join(r.rootDir, ".ctgbot", "threads", threadID.String(), "workspace")
	if err := os.MkdirAll(filepath.Join(hostPath, "inbox"), 0o755); err != nil {
		return "", "", err
	}
	return hostPath, "/workspace", nil
}

func (r *fakeRuntime) Exec(ctx context.Context, threadID modeluuid.UUID, commands commandengine.CommandExecutor, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	_, _, _, _ = ctx, commands, stdout, stderr
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	r.state.execCalls++
	r.state.lastThreadID = threadID
	r.state.lastName = name
	r.state.lastArgs = append([]string(nil), args...)
	return nil
}

func (r *fakeRuntime) CombinedOutput(ctx context.Context, threadID modeluuid.UUID, commands commandengine.CommandExecutor, name string, args ...string) ([]byte, error) {
	_, _, _, _, _ = ctx, threadID, commands, name, args
	return []byte("ok"), nil
}

func (r *fakeRuntime) OpenHTTPRelayPort(ctx context.Context, threadID modeluuid.UUID, commands commandengine.CommandExecutor, callbackPort int, callbackTimeout time.Duration) (func(context.Context) error, error) {
	_, _, _, _, _ = ctx, threadID, commands, callbackPort, callbackTimeout
	return func(context.Context) error { return nil }, nil
}

type messengerState struct {
	mu            sync.Mutex
	runCalls      int
	event         component.InboundEvent
	relayPayloads []messenger.OutboundPayload
}

type mockMessenger struct {
	componentID modeluuid.UUID
	state       *messengerState
}

func (m *mockMessenger) Type() string {
	return "mockmsg"
}

func (m *mockMessenger) RunInbound(ctx context.Context, emit component.InboundEmitter) error {
	m.state.mu.Lock()
	m.state.runCalls++
	event := m.state.event
	m.state.mu.Unlock()

	event.ComponentID = m.componentID
	return emit(ctx, event)
}

func (m *mockMessenger) Send(ctx context.Context, payload messenger.OutboundPayload) error {
	_ = ctx
	m.state.mu.Lock()
	defer m.state.mu.Unlock()
	m.state.relayPayloads = append(m.state.relayPayloads, payload)
	return nil
}

func (m *mockMessenger) StartChatAction(ctx context.Context, target messenger.ChatTarget, action messenger.ChatAction) (func(), error) {
	_, _, _ = ctx, target, action
	return func() {}, nil
}

type agentState struct {
	mu        sync.Mutex
	authCalls int
	turnCalls int
	prompt    string
}

func newSQLiteStorage(t *testing.T) repository.Storage {
	t.Helper()
	name := strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%s?mode=memory&cache=shared", name, modeluuid.New().String())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	storage := repository.NewGORM(db)
	if err := storage.AutoMigrate(context.Background()); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	return storage
}

func withTempCwd(t *testing.T, fn func(root string)) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir(%q) error = %v", root, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})
	fn(root)
}
