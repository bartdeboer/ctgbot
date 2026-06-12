package runtime

import (
	"context"
	"io"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
)

type Profile struct {
	Path string
}

type BindConfig struct {
	Image       string               `json:"image"`
	Dockerfile  string               `json:"dockerfile,omitempty"`
	Entrypoint  string               `json:"entrypoint,omitempty"`
	Env         []string             `json:"env"`
	GPUs        string               `json:"gpus"`
	Seccomp     string               `json:"seccomp"`
	Cmd         []string             `json:"cmd,omitempty"`
	IdleTimeout string               `json:"idle_timeout,omitempty"`
	UID         *int                 `json:"uid,omitempty"`
	GID         *int                 `json:"gid,omitempty"`
	NoCache     bool                 `json:"no_cache,omitempty"`
	Uses        *runtimeimage.Target `json:"uses,omitempty"`
}

type ThreadConfig struct {
	Ports []string
}

type ThreadConfigResolver interface {
	RuntimeThreadConfig(ctx context.Context, threadID modeluuid.UUID) (ThreadConfig, error)
}

type Status struct {
	Name                 string
	State                string
	RuntimeProfilePath   string
	RuntimeWorkspacePath string
	RuntimeHomePath      string
	Ports                []string
	ActiveCommandName    string
	ActiveCommandArgs    []string
	RuntimeNotices       []string
}

const DefaultWorkspaceRuntimePath = "/workspace"

type Factory interface {
	Kind() string
	ComponentProfile(registration coremodel.Component) Profile
	RuntimeComponentProfilePath(registration coremodel.Component, profile Profile) string
	RuntimeWorkspacePath(workspacePath string) string
}

type ThreadRuntimeFactory interface {
	Factory
	Bind(
		registration coremodel.Component,
		profile Profile,
		config BindConfig,
	) ThreadRuntime
}

type ServiceRuntime interface {
	ComponentProfile() Profile
	BaseURL() string
	Start(ctx context.Context) (Status, error)
	Stop(ctx context.Context) error
	Refresh(ctx context.Context) error
	Status(ctx context.Context) (Status, error)
}

type ThreadRuntime interface {
	Kind() string
	ComponentProfile() Profile
	RuntimeComponentProfilePath() string
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
