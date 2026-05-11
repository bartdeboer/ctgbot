package docker

import (
	"testing"

	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

func TestRuntimeFreshnessNoticesDetectContainerImageMismatch(t *testing.T) {
	t.Parallel()

	notices := runtimeFreshnessNotices(
		dockerContainerInfo{State: sandboxengine.StateRunning, ImageID: "sha256:old"},
		dockerImageInfo{ID: "sha256:new", Labels: map[string]string{runtimeimage.LabelGitCommit: "abc"}},
		"abc",
	)
	if len(notices) != 1 || notices[0] != containerStaleNotice {
		t.Fatalf("notices = %#v, want container stale notice", notices)
	}
}

func TestRuntimeFreshnessNoticesDetectImageGitMismatch(t *testing.T) {
	t.Parallel()

	notices := runtimeFreshnessNotices(
		dockerContainerInfo{State: sandboxengine.StateMissing},
		dockerImageInfo{ID: "sha256:new", Labels: map[string]string{runtimeimage.LabelGitCommit: "old"}},
		"new",
	)
	if len(notices) != 1 || notices[0] != imageStaleNotice {
		t.Fatalf("notices = %#v, want image stale notice", notices)
	}
}

func TestRuntimeFreshnessNoticesDetectUnstampedImage(t *testing.T) {
	t.Parallel()

	notices := runtimeFreshnessNotices(
		dockerContainerInfo{State: sandboxengine.StateMissing},
		dockerImageInfo{ID: "sha256:new"},
		"new",
	)
	if len(notices) != 1 || notices[0] != imageUnstampedNotice {
		t.Fatalf("notices = %#v, want unstamped image notice", notices)
	}
}

func TestRuntimeFreshnessNoticesCleanWhenImageAndCommitMatch(t *testing.T) {
	t.Parallel()

	notices := runtimeFreshnessNotices(
		dockerContainerInfo{State: sandboxengine.StateRunning, ImageID: "sha256:new"},
		dockerImageInfo{ID: "sha256:new", Labels: map[string]string{runtimeimage.LabelGitCommit: "new"}},
		"new",
	)
	if len(notices) != 0 {
		t.Fatalf("notices = %#v, want none", notices)
	}
}
