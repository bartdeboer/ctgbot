package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func registerMaintenanceRoutes(r *clir.Router, globalStore *clistate.Store) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("go-generate", "Run go generate for embedded container assets", func(req *clir.Request) error {
			projectDir, err := resolveGenerateProjectDir(globalStore)
			if err != nil {
				return err
			}
			return runProjectCommand(req.Context(), projectDir, nil, "go", "generate", "./internal/containerassets")
		})

		b.Handle("git-pull", "Run git pull --ff-only in project_dir", func(req *clir.Request) error {
			projectDir, err := requireProjectDir(globalStore)
			if err != nil {
				return err
			}
			return runProjectCommand(req.Context(), projectDir, nil, "git", "pull", "--ff-only")
		})

		b.Handle("install", "Install ctgbot from project_dir", func(req *clir.Request) error {
			projectDir, err := requireProjectDir(globalStore)
			if err != nil {
				return err
			}
			env := buildInstallEnv(globalStore)
			if err := runProjectCommand(req.Context(), projectDir, env, "go", "generate", "./internal/containerassets"); err != nil {
				return err
			}
			return runProjectCommand(req.Context(), projectDir, env, "go", "install", "./cmd/ctgbot", "./cmd/hostbridge")
		})

		b.Handle("upgrade", "Update ctgbot from project_dir and rebuild the Docker image", func(req *clir.Request) error {
			projectDir, err := requireProjectDir(globalStore)
			if err != nil {
				return err
			}
			env := buildInstallEnv(globalStore)

			if err := runUpgradeStep(req.Context(), "git pull", func(ctx context.Context) error {
				return runProjectCommand(ctx, projectDir, nil, "git", "pull", "--ff-only")
			}); err != nil {
				return err
			}
			if err := runUpgradeStep(req.Context(), "go generate", func(ctx context.Context) error {
				return runProjectCommand(ctx, projectDir, env, "go", "generate", "./internal/containerassets")
			}); err != nil {
				return err
			}
			if err := runUpgradeStep(req.Context(), "go install", func(ctx context.Context) error {
				return runProjectCommand(ctx, projectDir, env, "go", "install", "./cmd/ctgbot", "./cmd/hostbridge")
			}); err != nil {
				return err
			}
			return runUpgradeStep(req.Context(), "ctgbot image build --no-cache", func(ctx context.Context) error {
				return runInstalledCtgbotCommand(ctx, "image", "build", "--no-cache")
			})
		})
	})
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
	binPath, err := exec.LookPath("ctgbot")
	if err != nil {
		exePath, exeErr := os.Executable()
		if exeErr != nil {
			return fmt.Errorf("resolve ctgbot executable: %w", err)
		}
		binPath = exePath
	}
	cmd := exec.CommandContext(ctx, binPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
