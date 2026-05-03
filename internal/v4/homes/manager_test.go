package homes

import (
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/v4/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v4/profiles"
)

func TestManagerHomeUsesTypeAndNameConvention(t *testing.T) {
	root := t.TempDir()
	manager := New(profiles.New(root, nil))

	home, err := manager.Home(coremodel.Component{
		Type:    "gmail",
		Name:    "work",
		Profile: "personal",
	})
	if err != nil {
		t.Fatalf("Home() error = %v", err)
	}
	if got, want := home.HostPath, filepath.Join(root, ".ctgbot", "profiles", "personal", "components", "gmail", "work"); got != want {
		t.Fatalf("HostPath = %q, want %q", got, want)
	}
	if got, want := home.ContainerPath, "/profile/components/gmail/work"; got != want {
		t.Fatalf("ContainerPath = %q, want %q", got, want)
	}
}
