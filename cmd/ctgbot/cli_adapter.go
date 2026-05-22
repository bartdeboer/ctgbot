package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/app"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/component"
	processcomponent "github.com/bartdeboer/ctgbot/internal/component/process"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clistate"
)

type cliRuntime struct {
	engine *commandengine.Engine
}

func openCLIRuntime(ctx context.Context, store *clistate.Store, globalStore *clistate.Store) (*cliRuntime, error) {
	logger := log.New(io.Discard, "", 0)
	processActions := newRuntimeProcessActions(globalStore, nil, logger)
	rtSystem, err := openSystem(ctx, store, processActions, logger)
	if err != nil {
		return nil, err
	}
	appService := app.NewServiceWithLogger(rtSystem.Storage, rtSystem, logger.Printf)
	appSurfaces, err := appService.CLICommandSurfaces(ctx)
	if err != nil {
		return nil, err
	}
	surfaces := append([]component.CommandSurface{processcomponent.New(processActions)}, appSurfaces...)
	engine, err := commandset.NewEngineForSource(commandengine.SourceCLI, surfaces...)
	if err != nil {
		return nil, err
	}
	return &cliRuntime{engine: engine}, nil
}

func runCLICommand(ctx context.Context, argv []string, store *clistate.Store, globalStore *clistate.Store, stdout io.Writer) error {
	cli, err := openCLIRuntime(ctx, store, globalStore)
	if err != nil {
		return err
	}
	actor := commandengine.Actor{ID: "cli", Roles: []simplerbac.Role{simplerbac.RoleRoot}}
	if len(argv) == 0 {
		printCLIHelp(ctx, stdout, cli.engine, actor)
		return nil
	}
	if help, ok := commandengine.ParseHelpRequest(argv); ok {
		if len(help.Scope) == 0 {
			printCLIHelp(ctx, stdout, cli.engine, actor)
			return nil
		}
		if err := cli.engine.Router.FPrintHelp(ctx, stdout, help.Scope, actor); err != nil {
			return err
		}
		return nil
	}
	result, err := cli.engine.Run(ctx, commandengine.Request{
		Context: commandengine.Context{
			Source: commandengine.SourceCLI,
			Actor:  actor,
		},
	}, argv)
	if err != nil {
		return err
	}
	if text := strings.TrimSpace(result.Text); text != "" {
		fmt.Fprintln(stdout, text)
	}
	return nil
}

func printCLIHelp(ctx context.Context, w io.Writer, engine *commandengine.Engine, actor commandengine.Actor) {
	fmt.Fprintln(w, "available commands:")
	fmt.Fprintln(w, "  run - Run the ctgbot runtime")
	if engine == nil || engine.Router == nil {
		return
	}
	var buf strings.Builder
	if err := engine.Router.FPrintHelpIndex(ctx, &buf, actor); err != nil {
		return
	}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fmt.Fprintln(w, "  "+line)
	}
}

func runCLIOrExit(ctx context.Context, argv []string, store *clistate.Store, globalStore *clistate.Store) {
	if err := runCLICommand(ctx, argv, store, globalStore, os.Stdout); err != nil {
		fmt.Println("error:", err)
		fmt.Println("usage: ctgbot <command>... [args]")
		fmt.Println("run `ctgbot help` for available commands")
		os.Exit(1)
	}
}
