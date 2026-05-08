package bridge

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	hostbridgeapi "github.com/bartdeboer/ctgbot/internal/hostbridge"
	hostbridgeclient "github.com/bartdeboer/ctgbot/internal/hostbridge/client"
	_ "github.com/bartdeboer/ctgbot/internal/hostbridge/gobregister"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	hostbridgetls "github.com/bartdeboer/ctgbot/internal/hostbridge/tls"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

const TLSDir = "/ctgbot/hostbridge-tls"

// Match the established Docker Desktop/WSL contract:
// listen on host loopback and advertise host.docker.internal to containers.
const DefaultListenAddress = "127.0.0.1:4568"

type Bridge struct {
	stateRoot string
	storage   repository.Storage
	logger    *log.Logger
	listen    string

	mu               sync.Mutex
	entries          map[modeluuid.UUID]*threadEntry
	started          bool
	closed           bool
	hostAddress      string
	containerAddress string
	cancel           context.CancelFunc
}

type threadEntry struct {
	commands commandengine.CommandExecutor
	refs     int
}

func NewBridge(stateRoot string, storage repository.Storage, logger *log.Logger) *Bridge {
	return &Bridge{
		stateRoot: strings.TrimSpace(stateRoot),
		storage:   storage,
		logger:    logger,
		entries:   map[modeluuid.UUID]*threadEntry{},
	}
}

func (b *Bridge) WithListenAddress(address string) *Bridge {
	if b != nil {
		b.listen = strings.TrimSpace(address)
	}
	return b
}

func (b *Bridge) Start() (containerAddress string, hostAddress string, err error) {
	if b == nil {
		return "", "", fmt.Errorf("missing hostbridge")
	}
	return b.ensureStarted()
}

func (b *Bridge) BindThread(
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
) ([]string, sandboxengine.Mount, func(), error) {
	address, _, tlsDir, unregister, err := b.bindThread(threadID, commands)
	if err != nil {
		return nil, sandboxengine.Mount{}, nil, err
	}
	env := []string{
		"HOSTBRIDGE_ADDR=" + address,
		"HOSTBRIDGE_TLS_DIR=" + TLSDir,
		"CTGBOT_SANDBOX_ID=" + threadID.String(),
	}
	mount := sandboxengine.Mount{
		Source:   tlsDir,
		Target:   TLSDir,
		ReadOnly: true,
	}
	return env, mount, unregister, nil
}

func (b *Bridge) DoCommand(
	ctx context.Context,
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	req commandengine.Request,
) (commandengine.Result, error) {
	_, address, tlsDir, unregister, err := b.bindThread(threadID, commands)
	if err != nil {
		return commandengine.Result{}, err
	}
	defer unregister()

	if req.Context.SandboxID.IsNull() {
		req.Context.SandboxID = threadID
	}
	resp, err := hostbridgeclient.DoCommand(ctx, address, tlsDir, hostbridgeapi.CommandRequest{Request: req})
	if err != nil {
		return commandengine.Result{}, err
	}
	return resp.Result, nil
}

func (b *Bridge) Close() error {
	if b == nil {
		return nil
	}

	b.mu.Lock()
	cancel := b.cancel
	b.cancel = nil
	b.started = false
	b.closed = true
	b.entries = map[modeluuid.UUID]*threadEntry{}
	b.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	return nil
}

func (b *Bridge) Execute(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	if b == nil {
		return commandengine.Result{}, fmt.Errorf("missing hostbridge")
	}
	threadID := req.Context.ThreadID
	if threadID.IsNull() {
		threadID = req.Context.SandboxID
	}
	if threadID.IsNull() {
		return commandengine.Result{}, fmt.Errorf("missing thread id")
	}

	b.mu.Lock()
	entry := b.entries[threadID]
	b.mu.Unlock()
	if entry == nil || entry.commands == nil {
		return commandengine.Result{}, fmt.Errorf("hostbridge command executor is unavailable for thread %s", threadID)
	}
	return entry.commands.Execute(ctx, req)
}

func (b *Bridge) prepareRequest(
	ctx context.Context,
	clientIdentity string,
	req commandengine.Request,
) (commandengine.Request, error) {
	req.Context.Source = commandengine.SourceHostbridge
	req.Context.Actor = commandengine.Actor{
		ID:    clientIdentity,
		Roles: []simplerbac.Role{simplerbac.RoleAgent},
	}

	threadID := req.Context.ThreadID
	if threadID.IsNull() {
		threadID = req.Context.SandboxID
	}
	if threadID.IsNull() {
		return req, nil
	}
	req.Context.ThreadID = threadID

	if !req.Context.ChatID.IsNull() {
		return req, nil
	}
	if b == nil || b.storage == nil {
		return req, fmt.Errorf("missing hostbridge storage")
	}

	thread, err := b.storage.Threads().GetByID(ctx, threadID)
	if err != nil {
		return commandengine.Request{}, err
	}
	if thread == nil {
		return commandengine.Request{}, fmt.Errorf("thread not found: %s", threadID)
	}
	req.Context.ChatID = thread.ChatID
	return req, nil
}

