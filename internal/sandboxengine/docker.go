package sandboxengine

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/containerengine"
)

type DockerManager struct {
	Containers *containerengine.Manager
	Logger     *log.Logger
}

type DockerSandbox struct {
	ContainerName string
	Workdir       string
	Env           []string
}

func (m *DockerManager) InspectState(ctx context.Context, name string) (State, error) {
	state, err := m.containerManager().InspectState(ctx, name)
	if err != nil {
		return StateMissing, err
	}
	return State(state), nil
}

func (m *DockerManager) Ensure(ctx context.Context, spec Spec) (Sandbox, bool, error) {
	if strings.TrimSpace(spec.Name) == "" {
		return nil, false, fmt.Errorf("missing sandbox name")
	}
	state, err := m.InspectState(ctx, spec.Name)
	if err != nil {
		return nil, false, err
	}
	switch state {
	case StateRunning:
		return DockerSandbox{ContainerName: spec.Name, Workdir: spec.Workdir, Env: append([]string{}, spec.Env...)}, false, nil
	case StateCreated, StateExited:
		if err := m.containerManager().Start(ctx, spec.Name); err != nil {
			return nil, false, err
		}
		return DockerSandbox{ContainerName: spec.Name, Workdir: spec.Workdir, Env: append([]string{}, spec.Env...)}, false, nil
	case StateMissing:
		if err := m.containerManager().Create(ctx, m.toContainerSpec(spec)); err != nil {
			return nil, false, err
		}
		if err := m.containerManager().Start(ctx, spec.Name); err != nil {
			return nil, false, err
		}
		return DockerSandbox{ContainerName: spec.Name, Workdir: spec.Workdir, Env: append([]string{}, spec.Env...)}, true, nil
	default:
		return nil, false, fmt.Errorf("unsupported sandbox state %q for %s", state, spec.Name)
	}
}

func (m *DockerManager) Stop(ctx context.Context, name string) error {
	return m.containerManager().Stop(ctx, name)
}

func (m *DockerManager) Remove(ctx context.Context, name string) error {
	return m.containerManager().Remove(ctx, name)
}

func (s DockerSandbox) CommandContext(ctx context.Context, name string, commandArgs ...string) *exec.Cmd {
	args := []string{"exec"}
	for _, env := range s.Env {
		if strings.TrimSpace(env) == "" {
			continue
		}
		args = append(args, "-e", env)
	}
	if workdir := strings.TrimSpace(s.Workdir); workdir != "" {
		args = append(args, "-w", workdir)
	}
	args = append(args, s.ContainerName, name)
	args = append(args, commandArgs...)
	return exec.CommandContext(ctx, "docker", args...)
}

func (m *DockerManager) containerManager() *containerengine.Manager {
	if m.Containers == nil {
		m.Containers = &containerengine.Manager{Logger: m.Logger}
	}
	return m.Containers
}

func (m *DockerManager) toContainerSpec(spec Spec) containerengine.ContainerSpec {
	mounts := make([]containerengine.Mount, 0, len(spec.Mounts))
	for _, mount := range spec.Mounts {
		mounts = append(mounts, containerengine.Mount{
			Source:   mount.Source,
			Target:   mount.Target,
			ReadOnly: mount.ReadOnly,
		})
	}
	return containerengine.ContainerSpec{
		Name:         spec.Name,
		Hostname:     spec.Hostname,
		Image:        spec.Image,
		Workdir:      spec.Workdir,
		Labels:       spec.Labels,
		Env:          spec.Env,
		Mounts:       mounts,
		SecurityOpts: spec.SecurityOpts,
		AddHosts:     spec.AddHosts,
		Cmd:          spec.Cmd,
	}
}
