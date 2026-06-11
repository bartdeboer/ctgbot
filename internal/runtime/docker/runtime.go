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

	"github.com/bartdeboer/ctgbot/internal/buildassets"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/containerengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	hostbridgebridge "github.com/bartdeboer/ctgbot/internal/hostbridge/bridge"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

const dockerDefaultImage = "ctgbot-codex:latest"

type Factory struct {
	rootDir        string
	componentsRoot string
	sandboxes      sandboxengine.RuntimeManager
	bridge         *hostbridgebridge.Bridge
	env            []string
	threadConfig   runtimepkg.ThreadConfigResolver
}

func New(rootDir string, componentsRoot string, sandboxes sandboxengine.RuntimeManager, bridge *hostbridgebridge.Bridge) *Factory {
	return &Factory{
		rootDir:        strings.TrimSpace(rootDir),
		componentsRoot: strings.TrimSpace(componentsRoot),
		sandboxes:      sandboxes,
		bridge:         bridge,
	}
}

func (f *Factory) SandboxManager() sandboxengine.RuntimeManager {
	if f == nil {
		return nil
	}
	return f.sandboxes
}

func (f *Factory) WithEnv(env ...string) *Factory {
	if f == nil {
		return nil
	}
	clone := *f
	clone.env = runtimepkg.MergeEnv(clone.env, env)
	return &clone
}

func (f *Factory) WithThreadConfigResolver(resolver runtimepkg.ThreadConfigResolver) *Factory {
	if f == nil {
		return nil
	}
	clone := *f
	clone.threadConfig = resolver
	return &clone
}

func (f *Factory) Kind() string {
	return "docker"
}

func (f *Factory) ComponentHome(registration coremodel.Component) runtimepkg.Home {
	hostPath := strings.TrimSpace(registration.HomePath)
	if hostPath == "" {
		hostPath = filepath.Join(f.componentsRoot, registration.Type, registration.Name)
	}
	return runtimepkg.Home{Path: hostPath}
}

func (f *Factory) RuntimeComponentHomePath(registration coremodel.Component, home runtimepkg.Home) string {
	_ = home
	return componentRuntimeHomePath(registration)
}

func (f *Factory) RuntimeWorkspacePath(workspacePath string) string {
	_ = workspacePath
	return runtimepkg.DefaultWorkspaceRuntimePath
}

func (f *Factory) Bind(
	registration coremodel.Component,
	home runtimepkg.Home,
	config runtimepkg.BindConfig,
) runtimepkg.ThreadRuntime {
	config = config.WithEnvOverride(f.env...)
	return &Runtime{
		rootDir:      f.rootDir,
		sandboxes:    f.sandboxes,
		bridge:       f.bridge,
		registration: registration,
		home:         home,
		image:        resolveImage(config.Image),
		entrypoint:   strings.TrimSpace(config.Entrypoint),
		env:          append([]string{}, config.Env...),
		gpus:         strings.TrimSpace(config.GPUs),
		seccomp:      strings.TrimSpace(config.Seccomp),
		cmd:          append([]string{}, config.Cmd...),
		user:         config.UserString(),
		threadConfig: f.threadConfig,
	}
}

type Runtime struct {
	rootDir      string
	sandboxes    sandboxengine.RuntimeManager
	bridge       *hostbridgebridge.Bridge
	registration coremodel.Component
	home         runtimepkg.Home
	image        string
	entrypoint   string
	env          []string
	gpus         string
	seccomp      string
	cmd          []string
	user         string
	threadConfig runtimepkg.ThreadConfigResolver
}

func (r *Runtime) Kind() string {
	return "docker"
}

func (r *Runtime) ComponentHome() runtimepkg.Home {
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
	return runtimepkg.DefaultWorkspaceRuntimePath
}

func (r *Runtime) Refresh(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
) error {
	sbx, cleanup, err := r.sandbox(ctx, workspacePath, threadID, nil, false)
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
) (runtimepkg.Status, error) {
	sbx, cleanup, err := r.sandbox(ctx, workspacePath, threadID, nil, true)
	if err != nil {
		return runtimepkg.Status{}, err
	}
	defer cleanup()
	if _, err := sbx.Ensure(ctx); err != nil {
		return runtimepkg.Status{}, err
	}
	return r.statusForSandbox(ctx, workspacePath, sbx)
}

