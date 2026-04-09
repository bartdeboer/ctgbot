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

	r := clir.New()

	registerMaintenanceRoutes(r, globalStore)
	registerConfigRoutes(r, store, globalStore)
	registerImageRoutes(r, store)
	registerCodexRoutes(r, store)
	registerTelegramRoutes(r, store)
	registerHostbridgeRoutes(r, store)
	registerSessionRoutes(r, store)

	if err := r.Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Println("error:", err)
		fmt.Println("usage: ctgbot <command>... [args]")
		fmt.Println("available commands:")
		r.PrintHelp(os.Stdout)
		os.Exit(1)
	}
}
