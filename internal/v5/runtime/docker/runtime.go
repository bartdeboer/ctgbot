package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	rootDir   string
	profile   v5runtime.Profile
	sandboxes sandboxengine.RuntimeManager
	bridge    *v5hostbridgeserver.Bridge
}

func New(rootDir string, sandboxes sandboxengine.RuntimeManager, bridge *v5hostbridgeserver.Bridge, profile v5runtime.Profile) *Factory {
	return &Factory{
		rootDir:   strings.TrimSpace(rootDir),
		profile:   profile,
		sandboxes: sandboxes,
		bridge:    bridge,
	}
}

func (f *Factory) Kind() string {
	return "docker"
}

func (f *Factory) Profile() v5runtime.Profile {
	return f.profile
}

func (f *Factory) ComponentHome(registration coremodel.Component) v5runtime.Home {
	hostPath := filepath.Join(f.profile.Root, "components", registration.Type, registration.Name)
	return v5runtime.Home{
		HostPath:      hostPath,
		ContainerPath: "/profile/components/" + registration.Type + "/" + registration.Name,
	}
}

func (f *Factory) Bind(
	registration coremodel.Component,
	home v5runtime.Home,
	image string,
	env []string,
) v5runtime.Runtime {
	return &Runtime{
		rootDir:      f.rootDir,
		profile:      f.profile,
		sandboxes:    f.sandboxes,
		bridge:       f.bridge,
		registration: registration,
		home:         home,
		image:        resolveImage(image),
		env:          append([]string{}, env...),
	}
}

type Runtime struct {
	rootDir      string
	profile      v5runtime.Profile
	sandboxes    sandboxengine.RuntimeManager
	bridge       *v5hostbridgeserver.Bridge
	registration coremodel.Component
	home         v5runtime.Home
	image        string
	env          []string
}

func (r *Runtime) Kind() string {
	return "docker"
}

func (r *Runtime) Profile() v5runtime.Profile {
	return r.profile
}

func (r *Runtime) ComponentHome() v5runtime.Home {
	return r.home
}

func (r *Runtime) ThreadWorkspace(threadID modeluuid.UUID) (string, string, error) {
	if threadID.IsNull() {
		return "", "", fmt.Errorf("missing thread id")
	}
	hostPath := filepath.Join(r.rootDir, ".ctgbot", "threads", threadID.String(), "workspace")
	if err := os.MkdirAll(filepath.Join(hostPath, "inbox"), 0o755); err != nil {
		return "", "", err
	}
	return hostPath, "/workspace", nil
}

func (r *Runtime) Exec(
	ctx context.Context,
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	stdout io.Writer,
	stderr io.Writer,
	name string,
	args ...string,
) error {
	sbx, cleanup, err := r.sandbox(threadID, commands)
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
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	name string,
	args ...string,
) ([]byte, error) {
	sbx, cleanup, err := r.sandbox(threadID, commands)
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
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	callbackPort int,
	callbackTimeout time.Duration,
) (func(context.Context) error, error) {
	sbx, cleanup, err := r.sandbox(threadID, commands)
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
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
) (*sandboxengine.Sandbox, func(), error) {
	if r == nil || r.sandboxes == nil {
		return nil, nil, fmt.Errorf("missing docker runtime")
	}
	if threadID.IsNull() {
		spec := sandboxengine.NewBuilder(authSandboxName(r.registration, r.profile.Name)).
			Image(r.image).
			Workdir(r.home.ContainerPath).
			// Keep mounted profile files writable by the host ctgbot process.
			UserMode("host").
			Env(append([]string{}, r.env...)).
			Mounts([]sandboxengine.Mount{{Source: r.home.HostPath, Target: r.home.ContainerPath}}).
			SecurityOpts([]string{"seccomp=unconfined"}).
			AddHosts([]string{"host.docker.internal:host-gateway"}).
			Cmd([]string{"tail", "-f", "/dev/null"}).
			Build()
		return r.sandboxes.CreateSandbox(spec), func() {}, nil
	}

	workspaceHost, workspaceContainer, err := r.ThreadWorkspace(threadID)
	if err != nil {
		return nil, nil, err
	}
	env := append([]string{}, r.env...)
	mounts := []sandboxengine.Mount{
		{Source: r.home.HostPath, Target: r.home.ContainerPath},
		{Source: workspaceHost, Target: workspaceContainer},
	}
	cleanup := func() {}
	if r.bridge != nil {
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
		ProfileDir(r.home.HostPath).
		ContainerWorkspace(workspaceContainer).
		ContainerHome(r.home.ContainerPath).
		Hostname(turnSandboxName(r.registration, threadID)).
		Image(r.image).
		Workdir(workspaceContainer).
		// Keep mounted profile/workspace files writable by the host ctgbot process.
		UserMode("host").
		Env(env).
		Mounts(mounts).
		SecurityOpts([]string{"seccomp=unconfined"}).
		AddHosts([]string{"host.docker.internal:host-gateway"}).
		Cmd([]string{"tail", "-f", "/dev/null"}).
		Build()
	return r.sandboxes.CreateSandbox(spec), cleanup, nil
}

func authSandboxName(registration coremodel.Component, profileName string) string {
	return safeName("ctgbot-v5-auth-"+registration.Ref()+"-"+strings.TrimSpace(profileName), "ctgbot-v5-auth")
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
