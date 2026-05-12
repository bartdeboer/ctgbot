package main

import (
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/buildassets"
	"github.com/bartdeboer/go-clir"
)

func registerVersionRoutes(r *clir.Router) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("version", "Show ctgbot version", func(req *clir.Request) error {
			_ = req
			fmt.Println(buildassets.Version())
			return nil
		})
	})
}
