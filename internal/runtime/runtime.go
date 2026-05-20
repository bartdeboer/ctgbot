package runtime

import (
	"context"
	"io"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type Home struct {
	Path string
}

type BindConfig struct {
	Image       string   `json:"image"`
	Env         []string `json:"env"`
	GPUs        string   `json:"gpus"`
	Seccomp     string   `json:"seccomp"`
	Cmd         []string `json:"cmd,omitempty"`
	IdleTimeout string   `json:"idle_timeout,omitempty"`
}

type Status struct {
	Name                 string
	State                string
	RuntimeHomePath      string
	RuntimeWorkspacePath string
	ActiveCommandName    string
	ActiveCommandArgs    []string
	RuntimeNotices       []string
}

const DefaultWorkspaceRuntimePath = "/workspace"

type Factory interface {
	Kind() string
	ComponentHome(registration coremodel.Component) Home
	RuntimeComponentHomePath(registration coremodel.Component, home Home) string
	RuntimeWorkspacePath(workspacePath string) string
}

type ThreadRuntimeFactory interface {
	Factory
	Bind(
		registration coremodel.Component,
		home Home,
		config BindConfig,
	) ThreadRuntime
}

type ServiceRuntime interface {
	ComponentHome() Home
	BaseURL() string
	Start(ctx context.Context) (Status, error)
	Stop(ctx context.Context) error
	Refresh(ctx context.Context) error
	Status(ctx context.Context) (Status, error)
}

type ThreadRuntime interface {
	Kind() string
	ComponentHome() Home
	RuntimeComponentHomePath() string
	RuntimeWorkspacePath(workspacePath string) string
	Refresh(ctx context.Context, workspacePath string, threadID modeluuid.UUID) error
	Start(ctx context.Context, workspacePath string, threadID modeluuid.UUID) (Status, error)
	Stop(ctx context.Context, workspacePath string, threadID modeluuid.UUID) error
	Interrupt(ctx context.Context, workspacePath string, threadID modeluuid.UUID) (bool, error)
	Status(ctx context.Context, workspacePath string, threadID modeluuid.UUID) (Status, error)

	Exec(
		ctx context.Context,
		workspacePath string,
		threadID modeluuid.UUID,
		commands commandengine.CommandExecutor,
		stdout io.Writer,
		stderr io.Writer,
		name string,
		args ...string,
	) error

	ExecTTY(
		ctx context.Context,
		workspacePath string,
		threadID modeluuid.UUID,
		commands commandengine.CommandExecutor,
		stdout io.Writer,
		stderr io.Writer,
		name string,
		args ...string,
	) error

	CombinedOutput(
		ctx context.Context,
		workspacePath string,
		threadID modeluuid.UUID,
		commands commandengine.CommandExecutor,
		name string,
		args ...string,
	) ([]byte, error)

	OpenHTTPRelayPort(
		ctx context.Context,
		workspacePath string,
		threadID modeluuid.UUID,
		commands commandengine.CommandExecutor,
		callbackPort int,
		callbackTimeout time.Duration,
	) (func(context.Context) error, error)
}
