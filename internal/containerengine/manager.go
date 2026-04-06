package containerengine

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strings"
)

type Manager struct {
	Logger *log.Logger
}

func (m *Manager) InspectState(ctx context.Context, containerName string) (State, error) {
	out, err := runCommand(ctx, "docker", "inspect", "-f", "{{.State.Status}}", containerName)
	if err != nil {
		trimmed := strings.TrimSpace(out)
		if isMissingContainerOutput(trimmed) {
			return StateMissing, nil
		}
		return StateMissing, fmt.Errorf("docker inspect %s: %w: %s", containerName, err, trimmed)
	}
	return State(strings.TrimSpace(out)), nil
}

func (m *Manager) Create(ctx context.Context, spec ContainerSpec) error {
	args := []string{"create"}
	for _, opt := range spec.SecurityOpts {
		if strings.TrimSpace(opt) == "" {
			continue
		}
		args = append(args, "--security-opt", opt)
	}
	if strings.TrimSpace(spec.Name) != "" {
		args = append(args, "--name", spec.Name)
	}
	hostname := strings.TrimSpace(spec.Hostname)
	if hostname != "" {
		args = append(args, "--hostname", hostname)
	}
	labelKeys := make([]string, 0, len(spec.Labels))
	for key := range spec.Labels {
		labelKeys = append(labelKeys, key)
	}
	sort.Strings(labelKeys)
	for _, key := range labelKeys {
		args = append(args, "--label", fmt.Sprintf("%s=%s", key, spec.Labels[key]))
	}
	for _, env := range spec.Env {
		if strings.TrimSpace(env) == "" {
			continue
		}
		args = append(args, "--env", env)
	}
	if workdir := strings.TrimSpace(spec.Workdir); workdir != "" {
		args = append(args, "--workdir", workdir)
	}
	for _, mount := range spec.Mounts {
		if strings.TrimSpace(mount.Source) == "" || strings.TrimSpace(mount.Target) == "" {
			continue
		}
		value := fmt.Sprintf("type=bind,source=%s,target=%s", mount.Source, mount.Target)
		if mount.ReadOnly {
			value += ",readonly"
		}
		args = append(args, "--mount", value)
	}
	for _, entry := range spec.AddHosts {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		args = append(args, "--add-host", entry)
	}
	if strings.TrimSpace(spec.Image) == "" {
		return fmt.Errorf("missing container image")
	}
	args = append(args, spec.Image)
	args = append(args, spec.Cmd...)
	out, err := runCommand(ctx, "docker", args...)
	if err != nil {
		return fmt.Errorf("docker create: %w: %s", err, strings.TrimSpace(out))
	}
	m.logf("conversation container created name=%s docker=%s", spec.Name, strings.TrimSpace(out))
	return nil
}

func (m *Manager) Start(ctx context.Context, containerName string) error {
	if _, err := runCommand(ctx, "docker", "start", containerName); err != nil {
		return fmt.Errorf("docker start %s: %w", containerName, err)
	}
	m.logf("conversation container started name=%s", containerName)
	return nil
}

func (m *Manager) Stop(ctx context.Context, containerName string) error {
	state, err := m.InspectState(ctx, containerName)
	if err != nil {
		return err
	}
	if state == StateMissing || state == StateCreated || state == StateExited {
		return nil
	}
	if _, err := runCommand(ctx, "docker", "stop", "-t", "1", containerName); err != nil {
		return fmt.Errorf("docker stop %s: %w", containerName, err)
	}
	m.logf("conversation container stopped name=%s", containerName)
	return nil
}

func (m *Manager) Remove(ctx context.Context, containerName string) error {
	state, err := m.InspectState(ctx, containerName)
	if err != nil {
		return err
	}
	if state == StateMissing {
		return nil
	}
	if _, err := runCommand(ctx, "docker", "rm", "-f", containerName); err != nil {
		return fmt.Errorf("docker rm -f %s: %w", containerName, err)
	}
	m.logf("conversation container removed name=%s", containerName)
	return nil
}

func (m *Manager) logf(format string, args ...any) {
	if m != nil && m.Logger != nil {
		m.Logger.Printf(format, args...)
	}
}

func isMissingContainerOutput(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	return strings.Contains(lower, "no such object") || strings.Contains(lower, "no such container")
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
