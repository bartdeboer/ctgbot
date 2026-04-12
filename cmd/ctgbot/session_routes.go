package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/chatbroker"
	"github.com/bartdeboer/ctgbot/internal/codexengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
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

			cfg, err := appstate.NewConfig("", store)
			if err != nil {
				return err
			}
			if err := cfg.EnsurePaths(); err != nil {
				return err
			}

			logger := log.New(os.Stdout, "", log.LstdFlags)
			broker := chatbroker.New(cfg, nil, sandboxengine.NewSandboxManager(logger), logger)
			broker.RegisterAgent("codex", codexengine.NewSessionExecutor(cfg, logger))
			chatID := modeluuid.New()
			thread := &chatbroker.Thread{ID: modeluuid.New(), ChatID: chatID, ProviderThreadID: "session-new"}
			conv, err := broker.StartSession(req.Context(), chatID, thread, *workspace, true)
			if err != nil {
				return err
			}
			if err := broker.PrepareSession(req.Context(), conv); err != nil {
				return err
			}

			fmt.Printf("conversation prepared: %s\n", conv.ContainerName(cfg))
			fmt.Printf("workspace: %s\n", conv.WorkspaceHost)
			return nil
		})
	})
}
