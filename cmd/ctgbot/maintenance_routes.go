package main

import (
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func registerMaintenanceRoutes(r *clir.Router, globalStore *clistate.Store) {
	actions := &projectProcessActions{globalStore: globalStore}
	r.Routes(func(b *clir.Builder) {
		b.Handle("go-generate", "Run go generate for embedded container assets", func(req *clir.Request) error {
			return actions.GoGenerate(req.Context())
		})

		b.Handle("git-pull", "Run git pull --ff-only in project_dir", func(req *clir.Request) error {
			return actions.GitPull(req.Context())
		})

		b.Handle("process install", "Install ctgbot from project_dir", func(req *clir.Request) error {
			return actions.Install(req.Context())
		})
		b.Handle("install", "Alias for process install", func(req *clir.Request) error {
			return actions.Install(req.Context())
		})

		b.Handle("process upgrade", "Update ctgbot from project_dir and rebuild runtime images", func(req *clir.Request) error {
			return actions.Upgrade(req.Context(), false)
		})
		b.Handle("upgrade", "Alias for process upgrade", func(req *clir.Request) error {
			return actions.Upgrade(req.Context(), false)
		})
		b.Handle("process upgrade all", "Update ctgbot and rebuild runtime images without cache", func(req *clir.Request) error {
			return actions.Upgrade(req.Context(), true)
		})
		b.Handle("upgrade all", "Alias for process upgrade all", func(req *clir.Request) error {
			return actions.Upgrade(req.Context(), true)
		})

		b.Handle("process quit", "Stop the running ctgbot process", func(req *clir.Request) error {
			return actions.Quit(req.Context())
		})
		b.Handle("quit", "Alias for process quit", func(req *clir.Request) error {
			return actions.Quit(req.Context())
		})
	})
}
