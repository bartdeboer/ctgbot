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

func main() {
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot open cwd config: %v\n", err)
	}

	globalStore, err := clistate.NewGlobal("ctgbot", "config")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot open global config: %v\n", err)
	}

	r := clir.New()

	registerConfigRoutes(r, store, globalStore)
	registerImageRoutes(r, store)
	registerCodexRoutes(r, store)
	registerTelegramRoutes(r, store)
	registerHostbridgeRoutes(r, store)
	registerSessionRoutes(r, store)

	r.Routes(func(b *clir.Builder) {
		b.Handle("go-generate", "Run go generate for embedded container assets", func(req *clir.Request) error {
			projectDir := ""
			if globalStore != nil {
				projectDir = globalStore.GetProjectDir()
			}
			if projectDir == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				projectDir = cwd
			}

			cmd := exec.CommandContext(req.Context(), "go", "generate", "./internal/containerassets")
			cmd.Dir = projectDir
			cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
			return cmd.Run()
		})

		b.Handle("install", "Install ctgbot from project_dir", func(req *clir.Request) error {
			if globalStore == nil {
				return fmt.Errorf("global config store is not available")
			}

			projectDir := globalStore.GetProjectDir()
			if projectDir == "" {
				return fmt.Errorf("project_dir not configured; run `go run ./cmd/ctgbot install` from the ctgbot source repo first")
			}

			generateCmd := exec.CommandContext(req.Context(), "go", "generate", "./internal/containerassets")
			generateCmd.Dir = projectDir
			generateCmd.Env = buildInstallEnv(globalStore)
			generateCmd.Stdout, generateCmd.Stderr = os.Stdout, os.Stderr
			if err := generateCmd.Run(); err != nil {
				return err
			}

			installCmd := exec.CommandContext(req.Context(), "go", "install", "./cmd/ctgbot", "./cmd/hostbridge")
			installCmd.Dir = projectDir
			installCmd.Env = buildInstallEnv(globalStore)
			installCmd.Stdout, installCmd.Stderr = os.Stdout, os.Stderr
			return installCmd.Run()
		})
	})

	if err := r.Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Println("error:", err)
		fmt.Println("usage: ctgbot <command>... [args]")
		fmt.Println("available commands:")
		r.PrintHelp(os.Stdout)
		os.Exit(1)
	}
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
