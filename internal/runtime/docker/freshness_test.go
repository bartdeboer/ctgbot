package docker

import (
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/buildassets"
	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

func TestRuntimeFreshnessNoticesDetectContainerImageMismatch(t *testing.T) {
	t.Parallel()

	notices := runtimeFreshnessNotices(
		dockerContainerInfo{State: sandboxengine.StateRunning, ImageID: "sha256:old"},
		dockerImageInfo{ID: "sha256:new", Labels: map[string]string{runtimeimage.LabelGitCommit: "abc"}},
		buildassets.FallbackVersion,
		"abc",
		"claude",
	)
	if len(notices) != 1 {
		t.Fatalf("notices = %#v, want one container stale notice", notices)
	}
	if !strings.Contains(notices[0], "/claude container refresh") || strings.Contains(notices[0], "/codex container refresh") {
		t.Fatalf("container stale notice = %q, want component-specific refresh command", notices[0])
	}
}

func TestRuntimeFreshnessNoticesDetectImageGitMismatch(t *testing.T) {
	t.Parallel()

	notices := runtimeFreshnessNotices(
		dockerContainerInfo{State: sandboxengine.StateMissing},
		dockerImageInfo{ID: "sha256:new", Labels: map[string]string{runtimeimage.LabelGitCommit: "old"}},
		buildassets.FallbackVersion,
		"new",
		"codex",
	)
	if len(notices) != 1 || notices[0] != imageVersionNotice {
		t.Fatalf("notices = %#v, want image stale notice", notices)
	}
}

func TestRuntimeFreshnessNoticesDetectUnstampedImage(t *testing.T) {
	t.Parallel()

	notices := runtimeFreshnessNotices(
		dockerContainerInfo{State: sandboxengine.StateMissing},
		dockerImageInfo{ID: "sha256:new"},
		buildassets.FallbackVersion,
		"new",
		"codex",
	)
	if len(notices) != 1 || notices[0] != imageVersionNotice {
		t.Fatalf("notices = %#v, want unstamped image notice", notices)
	}
}

func TestRuntimeFreshnessNoticesCleanWhenImageAndCommitMatch(t *testing.T) {
	t.Parallel()

	notices := runtimeFreshnessNotices(
		dockerContainerInfo{State: sandboxengine.StateRunning, ImageID: "sha256:new"},
		dockerImageInfo{ID: "sha256:new", Labels: map[string]string{runtimeimage.LabelGitCommit: "new"}},
		buildassets.FallbackVersion,
		"new",
		"codex",
	)
	if len(notices) != 0 {
		t.Fatalf("notices = %#v, want none", notices)
	}
}

func TestRuntimeFreshnessNoticesPreferVersionMismatch(t *testing.T) {
	t.Parallel()

	notices := runtimeFreshnessNotices(
		dockerContainerInfo{State: sandboxengine.StateMissing},
		dockerImageInfo{ID: "sha256:new", Labels: map[string]string{
			runtimeimage.LabelVersion:   "v0.1.0-old",
			runtimeimage.LabelGitCommit: "new",
		}},
		"v0.1.0-new",
		"new",
		"codex",
	)
	if len(notices) != 1 || notices[0] != imageVersionNotice {
		t.Fatalf("notices = %#v, want version-driven image stale notice", notices)
	}
}
