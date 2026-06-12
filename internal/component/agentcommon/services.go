package agentcommon

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/go-clir"
)

type ServiceAdd struct {
	Name    string
	Command string
}

type ServiceStart struct{ Name string }
type ServiceStop struct{ Name string }
type ServiceRestart struct{ Name string }
type ServiceStatus struct{ Name string }
type ServiceLogs struct{ Name string }
type ServiceRemove struct{ Name string }

func ServiceCommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		serviceDefinition("service add <name>", "Add or update a sandbox service; extra args are the shell command", buildServiceAdd),
		serviceDefinition("service start <name>", "Start a sandbox service", buildServiceName[ServiceStart]),
		serviceDefinition("service stop <name>", "Stop a sandbox service", buildServiceName[ServiceStop]),
		serviceDefinition("service restart <name>", "Restart a sandbox service", buildServiceName[ServiceRestart]),
		serviceDefinition("service status", "List sandbox services", func(_ *clir.Request) (any, error) { return ServiceStatus{}, nil }),
		serviceDefinition("service status <name>", "Show sandbox service status", buildServiceName[ServiceStatus]),
		serviceDefinition("service logs <name>", "Show sandbox service logs", buildServiceName[ServiceLogs]),
		serviceDefinition("service remove <name>", "Remove a sandbox service", buildServiceName[ServiceRemove]),
	}
}

func serviceDefinition(pattern string, help string, build func(*clir.Request) (any, error)) commandengine.Definition {
	return commandengine.Definition{
		Pattern:               pattern,
		Help:                  help,
		Build:                 build,
		Sources:               AgentCommandSources(),
		Policy:                AgentCommandPolicy(),
		InstructionVisibility: commandengine.InstructionImportant,
	}
}

func buildServiceName[T interface {
	ServiceStart | ServiceStop | ServiceRestart | ServiceStatus | ServiceLogs | ServiceRemove
}](req *clir.Request) (any, error) {
	name := strings.TrimSpace(req.Params["name"])
	switch any(*new(T)).(type) {
	case ServiceStart:
		return ServiceStart{Name: name}, nil
	case ServiceStop:
		return ServiceStop{Name: name}, nil
	case ServiceRestart:
		return ServiceRestart{Name: name}, nil
	case ServiceStatus:
		return ServiceStatus{Name: name}, nil
	case ServiceLogs:
		return ServiceLogs{Name: name}, nil
	case ServiceRemove:
		return ServiceRemove{Name: name}, nil
	default:
		return nil, fmt.Errorf("unsupported service command")
	}
}

func buildServiceAdd(req *clir.Request) (any, error) {
	name := strings.TrimSpace(req.Params["name"])
	command := strings.TrimSpace(strings.Join(req.Extra, " "))
	if command == "" {
		return nil, fmt.Errorf("missing service command")
	}
	return ServiceAdd{Name: name, Command: command}, nil
}

