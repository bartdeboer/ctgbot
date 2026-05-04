package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
)

type Factory struct {
	rootDir        string
	componentsRoot string
	profile        v5runtime.Profile
}

func New(rootDir string, componentsRoot string, profile v5runtime.Profile) *Factory {
	return &Factory{
		rootDir:        strings.TrimSpace(rootDir),
		componentsRoot: strings.TrimSpace(componentsRoot),
		profile:        profile,
	}
}

func (f *Factory) Kind() string {
	return "local"
}

func (f *Factory) Profile() v5runtime.Profile {
	return f.profile
}

func (f *Factory) ComponentHome(registration coremodel.Component) v5runtime.Home {
	hostPath := filepath.Join(f.componentsRoot, registration.Type, registration.Name)
	return v5runtime.Home{
		HostPath:      hostPath,
		ContainerPath: "/profile/components/" + registration.Type + "/" + registration.Name,
	}
}

func (f *Factory) Bind(
	registration coremodel.Component,
	home v5runtime.Home,
	image string,
	env []string,
) v5runtime.Runtime {
	_ = registration
	return &Runtime{
		rootDir: f.rootDir,
		profile: f.profile,
		home:    home,
		image:   strings.TrimSpace(image),
		env:     append([]string{}, env...),
	}
}

type Runtime struct {
	rootDir string
	profile v5runtime.Profile
	home    v5runtime.Home
	image   string
	env     []string
}

func (r *Runtime) Kind() string {
	return "local"
}

func (r *Runtime) Profile() v5runtime.Profile {
	return r.profile
}

func (r *Runtime) ComponentHome() v5runtime.Home {
	return r.home
}

func (r *Runtime) ThreadWorkspace(threadID modeluuid.UUID) (string, string, error) {
	if threadID.IsNull() {
		return "", "", fmt.Errorf("missing thread id")
	}
	hostPath := filepath.Join(r.rootDir, ".ctgbot", "threads", threadID.String(), "workspace")
	if err := os.MkdirAll(filepath.Join(hostPath, "inbox"), 0o755); err != nil {
		return "", "", err
	}
	return hostPath, "/workspace", nil
}

func (r *Runtime) Exec(
	ctx context.Context,
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	stdout io.Writer,
	stderr io.Writer,
	name string,
	args ...string,
) error {
	_, _, _, _, _, _, _, _ = ctx, threadID, commands, stdout, stderr, name, args, r.image
	_ = r.env
	return fmt.Errorf("local runtime is not implemented yet")
}

func (r *Runtime) CombinedOutput(
	ctx context.Context,
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	name string,
	args ...string,
) ([]byte, error) {
	_, _, _, _, _, _ = ctx, threadID, commands, name, args, r.image
	_ = r.env
	return nil, fmt.Errorf("local runtime is not implemented yet")
}

func (r *Runtime) OpenHTTPRelayPort(
	ctx context.Context,
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	callbackPort int,
	callbackTimeout time.Duration,
) (func(context.Context) error, error) {
	_, _, _, _, _, _ = ctx, threadID, commands, callbackPort, callbackTimeout, r.image
	_ = r.env
	return nil, fmt.Errorf("local runtime is not implemented yet")
}
