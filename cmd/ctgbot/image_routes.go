package main

import (
	"flag"
	"log"
	"os"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/ctgbotimage"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func registerImageRoutes(r *clir.Router, store *clistate.Store) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("image build", "Build the ctgbot Docker image", func(req *clir.Request) error {
			fs := flag.NewFlagSet("image build", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			noCache := fs.Bool("no-cache", false, "Build without Docker layer cache")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			cfg, err := appstate.NewConfig("", store)
			if err != nil {
				return err
			}
			logger := log.New(os.Stdout, "", log.LstdFlags)
			builder := &ctgbotimage.Builder{Config: cfg, Logger: logger}
			return builder.Build(req.Context(), *noCache)
		})
	})
}
