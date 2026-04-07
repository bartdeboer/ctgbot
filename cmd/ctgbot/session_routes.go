package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/bartdeboer/ctgbot/internal/appconfig"
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

			cfg, err := appconfig.NewConfig("", store)
			if err != nil {
				return err
			}
			if err := cfg.EnsurePaths(); err != nil {
				return err
			}

			logger := log.New(os.Stdout, "", log.LstdFlags)
			broker := chatbroker.New(cfg, nil, &sandboxengine.DockerManager{Logger: logger}, logger)
			broker.RegisterAgent("codex", &codexengine.SessionExecutor{Config: cfg, Logger: logger})
			chat := &chatbroker.Chat{ID: modeluuid.New(), ProviderType: "local", ProviderChatID: "session-new"}
			thread := &chatbroker.Thread{ID: modeluuid.New(), ChatID: chat.ID, ProviderThreadID: "session-new"}
			conv, err := broker.StartSession(req.Context(), chat, thread, *workspace, true)
			if err != nil {
				return err
			}
			if err := broker.PrepareSession(req.Context(), chat, conv); err != nil {
				return err
			}

			fmt.Printf("conversation prepared: %s\n", conv.ContainerName)
			fmt.Printf("workspace: %s\n", conv.WorkspaceHost)
			return nil
		})
	})
}
