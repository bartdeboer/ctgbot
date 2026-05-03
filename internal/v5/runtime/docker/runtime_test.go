package docker

import (
	"testing"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	v5hostbridgeserver "github.com/bartdeboer/ctgbot/internal/v5/hostbridge/server"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
)

func TestSandboxAddsHostbridgeEnvAndMount(t *testing.T) {
	root := t.TempDir()
	bridge := v5hostbridgeserver.NewBridge(root, nil, nil)
	t.Cleanup(func() {
		_ = bridge.Close()
	})

	factory := New(root, fakeSandboxManager{}, bridge, v5runtime.Profile{
		Name:    "test",
		Runtime: "docker",
		Root:    root,
	})
	registration := coremodel.Component{
		Type:    "mockagent",
		Name:    "default",
		Profile: "test",
	}
	home := factory.ComponentHome(registration)
	runtime := factory.Bind(registration, home, "", nil).(*Runtime)

	threadID := modeluuid.New()
	sandbox, cleanup, err := runtime.sandbox(threadID, nil)
	if err != nil {
		t.Fatalf("sandbox() error = %v", err)
	}
	defer cleanup()

	if got, want := findEnv(sandbox.Env, "HOSTBRIDGE_ADDR"), "host.docker.internal:"; len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("HOSTBRIDGE_ADDR = %q, want prefix %q", got, want)
	}
	if got, want := findEnv(sandbox.Env, "HOSTBRIDGE_TLS_DIR"), "/ctgbot/hostbridge-tls"; got != want {
		t.Fatalf("HOSTBRIDGE_TLS_DIR = %q, want %q", got, want)
	}
	if got, want := findEnv(sandbox.Env, "CTGBOT_SANDBOX_ID"), threadID.String(); got != want {
		t.Fatalf("CTGBOT_SANDBOX_ID = %q, want %q", got, want)
	}
	if !hasMount(sandbox.Mounts, "/ctgbot/hostbridge-tls", true) {
		t.Fatalf("expected hostbridge TLS mount in %#v", sandbox.Mounts)
	}
	if !hasAddHost(sandbox.AddHosts, "host.docker.internal:host-gateway") {
		t.Fatalf("expected host.docker.internal add-host in %#v", sandbox.AddHosts)
	}
}

type fakeSandboxManager struct{}

func (fakeSandboxManager) CreateSandbox(spec *sandboxengine.SandboxSpec) *sandboxengine.Sandbox {
	if spec == nil {
		return nil
	}
	return &sandboxengine.Sandbox{SandboxSpec: *spec}
}

func (m fakeSandboxManager) CreateRuntime(spec sandboxengine.RuntimeSpec) *sandboxengine.SandboxRuntime {
	return &sandboxengine.SandboxRuntime{
		// Not used in this test path.
	}
}

func findEnv(env []string, key string) string {
	prefix := key + "="
	for _, value := range env {
		if len(value) >= len(prefix) && value[:len(prefix)] == prefix {
			return value[len(prefix):]
		}
	}
	return ""
}

func hasMount(mounts []sandboxengine.Mount, target string, readOnly bool) bool {
	for _, mount := range mounts {
		if mount.Target == target && mount.ReadOnly == readOnly {
			return true
		}
	}
	return false
}

func hasAddHost(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
