package system

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

func TestThreadRuntimeConfigCarriesProfile(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	row := &coremodel.Thread{ID: modeluuid.New(), RuntimeSeccompProfile: "/Operator/Browser.JSON", RuntimePorts: "18451:8080"}
	if err := storage.Threads().Save(ctx, row); err != nil {
		t.Fatal(err)
	}
	resolver := threadRuntimeConfigResolver{storage: storage}
	cfg, err := resolver.RuntimeThreadConfig(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SeccompProfile != row.RuntimeSeccompProfile || len(cfg.Ports) != 1 || cfg.Ports[0] != row.RuntimePorts {
		t.Fatalf("bad config: %+v", cfg)
	}
	cfg, err = resolver.RuntimeThreadConfig(ctx, modeluuid.UUID{})
	if err != nil || cfg.SeccompProfile != "" || len(cfg.Ports) != 0 {
		t.Fatalf("auth inherited config: %+v %v", cfg, err)
	}
}
