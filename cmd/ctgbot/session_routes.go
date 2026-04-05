package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
	"github.com/bartdeboer/ctgbot/internal/appconfig"
	"github.com/bartdeboer/ctgbot/internal/codexengine"
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
			conv, err := exec.StartConversation(req.Context(), 1, 0, *workspace)
			if err != nil {
				return err
			}

			fmt.Printf("conversation started: %s\n", conv.ContainerName)
			fmt.Printf("workspace: %s\n", conv.WorkspaceHost)
			return nil
		})
	})
}
