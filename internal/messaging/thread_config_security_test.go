package messaging

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	threadconfig "github.com/bartdeboer/ctgbot/internal/app/config/thread"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestThreadSecurityConfigRequiresRoot(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	row := &coremodel.Thread{ID: modeluuid.New()}
	if err := storage.Threads().Save(ctx, row); err != nil {
		t.Fatal(err)
	}
	s := New(storage)
	path := filepath.Join(t.TempDir(), "Browser.JSON")
	if err := os.WriteFile(path, []byte(`{"defaultAction":"SCMP_ACT_ERRNO","syscalls":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	root := coremodel.Actor{ID: "operator", Roles: []simplerbac.Role{simplerbac.RoleRoot}}
	agent := coremodel.Actor{ID: "agent", Roles: []simplerbac.Role{simplerbac.RoleAgent}}
	if _, _, err := s.ThreadConfigSet(ctx, root, row.ID, threadconfig.RuntimeSeccompProfile, path); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []coremodel.Actor{agent, {ID: "user"}, {}} {
		for _, key := range []string{threadconfig.RuntimeSeccompProfile, " RUNTIME.SECCOMP-PROFILE "} {
			if _, _, err := s.ThreadConfigSet(ctx, actor, row.ID, key, "/missing"); err == nil {
				t.Fatal("non-root set accepted")
			}
			if _, _, err := s.ThreadConfigUnset(ctx, actor, row.ID, key); err == nil {
				t.Fatal("non-root unset accepted")
			}
		}
	}
	loaded, err := storage.Threads().GetByID(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RuntimeSeccompProfile != path {
		t.Fatal("denied mutation changed stored profile")
	}
	if _, _, err := s.ThreadConfigSet(ctx, agent, row.ID, threadconfig.RuntimePorts, "18451:8080"); err != nil {
		t.Fatal(err)
	}
	loaded, _ = storage.Threads().GetByID(ctx, row.ID)
	if loaded.RuntimePorts != "18451:8080" || loaded.RuntimeSeccompProfile != path {
		t.Fatal("ordinary config regression")
	}
	if _, _, err := s.ThreadConfigSet(ctx, root, row.ID, threadconfig.RuntimeSeccompProfile, "relative"); err == nil {
		t.Fatal("invalid root path accepted")
	}
	loaded, _ = storage.Threads().GetByID(ctx, row.ID)
	if loaded.RuntimeSeccompProfile != path {
		t.Fatal("failed validation mutated state")
	}
	if _, _, err := s.ThreadConfigUnset(ctx, root, row.ID, threadconfig.RuntimeSeccompProfile); err != nil {
		t.Fatal(err)
	}
	loaded, _ = storage.Threads().GetByID(ctx, row.ID)
	if loaded.RuntimeSeccompProfile != "" {
		t.Fatal("unset not persisted")
	}
}
