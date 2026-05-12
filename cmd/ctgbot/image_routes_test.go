package main

import (
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func TestImageListShowsNoTargetsWithoutProviders(t *testing.T) {
	withTempCwd(t, func(root string) {
		_ = root
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewCwd: %v", err)
		}
		router := clir.New()
		registerImageRoutes(router, store)

		output := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"image", "list"}); err != nil {
				t.Fatalf("image list: %v", err)
			}
		})
		if !strings.Contains(output, "no runtime image targets") {
			t.Fatalf("image list output = %q, want no targets", output)
		}
	})
}

func TestImageBuildSkipsWhenNoTargets(t *testing.T) {
	withTempCwd(t, func(root string) {
		_ = root
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewCwd: %v", err)
		}
		router := clir.New()
		registerImageRoutes(router, store)

		output := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"image", "build"}); err != nil {
				t.Fatalf("image build: %v", err)
			}
		})
		if !strings.Contains(output, "no runtime image targets") {
			t.Fatalf("image build output = %q, want no targets", output)
		}
	})
}

func TestImageListShowsRegisteredCodexRuntimeImageTarget(t *testing.T) {
	withTempCwd(t, func(root string) {
		_ = root
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("NewCwd: %v", err)
		}
		router := clir.New()
		registerRuntimeRoutes(router, store, nil)
		registerImageRoutes(router, store)

		if err := router.Run(context.Background(), []string{"component", "register", "codex/work"}); err != nil {
			t.Fatalf("component register: %v", err)
		}
		output := captureStdout(t, func() {
			if err := router.Run(context.Background(), []string{"image", "list"}); err != nil {
				t.Fatalf("image list: %v", err)
			}
		})
		for _, want := range []string{"codex/work", "name=codex", "image=ctgbot-codex:latest", "dockerfile=Dockerfile"} {
			if !strings.Contains(output, want) {
				t.Fatalf("image list output = %q, want %q", output, want)
			}
		}
	})
}

func TestParseImageBuildFlagsRejectsUnexpectedArgs(t *testing.T) {
	if _, err := parseImageBuildFlags("image build", []string{"all"}); err == nil {
		t.Fatalf("expected unexpected argument error")
	}
}
