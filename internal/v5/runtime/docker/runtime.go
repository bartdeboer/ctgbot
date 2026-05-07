package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	v5hostbridgeserver "github.com/bartdeboer/ctgbot/internal/v5/hostbridge/server"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
)

const dockerDefaultImage = "ctgbot-codex:latest"

type Factory struct {
	rootDir        string
	componentsRoot string
	sandboxes      sandboxengine.RuntimeManager
	bridge         *v5hostbridgeserver.Bridge
}

func New(rootDir string, componentsRoot string, sandboxes sandboxengine.RuntimeManager, bridge *v5hostbridgeserver.Bridge) *Factory {
	return &Factory{
		rootDir:        strings.TrimSpace(rootDir),
		componentsRoot: strings.TrimSpace(componentsRoot),
		sandboxes:      sandboxes,
		bridge:         bridge,
	}
}

func (f *Factory) Kind() string {
	return "docker"
}

func (f *Factory) ComponentHome(registration coremodel.Component) v5runtime.Home {
	hostPath := strings.TrimSpace(registration.HomePath)
	if hostPath == "" {
		hostPath = filepath.Join(f.componentsRoot, registration.Type, registration.Name)
	}
	return v5runtime.Home{Path: hostPath}
}

func (f *Factory) RuntimeComponentHomePath(registration coremodel.Component, home v5runtime.Home) string {
	_ = home
	return componentRuntimeHomePath(registration)
}

func (f *Factory) RuntimeWorkspacePath(workspacePath string) string {
	_ = workspacePath
	return v5runtime.DefaultWorkspaceRuntimePath
}

func (f *Factory) Bind(
	registration coremodel.Component,
	home v5runtime.Home,
	config v5runtime.BindConfig,
) v5runtime.Runtime {
	return &Runtime{
		rootDir:      f.rootDir,
		sandboxes:    f.sandboxes,
		bridge:       f.bridge,
		registration: registration,
		home:         home,
		image:        resolveImage(config.Image),
		env:          append([]string{}, config.Env...),
		gpus:         strings.TrimSpace(config.GPUs),
	}
}

type Runtime struct {
	rootDir      string
	sandboxes    sandboxengine.RuntimeManager
	bridge       *v5hostbridgeserver.Bridge
	registration coremodel.Component
	home         v5runtime.Home
	image        string
	env          []string
	gpus         string
}

func (r *Runtime) Kind() string {
	return "docker"
}

func (r *Runtime) ComponentHome() v5runtime.Home {
	return r.home
}

func (r *Runtime) RuntimeComponentHomePath() string {
	if r == nil {
		return componentRuntimeHomePath(coremodel.Component{})
	}
	return componentRuntimeHomePath(r.registration)
}

func (r *Runtime) RuntimeWorkspacePath(workspacePath string) string {
	_ = workspacePath
	return v5runtime.DefaultWorkspaceRuntimePath
}

func (r *Runtime) Refresh(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
) error {
	sbx, cleanup, err := r.sandbox(workspacePath, threadID, nil, false)
	if err != nil {
		return err
	}
	defer cleanup()
	return sbx.Remove(ctx)
}

func (r *Runtime) Start(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
) (v5runtime.Status, error) {
	sbx, cleanup, err := r.sandbox(workspacePath, threadID, nil, true)
	if err != nil {
		return v5runtime.Status{}, err
	}
	defer cleanup()
	if _, err := sbx.Ensure(ctx); err != nil {
		return v5runtime.Status{}, err
	}
	return r.statusForSandbox(ctx, workspacePath, sbx)
}

func (r *Runtime) Stop(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
) error {
	sbx, cleanup, err := r.sandbox(workspacePath, threadID, nil, false)
	if err != nil {
		return err
	}
	defer cleanup()
	return sbx.Stop(ctx)
}

func (r *Runtime) Interrupt(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
) (bool, error) {
	sbx, cleanup, err := r.sandbox(workspacePath, threadID, nil, false)
	if err != nil {
		return false, err
	}
	defer cleanup()
	if _, ok := sbx.ActiveCommand(); !ok {
		return false, nil
	}
	return true, sbx.Interrupt()
}

func (r *Runtime) Status(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
) (v5runtime.Status, error) {
	sbx, cleanup, err := r.sandbox(workspacePath, threadID, nil, false)
	if err != nil {
		return v5runtime.Status{}, err
	}
	defer cleanup()
	return r.statusForSandbox(ctx, workspacePath, sbx)
}

func (r *Runtime) Exec(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	stdout io.Writer,
	stderr io.Writer,
	name string,
	args ...string,
) error {
	sbx, cleanup, err := r.sandbox(workspacePath, threadID, commands, true)
	if err != nil {
		return err
	}
	defer cleanup()
	err = sbx.Exec(ctx, stdout, stderr, name, args...)
	if err != nil && sbx.Interrupted() {
		return context.Canceled
	}
	return err
}

func (r *Runtime) CombinedOutput(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	name string,
	args ...string,
) ([]byte, error) {
	sbx, cleanup, err := r.sandbox(workspacePath, threadID, commands, true)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	out, err := sbx.CombinedOutput(ctx, name, args...)
	if err != nil && sbx.Interrupted() {
		return nil, context.Canceled
	}
	return out, err
}

