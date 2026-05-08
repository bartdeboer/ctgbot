package main

import (
	"context"
	"testing"

	"github.com/bartdeboer/go-clir"
)

func TestMaintenanceRoutesExposeQuitAliases(t *testing.T) {
	for _, args := range [][]string{
		{"quit"},
		{"process", "quit"},
	} {
		router := clir.New()
		registerMaintenanceRoutes(router, nil)

		if err := router.Run(context.Background(), args); err != nil {
			t.Fatalf("Run(%q) error = %v", args, err)
		}
	}
}
