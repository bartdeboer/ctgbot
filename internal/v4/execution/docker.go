package execution

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	v4component "github.com/bartdeboer/ctgbot/internal/v4/component"
	"github.com/bartdeboer/ctgbot/internal/v4/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v4/profiles"
	"github.com/bartdeboer/ctgbot/internal/v4/workspaces"
)

const dockerDefaultImage = "ctgbot-codex:latest"

type DockerFactory struct{}

func (DockerFactory) Driver() string { return "docker" }

func (DockerFactory) Create(ctx context.Context, profile profiles.Profile, req CreateRequest) (v4component.Runtime, error) {
	_ = ctx
	if req.Sandboxes == nil {
		return nil, fmt.Errorf("missing sandbox manager")
	}
	if req.Workspaces == nil {
		return nil, fmt.Errorf("missing workspace manager")
	}
	return &DockerRuntime{
		profile:    profile,
		sandboxes:  req.Sandboxes,
		workspaces: req.Workspaces,
	}, nil
}

type DockerRuntime struct {
	profile    profiles.Profile
	sandboxes  sandboxengine.RuntimeManager
	workspaces *workspaces.Manager
}

func (r *DockerRuntime) Driver() string {
	return "docker"
}

func (r *DockerRuntime) Profile() profiles.Profile {
	return r.profile
}

func (r *DockerRuntime) ContainerWorkspace() string {
	if r == nil || r.workspaces == nil {
		return "/workspace"
	}
	return r.workspaces.ContainerWorkspace()
}

func (r *DockerRuntime) StartAuth(ctx context.Context, req v4component.RuntimeAuthRequest) (*sandboxengine.Sandbox, error) {
	_ = ctx
	if r == nil || r.sandboxes == nil {
		return nil, fmt.Errorf("missing docker runtime")
	}
	spec := sandboxengine.NewBuilder(authSandboxName(req.Registration, r.profile.Name)).
		Image(resolveImage(req.Image)).
		Workdir(resolveWorkdir(req.Workdir, req.Home.ContainerPath)).
		Env(append([]string{}, req.Env...)).
		Mounts([]sandboxengine.Mount{{Source: req.Home.HostPath, Target: req.Home.ContainerPath}}).
		SecurityOpts([]string{"seccomp=unconfined"}).
		Cmd([]string{"tail", "-f", "/dev/null"}).
		Build()
	return r.sandboxes.CreateSandbox(spec), nil
}

func (r *DockerRuntime) StartTurn(ctx context.Context, req v4component.RuntimeTurnRequest) (*sandboxengine.SandboxRuntime, error) {
	if r == nil || r.sandboxes == nil || r.workspaces == nil {
		return nil, fmt.Errorf("missing docker runtime")
	}
	workspaceHost, err := r.workspaces.Ensure(req.Thread.ID)
	if err != nil {
		return nil, err
	}
	containerWorkspace := r.workspaces.ContainerWorkspace()
	spec := sandboxengine.NewBuilder(turnSandboxName(req.Registration, req.Thread)).
		WorkspaceDir(workspaceHost).
		ProfileDir(req.Home.HostPath).
		ContainerWorkspace(containerWorkspace).
		ContainerHome(req.Home.ContainerPath).
		DeveloperInstructions(strings.TrimSpace(req.DeveloperInstructions)).
		Hostname(turnSandboxName(req.Registration, req.Thread)).
		Image(resolveImage(req.Image)).
		Workdir(resolveWorkdir(req.Workdir, containerWorkspace)).
		Env(append([]string{}, req.Env...)).
		Mounts([]sandboxengine.Mount{
			{Source: req.Home.HostPath, Target: req.Home.ContainerPath},
			{Source: workspaceHost, Target: containerWorkspace},
		}).
		SecurityOpts([]string{"seccomp=unconfined"}).
		Cmd([]string{"tail", "-f", "/dev/null"}).
		Build()
	return r.sandboxes.CreateRuntime(sandboxengine.RuntimeSpec{
		Sandbox:       *spec,
		AgentCommands: req.Commands,
	}), nil
}

func authSandboxName(component coremodel.Component, profileName string) string {
	return safeName("ctgbot-v4-auth-"+component.Ref()+"-"+strings.TrimSpace(profileName), "ctgbot-v4-auth")
}

func turnSandboxName(component coremodel.Component, thread coremodel.Thread) string {
	return safeName("ctgbot-v4-"+component.Ref()+"-"+thread.ID.String(), "ctgbot-v4-runtime")
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
