package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
)

const dockerDefaultImage = "ctgbot-codex:latest"

type DockerRuntime struct {
	rootDir   string
	profile   component.Profile
	sandboxes sandboxengine.RuntimeManager
}

func newDockerRuntime(rootDir string, sandboxes sandboxengine.RuntimeManager, profile component.Profile) *DockerRuntime {
	return &DockerRuntime{
		rootDir:   strings.TrimSpace(rootDir),
		profile:   profile,
		sandboxes: sandboxes,
	}
}

func (r *DockerRuntime) Kind() string {
	return "docker"
}

func (r *DockerRuntime) Profile() component.Profile {
	return r.profile
}

func (r *DockerRuntime) ComponentHome(registration coremodel.Component) component.Home {
	hostPath := filepath.Join(r.profile.Root, "components", registration.Type, registration.Name)
	return component.Home{
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

func (r *DockerRuntime) StartAuth(ctx context.Context, registration coremodel.Component, home component.Home, image string, workdir string, env []string) (*sandboxengine.Sandbox, error) {
	_ = ctx
	if r == nil || r.sandboxes == nil {
		return nil, fmt.Errorf("missing docker runtime")
	}
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

func (r *DockerRuntime) StartTurn(ctx context.Context, registration coremodel.Component, thread coremodel.Thread, home component.Home, image string, workdir string, env []string, developerInstructions string, commands commandengine.CommandExecutor) (*sandboxengine.SandboxRuntime, error) {
	if r == nil || r.sandboxes == nil {
		return nil, fmt.Errorf("missing docker runtime")
	}
	workspaceHost, workspaceContainer, err := r.ThreadWorkspace(thread.ID)
	if err != nil {
		return nil, err
	}
	spec := sandboxengine.NewBuilder(turnSandboxName(registration, thread)).
		WorkspaceDir(workspaceHost).
		ProfileDir(home.HostPath).
		ContainerWorkspace(workspaceContainer).
		ContainerHome(home.ContainerPath).
		DeveloperInstructions(strings.TrimSpace(developerInstructions)).
		Hostname(turnSandboxName(registration, thread)).
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
	return r.sandboxes.CreateRuntime(sandboxengine.RuntimeSpec{
		Sandbox:       *spec,
		AgentCommands: commands,
	}), nil
}

func authSandboxName(registration coremodel.Component, profileName string) string {
	return safeName("ctgbot-v5-auth-"+registration.Ref()+"-"+strings.TrimSpace(profileName), "ctgbot-v5-auth")
}

func turnSandboxName(registration coremodel.Component, thread coremodel.Thread) string {
	return safeName("ctgbot-v5-"+registration.Ref()+"-"+thread.ID.String(), "ctgbot-v5-runtime")
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
