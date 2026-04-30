package codex

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

func TestRuntimeSandboxSpec(t *testing.T) {
	threadID := modeluuid.New()
	spec, err := RuntimeSandboxSpec(Config{
		ProfileHostPath:      "/host/profile",
		ProfileContainerPath: "/profile",
		WorkspaceRoot:        "/host/workspaces",
		Image:                "ctgbot:test",
	}, coremodel.ThreadMessage{ThreadID: threadID})
	if err != nil {
		t.Fatalf("RuntimeSandboxSpec() error = %v", err)
	}
	if spec.Name != "ctgbot-v2-codex-"+safeName(threadID.String(), "thread") {
		t.Fatalf("Name = %q", spec.Name)
	}
	if spec.Image != "ctgbot:test" || spec.Workdir != "/workspace" {
		t.Fatalf("unexpected image/workdir: %#v", spec)
	}
	if len(spec.Env) != 2 || spec.Env[0] != "HOME=/profile" || spec.Env[1] != "CODEX_HOME=/profile" {
		t.Fatalf("Env = %#v", spec.Env)
	}
	if len(spec.Mounts) != 2 {
		t.Fatalf("Mounts len = %d, want 2: %#v", len(spec.Mounts), spec.Mounts)
	}
	if spec.Mounts[0].Source != "/host/profile" || spec.Mounts[0].Target != "/profile" {
		t.Fatalf("profile mount = %#v", spec.Mounts[0])
	}
	if spec.Mounts[1].Source != "/host/workspaces/"+threadID.String() || spec.Mounts[1].Target != "/workspace" {
		t.Fatalf("workspace mount = %#v", spec.Mounts[1])
	}
}

func TestRuntimeSandboxSpecRequiresProfile(t *testing.T) {
	if _, err := RuntimeSandboxSpec(Config{}, coremodel.ThreadMessage{}); err == nil {
		t.Fatal("RuntimeSandboxSpec() succeeded without profile")
	}
}
