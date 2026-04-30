package codex

import (
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/v2/component"
)

func TestComponentCapabilities(t *testing.T) {
	codex := New()
	registry := component.NewRegistry(codex)

	if codex.Type() != ComponentType {
		t.Fatalf("Type() = %q, want %q", codex.Type(), ComponentType)
	}
	if got := len(component.Capabilities[component.ProfileOwner](registry)); got != 1 {
		t.Fatalf("profile owner capabilities len = %d, want 1", got)
	}
	if got := len(component.Capabilities[component.Authenticator](registry)); got != 1 {
		t.Fatalf("authenticator capabilities len = %d, want 1", got)
	}
}

func TestManagedFiles(t *testing.T) {
	files := New().ManagedFiles()
	if len(files) != 2 {
		t.Fatalf("managed files len = %d, want 2", len(files))
	}
	if files[0].RelativePath != "auth.json" || !files[0].Required || !files[0].Sensitive {
		t.Fatalf("unexpected auth file: %#v", files[0])
	}
	if files[1].RelativePath != "config.toml" || files[1].Required || files[1].Sensitive {
		t.Fatalf("unexpected config file: %#v", files[1])
	}
}

func TestAuthSandboxSpec(t *testing.T) {
	spec, err := AuthSandboxSpec(component.AuthRequest{
		ComponentType:        "codex",
		ProfileName:          "personal",
		ProfileHostPath:      "/host/profile",
		ProfileContainerPath: "/profile",
		Image:                "ctgbot:test",
		CallbackPort:         1455,
		CallbackTimeout:      time.Minute,
	})
	if err != nil {
		t.Fatalf("AuthSandboxSpec() error = %v", err)
	}
	if spec.Name != "ctgbot-auth-codex-personal" {
		t.Fatalf("Name = %q", spec.Name)
	}
	if spec.Image != "ctgbot:test" {
		t.Fatalf("Image = %q", spec.Image)
	}
	if spec.Workdir != "/profile" {
		t.Fatalf("Workdir = %q", spec.Workdir)
	}
	if len(spec.Env) != 2 || spec.Env[0] != "HOME=/profile" || spec.Env[1] != "CODEX_HOME=/profile" {
		t.Fatalf("Env = %#v", spec.Env)
	}
	if len(spec.Mounts) != 1 || spec.Mounts[0].Source != "/host/profile" || spec.Mounts[0].Target != "/profile" {
		t.Fatalf("Mounts = %#v", spec.Mounts)
	}
	if len(spec.SecurityOpts) != 1 || spec.SecurityOpts[0] != "seccomp=unconfined" {
		t.Fatalf("SecurityOpts = %#v", spec.SecurityOpts)
	}
	if len(spec.Cmd) != 3 || spec.Cmd[0] != "tail" {
		t.Fatalf("Cmd = %#v", spec.Cmd)
	}
}

func TestAuthSandboxSpecRequiresProfileHostPath(t *testing.T) {
	if _, err := AuthSandboxSpec(component.AuthRequest{}); err == nil {
		t.Fatal("AuthSandboxSpec() succeeded without profile host path")
	}
}
