package main

import (
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func TestImageListShowsDefaultRuntimeImageTarget(t *testing.T) {
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
		for _, want := range []string{"codex", "name=codex", "image=ctgbot-codex:latest", "dockerfile=Dockerfile"} {
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
