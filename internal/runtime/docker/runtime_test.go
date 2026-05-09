package docker

import (
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	hostbridgebridge "github.com/bartdeboer/ctgbot/internal/hostbridge/bridge"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

func TestSandboxAddsHostbridgeEnvAndMount(t *testing.T) {
	root := t.TempDir()
	bridge := hostbridgebridge.NewBridge(root, nil, nil)
	t.Cleanup(func() {
		_ = bridge.Close()
	})

	factory := New(root, filepath.Join(root, "components"), fakeSandboxManager{}, bridge)
	registration := coremodel.Component{
		Type:    "mockagent",
		Name:    "default",
		Runtime: "docker",
	}
	home := factory.ComponentHome(registration)
	runtime := factory.Bind(registration, home, runtimepkg.BindConfig{}).(*Runtime)

	threadID := modeluuid.New()
	sandbox, cleanup, err := runtime.sandbox(filepath.Join(root, "workspace"), threadID, nil, true)
	if err != nil {
		if strings.Contains(err.Error(), "bind: operation not permitted") || (strings.Contains(err.Error(), "listen tcp") && strings.Contains(err.Error(), "operation not permitted")) {
			t.Skipf("hostbridge listener unavailable in this environment: %v", err)
		}
		t.Fatalf("sandbox() error = %v", err)
	}
	defer cleanup()

	if got, want := findEnv(sandbox.Env, "HOSTBRIDGE_ADDR"), "host.docker.internal:"; len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("HOSTBRIDGE_ADDR = %q, want prefix %q", got, want)
	}
	if got, want := sandbox.UserMode, "host"; got != want {
		t.Fatalf("UserMode = %q, want %q", got, want)
	}
	if got, want := findEnv(sandbox.Env, "HOSTBRIDGE_TLS_DIR"), "/ctgbot/hostbridge-tls"; got != want {
		t.Fatalf("HOSTBRIDGE_TLS_DIR = %q, want %q", got, want)
	}
	if got, want := findEnv(sandbox.Env, "CTGBOT_SANDBOX_ID"), threadID.String(); got != want {
		t.Fatalf("CTGBOT_SANDBOX_ID = %q, want %q", got, want)
	}
	if got, want := findEnv(sandbox.Env, "CTGBOT_COMPONENT_REF"), registration.Ref(); got != want {
		t.Fatalf("CTGBOT_COMPONENT_REF = %q, want %q", got, want)
	}
	if !hasMount(sandbox.Mounts, "/ctgbot/hostbridge-tls", true) {
		t.Fatalf("expected hostbridge TLS mount in %#v", sandbox.Mounts)
	}
	if goruntime.GOOS == "linux" {
		if !hasAddHost(sandbox.AddHosts, "host.docker.internal:host-gateway") {
			t.Fatalf("expected host.docker.internal add-host in %#v", sandbox.AddHosts)
		}
	} else if len(sandbox.AddHosts) != 0 {
		t.Fatalf("unexpected add-hosts on %s: %#v", goruntime.GOOS, sandbox.AddHosts)
	}
}

func TestSandboxPropagatesConfiguredGPUs(t *testing.T) {
	root := t.TempDir()
	factory := New(root, filepath.Join(root, "components"), fakeSandboxManager{}, nil)
	registration := coremodel.Component{
		Type:    "mockagent",
		Name:    "gpu",
		Runtime: "docker",
	}
	home := factory.ComponentHome(registration)
	runtime := factory.Bind(registration, home, runtimepkg.BindConfig{GPUs: "all"}).(*Runtime)

	threadID := modeluuid.New()
	sandbox, cleanup, err := runtime.sandbox(filepath.Join(root, "workspace"), threadID, nil, false)
	if err != nil {
		t.Fatalf("sandbox() error = %v", err)
	}
	defer cleanup()

	if got, want := sandbox.GPUs, "all"; got != want {
		t.Fatalf("Sandbox GPUs = %q, want %q", got, want)
	}
}

func TestSandboxUsesDockerDefaultSeccompByDefault(t *testing.T) {
	root := t.TempDir()
	factory := New(root, filepath.Join(root, "components"), fakeSandboxManager{}, nil)
	registration := coremodel.Component{Type: "mockagent", Name: "default", Runtime: "docker"}
	home := factory.ComponentHome(registration)
	runtime := factory.Bind(registration, home, runtimepkg.BindConfig{}).(*Runtime)

	sandbox, cleanup, err := runtime.sandbox(filepath.Join(root, "workspace"), modeluuid.New(), nil, false)
	if err != nil {
		t.Fatalf("sandbox() error = %v", err)
	}
	defer cleanup()

	if len(sandbox.SecurityOpts) != 0 {
		t.Fatalf("SecurityOpts = %#v, want Docker default", sandbox.SecurityOpts)
	}
}

func TestSandboxPropagatesConfiguredUnconfinedSeccomp(t *testing.T) {
	root := t.TempDir()
	factory := New(root, filepath.Join(root, "components"), fakeSandboxManager{}, nil)
	registration := coremodel.Component{Type: "mockagent", Name: "default", Runtime: "docker"}
	home := factory.ComponentHome(registration)
	runtime := factory.Bind(registration, home, runtimepkg.BindConfig{Seccomp: "unconfined"}).(*Runtime)

	sandbox, cleanup, err := runtime.sandbox(filepath.Join(root, "workspace"), modeluuid.New(), nil, false)
	if err != nil {
		t.Fatalf("sandbox() error = %v", err)
	}
	defer cleanup()

	if got := sandbox.SecurityOpts; len(got) != 1 || got[0] != "seccomp=unconfined" {
		t.Fatalf("SecurityOpts = %#v, want seccomp=unconfined", got)
	}
}

func TestSandboxRejectsUnsupportedSeccompMode(t *testing.T) {
	root := t.TempDir()
	factory := New(root, filepath.Join(root, "components"), fakeSandboxManager{}, nil)
	registration := coremodel.Component{Type: "mockagent", Name: "default", Runtime: "docker"}
	home := factory.ComponentHome(registration)
	runtime := factory.Bind(registration, home, runtimepkg.BindConfig{Seccomp: "strict"}).(*Runtime)

	_, _, err := runtime.sandbox(filepath.Join(root, "workspace"), modeluuid.New(), nil, false)
	if err == nil {
		t.Fatal("sandbox() error = nil, want unsupported seccomp error")
	}
	if !strings.Contains(err.Error(), "unsupported docker seccomp mode") {
		t.Fatalf("sandbox() error = %v, want unsupported seccomp mode", err)
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
