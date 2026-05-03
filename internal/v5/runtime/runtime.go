package runtime

import (
	"context"
	"io"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
)

type Profile struct {
	Name    string
	Runtime string
	Root    string
}

type Home struct {
	HostPath      string
	ContainerPath string
}

type Factory interface {
	Kind() string
	Profile() Profile
	ComponentHome(registration coremodel.Component) Home
	Bind(
		registration coremodel.Component,
		home Home,
		image string,
		env []string,
	) Runtime
}

type Runtime interface {
	Kind() string
	Profile() Profile
	ComponentHome() Home
	ThreadWorkspace(threadID modeluuid.UUID) (string, string, error)

	Exec(
		ctx context.Context,
		threadID modeluuid.UUID,
		commands commandengine.CommandExecutor,
		stdout io.Writer,
		stderr io.Writer,
		name string,
		args ...string,
	) error

	CombinedOutput(
		ctx context.Context,
		threadID modeluuid.UUID,
		commands commandengine.CommandExecutor,
		name string,
		args ...string,
	) ([]byte, error)

	OpenHTTPRelayPort(
		ctx context.Context,
		threadID modeluuid.UUID,
		commands commandengine.CommandExecutor,
		callbackPort int,
		callbackTimeout time.Duration,
	) (func(context.Context) error, error)
}
