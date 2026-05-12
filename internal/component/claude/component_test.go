package claude

import (
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/component"
)

func TestClaudeBootstrapIncludesRuntimeNotices(t *testing.T) {
	text := claudeBootstrap("/workspace", component.TurnInstructions{
		RuntimeNotices: []string{"[Runtime notice] image stale"},
	})
	if !strings.Contains(text, "[Runtime notice] image stale") {
		t.Fatalf("bootstrap text = %q, want runtime notice", text)
	}
}
