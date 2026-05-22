package main

import (
	"context"
	"fmt"
	"os"

	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func main() {
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot open cwd config: %v\n", err)
	}

	globalStore, err := clistate.NewGlobal("ctgbot", "config")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: cannot open global config: %v\n", err)
	}

	ctx := context.Background()
	argv := os.Args[1:]
	if len(argv) > 0 && argv[0] == "run" {
		r := clir.New()
		registerRuntimeRoutes(r, store, globalStore)
		if err := r.Run(ctx, argv); err != nil {
			fmt.Println("error:", err)
			fmt.Println("usage: ctgbot run [args]")
			os.Exit(1)
		}
		return
	}
	runCLIOrExit(ctx, argv, store, globalStore)
}
