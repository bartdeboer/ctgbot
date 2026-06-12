package main

import (
	"context"
	"fmt"
	"os"

	"github.com/bartdeboer/ctgbot/internal/supervisor"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: ctgbot-supervisor <service.json>")
		os.Exit(2)
	}
	if err := supervisor.RunFile(context.Background(), os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
