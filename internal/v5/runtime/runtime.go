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

type Runtime interface {
	Kind() string
	Profile() Profile
	ComponentHome(registration coremodel.Component) Home
	ThreadWorkspace(threadID modeluuid.UUID) (string, string, error)

	Exec(
		ctx context.Context,
		registration coremodel.Component,
		threadID modeluuid.UUID,
		home Home,
		image string,
		workdir string,
		env []string,
		developerInstructions string,
		commands commandengine.CommandExecutor,
		stdout io.Writer,
		stderr io.Writer,
		name string,
		args ...string,
	) error

	CombinedOutput(
		ctx context.Context,
		registration coremodel.Component,
		threadID modeluuid.UUID,
		home Home,
		image string,
		workdir string,
		env []string,
		developerInstructions string,
		commands commandengine.CommandExecutor,
		name string,
		args ...string,
	) ([]byte, error)

	OpenHTTPRelayPort(
		ctx context.Context,
		registration coremodel.Component,
		threadID modeluuid.UUID,
		home Home,
		image string,
		workdir string,
		env []string,
		developerInstructions string,
		commands commandengine.CommandExecutor,
		callbackPort int,
		callbackTimeout time.Duration,
	) (func(context.Context) error, error)
}
