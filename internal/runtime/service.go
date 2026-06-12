package runtime

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type ServiceDefinition struct {
	Name         string
	Workdir      string
	Command      []string
	Env          []string
	Restart      string
	RestartDelay string
}

type ServiceStatus struct {
	Name       string
	ConfigPath string
	LogPath    string
	PIDPath    string
	PID        string
	Running    bool
}

type ThreadServiceRuntime interface {
	ServiceAdd(ctx context.Context, workspacePath string, threadID modeluuid.UUID, service ServiceDefinition) (ServiceStatus, error)
	ServiceStart(ctx context.Context, workspacePath string, threadID modeluuid.UUID, name string) (ServiceStatus, error)
	ServiceStop(ctx context.Context, workspacePath string, threadID modeluuid.UUID, name string) (ServiceStatus, error)
	ServiceStatus(ctx context.Context, workspacePath string, threadID modeluuid.UUID, name string) (ServiceStatus, error)
	ServiceList(ctx context.Context, workspacePath string, threadID modeluuid.UUID) ([]ServiceStatus, error)
	ServiceLogs(ctx context.Context, workspacePath string, threadID modeluuid.UUID, name string, limit int) (string, error)
	ServiceRemove(ctx context.Context, workspacePath string, threadID modeluuid.UUID, name string) error
}
