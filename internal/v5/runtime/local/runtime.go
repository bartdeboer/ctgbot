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

type LocalRuntime struct {
	rootDir string
	profile v5runtime.Profile
}

func New(rootDir string, profile v5runtime.Profile) *LocalRuntime {
	return &LocalRuntime{
		rootDir: strings.TrimSpace(rootDir),
		profile: profile,
	}
}

func (r *LocalRuntime) Kind() string {
	return "local"
}

func (r *LocalRuntime) Profile() v5runtime.Profile {
	return r.profile
}

func (r *LocalRuntime) ComponentHome(registration coremodel.Component) v5runtime.Home {
	hostPath := filepath.Join(r.profile.Root, "components", registration.Type, registration.Name)
	return v5runtime.Home{
		HostPath:      hostPath,
		ContainerPath: "/profile/components/" + registration.Type + "/" + registration.Name,
	}
}

func (r *LocalRuntime) ThreadWorkspace(threadID modeluuid.UUID) (string, string, error) {
	if threadID.IsNull() {
		return "", "", fmt.Errorf("missing thread id")
	}
	hostPath := filepath.Join(r.rootDir, ".ctgbot", "threads", threadID.String(), "workspace")
	if err := os.MkdirAll(filepath.Join(hostPath, "inbox"), 0o755); err != nil {
		return "", "", err
	}
	return hostPath, "/workspace", nil
}

func (r *LocalRuntime) Exec(
	ctx context.Context,
	registration coremodel.Component,
	threadID modeluuid.UUID,
	home v5runtime.Home,
	image string,
	workdir string,
	env []string,
	developerInstructions string,
	commands commandengine.CommandExecutor,
	stdout io.Writer,
	stderr io.Writer,
	name string,
	args ...string,
) error {
	_, _, _, _, _, _, _, _, _, _, _, _ = ctx, registration, threadID, home, image, workdir, env, developerInstructions, commands, stdout, stderr, name
	_ = args
	return fmt.Errorf("local runtime is not implemented yet")
}

func (r *LocalRuntime) CombinedOutput(
	ctx context.Context,
	registration coremodel.Component,
	threadID modeluuid.UUID,
	home v5runtime.Home,
	image string,
	workdir string,
	env []string,
	developerInstructions string,
	commands commandengine.CommandExecutor,
	name string,
	args ...string,
) ([]byte, error) {
	_, _, _, _, _, _, _, _, _, _ = ctx, registration, threadID, home, image, workdir, env, developerInstructions, commands, name
	_ = args
	return nil, fmt.Errorf("local runtime is not implemented yet")
}

func (r *LocalRuntime) OpenHTTPRelayPort(
	ctx context.Context,
	registration coremodel.Component,
	threadID modeluuid.UUID,
	home v5runtime.Home,
	image string,
	workdir string,
	env []string,
	developerInstructions string,
	commands commandengine.CommandExecutor,
	callbackPort int,
	callbackTimeout time.Duration,
) (func(context.Context) error, error) {
	_, _, _, _, _, _, _, _, _, _, _ = ctx, registration, threadID, home, image, workdir, env, developerInstructions, commands, callbackPort, callbackTimeout
	return nil, fmt.Errorf("local runtime is not implemented yet")
}
