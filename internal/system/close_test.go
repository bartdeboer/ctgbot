package system

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/runtime/backend"
)

func TestCloseNativeBackendsClosesFactoryAdmission(t *testing.T) {
	factory := backend.New(t.TempDir(), nil)
	s := &System{Runtimes: map[string]runtimepkg.Factory{"backend": factory}}
	if err := s.CloseNativeBackends(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := factory.BindBackend(coremodel.Component{}, runtimepkg.Profile{}, runtimepkg.BindConfig{}, backend.ServiceSpec{
		Identity: "test", Native: &backend.NativeConfig{Executable: "/not-launched"},
	})
	if err == nil {
		t.Fatal("native admission remains open after system cleanup")
	}
	// Docker binding still works after native cleanup; no Docker calls are made.
	if _, err := factory.BindBackend(coremodel.Component{}, runtimepkg.Profile{}, runtimepkg.BindConfig{}, backend.ServiceSpec{}); err != nil {
		t.Fatal(err)
	}
}
