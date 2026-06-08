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
		"component [ <component> | list | register | unregister | help ] - Show component command help",
		"config [ get | list | set | help ] - Global config commands",
		"image [ build | list | help ] - image commands",
	} {
		if !strings.Contains(help.String(), want) {
			t.Fatalf("help output missing %q in:\n%s", want, help.String())
		}
	}

	var componentHelp bytes.Buffer
	if err := runCLICommand(context.Background(), []string{"component", "help"}, store, nil, &componentHelp); err != nil {
		t.Fatalf("component help: %v", err)
	}
	if out := componentHelp.String(); !strings.Contains(out, "component [ <component> | list | register | unregister | help ] - Show component command help") {
		t.Fatalf("component help missing compact component group in:\n%s", out)
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

func TestCLISetupCommandsFromReadme(t *testing.T) {
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	workspace := t.TempDir()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatal(err)
	}

	var workspaceSet bytes.Buffer
	if err := runCLICommand(context.Background(), []string{"workspace", "set", "default", "--path", workspace}, store, nil, &workspaceSet); err != nil {
		t.Fatalf("workspace set: %v", err)
	}
	if out := workspaceSet.String(); !strings.Contains(out, "workspace saved") || !strings.Contains(out, workspace) {
		t.Fatalf("workspace set output = %q", out)
	}

	var register bytes.Buffer
	if err := runCLICommand(context.Background(), []string{"component", "register", "process/process", "--runtime", "local"}, store, nil, &register); err != nil {
		t.Fatalf("component register process: %v", err)
	}

	var chatCreate bytes.Buffer
	if err := runCLICommand(context.Background(), []string{"chat", "create", "setup-chat"}, store, nil, &chatCreate); err != nil {
		t.Fatalf("chat create: %v", err)
	}
	chatID := firstOutputValue(t, chatCreate.String(), "id:")

	var workspaceBind bytes.Buffer
	if err := runCLICommand(context.Background(), []string{"chat", chatID, "workspace", "set", "default"}, store, nil, &workspaceBind); err != nil {
		t.Fatalf("chat workspace set: %v", err)
	}
	if out := workspaceBind.String(); !strings.Contains(out, "chat workspace updated") {
		t.Fatalf("chat workspace output = %q", out)
	}

	var componentBind bytes.Buffer
	if err := runCLICommand(context.Background(), []string{"chat", chatID, "component", "add", "command", "process/process"}, store, nil, &componentBind); err != nil {
		t.Fatalf("chat component add: %v", err)
	}
	if out := componentBind.String(); !strings.Contains(out, "chat component bound") || !strings.Contains(out, "role: command") {
		t.Fatalf("chat component add output = %q", out)
	}

	var componentList bytes.Buffer
	if err := runCLICommand(context.Background(), []string{"chat", chatID, "component", "list"}, store, nil, &componentList); err != nil {
		t.Fatalf("chat component list: %v", err)
	}
	if out := componentList.String(); !strings.Contains(out, "process") || !strings.Contains(out, "role=command") {
		t.Fatalf("chat component list output = %q", out)
	}
}

func firstOutputValue(t *testing.T, output string, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), prefix); ok {
			return strings.TrimSpace(value)
		}
	}
	t.Fatalf("output missing %q in:\n%s", prefix, output)
	return ""
}
