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
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5gormstorage "github.com/bartdeboer/ctgbot/internal/v5/repository/gormstorage"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type runtimeState struct {
	mu           sync.Mutex
	execCalls    int
	stopCalls    int
	lastThreadID modeluuid.UUID
	lastName     string
	lastArgs     []string
	execs        []execRecord
}

func actorWithRoles(id string, label string, roles ...simplerbac.Role) messenger.Actor {
	id = strings.TrimSpace(id)
	label = strings.TrimSpace(label)
	if id == "" {
		id = label
	}
	if label == "" {
		label = id
	}
	if len(roles) == 0 {
		roles = []simplerbac.Role{simplerbac.RoleUser}
	}
	return messenger.Actor{
		ID:    id,
		Label: label,
		Roles: append([]simplerbac.Role(nil), roles...),
	}
}

type execRecord struct {
	ThreadID     modeluuid.UUID
	Name         string
	Args         []string
	HomeHostPath string
	RuntimeKind  string
	Workspace    string
}

type fakeRuntimeFactory struct {
	runtimeKind    string
	rootDir        string
	componentsRoot string
	state          *runtimeState
}

func (f fakeRuntimeFactory) Kind() string {
	if strings.TrimSpace(f.runtimeKind) == "" {
		return "local"
	}
	return strings.TrimSpace(f.runtimeKind)
}

func (f fakeRuntimeFactory) ComponentHome(registration coremodel.Component) v5runtime.Home {
	hostPath := strings.TrimSpace(registration.HomePath)
	if hostPath == "" {
		hostPath = filepath.Join(f.componentsRoot, registration.Type, registration.Name)
	}
	return v5runtime.Home{Path: hostPath}
}

func (f fakeRuntimeFactory) RuntimeComponentHomePath(registration coremodel.Component, home v5runtime.Home) string {
	_, _ = registration, home
	return home.Path
}

func (f fakeRuntimeFactory) RuntimeWorkspacePath(workspacePath string) string {
	return strings.TrimSpace(workspacePath)
}

func (f fakeRuntimeFactory) Bind(registration coremodel.Component, home v5runtime.Home, config v5runtime.BindConfig) v5runtime.Runtime {
	_, _, _ = registration, home, config
	return &fakeRuntime{
		rootDir: f.rootDir,
		kind:    f.Kind(),
		home:    home,
		state:   f.state,
	}
}

type fakeRuntime struct {
	rootDir string
	kind    string
	home    v5runtime.Home
	state   *runtimeState
}

func (r *fakeRuntime) Kind() string {
	if strings.TrimSpace(r.kind) == "" {
		return "local"
	}
	return strings.TrimSpace(r.kind)
}

func (r *fakeRuntime) ComponentHome() v5runtime.Home {
	return r.home
}

func (r *fakeRuntime) RuntimeComponentHomePath() string {
	return r.home.Path
}

func (r *fakeRuntime) RuntimeWorkspacePath(workspacePath string) string {
	return strings.TrimSpace(workspacePath)
}
func (r *fakeRuntime) Refresh(ctx context.Context, workspacePath string, threadID modeluuid.UUID) error {
	_, _, _ = ctx, workspacePath, threadID
	return nil
}
func (r *fakeRuntime) Start(ctx context.Context, workspacePath string, threadID modeluuid.UUID) (v5runtime.Status, error) {
	_, _, _ = ctx, workspacePath, threadID
	return v5runtime.Status{
		Name:                 "fake-runtime",
		State:                "running",
		RuntimeHomePath:      r.home.Path,
		RuntimeWorkspacePath: strings.TrimSpace(workspacePath),
	}, nil
}
func (r *fakeRuntime) Stop(ctx context.Context, workspacePath string, threadID modeluuid.UUID) error {
	_, _, _ = ctx, workspacePath, threadID
	r.state.mu.Lock()
	r.state.stopCalls++
	r.state.mu.Unlock()
	return nil
}
func (r *fakeRuntime) Interrupt(ctx context.Context, workspacePath string, threadID modeluuid.UUID) (bool, error) {
	_, _, _ = ctx, workspacePath, threadID
	return false, nil
}
func (r *fakeRuntime) Status(ctx context.Context, workspacePath string, threadID modeluuid.UUID) (v5runtime.Status, error) {
	_, _, _ = ctx, workspacePath, threadID
	return v5runtime.Status{
		Name:                 "fake-runtime",
		State:                "missing",
		RuntimeHomePath:      r.home.Path,
		RuntimeWorkspacePath: strings.TrimSpace(workspacePath),
	}, nil
}

func (r *fakeRuntime) Exec(ctx context.Context, workspacePath string, threadID modeluuid.UUID, commands commandengine.CommandExecutor, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	_, _, _, _ = ctx, commands, stdout, stderr
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	r.state.execCalls++
	r.state.lastThreadID = threadID
	r.state.lastName = name
	r.state.lastArgs = append([]string(nil), args...)
	r.state.execs = append(r.state.execs, execRecord{
		ThreadID:     threadID,
		Name:         name,
		Args:         append([]string(nil), args...),
		HomeHostPath: r.home.Path,
		RuntimeKind:  r.Kind(),
		Workspace:    workspacePath,
	})
	return nil
}

func (r *fakeRuntime) CombinedOutput(ctx context.Context, workspacePath string, threadID modeluuid.UUID, commands commandengine.CommandExecutor, name string, args ...string) ([]byte, error) {
	_, _, _, _, _, _ = ctx, workspacePath, threadID, commands, name, args
	return []byte("ok"), nil
}

func (r *fakeRuntime) OpenHTTPRelayPort(ctx context.Context, workspacePath string, threadID modeluuid.UUID, commands commandengine.CommandExecutor, callbackPort int, callbackTimeout time.Duration) (func(context.Context) error, error) {
	_, _, _, _, _, _ = ctx, workspacePath, threadID, commands, callbackPort, callbackTimeout
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
	dsn := fmt.Sprintf("file:%s-%s?mode=memory&cache=shared&_busy_timeout=5000", name, modeluuid.New().String())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB() error = %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	storage := v5gormstorage.New(db)
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
