package runtime

import (
	"context"
	"io"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
)

type Home struct {
	Path string
}

const DefaultWorkspaceRuntimePath = "/workspace"

type Factory interface {
	Kind() string
	ComponentHome(registration coremodel.Component) Home
	RuntimeComponentHomePath(registration coremodel.Component, home Home) string
	RuntimeWorkspacePath(workspacePath string) string
	Bind(
		registration coremodel.Component,
		home Home,
		image string,
		env []string,
	) Runtime
}

type Runtime interface {
	Kind() string
	ComponentHome() Home
	RuntimeComponentHomePath() string
	RuntimeWorkspacePath(workspacePath string) string

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
