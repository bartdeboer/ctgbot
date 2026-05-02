package homes

import (
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/v3/coremodel"
)

func TestManagerHomeUsesTypeAndNameConvention(t *testing.T) {
	root := t.TempDir()
	manager := New(root)

	home, err := manager.Home(coremodel.Component{
		Type: "gmail",
		Name: "work",
	})
	if err != nil {
		t.Fatalf("Home() error = %v", err)
	}
	if got, want := home.HostPath, filepath.Join(root, ".ctgbot", "components", "gmail", "work"); got != want {
		t.Fatalf("HostPath = %q, want %q", got, want)
	}
	if got, want := home.ContainerPath, "/components/gmail/work"; got != want {
		t.Fatalf("ContainerPath = %q, want %q", got, want)
	}
}
