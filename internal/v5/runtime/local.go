package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
)

type LocalRuntime struct {
	rootDir string
	profile component.Profile
}

func newLocalRuntime(rootDir string, profile component.Profile) *LocalRuntime {
	return &LocalRuntime{
		rootDir: strings.TrimSpace(rootDir),
		profile: profile,
	}
}

func (r *LocalRuntime) Kind() string {
	return "local"
}

func (r *LocalRuntime) Profile() component.Profile {
	return r.profile
}

func (r *LocalRuntime) ComponentHome(registration coremodel.Component) component.Home {
	hostPath := filepath.Join(r.profile.Root, "components", registration.Type, registration.Name)
	return component.Home{
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

func (r *LocalRuntime) StartAuth(ctx context.Context, registration coremodel.Component, home component.Home, image string, workdir string, env []string) (*sandboxengine.Sandbox, error) {
	_, _, _, _, _ = ctx, registration, home, image, workdir
	_ = env
	return nil, fmt.Errorf("local runtime is not implemented yet")
}

func (r *LocalRuntime) StartTurn(ctx context.Context, registration coremodel.Component, thread coremodel.Thread, home component.Home, image string, workdir string, env []string, developerInstructions string, commands commandengine.CommandExecutor) (*sandboxengine.SandboxRuntime, error) {
	_, _, _, _, _, _, _, _ = ctx, registration, thread, home, image, workdir, env, developerInstructions
	_ = commands
	return nil, fmt.Errorf("local runtime is not implemented yet")
}
