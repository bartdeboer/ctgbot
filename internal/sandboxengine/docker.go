package sandboxengine

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/containerengine"
)

type DockerManager struct {
	Containers *containerengine.Manager
	Logger     *log.Logger
}

func (m *DockerManager) NewSandbox(name string) *Sandbox {
	return &Sandbox{
		Name:    strings.TrimSpace(name),
		runtime: m,
	}
}

func (m *DockerManager) ensure(ctx context.Context, sbx *Sandbox) error {
	if strings.TrimSpace(sbx.Name) == "" {
		return fmt.Errorf("missing sandbox name")
	}
	state, err := m.inspectState(ctx, sbx.Name)
	if err != nil {
		return err
	}
	switch state {
	case StateRunning:
		return nil
	case StateCreated, StateExited:
		return m.containerManager().Start(ctx, sbx.Name)
	case StateMissing:
		if err := m.containerManager().Create(ctx, m.toContainerSpec(sbx)); err != nil {
			return err
		}
		return m.containerManager().Start(ctx, sbx.Name)
	default:
		return fmt.Errorf("unsupported sandbox state %q for %s", state, sbx.Name)
	}
}

func (m *DockerManager) stop(ctx context.Context, sbx *Sandbox) error {
	if sbx == nil || strings.TrimSpace(sbx.Name) == "" {
		return nil
	}
	return m.containerManager().Stop(ctx, sbx.Name)
}

func (m *DockerManager) remove(ctx context.Context, sbx *Sandbox) error {
	if sbx == nil || strings.TrimSpace(sbx.Name) == "" {
		return nil
	}
	return m.containerManager().Remove(ctx, sbx.Name)
}

func (m *DockerManager) inspectState(ctx context.Context, name string) (State, error) {
	state, err := m.containerManager().InspectState(ctx, name)
	if err != nil {
		return StateMissing, err
	}
	return State(state), nil
}

func (m *DockerManager) containerManager() *containerengine.Manager {
	if m.Containers == nil {
		m.Containers = &containerengine.Manager{Logger: m.Logger}
	}
	return m.Containers
}

func (m *DockerManager) toContainerSpec(sbx *Sandbox) containerengine.ContainerSpec {
	mounts := make([]containerengine.Mount, 0, len(sbx.Mounts))
	for _, mount := range sbx.Mounts {
		mounts = append(mounts, containerengine.Mount{
			Source:   mount.Source,
			Target:   mount.Target,
			ReadOnly: mount.ReadOnly,
		})
	}
	return containerengine.ContainerSpec{
		Name:         sbx.Name,
		Hostname:     sbx.Hostname,
		Image:        sbx.Image,
		Workdir:      sbx.Workdir,
		Labels:       sbx.Labels,
		Env:          sbx.Env,
		Mounts:       mounts,
		SecurityOpts: sbx.SecurityOpts,
		AddHosts:     sbx.AddHosts,
		Cmd:          sbx.Cmd,
	}
}
