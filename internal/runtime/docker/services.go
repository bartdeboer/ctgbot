package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/supervisor"
)

func (r *Runtime) ServiceAdd(ctx context.Context, workspacePath string, threadID modeluuid.UUID, service runtimepkg.ServiceDefinition) (runtimepkg.ServiceStatus, error) {
	_ = ctx
	homeHost, homeRuntime, err := r.resolveHome(threadID)
	if err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	name, err := serviceName(service.Name)
	if err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	if len(service.Command) == 0 {
		return runtimepkg.ServiceStatus{}, fmt.Errorf("missing service command")
	}
	workdir := strings.TrimSpace(service.Workdir)
	if workdir == "" {
		workdir = runtimepkg.DefaultWorkspaceRuntimePath
	}
	restart := supervisor.RestartPolicy(strings.TrimSpace(service.Restart))
	if restart == "" {
		restart = supervisor.RestartError
	}
	restartDelay := strings.TrimSpace(service.RestartDelay)
	if restartDelay == "" {
		restartDelay = "1s"
	}
	serviceDirHost := filepath.Join(homeHost, "services", name)
	serviceDirRuntime := homeRuntime + "/services/" + name
	if err := os.MkdirAll(serviceDirHost, 0o755); err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	cfg := supervisor.Config{
		Name:         name,
		Workdir:      workdir,
		Command:      append([]string{}, service.Command...),
		Env:          append([]string{}, service.Env...),
		Restart:      restart,
		RestartDelay: restartDelay,
		LogPath:      serviceDirRuntime + "/service.log",
		PIDPath:      serviceDirRuntime + "/supervisor.pid",
	}
	if err := cfg.Validate(); err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	if err := os.WriteFile(filepath.Join(serviceDirHost, "service.json"), append(b, '\n'), 0o644); err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	return r.serviceStatusFromHost(homeHost, homeRuntime, name), nil
}

func (r *Runtime) ServiceStart(ctx context.Context, workspacePath string, threadID modeluuid.UUID, name string) (runtimepkg.ServiceStatus, error) {
	homeHost, homeRuntime, err := r.resolveHome(threadID)
	if err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	name, err = serviceName(name)
	if err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	configRuntime := homeRuntime + "/services/" + name + "/service.json"
	if _, err := os.Stat(filepath.Join(homeHost, "services", name, "service.json")); err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	if _, err := r.ServiceStop(ctx, workspacePath, threadID, name); err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	sbx, cleanup, err := r.sandbox(ctx, workspacePath, threadID, nil, false)
	if err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	defer cleanup()
	if err := r.ensureSandboxReady(ctx, sbx); err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	if err := sbx.ExecDetached(ctx, "ctgbot-supervisor", configRuntime); err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	return r.serviceStatusFromHost(homeHost, homeRuntime, name), nil
}

func (r *Runtime) ServiceStop(ctx context.Context, workspacePath string, threadID modeluuid.UUID, name string) (runtimepkg.ServiceStatus, error) {
	homeHost, homeRuntime, err := r.resolveHome(threadID)
	if err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	name, err = serviceName(name)
	if err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	sbx, cleanup, err := r.sandbox(ctx, workspacePath, threadID, nil, false)
	if err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	defer cleanup()
	state, err := sbx.InspectState(ctx)
	if err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	if state != sandboxengine.StateRunning {
		_ = os.Remove(filepath.Join(homeHost, "services", name, "supervisor.pid"))
		return r.serviceStatusFromHost(homeHost, homeRuntime, name), nil
	}
	cmd := "pidfile='" + homeRuntime + "/services/" + name + "/supervisor.pid'; if [ -s \"$pidfile\" ]; then kill $(cat \"$pidfile\") 2>/dev/null || true; fi"
	if err := sbx.Exec(ctx, io.Discard, io.Discard, "sh", "-lc", cmd); err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	return r.serviceStatusFromHost(homeHost, homeRuntime, name), nil
}

func (r *Runtime) ServiceStatus(ctx context.Context, workspacePath string, threadID modeluuid.UUID, name string) (runtimepkg.ServiceStatus, error) {
	_ = ctx
	_ = workspacePath
	homeHost, homeRuntime, err := r.resolveHome(threadID)
	if err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	name, err = serviceName(name)
	if err != nil {
		return runtimepkg.ServiceStatus{}, err
	}
	return r.serviceStatusFromHost(homeHost, homeRuntime, name), nil
}

func (r *Runtime) ServiceList(ctx context.Context, workspacePath string, threadID modeluuid.UUID) ([]runtimepkg.ServiceStatus, error) {
	_ = ctx
	_ = workspacePath
	homeHost, homeRuntime, err := r.resolveHome(threadID)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(homeHost, "services"))
	if err != nil {
		return nil, err
	}
	var out []runtimepkg.ServiceStatus
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(homeHost, "services", entry.Name(), "service.json")); err != nil {
			continue
		}
		out = append(out, r.serviceStatusFromHost(homeHost, homeRuntime, entry.Name()))
	}
	return out, nil
}

func (r *Runtime) ServiceLogs(ctx context.Context, workspacePath string, threadID modeluuid.UUID, name string, limit int) (string, error) {
	_ = ctx
	_ = workspacePath
	homeHost, _, err := r.resolveHome(threadID)
	if err != nil {
		return "", err
	}
	name, err = serviceName(name)
	if err != nil {
		return "", err
	}
	path := filepath.Join(homeHost, "services", name, "service.log")
	return tailFile(path, limit)
}

func (r *Runtime) ServiceRemove(ctx context.Context, workspacePath string, threadID modeluuid.UUID, name string) error {
	if _, err := r.ServiceStop(ctx, workspacePath, threadID, name); err != nil {
		return err
	}
	homeHost, _, err := r.resolveHome(threadID)
	if err != nil {
		return err
	}
	name, err = serviceName(name)
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(homeHost, "services", name))
}

func (r *Runtime) serviceStatusFromHost(homeHost string, homeRuntime string, name string) runtimepkg.ServiceStatus {
	serviceDirHost := filepath.Join(homeHost, "services", name)
	serviceDirRuntime := homeRuntime + "/services/" + name
	pidPath := filepath.Join(serviceDirHost, "supervisor.pid")
	pid := strings.TrimSpace(readSmallFile(pidPath))
	return runtimepkg.ServiceStatus{
		Name:       name,
		ConfigPath: serviceDirRuntime + "/service.json",
		LogPath:    serviceDirRuntime + "/service.log",
		PIDPath:    serviceDirRuntime + "/supervisor.pid",
		PID:        pid,
		Running:    pid != "",
	}
}

func serviceName(raw string) (string, error) {
	name := safeName(raw, "")
	if name == "" {
		return "", fmt.Errorf("missing service name")
	}
	return name, nil
}

func readSmallFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil || len(b) > 4096 {
		return ""
	}
	return string(b)
}

func tailFile(path string, limit int) (string, error) {
	if limit <= 0 {
		limit = 80
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	defer f.Close()
	lines := make([]string, 0, limit)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > limit {
			copy(lines, lines[1:])
			lines = lines[:limit]
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return strings.Join(lines, "\n"), nil
}
