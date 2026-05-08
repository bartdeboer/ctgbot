package local

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

type Factory struct {
	rootDir        string
	componentsRoot string
}

func New(rootDir string, componentsRoot string) *Factory {
	return &Factory{
		rootDir:        strings.TrimSpace(rootDir),
		componentsRoot: strings.TrimSpace(componentsRoot),
	}
}

func (f *Factory) Kind() string {
	return "local"
}

func (f *Factory) ComponentHome(registration coremodel.Component) runtimepkg.Home {
	hostPath := strings.TrimSpace(registration.HomePath)
	if hostPath == "" {
		hostPath = filepath.Join(f.componentsRoot, registration.Type, registration.Name)
	}
	return runtimepkg.Home{Path: hostPath}
}

func (f *Factory) RuntimeComponentHomePath(registration coremodel.Component, home runtimepkg.Home) string {
	_, _ = registration, home
	return home.Path
}

func (f *Factory) RuntimeWorkspacePath(workspacePath string) string {
	return strings.TrimSpace(workspacePath)
}

func (f *Factory) Bind(
	registration coremodel.Component,
	home runtimepkg.Home,
	config runtimepkg.BindConfig,
) runtimepkg.Runtime {
	_ = registration
	return &Runtime{
		rootDir: f.rootDir,
		home:    home,
		image:   strings.TrimSpace(config.Image),
		env:     append([]string{}, config.Env...),
		gpus:    strings.TrimSpace(config.GPUs),
	}
}

type Runtime struct {
	rootDir string
	home    runtimepkg.Home
	image   string
	env     []string
	gpus    string
}

func (r *Runtime) Kind() string {
	return "local"
}

func (r *Runtime) ComponentHome() runtimepkg.Home {
	return r.home
}

func (r *Runtime) RuntimeComponentHomePath() string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.home.Path)
}

func (r *Runtime) RuntimeWorkspacePath(workspacePath string) string {
	return strings.TrimSpace(workspacePath)
}

func (r *Runtime) Refresh(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
) error {
	_, _, _ = ctx, workspacePath, threadID
	return fmt.Errorf("local runtime is not implemented yet")
}

func (r *Runtime) Start(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
) (runtimepkg.Status, error) {
	_, _, _ = ctx, workspacePath, threadID
	return runtimepkg.Status{}, fmt.Errorf("local runtime is not implemented yet")
}

func (r *Runtime) Stop(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
) error {
	_, _, _ = ctx, workspacePath, threadID
	return fmt.Errorf("local runtime is not implemented yet")
}

func (r *Runtime) Interrupt(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
) (bool, error) {
	_, _, _ = ctx, workspacePath, threadID
	return false, fmt.Errorf("local runtime is not implemented yet")
}

func (r *Runtime) Status(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
) (runtimepkg.Status, error) {
	_, _, _ = ctx, workspacePath, threadID
	return runtimepkg.Status{}, fmt.Errorf("local runtime is not implemented yet")
}

func (r *Runtime) Exec(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	stdout io.Writer,
	stderr io.Writer,
	name string,
	args ...string,
) error {
	_, _, _, _, _, _, _, _, _, _ = ctx, workspacePath, threadID, commands, stdout, stderr, name, args, r.image, r.gpus
	_ = r.env
	return fmt.Errorf("local runtime is not implemented yet")
}

func (r *Runtime) CombinedOutput(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	name string,
	args ...string,
) ([]byte, error) {
	_, _, _, _, _, _, _, _ = ctx, workspacePath, threadID, commands, name, args, r.image, r.gpus
	_ = r.env
	return nil, fmt.Errorf("local runtime is not implemented yet")
}

func (r *Runtime) OpenHTTPRelayPort(
	ctx context.Context,
	workspacePath string,
	threadID modeluuid.UUID,
	commands commandengine.CommandExecutor,
	callbackPort int,
	callbackTimeout time.Duration,
) (func(context.Context) error, error) {
	_, _, _, _, _, _, _, _ = ctx, workspacePath, threadID, commands, callbackPort, callbackTimeout, r.image, r.gpus
	_ = r.env
	return nil, fmt.Errorf("local runtime is not implemented yet")
}
