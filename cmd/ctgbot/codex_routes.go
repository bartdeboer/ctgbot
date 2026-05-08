package main

import (
	"flag"
	"log"
	"os"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/codexcli"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func registerCodexRoutes(r *clir.Router, store *clistate.Store) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("codex", "Run the normal Codex CLI inside the ctgbot Docker image", func(req *clir.Request) error {
			cfg, err := appstate.NewConfig("", store)
			if err != nil {
				return err
			}
			logger := log.New(os.Stdout, "", log.LstdFlags)
			manager := &codexcli.Manager{Config: cfg, Logger: logger}
			return manager.RunCLI(req.Context(), "", req.Extra)
		})

		b.Handle("codex signin", "Sign in to Codex inside the bot image and persist auth on the host", func(req *clir.Request) error {
			fs := flag.NewFlagSet("codex signin", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			deviceAuth := fs.Bool("device-auth", false, "Use device auth flow")
			withAPIKey := fs.Bool("with-api-key", false, "Read OPENAI_API_KEY from stdin")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			cfg, err := appstate.NewConfig("", store)
			if err != nil {
				return err
			}
			logger := log.New(os.Stdout, "", log.LstdFlags)
			manager := &codexcli.Manager{Config: cfg, Logger: logger}
			return manager.SignIn(req.Context(), *deviceAuth, *withAPIKey)
		})

		b.Handle("codex status", "Show Codex login status using the bot's shared Codex home", func(req *clir.Request) error {
			cfg, err := appstate.NewConfig("", store)
			if err != nil {
				return err
			}
			logger := log.New(os.Stdout, "", log.LstdFlags)
			manager := &codexcli.Manager{Config: cfg, Logger: logger}
			return manager.LoginStatus(req.Context())
		})
	})
}
