package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
	"github.com/bartdeboer/go-codextgbot/internal/botengine"
)

func registerCodexRoutes(r *clir.Router, store *clistate.Store) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("codex", "Run the normal Codex CLI inside the codextgbot Docker image", func(req *clir.Request) error {
			cfg, err := botengine.NewConfig("", store)
			if err != nil {
				return err
			}
			logger := log.New(os.Stdout, "", log.LstdFlags)
			manager := &botengine.CodexManager{Config: cfg, Logger: logger}
			return manager.RunCLI(context.Background(), "", req.Extra)
		})

		b.Handle("codex signin", "Sign in to Codex inside the bot image and persist auth on the host", func(req *clir.Request) error {
			fs := flag.NewFlagSet("codex signin", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			deviceAuth := fs.Bool("device-auth", true, "Use device auth flow")
			withAPIKey := fs.Bool("with-api-key", false, "Read OPENAI_API_KEY from stdin")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			cfg, err := botengine.NewConfig("", store)
			if err != nil {
				return err
			}
			logger := log.New(os.Stdout, "", log.LstdFlags)
			manager := &botengine.CodexManager{Config: cfg, Logger: logger}
			return manager.SignIn(context.Background(), *deviceAuth, *withAPIKey)
		})

		b.Handle("codex status", "Show Codex login status using the bot's shared Codex home", func(req *clir.Request) error {
			cfg, err := botengine.NewConfig("", store)
			if err != nil {
				return err
			}
			logger := log.New(os.Stdout, "", log.LstdFlags)
			manager := &botengine.CodexManager{Config: cfg, Logger: logger}
			return manager.LoginStatus(context.Background())
		})
	})
}