func (c *Core) RegisterServiceCommandHandlers(registry *commandengine.Registry, componentType string) error {
	if err := commandengine.RegisterPattern[ServiceAdd](registry, "service add <name>", func(ctx context.Context, req commandengine.Request, cmd ServiceAdd) (commandengine.Result, error) {
		return c.serviceAdd(ctx, req, componentType, cmd)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[ServiceStart](registry, "service start <name>", func(ctx context.Context, req commandengine.Request, cmd ServiceStart) (commandengine.Result, error) {
		return c.serviceStart(ctx, req, componentType, cmd.Name)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[ServiceStop](registry, "service stop <name>", func(ctx context.Context, req commandengine.Request, cmd ServiceStop) (commandengine.Result, error) {
		return c.serviceStop(ctx, req, componentType, cmd.Name)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[ServiceRestart](registry, "service restart <name>", func(ctx context.Context, req commandengine.Request, cmd ServiceRestart) (commandengine.Result, error) {
		if _, err := c.serviceStop(ctx, req, componentType, cmd.Name); err != nil {
			return commandengine.Result{}, err
		}
		return c.serviceStart(ctx, req, componentType, cmd.Name)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[ServiceStatus](registry, "service status", func(ctx context.Context, req commandengine.Request, _ ServiceStatus) (commandengine.Result, error) {
		return c.serviceList(ctx, req, componentType)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[ServiceStatus](registry, "service status <name>", func(ctx context.Context, req commandengine.Request, cmd ServiceStatus) (commandengine.Result, error) {
		return c.serviceStatus(ctx, req, componentType, cmd.Name)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[ServiceLogs](registry, "service logs <name>", func(ctx context.Context, req commandengine.Request, cmd ServiceLogs) (commandengine.Result, error) {
		return c.serviceLogs(ctx, req, componentType, cmd.Name)
	}); err != nil {
		return err
	}
	return commandengine.RegisterPattern[ServiceRemove](registry, "service remove <name>", func(ctx context.Context, req commandengine.Request, cmd ServiceRemove) (commandengine.Result, error) {
		return c.serviceRemove(ctx, req, componentType, cmd.Name)
	})
}

func (c *Core) serviceRuntime() (runtimepkg.ThreadServiceRuntime, error) {
	runtime, ok := c.Runtime.(runtimepkg.ThreadServiceRuntime)
	if !ok || runtime == nil {
		return nil, fmt.Errorf("runtime does not support services")
	}
	return runtime, nil
}

func (c *Core) serviceThread(ctx context.Context, req commandengine.Request, componentType string) (*threadWorkspace, runtimepkg.ThreadServiceRuntime, error) {
	thread, workspacePath, err := ThreadWorkspace(ctx, c.Storage, c.ResolveWorkspace, req, componentType)
	if err != nil {
		return nil, nil, err
	}
	runtime, err := c.serviceRuntime()
	if err != nil {
		return nil, nil, err
	}
	return &threadWorkspace{ThreadID: thread.ID, WorkspacePath: workspacePath}, runtime, nil
}

type threadWorkspace struct {
	ThreadID      modeluuid.UUID
	WorkspacePath string
}

func (c *Core) serviceAdd(ctx context.Context, req commandengine.Request, componentType string, cmd ServiceAdd) (commandengine.Result, error) {
	tw, runtime, err := c.serviceThread(ctx, req, componentType)
	if err != nil {
		return commandengine.Result{}, err
	}
	status, err := runtime.ServiceAdd(ctx, tw.WorkspacePath, tw.ThreadID, runtimepkg.ServiceDefinition{
		Name:    cmd.Name,
		Workdir: runtimepkg.DefaultWorkspaceRuntimePath,
		Command: []string{"sh", "-lc", cmd.Command},
		Restart: "error",
	})
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: formatServiceStatus("service added", status)}, nil
}

func (c *Core) serviceStart(ctx context.Context, req commandengine.Request, componentType string, name string) (commandengine.Result, error) {
	tw, runtime, err := c.serviceThread(ctx, req, componentType)
	if err != nil {
		return commandengine.Result{}, err
	}
	status, err := runtime.ServiceStart(ctx, tw.WorkspacePath, tw.ThreadID, name)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: formatServiceStatus("service started", status)}, nil
}

func (c *Core) serviceStop(ctx context.Context, req commandengine.Request, componentType string, name string) (commandengine.Result, error) {
	tw, runtime, err := c.serviceThread(ctx, req, componentType)
	if err != nil {
		return commandengine.Result{}, err
	}
	status, err := runtime.ServiceStop(ctx, tw.WorkspacePath, tw.ThreadID, name)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: formatServiceStatus("service stopped", status)}, nil
}

func (c *Core) serviceStatus(ctx context.Context, req commandengine.Request, componentType string, name string) (commandengine.Result, error) {
	tw, runtime, err := c.serviceThread(ctx, req, componentType)
	if err != nil {
		return commandengine.Result{}, err
	}
	status, err := runtime.ServiceStatus(ctx, tw.WorkspacePath, tw.ThreadID, name)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: formatServiceStatus("service status", status)}, nil
}

func (c *Core) serviceList(ctx context.Context, req commandengine.Request, componentType string) (commandengine.Result, error) {
	tw, runtime, err := c.serviceThread(ctx, req, componentType)
	if err != nil {
		return commandengine.Result{}, err
	}
	services, err := runtime.ServiceList(ctx, tw.WorkspacePath, tw.ThreadID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(services) == 0 {
		return commandengine.Result{Text: "services\n(no services)"}, nil
	}
	lines := []string{"services"}
	for _, service := range services {
		state := "stopped"
		if service.Running {
			state = "running"
		}
		lines = append(lines, fmt.Sprintf("- %s %s pid=%s", service.Name, state, firstNonEmpty(service.PID, "-")))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (c *Core) serviceLogs(ctx context.Context, req commandengine.Request, componentType string, name string) (commandengine.Result, error) {
	tw, runtime, err := c.serviceThread(ctx, req, componentType)
	if err != nil {
		return commandengine.Result{}, err
	}
	logs, err := runtime.ServiceLogs(ctx, tw.WorkspacePath, tw.ThreadID, name, 120)
	if err != nil {
		return commandengine.Result{}, err
	}
	logs = strings.TrimSpace(logs)
	if logs == "" {
		logs = "(no logs)"
	}
	return commandengine.Result{Text: logs}, nil
}

func (c *Core) serviceRemove(ctx context.Context, req commandengine.Request, componentType string, name string) (commandengine.Result, error) {
	tw, runtime, err := c.serviceThread(ctx, req, componentType)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := runtime.ServiceRemove(ctx, tw.WorkspacePath, tw.ThreadID, name); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "service removed: " + name}, nil
}

func formatServiceStatus(title string, status runtimepkg.ServiceStatus) string {
	state := "stopped"
	if status.Running {
		state = "running"
	}
	return strings.Join([]string{
		title,
		"name: " + status.Name,
		"state: " + state,
		"pid: " + firstNonEmpty(status.PID, "-"),
		"config: " + status.ConfigPath,
		"log: " + status.LogPath,
	}, "\n")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
