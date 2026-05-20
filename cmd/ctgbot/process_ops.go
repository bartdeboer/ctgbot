package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/app"
	systempkg "github.com/bartdeboer/ctgbot/internal/system"
	"github.com/bartdeboer/go-clistate"
)

type projectProcessActions struct {
	globalStore *clistate.Store
	stop        context.CancelFunc
	logger      *log.Logger
}

func (p *projectProcessActions) GoGenerate(ctx context.Context) error {
	projectDir, err := resolveGenerateProjectDir(p.globalStore)
	if err != nil {
		return err
	}
	return runProjectCommand(ctx, projectDir, nil, "go", "generate", "./internal/buildassets")
}

func (p *projectProcessActions) GitPull(ctx context.Context) error {
	projectDir, err := requireProjectDir(p.globalStore)
	if err != nil {
		return err
	}
	return runProjectCommand(ctx, projectDir, nil, "git", "pull", "--ff-only")
}

func (p *projectProcessActions) Install(ctx context.Context) error {
	p.logf("running ctgbot install")
	projectDir, err := requireProjectDir(p.globalStore)
	if err != nil {
		return err
	}
	env := buildInstallEnv(p.globalStore)
	if err := runProjectCommand(ctx, projectDir, env, "go", "generate", "./internal/buildassets"); err != nil {
		return err
	}
	return runProjectCommand(ctx, projectDir, env, "go", "install", "./cmd/ctgbot", "./cmd/hostbridge")
}

func (p *projectProcessActions) Upgrade(ctx context.Context, all bool) error {
	if all {
		p.logf("running ctgbot upgrade all")
	} else {
		p.logf("running ctgbot upgrade")
	}
	projectDir, err := requireProjectDir(p.globalStore)
	if err != nil {
		return err
	}
	env := buildInstallEnv(p.globalStore)

	if err := runUpgradeStep(ctx, "git pull", func(ctx context.Context) error {
		return runProjectCommand(ctx, projectDir, nil, "git", "pull", "--ff-only")
	}); err != nil {
		return err
	}
	if err := runUpgradeStep(ctx, "go generate", func(ctx context.Context) error {
		return runProjectCommand(ctx, projectDir, env, "go", "generate", "./internal/buildassets")
	}); err != nil {
		return err
	}
	if err := runUpgradeStep(ctx, "go install", func(ctx context.Context) error {
		return runProjectCommand(ctx, projectDir, env, "go", "install", "./cmd/ctgbot", "./cmd/hostbridge")
	}); err != nil {
		return err
	}
	imageBuildArgs := []string{"image", "build"}
	imageBuildLabel := "ctgbot image build"
	if all {
		imageBuildArgs = append(imageBuildArgs, "--no-cache")
		imageBuildLabel = "ctgbot image build --no-cache"
	}
	return runUpgradeStep(ctx, imageBuildLabel, func(ctx context.Context) error {
		return runInstalledCtgbotCommand(ctx, imageBuildArgs...)
	})
}

func (p *projectProcessActions) ImageList(ctx context.Context) (string, error) {
	p.logf("listing ctgbot runtime images")
	logger := p.logger
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	rtSystem, err := systempkg.Open(ctx, "", "", p.globalStore, logger)
	if err != nil {
		return "", err
	}
	rtSystem.Registry, err = newRuntimeRegistry(rtSystem, p)
	if err != nil {
		return "", err
	}
	appService := app.NewServiceWithLogger(rtSystem.Storage, rtSystem, logger.Printf)
	targets, err := appService.RuntimeImageTargets(ctx)
	if err != nil {
		return "", err
	}
	return formatRuntimeImageTargets(targets), nil
}

func (p *projectProcessActions) Quit(ctx context.Context) error {
	_ = ctx
	p.logf("shutting down ctgbot")
	if p == nil || p.stop == nil {
		return nil
	}
	time.AfterFunc(250*time.Millisecond, p.stop)
	return nil
}

func (p *projectProcessActions) logf(format string, args ...any) {
	if p != nil && p.logger != nil {
		p.logger.Printf(format, args...)
	}
}

func resolveGenerateProjectDir(globalStore *clistate.Store) (string, error) {
	if globalStore != nil {
		if projectDir := globalStore.GetProjectDir(); projectDir != "" {
			return projectDir, nil
		}
	}
	return os.Getwd()
}

func requireProjectDir(globalStore *clistate.Store) (string, error) {
	if globalStore == nil {
		return "", fmt.Errorf("global config store is not available")
	}
	projectDir := globalStore.GetProjectDir()
	if projectDir == "" {
		return "", fmt.Errorf("project_dir not configured; run `go run ./cmd/ctgbot install` from the ctgbot source repo first")
	}
	return projectDir, nil
}

func runUpgradeStep(ctx context.Context, label string, fn func(context.Context) error) error {
	fmt.Fprintf(os.Stdout, "==> %s\n", label)
	return fn(ctx)
}

func runInstalledCtgbotCommand(ctx context.Context, args ...string) error {
	return runInstalledCtgbotCommandInDir(ctx, "", args...)
}

func runInstalledCtgbotCommandInDir(ctx context.Context, dir string, args ...string) error {
	binPath, err := resolveInstalledCtgbotPath()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, binPath, args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = strings.TrimSpace(dir)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func resolveInstalledCtgbotPath() (string, error) {
	binPath, err := exec.LookPath("ctgbot")
	if err != nil {
		exePath, exeErr := os.Executable()
		if exeErr != nil {
			return "", fmt.Errorf("resolve ctgbot executable: %w", err)
		}
		binPath = exePath
	}
	return binPath, nil
}

func runProjectCommand(ctx context.Context, projectDir string, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = projectDir
	if len(env) > 0 {
		cmd.Env = env
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func buildInstallEnv(globalStore *clistate.Store) []string {
	env := os.Environ()
	env = upsertEnv(env, "CGO_ENABLED", "1")
	if globalStore == nil {
		return env
	}

	compilerPath := strings.TrimSpace(globalStore.GetString("build.compiler_path", ""))
	if compilerPath == "" {
		return env
	}

	currentPath := os.Getenv("PATH")
	if currentPath == "" {
		return upsertEnv(env, "PATH", compilerPath)
	}
	return upsertEnv(env, "PATH", compilerPath+string(os.PathListSeparator)+currentPath)
}

func upsertEnv(env []string, key string, value string) []string {
	prefix := key + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
