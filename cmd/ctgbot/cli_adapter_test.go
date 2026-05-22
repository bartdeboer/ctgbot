package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/bartdeboer/go-clistate"
)

func TestCLICommandSurfacesExposeProcessAndServiceCommands(t *testing.T) {
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatal(err)
	}

	var help bytes.Buffer
	if err := runCLICommand(context.Background(), []string{"help"}, store, nil, &help); err != nil {
		t.Fatalf("help: %v", err)
	}
	for _, want := range []string{
		"run - Run the ctgbot runtime",
		"component help - Show component command help",
		"config list - List config keys",
		"image help - image commands",
	} {
		if !strings.Contains(help.String(), want) {
			t.Fatalf("help output missing %q in:\n%s", want, help.String())
		}
	}

	var componentHelp bytes.Buffer
	if err := runCLICommand(context.Background(), []string{"component", "help"}, store, nil, &componentHelp); err != nil {
		t.Fatalf("component help: %v", err)
	}
	if out := componentHelp.String(); !strings.Contains(out, "component register <component> - Register a component instance") {
		t.Fatalf("component help missing register command in:\n%s", out)
	}

	var register bytes.Buffer
	if err := runCLICommand(context.Background(), []string{"component", "register", "telegram/test", "--runtime", "local"}, store, nil, &register); err != nil {
		t.Fatalf("component register: %v", err)
	}
	if out := register.String(); !strings.Contains(out, "component registered") || !strings.Contains(out, "ref: telegram/test") {
		t.Fatalf("register output = %q", out)
	}

	var list bytes.Buffer
	if err := runCLICommand(context.Background(), []string{"component", "list"}, store, nil, &list); err != nil {
		t.Fatalf("component list: %v", err)
	}
	if out := list.String(); !strings.Contains(out, "telegram/test") {
		t.Fatalf("list output = %q", out)
	}
}
