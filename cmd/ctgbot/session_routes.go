package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/bartdeboer/ctgbot/internal/appconfig"
	"github.com/bartdeboer/ctgbot/internal/codexengine"
	"github.com/bartdeboer/ctgbot/internal/hostbridgetls"
	"github.com/bartdeboer/ctgbot/internal/providerengine"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/telegramengine"
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
			conv := &telegramengine.ChatSession{
				ChatID:             1,
				ThreadID:           0,
				Active:             true,
				ProviderType:       "codex",
				ContainerName:      cfg.ChatContainerName(1, 0),
				WorkspaceHost:      workspaceHostPath,
				HomeHost:           cfg.ChatCodexHomeDirByID(1),
				ThreadRuntimeHost:  cfg.ChatThreadsRoot(1),
				ContainerWorkspace: cfg.ContainerWorkspacePath(),
				ContainerHome:      cfg.ContainerHomePath(),
			}
			if err := hostbridgetls.EnsureChatClientMaterials(cfg.HostbridgeTLSRoot(), cfg.ChatThreadTLSDir(conv.ChatID, conv.ThreadID), conv.ContainerName); err != nil {
				return err
			}
			if err := exec.PrepareSandbox(providerengine.PrepareSandboxRequest{
				ProfilePath:         conv.HomeHost,
				WorkspacePath:       conv.WorkspaceHost,
				ContainerHome:       conv.ContainerHome,
				ContainerWorkspace:  conv.ContainerWorkspace,
				HostOS:              "darwin",
				HostbridgeAddr:      cfg.ContainerHostbridgeTCPAddr(),
				AllowedHostCommands: nil,
			}); err != nil {
				return err
			}
			sandboxes := &sandboxengine.DockerManager{Logger: logger}
			if err := sandboxes.Remove(req.Context(), conv.ContainerName); err != nil {
				logger.Printf("ignoring stale sandbox cleanup error for %s: %v", conv.ContainerName, err)
			}
			spec := exec.SandboxSpec(providerengine.SandboxSpecRequest{
				SandboxName:        conv.ContainerName,
				ProfilePath:        conv.HomeHost,
				WorkspacePath:      conv.WorkspaceHost,
				ContainerHome:      conv.ContainerHome,
				ContainerWorkspace: conv.ContainerWorkspace,
			})
			spec.SecurityOpts = append(spec.SecurityOpts, "seccomp=unconfined")
			spec.Env = append(spec.Env,
				"HOSTBRIDGE_ADDR="+cfg.ContainerHostbridgeTCPAddr(),
				"HOSTBRIDGE_TLS_DIR="+cfg.ContainerHostbridgeTLSDir(),
			)
			spec.Mounts = append(spec.Mounts, sandboxengine.Mount{
				Source:   cfg.ChatThreadTLSDir(conv.ChatID, conv.ThreadID),
				Target:   cfg.ContainerHostbridgeTLSDir(),
				ReadOnly: true,
			})
			if _, _, err := sandboxes.Ensure(req.Context(), spec); err != nil {
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
