package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/bartdeboer/ctgbot/internal/appconfig"
	"github.com/bartdeboer/ctgbot/internal/codexengine"
	"github.com/bartdeboer/ctgbot/internal/conversationmodel"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func registerSessionRoutes(r *clir.Router, store *clistate.Store) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("session new", "Start a local test conversation container", func(req *clir.Request) error {
			fs := flag.NewFlagSet("session new", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)

			workspace := fs.String("workspace", "", "Host workspace path to mount into the container")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			cfg, err := appconfig.NewConfig("", store)
			if err != nil {
				return err
			}
			if err := cfg.EnsurePaths(); err != nil {
				return err
			}

			logger := log.New(os.Stdout, "", log.LstdFlags)
			exec := &codexengine.SessionExecutor{Config: cfg, Logger: logger}
			if _, err := cfg.EnsureChatRuntimePaths(1); err != nil {
				return err
			}
			workspaceHostPath, err := cfg.ResolveChatWorkspaceHostPath(1, 0, *workspace)
			if err != nil {
				return err
			}
			conv := &conversationmodel.ChatSession{
				ChatID:             1,
				ThreadID:           0,
				Active:             true,
				ProviderType:       "codex",
				ContainerName:      cfg.ChatContainerName(1, 0),
				WorkspaceHost:      workspaceHostPath,
				HomeHost:           cfg.ChatCodexHomeDirByID(1),
				ContainerWorkspace: cfg.ContainerWorkspacePath(),
				ContainerHome:      cfg.ContainerHomePath(),
			}
			if err := exec.PrepareConversation(req.Context(), conv); err != nil {
				return err
			}
			sandboxes := &sandboxengine.DockerManager{Logger: logger}
			if err := sandboxes.Remove(req.Context(), conv.ContainerName); err != nil {
				logger.Printf("ignoring stale sandbox cleanup error for %s: %v", conv.ContainerName, err)
			}
			if _, err := sandboxes.Ensure(req.Context(), exec.SandboxSpec(conv)); err != nil {
				return err
			}
			if err := sandboxes.Stop(req.Context(), conv.ContainerName); err != nil {
				return err
			}

			fmt.Printf("conversation prepared: %s\n", conv.ContainerName)
			fmt.Printf("workspace: %s\n", conv.WorkspaceHost)
			return nil
		})
	})
}