func (r *Runtime) Stop(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
) error {
	sbx, cleanup, err := r.sandbox(ctx, workspacePath, threadID, nil, false)
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
	sbx, cleanup, err := r.sandbox(ctx, workspacePath, threadID, nil, false)
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
) (runtimepkg.Status, error) {
	sbx, cleanup, err := r.sandbox(ctx, workspacePath, threadID, nil, false)
	if err != nil {
		return runtimepkg.Status{}, err
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
	sbx, cleanup, err := r.sandbox(ctx, workspacePath, threadID, commands, true)
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

func (r *Runtime) ExecTTY(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	stdout io.Writer,
	stderr io.Writer,
	name string,
	args ...string,
) error {
	sbx, cleanup, err := r.sandbox(ctx, workspacePath, threadID, commands, true)
	if err != nil {
		return err
	}
	defer cleanup()
	err = sbx.ExecTTY(ctx, stdout, stderr, name, args...)
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
	sbx, cleanup, err := r.sandbox(ctx, workspacePath, threadID, commands, true)
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
	sbx, cleanup, err := r.sandbox(ctx, workspacePath, threadID, commands, true)
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
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	prepareBridge bool,
) (*sandboxengine.Sandbox, func(), error) {
	if r == nil || r.sandboxes == nil {
		return nil, nil, fmt.Errorf("missing docker runtime")
	}
	securityOpts, err := containerengine.SeccompSecurityOpts(r.seccomp)
	if err != nil {
		return nil, nil, err
	}
	if threadID.IsNull() {
		runtimeHomePath := r.RuntimeComponentHomePath()
		spec := sandboxengine.NewBuilder(authSandboxName(r.registration)).
			Image(r.image).
			Entrypoint(r.entrypoint).
			Workdir(runtimeHomePath).
			User(r.user).
			GPUs(r.gpus).
			Env(append([]string{}, r.env...)).
			Mounts([]sandboxengine.Mount{{Source: r.home.Path, Target: runtimeHomePath}}).
			SecurityOpts(securityOpts).
			AddHosts(sandboxAddHosts()).
			Cmd(r.idleCmd()).
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
	ports, err := r.threadPorts(ctx, threadID)
	if err != nil {
		return nil, nil, err
	}
	mounts := []sandboxengine.Mount{
		{Source: r.home.Path, Target: runtimeHomePath},
		{Source: workspaceHost, Target: workspaceRuntime},
	}
	cleanup := func() {}
	if prepareBridge && r.bridge != nil && commands != nil {
		bridgeEnv, bridgeMount, unregister, err := r.bridge.BindThread(threadID, commands)
		if err != nil {
			return nil, nil, err
		}
		env = append(env, bridgeEnv...)
		mounts = append(mounts, bridgeMount)
		cleanup = unregister
	}
	name := turnSandboxName(r.registration, threadID)
	spec := sandboxengine.NewBuilder(name).
		WorkspaceDir(workspaceHost).
		ProfileDir(r.home.Path).
		ContainerWorkspace(workspaceRuntime).
		ContainerHome(runtimeHomePath).
		Hostname(name).
		Image(r.image).
		Entrypoint(r.entrypoint).
		Workdir(workspaceRuntime).
		User(r.user).
		GPUs(r.gpus).
		Ports(ports).
		Env(env).
		Mounts(mounts).
		SecurityOpts(securityOpts).
		AddHosts(sandboxAddHosts()).
		Cmd(r.idleCmd()).
		Build()
	return r.sandboxes.CreateSandbox(spec), cleanup, nil
}

func (r *Runtime) threadPorts(ctx context.Context, threadID modeluuid.UUID) ([]string, error) {
	if r == nil || r.threadConfig == nil || threadID.IsNull() {
		return nil, nil
	}
	config, err := r.threadConfig.RuntimeThreadConfig(ctx, threadID)
	if err != nil {
		return nil, err
	}
	return append([]string{}, config.Ports...), nil
}

func (r *Runtime) idleCmd() []string {
	if r != nil && len(r.cmd) > 0 {
		return append([]string{}, r.cmd...)
	}
	return []string{"tail", "-f", "/dev/null"}
}

func sandboxAddHosts() []string {
	if goruntime.GOOS != "linux" {
		return nil
	}
	return []string{"host.docker.internal:host-gateway"}
}

func (r *Runtime) statusForSandbox(ctx context.Context, workspacePath string, sbx *sandboxengine.Sandbox) (runtimepkg.Status, error) {
	if sbx == nil {
		return runtimepkg.Status{}, fmt.Errorf("missing sandbox")
	}
	state, err := sbx.InspectState(ctx)
	if err != nil {
		return runtimepkg.Status{}, err
	}
	status := runtimepkg.Status{
		Name:                 sbx.Name,
		State:                string(state),
		RuntimeHomePath:      r.RuntimeComponentHomePath(),
		RuntimeWorkspacePath: r.RuntimeWorkspacePath(workspacePath),
		Ports:                append([]string{}, sbx.Ports...),
	}
	if active, ok := sbx.ActiveCommand(); ok {
		status.ActiveCommandName = active.Name
		status.ActiveCommandArgs = append([]string(nil), active.Args...)
	}
	status.RuntimeNotices = r.runtimeNotices(ctx, sbx)
	return status, nil
}

func (r *Runtime) runtimeNotices(ctx context.Context, sbx *sandboxengine.Sandbox) []string {
	if r == nil || sbx == nil {
		return nil
	}
	image, err := inspectDockerImage(ctx, sbx.Image)
	if err != nil {
		return nil
	}
	container, err := inspectDockerContainer(ctx, sbx.Name)
	if err != nil {
		return nil
	}
	return runtimeFreshnessNotices(container, image, buildassets.Info(), runtimeimage.CurrentGitCommit(ctx, r.rootDir), r.registration.Type)
}

func authSandboxName(registration coremodel.Component) string {
	return safeName("ctgbot-auth-"+registration.Ref(), "ctgbot-auth")
}

func turnSandboxName(registration coremodel.Component, threadID modeluuid.UUID) string {
	return safeName("ctgbot-"+registration.Ref()+"-"+threadID.String(), "ctgbot-runtime")
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
	return hostPath, runtimepkg.DefaultWorkspaceRuntimePath, nil
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
