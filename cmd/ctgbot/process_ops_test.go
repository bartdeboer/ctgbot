package main

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

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
