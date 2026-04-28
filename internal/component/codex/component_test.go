package codex

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/agent"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

type fakeAgent struct {
	setupCalled       bool
	purgeThreadID     string
	installedSkillDir string
	model             string
	effort            string
}

func (f *fakeAgent) Name() string { return "codex-test" }

func (f *fakeAgent) SetupEnvironment(ctx context.Context, sbx *sandboxengine.Sandbox) error {
	f.setupCalled = true
	return nil
}

func (f *fakeAgent) HandleTurn(ctx context.Context, sbx *sandboxengine.Sandbox, output agent.OutputHandler, providerThreadID string, prompt string, options agent.TurnOptions) (agent.TurnResult, error) {
	f.model = options.Model
	f.effort = options.ReasoningEffort
	return agent.TurnResult{Reply: prompt, ProviderThreadID: providerThreadID}, nil
}

func (f *fakeAgent) Purge(ctx context.Context, sbx *sandboxengine.Sandbox, providerThreadID string) error {
	f.purgeThreadID = providerThreadID
	return nil
}

func (f *fakeAgent) InstallSkill(ctx context.Context, sbx *sandboxengine.Sandbox, skillDir string) error {
	f.installedSkillDir = skillDir
	return nil
}

func TestComponentCapabilities(t *testing.T) {
	codex := New(&fakeAgent{})
	registry := component.NewRegistry(codex)

	if codex.Type() != ComponentType {
		t.Fatalf("Type() = %q, want %q", codex.Type(), ComponentType)
	}
	if codex.Name() != "codex-test" {
		t.Fatalf("Name() = %q, want codex-test", codex.Name())
	}
	if got := len(component.Capabilities[component.ProfileOwner](registry)); got != 1 {
		t.Fatalf("profile owner capabilities len = %d, want 1", got)
	}
	if got := len(component.Capabilities[agent.Agent](registry)); got != 1 {
		t.Fatalf("agent capabilities len = %d, want 1", got)
	}
	if got := len(component.Capabilities[agent.PurgingAgent](registry)); got != 1 {
		t.Fatalf("purging agent capabilities len = %d, want 1", got)
	}
	if got := len(component.Capabilities[agent.SkillInstallingAgent](registry)); got != 1 {
		t.Fatalf("skill installing agent capabilities len = %d, want 1", got)
	}
}

func TestManagedFiles(t *testing.T) {
	files := New(nil).ManagedFiles()
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

func TestDelegatesAgentMethods(t *testing.T) {
	inner := &fakeAgent{}
	codex := New(inner)

	if err := codex.SetupEnvironment(context.Background(), &sandboxengine.Sandbox{}); err != nil {
		t.Fatalf("SetupEnvironment() error = %v", err)
	}
	if !inner.setupCalled {
		t.Fatal("inner setup was not called")
	}

	result, err := codex.HandleTurn(context.Background(), &sandboxengine.Sandbox{}, nil, "provider-thread", "hello", agent.TurnOptions{Model: "gpt-test", ReasoningEffort: "high"})
	if err != nil {
		t.Fatalf("HandleTurn() error = %v", err)
	}
	if result.Reply != "hello" || result.ProviderThreadID != "provider-thread" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if inner.model != "gpt-test" || inner.effort != "high" {
		t.Fatalf("options not forwarded: model=%q effort=%q", inner.model, inner.effort)
	}

	if err := codex.Purge(context.Background(), &sandboxengine.Sandbox{}, "provider-thread"); err != nil {
		t.Fatalf("Purge() error = %v", err)
	}
	if inner.purgeThreadID != "provider-thread" {
		t.Fatalf("purge thread id = %q, want provider-thread", inner.purgeThreadID)
	}

	if err := codex.InstallSkill(context.Background(), &sandboxengine.Sandbox{}, "/skills/test"); err != nil {
		t.Fatalf("InstallSkill() error = %v", err)
	}
	if inner.installedSkillDir != "/skills/test" {
		t.Fatalf("installed skill dir = %q, want /skills/test", inner.installedSkillDir)
	}
}

func TestMissingAgentErrors(t *testing.T) {
	codex := New(nil)
	if err := codex.SetupEnvironment(context.Background(), &sandboxengine.Sandbox{}); err == nil {
		t.Fatal("SetupEnvironment() with nil agent succeeded, want error")
	}
	if _, err := codex.HandleTurn(context.Background(), &sandboxengine.Sandbox{}, nil, "", "hello", agent.TurnOptions{}); err == nil {
		t.Fatal("HandleTurn() with nil agent succeeded, want error")
	}
	if err := codex.Purge(context.Background(), &sandboxengine.Sandbox{}, "provider-thread"); err == nil {
		t.Fatal("Purge() with nil agent succeeded, want error")
	}
	if err := codex.InstallSkill(context.Background(), &sandboxengine.Sandbox{}, "/skills/test"); err == nil {
		t.Fatal("InstallSkill() with nil agent succeeded, want error")
	}
}