func (r *Runtime) OpenHTTPRelayPort(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	callbackPort int,
	callbackTimeout time.Duration,
) (func(context.Context) error, error) {
	sbx, cleanup, err := r.sandbox(workspacePath, threadID, commands, true)
	if err != nil {
		return nil, err
	}
	if _, err := sbx.Ensure(ctx); err != nil {
		cleanup()
		return nil, err
	}
	relay, err := sbx.OpenHTTPRelayPort(ctx, callbackPort, callbackTimeout)
	if err != nil {
		cleanup()
		return nil, err
	}
	return func(closeCtx context.Context) error {
		defer cleanup()
		return relay.Close(closeCtx)
	}, nil
}

func (r *Runtime) sandbox(
	workspacePath string,
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	prepareBridge bool,
) (*sandboxengine.Sandbox, func(), error) {
	if r == nil || r.sandboxes == nil {
		return nil, nil, fmt.Errorf("missing docker runtime")
	}
	if threadID.IsNull() {
		runtimeHomePath := r.RuntimeComponentHomePath()
		spec := sandboxengine.NewBuilder(authSandboxName(r.registration)).
			Image(r.image).
			Workdir(runtimeHomePath).
			// Keep mounted profile files writable by the host ctgbot process.
			UserMode("host").
			GPUs(r.gpus).
			Env(append([]string{}, r.env...)).
			Mounts([]sandboxengine.Mount{{Source: r.home.Path, Target: runtimeHomePath}}).
			SecurityOpts([]string{"seccomp=unconfined"}).
			AddHosts(sandboxAddHosts()).
			Cmd([]string{"tail", "-f", "/dev/null"}).
			Build()
		return r.sandboxes.CreateSandbox(spec), func() {}, nil
	}

	workspaceHost, workspaceRuntime, err := resolveWorkspace(workspacePath)
	if err != nil {
		return nil, nil, err
	}
	runtimeHomePath := r.RuntimeComponentHomePath()
	env := append([]string{}, r.env...)
	env = append(env, "CTGBOT_COMPONENT_REF="+r.registration.Ref())
	mounts := []sandboxengine.Mount{
		{Source: r.home.Path, Target: runtimeHomePath},
		{Source: workspaceHost, Target: workspaceRuntime},
	}
	cleanup := func() {}
	if prepareBridge && r.bridge != nil {
		bridgeEnv, bridgeMount, unregister, err := r.bridge.BindThread(threadID, commands)
		if err != nil {
			return nil, nil, err
		}
		env = append(env, bridgeEnv...)
		mounts = append(mounts, bridgeMount)
		cleanup = unregister
	}
	spec := sandboxengine.NewBuilder(turnSandboxName(r.registration, threadID)).
		WorkspaceDir(workspaceHost).
		ProfileDir(r.home.Path).
		ContainerWorkspace(workspaceRuntime).
		ContainerHome(runtimeHomePath).
		Hostname(turnSandboxName(r.registration, threadID)).
		Image(r.image).
		Workdir(workspaceRuntime).
		// Keep mounted profile/workspace files writable by the host ctgbot process.
		UserMode("host").
		GPUs(r.gpus).
		Env(env).
		Mounts(mounts).
		SecurityOpts([]string{"seccomp=unconfined"}).
		AddHosts(sandboxAddHosts()).
		Cmd([]string{"tail", "-f", "/dev/null"}).
		Build()
	return r.sandboxes.CreateSandbox(spec), cleanup, nil
}

func sandboxAddHosts() []string {
	if goruntime.GOOS != "linux" {
		return nil
	}
	return []string{"host.docker.internal:host-gateway"}
}

func (r *Runtime) statusForSandbox(ctx context.Context, workspacePath string, sbx *sandboxengine.Sandbox) (v5runtime.Status, error) {
	if sbx == nil {
		return v5runtime.Status{}, fmt.Errorf("missing sandbox")
	}
	state, err := sbx.InspectState(ctx)
	if err != nil {
		return v5runtime.Status{}, err
	}
	status := v5runtime.Status{
		Name:                 sbx.Name,
		State:                string(state),
		RuntimeHomePath:      r.RuntimeComponentHomePath(),
		RuntimeWorkspacePath: r.RuntimeWorkspacePath(workspacePath),
	}
	if active, ok := sbx.ActiveCommand(); ok {
		status.ActiveCommandName = active.Name
		status.ActiveCommandArgs = append([]string(nil), active.Args...)
	}
	return status, nil
}

func authSandboxName(registration coremodel.Component) string {
	return safeName("ctgbot-v5-auth-"+registration.Ref(), "ctgbot-v5-auth")
}

func turnSandboxName(registration coremodel.Component, threadID modeluuid.UUID) string {
	return safeName("ctgbot-v5-"+registration.Ref()+"-"+threadID.String(), "ctgbot-v5-runtime")
}

func resolveImage(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return dockerDefaultImage
	}
	return value
}

func resolveWorkspace(workspacePath string) (string, string, error) {
	hostPath := strings.TrimSpace(workspacePath)
	if hostPath == "" {
		return "", "", fmt.Errorf("missing workspace host path")
	}
	if err := os.MkdirAll(filepath.Join(hostPath, "inbox"), 0o755); err != nil {
		return "", "", err
	}
	return hostPath, v5runtime.DefaultWorkspaceRuntimePath, nil
}

func componentRuntimeHomePath(registration coremodel.Component) string {
	return "/profile/components/" + registration.Type + "/" + registration.Name
}

func safeName(value string, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return fallback
	}
	var b strings.Builder
	prevDash := false
	for _, r := range value {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fallback
	}
	return out
}
