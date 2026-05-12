package main

import (
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/buildassets"
	"github.com/bartdeboer/go-clir"
)

func TestVersionRoutePrintsBuildAssetVersion(t *testing.T) {
	router := clir.New()
	registerVersionRoutes(router)

	output := captureStdout(t, func() {
		if err := router.Run(context.Background(), []string{"version"}); err != nil {
			t.Fatalf("version: %v", err)
		}
	})
	if got, want := strings.TrimSpace(output), buildassets.Version(); got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}
