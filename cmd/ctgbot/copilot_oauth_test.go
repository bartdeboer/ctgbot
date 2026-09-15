package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
	"github.com/bartdeboer/ctgbot/internal/component/copilot"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	dockerruntime "github.com/bartdeboer/ctgbot/internal/runtime/docker"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	systempkg "github.com/bartdeboer/ctgbot/internal/system"
)

// Fake the provider, not the production environment/mount composition. Only
// synthetic files inside t.TempDir are read. No Docker, OAuth, gh or keyring runs.
// This certifies mounted file persistence, NOT native credential resolution or
// exclusion of gh fallback when Copilot's own credential is missing.
type copilotOAuthFixture struct {
	runtimepkg.ThreadRuntime
	t         *testing.T
	thread    modeluuid.UUID
	workspace string
}

func (f copilotOAuthFixture) store(ctx context.Context, thread modeluuid.UUID) string {
	f.t.Helper()
	sbx, err := f.ThreadRuntime.(runtimepkg.ThreadSandboxProvider).ThreadSandbox(ctx, f.workspace, thread)
	if err != nil {
		f.t.Fatal(err)
	}
	env := map[string]string{}
	for _, entry := range sbx.Env {
		k, v, _ := strings.Cut(entry, "=")
		env[k] = v
	}
	runtimeProfile := f.RuntimeComponentProfilePath()
	wantHome := "/home/agent"
	if thread.IsNull() {
		wantHome = runtimeProfile
	}
	if env["HOME"] != wantHome || env["COPILOT_HOME"] != runtimeProfile+"/.copilot" {
		f.t.Fatalf("wrong auth/turn paths: HOME=%q COPILOT_HOME=%q", env["HOME"], env["COPILOT_HOME"])
	}
	for _, mount := range sbx.Mounts {
		if mount.Target == runtimeProfile && mount.Source == f.ComponentProfile().Path {
			// Translate the verified real bind mount for this in-process provider fake.
			return filepath.Join(mount.Source, ".copilot")
		}
	}
	f.t.Fatal("component profile is not persisted by the sandbox spec")
	return ""
}

func (f copilotOAuthFixture) ExecTTY(ctx context.Context, _ string, thread modeluuid.UUID, _ commandengine.CommandExecutor, _, _ io.Writer, name string, args ...string) error {
	f.t.Helper()
	all := append([]string{name}, args...)
	want := []string{"env", "-u", "GH_TOKEN", "-u", "GITHUB_TOKEN", "copilot", "--no-auto-update", "login", "--device-code"}
	if !thread.IsNull() || !slices.Equal(all, want) {
		f.t.Fatalf("not explicit device OAuth: %v", all)
	}
	// Model successful operator-consented plaintext persistence; never fabricate
	// health status or mimic a real secret. Keychain persistence is not modeled.
	return os.WriteFile(filepath.Join(f.store(ctx, thread), "config.json"), []byte(`{"fixture":"operator-consented-oauth"}`), 0600)
}

type copilotOAuthTurnFixture struct{ copilotOAuthFixture }

func (f copilotOAuthTurnFixture) Exec(ctx context.Context, out, _ io.Writer, name string, args ...string) error {
	f.t.Helper()
	all := append([]string{name}, args...)
	prefix := agentcommon.WrapWithPIDFile(nil)
	if len(all) < len(prefix) || !slices.Equal(all[:len(prefix)], prefix) {
		f.t.Fatal("missing PID wrapper")
	}
	id, resume := "", false
	for _, arg := range all {
		if arg == "--no-auto-login" || arg == "--with-token" || strings.HasPrefix(arg, "--auth-token-env") {
			f.t.Fatal("stored OAuth disabled or substituted")
		}
		if strings.HasPrefix(arg, "--session-id=") {
			id = strings.TrimPrefix(arg, "--session-id=")
		}
		if strings.HasPrefix(arg, "--resume=") {
			id, resume = strings.TrimPrefix(arg, "--resume="), true
		}
	}
	if id == "" {
		f.t.Fatal("missing explicit session identity")
	}
	store := f.store(ctx, f.thread)
	credential, err := os.ReadFile(filepath.Join(store, "config.json"))
	if err != nil {
		return fmt.Errorf("fixture OAuth unavailable: %w", err)
	}
	if string(credential) != `{"fixture":"operator-consented-oauth"}` {
		f.t.Fatal("wrong synthetic store")
	}
	session := filepath.Join(store, "session-state", id)
	if resume {
		if _, err := os.Stat(session); err != nil {
			return err
		}
	} else if err := os.MkdirAll(session, 0700); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "{\"type\":\"assistant.message\",\"data\":{\"messageId\":\"fixture\",\"content\":\"synthetic OAuth reused\"}}\n{\"type\":\"result\",\"sessionId\":%q,\"exitCode\":0}\n", id)
	return err
}

func TestCopilotOAuthProfilePersistsAcrossAuthTurnsAndRebind(t *testing.T) {
	root := t.TempDir()
	profile := runtimepkg.Profile{Path: filepath.Join(root, "profile")}
	if err := os.Mkdir(profile.Path, 0700); err != nil {
		t.Fatal(err)
	}
	registry, err := newRuntimeRegistry(&systempkg.System{Logger: log.New(io.Discard, "", 0)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	reg := coremodel.Component{ID: modeluuid.New(), Type: copilot.Type, Name: "oauth-fixture"}
	thread, workspace := modeluuid.New(), t.TempDir()
	build := func() (*copilot.Component, copilotOAuthFixture) {
		// A fresh manager/factory models reconstruction after container refresh;
		// its sandbox specifications must still mount the original component store.
		factory := dockerruntime.New(root, filepath.Join(root, "components"), sandboxengine.NewSandboxManager(nil), nil)
		loaded, err := registry.Build(t.Context(), reg, factory, profile, repository.NewMemory())
		if err != nil {
			t.Fatal(err)
		}
		c := loaded.Component.(*copilot.Component)
		fixture := copilotOAuthFixture{c.Runtime, t, thread, workspace}
		c.Runtime = fixture
		return c, fixture
	}
	c, fixture := build()
	if err := c.Auth(t.Context(), 0, time.Second, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	request := copilot.TurnRequest{PromptPath: "/synthetic/prompt.txt", Workspace: "/workspace"}
	first, err := (copilot.Runner{}).RunTurn(t.Context(), copilotOAuthTurnFixture{fixture}, request)
	if err != nil {
		t.Fatal(err)
	}
	request.ProviderThreadID = first.ProviderThreadID
	_, refreshed := build()
	resumed, err := (copilot.Runner{}).RunTurn(t.Context(), copilotOAuthTurnFixture{refreshed}, request)
	if err != nil || resumed.ProviderThreadID != first.ProviderThreadID || resumed.Reply != "synthetic OAuth reused" {
		t.Fatal(resumed, err)
	}
	// With our own store absent the fake refuses; this is deliberately NOT a
	// certificate that the real pinned CLI also refuses instead of trying gh.
	if err := os.Remove(filepath.Join(profile.Path, ".copilot", "config.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := (copilot.Runner{}).RunTurn(t.Context(), copilotOAuthTurnFixture{refreshed}, request); err == nil {
		t.Fatal("fixture unexpectedly had OAuth")
	}
}
