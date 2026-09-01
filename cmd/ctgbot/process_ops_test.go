package main

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/go-clistate"
)

func TestImageBuildArgs(t *testing.T) {
	tests := []struct {
		name    string
		noCache bool
		want    []string
	}{
		{
			name:    "cached",
			noCache: false,
			want:    []string{"image", "build"},
		},
		{
			name:    "no cache",
			noCache: true,
			want:    []string{"image", "build", "--no-cache"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := imageBuildArgs(tt.noCache); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("imageBuildArgs(%v) = %#v, want %#v", tt.noCache, got, tt.want)
			}
		})
	}
}

func TestGoInstallArgsInstallsHostCtgbotOnly(t *testing.T) {
	want := []string{
		"install",
		"./cmd/ctgbot",
	}
	if got := goInstallArgs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("goInstallArgs() = %#v, want %#v", got, want)
	}
}

func TestInstallGeneratesEmbeddedBuildContextBeforeInstalling(t *testing.T) {
	oldProject := runProjectCommandFunc
	t.Cleanup(func() { runProjectCommandFunc = oldProject })

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
	projectDir := t.TempDir()
	if err := store.PersistString("project_dir", projectDir); err != nil {
		t.Fatal(err)
	}

	var commands []string
	runProjectCommandFunc = func(ctx context.Context, dir string, env []string, name string, args ...string) error {
		_, _ = ctx, env
		commands = append(commands, fmt.Sprintf("%s:%s %s", dir, name, strings.Join(args, " ")))
		return nil
	}

	if err := (&projectProcessActions{globalStore: store}).Install(context.Background()); err != nil {
		t.Fatalf("Install: %v", err)
	}
	want := []string{
		projectDir + ":go generate ./internal/buildassets",
		projectDir + ":go install ./cmd/ctgbot",
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
}

func TestRunInstalledImageBuildCommand(t *testing.T) {
	old := runInstalledCtgbotCommandFunc
	t.Cleanup(func() { runInstalledCtgbotCommandFunc = old })

	var got []string
	runInstalledCtgbotCommandFunc = func(ctx context.Context, args ...string) error {
		_ = ctx
		got = append([]string(nil), args...)
		return nil
	}

	if err := runInstalledImageBuildCommand(context.Background(), true); err != nil {
		t.Fatalf("runInstalledImageBuildCommand: %v", err)
	}
	want := []string{"image", "build", "--no-cache"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("installed ctgbot args = %#v, want %#v", got, want)
	}
}

func TestUpgradeRunsDocumentedCLIContract(t *testing.T) {
	oldInstalled := runInstalledCtgbotCommandFunc
	oldProject := runProjectCommandFunc
	t.Cleanup(func() {
		runInstalledCtgbotCommandFunc = oldInstalled
		runProjectCommandFunc = oldProject
	})

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
	projectDir := t.TempDir()
	if err := store.PersistString("project_dir", projectDir); err != nil {
		t.Fatal(err)
	}

	var steps []string
	runProjectCommandFunc = func(ctx context.Context, dir string, env []string, name string, args ...string) error {
		_, _ = ctx, env
		steps = append(steps, fmt.Sprintf("project:%s:%s %s", dir, name, strings.Join(args, " ")))
		return nil
	}
	runInstalledCtgbotCommandFunc = func(ctx context.Context, args ...string) error {
		_ = ctx
		steps = append(steps, "ctgbot:"+strings.Join(args, " "))
		return nil
	}

	if err := (&projectProcessActions{globalStore: store}).Upgrade(context.Background(), true); err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	want := []string{
		"project:" + projectDir + ":git pull --ff-only",
		"ctgbot:install",
		"ctgbot:go-generate",
		"ctgbot:install",
		"ctgbot:image build --no-cache",
	}
	if !reflect.DeepEqual(steps, want) {
		t.Fatalf("upgrade steps = %#v, want %#v", steps, want)
	}
}

func TestQuitRefusesNormalShutdownWhileTurnsAreOutstanding(t *testing.T) {
	stopped := make(chan struct{}, 1)
	actions := &projectProcessActions{
		beginShutdown: func(force bool) (int, bool) {
			if force {
				t.Fatal("normal quit requested forced shutdown")
			}
			return 2, false
		},
		stop: func() { stopped <- struct{}{} },
	}

	err := actions.Quit(context.Background(), false)
	if err == nil || err.Error() != "quit refused: 2 turns active or queued; use /quit force" {
		t.Fatalf("Quit(false) error = %v", err)
	}
	select {
	case <-stopped:
		t.Fatal("refused quit stopped the runtime")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestQuitForceStopsWithOutstandingTurns(t *testing.T) {
	stopped := make(chan struct{}, 1)
	actions := &projectProcessActions{
		beginShutdown: func(force bool) (int, bool) {
			if !force {
				t.Fatal("forced quit requested normal shutdown")
			}
			return 2, true
		},
		stop: func() { stopped <- struct{}{} },
	}

	if err := actions.Quit(context.Background(), true); err != nil {
		t.Fatalf("Quit(true) error = %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("forced quit did not stop the runtime")
	}
}
