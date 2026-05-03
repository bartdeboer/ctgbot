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
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
)

const dockerDefaultImage = "ctgbot-codex:latest"

type DockerRuntime struct {
	rootDir   string
	profile   v5runtime.Profile
	sandboxes sandboxengine.RuntimeManager
}

func New(rootDir string, sandboxes sandboxengine.RuntimeManager, profile v5runtime.Profile) *DockerRuntime {
	return &DockerRuntime{
		rootDir:   strings.TrimSpace(rootDir),
		profile:   profile,
		sandboxes: sandboxes,
	}
}

func (r *DockerRuntime) Kind() string {
	return "docker"
}

func (r *DockerRuntime) Profile() v5runtime.Profile {
	return r.profile
}

func (r *DockerRuntime) ComponentHome(registration coremodel.Component) v5runtime.Home {
	hostPath := filepath.Join(r.profile.Root, "components", registration.Type, registration.Name)
	return v5runtime.Home{
		HostPath:      hostPath,
		ContainerPath: "/profile/components/" + registration.Type + "/" + registration.Name,
	}
}

func (r *DockerRuntime) ThreadWorkspace(threadID modeluuid.UUID) (string, string, error) {
	if threadID.IsNull() {
		return "", "", fmt.Errorf("missing thread id")
	}
	hostPath := filepath.Join(r.rootDir, ".ctgbot", "threads", threadID.String(), "workspace")
	if err := os.MkdirAll(filepath.Join(hostPath, "inbox"), 0o755); err != nil {
		return "", "", err
	}
	return hostPath, "/workspace", nil
}

func (r *DockerRuntime) Exec(
	ctx context.Context,
	registration coremodel.Component,
	threadID modeluuid.UUID,
	home v5runtime.Home,
	image string,
	workdir string,
	env []string,
	developerInstructions string,
	commands commandengine.CommandExecutor,
	stdout io.Writer,
	stderr io.Writer,
	name string,
	args ...string,
) error {
	sbx, err := r.sandbox(registration, threadID, home, image, workdir, env, developerInstructions, commands)
	if err != nil {
		return err
	}
	err = sbx.Exec(ctx, stdout, stderr, name, args...)
	if err != nil && sbx.Interrupted() {
		return context.Canceled
	}
	return err
}

func (r *DockerRuntime) CombinedOutput(
	ctx context.Context,
	registration coremodel.Component,
	threadID modeluuid.UUID,
	home v5runtime.Home,
	image string,
	workdir string,
	env []string,
	developerInstructions string,
	commands commandengine.CommandExecutor,
	name string,
	args ...string,
) ([]byte, error) {
	sbx, err := r.sandbox(registration, threadID, home, image, workdir, env, developerInstructions, commands)
	if err != nil {
		return nil, err
	}
	out, err := sbx.CombinedOutput(ctx, name, args...)
	if err != nil && sbx.Interrupted() {
		return nil, context.Canceled
	}
	return out, err
}

func (r *DockerRuntime) OpenHTTPRelayPort(
	ctx context.Context,
	registration coremodel.Component,
	threadID modeluuid.UUID,
	home v5runtime.Home,
	image string,
	workdir string,
	env []string,
	developerInstructions string,
	commands commandengine.CommandExecutor,
	callbackPort int,
	callbackTimeout time.Duration,
) (func(context.Context) error, error) {
	sbx, err := r.sandbox(registration, threadID, home, image, workdir, env, developerInstructions, commands)
	if err != nil {
		return nil, err
	}
	if _, err := sbx.Ensure(ctx); err != nil {
		return nil, err
	}
	relay, err := sbx.OpenHTTPRelayPort(ctx, callbackPort, callbackTimeout)
	if err != nil {
		return nil, err
	}
	return relay.Close, nil
}

func (r *DockerRuntime) sandbox(
	registration coremodel.Component,
	threadID modeluuid.UUID,
	home v5runtime.Home,
	image string,
	workdir string,
	env []string,
	developerInstructions string,
	commands commandengine.CommandExecutor,
) (*sandboxengine.Sandbox, error) {
	if r == nil || r.sandboxes == nil {
		return nil, fmt.Errorf("missing docker runtime")
	}
	if threadID.IsNull() {
		spec := sandboxengine.NewBuilder(authSandboxName(registration, r.profile.Name)).
			Image(resolveImage(image)).
			Workdir(resolveWorkdir(workdir, home.ContainerPath)).
			Env(append([]string{}, env...)).
			Mounts([]sandboxengine.Mount{{Source: home.HostPath, Target: home.ContainerPath}}).
			SecurityOpts([]string{"seccomp=unconfined"}).
			Cmd([]string{"tail", "-f", "/dev/null"}).
			Build()
		return r.sandboxes.CreateSandbox(spec), nil
	}

	workspaceHost, workspaceContainer, err := r.ThreadWorkspace(threadID)
	if err != nil {
		return nil, err
	}
	spec := sandboxengine.NewBuilder(turnSandboxName(registration, threadID)).
		WorkspaceDir(workspaceHost).
		ProfileDir(home.HostPath).
		ContainerWorkspace(workspaceContainer).
		ContainerHome(home.ContainerPath).
		DeveloperInstructions(strings.TrimSpace(developerInstructions)).
		Hostname(turnSandboxName(registration, threadID)).
		Image(resolveImage(image)).
		Workdir(resolveWorkdir(workdir, workspaceContainer)).
		Env(append([]string{}, env...)).
		Mounts([]sandboxengine.Mount{
			{Source: home.HostPath, Target: home.ContainerPath},
			{Source: workspaceHost, Target: workspaceContainer},
		}).
		SecurityOpts([]string{"seccomp=unconfined"}).
		Cmd([]string{"tail", "-f", "/dev/null"}).
		Build()
	runtime := r.sandboxes.CreateRuntime(sandboxengine.RuntimeSpec{
		Sandbox:       *spec,
		AgentCommands: commands,
	})
	return runtime.Sandbox(), nil
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

func resolveWorkdir(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return strings.TrimSpace(fallback)
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