func (b *Bridge) bindThread(
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
) (containerAddress string, hostAddress string, tlsDir string, unregister func(), err error) {
	if b == nil {
		return "", "", "", nil, fmt.Errorf("missing hostbridge")
	}
	if threadID.IsNull() {
		return "", "", "", nil, fmt.Errorf("missing thread id")
	}

	containerAddress, hostAddress, err = b.ensureStarted()
	if err != nil {
		return "", "", "", nil, err
	}
	tlsDir, err = b.ensureClientTLSDir(threadID)
	if err != nil {
		return "", "", "", nil, err
	}
	unregister = b.register(threadID, commands)
	return containerAddress, hostAddress, tlsDir, unregister, nil
}

func (b *Bridge) ensureStarted() (containerAddress string, hostAddress string, err error) {
	if b == nil {
		return "", "", fmt.Errorf("missing hostbridge")
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return "", "", fmt.Errorf("hostbridge is closed")
	}
	if b.started {
		containerAddress = b.containerAddress
		hostAddress = b.hostAddress
		b.mu.Unlock()
		return containerAddress, hostAddress, nil
	}
	b.mu.Unlock()

	root := b.serverRoot()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", "", err
	}
	if err := hostbridgetls.EnsureServerMaterials(root); err != nil {
		return "", "", err
	}
	tlsConfig, err := hostbridgetls.LoadServerTLSConfig(root)
	if err != nil {
		return "", "", err
	}
	listenAddress := strings.TrimSpace(b.listen)
	if listenAddress == "" {
		listenAddress = "0.0.0.0:0"
	}
	ln, err := hostbridgeserver.ListenTLS(listenAddress, tlsConfig)
	if err != nil {
		return "", "", err
	}
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		_ = ln.Close()
		return "", "", err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	srv := hostbridgeserver.NewCommandServer(b)
	srv.Prepare = b.prepareRequest
	go func() {
		err := hostbridgeserver.ServeCommandListener(runCtx, ln, srv)
		if err != nil && runCtx.Err() == nil {
			b.logf("hostbridge serve error: %v", err)
		}
	}()

	containerAddress = net.JoinHostPort(hostbridgetls.ServerName, port)
	hostAddress = net.JoinHostPort("127.0.0.1", port)
	b.logf("hostbridge listening host=%s container=%s", hostAddress, containerAddress)

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		cancel()
		return "", "", fmt.Errorf("hostbridge is closed")
	}
	if b.started {
		cancel()
		return b.containerAddress, b.hostAddress, nil
	}
	b.started = true
	b.cancel = cancel
	b.containerAddress = containerAddress
	b.hostAddress = hostAddress
	return containerAddress, hostAddress, nil
}

func (b *Bridge) ensureClientTLSDir(threadID modeluuid.UUID) (string, error) {
	if b == nil {
		return "", fmt.Errorf("missing hostbridge")
	}
	dir := filepath.Join(b.serverRoot(), "clients", threadID.String())
	if err := hostbridgetls.EnsureChatClientMaterials(b.serverRoot(), dir, threadID.String()); err != nil {
		return "", err
	}
	return dir, nil
}

func (b *Bridge) register(threadID modeluuid.UUID, commands commandengine.CommandExecutor) func() {
	b.mu.Lock()
	entry := b.entries[threadID]
	if entry == nil {
		entry = &threadEntry{}
		b.entries[threadID] = entry
	}
	entry.commands = commands
	entry.refs++
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		current := b.entries[threadID]
		if current == nil {
			return
		}
		current.refs--
		if current.refs <= 0 {
			delete(b.entries, threadID)
		}
	}
}

func (b *Bridge) serverRoot() string {
	stateRoot := strings.TrimSpace(b.stateRoot)
	if stateRoot == "" {
		stateRoot = filepath.Join(".", ".ctgbot")
	}
	return filepath.Join(stateRoot, "hostbridge")
}

func (b *Bridge) logf(format string, args ...any) {
	if b != nil && b.logger != nil {
		b.logger.Printf(format, args...)
	}
}
