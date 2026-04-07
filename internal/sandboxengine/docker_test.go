package sandboxengine

import (
	"context"
	"testing"
)

func TestDockerSandboxCommandContextBuildsDockerExec(t *testing.T) {
	t.Parallel()

	sbx := Sandbox{
		Name:    "ctgbot-test",
		Workdir: "/workspace",
		Env:     []string{"HOME=/codex-home", "CODEX_HOME=/codex-home"},
	}
	cmd := sbx.CommandContext(context.Background(), "codex", "exec", "hello")

	got := cmd.Args
	want := []string{
		"docker",
		"exec",
		"-e", "HOME=/codex-home",
		"-e", "CODEX_HOME=/codex-home",
		"-w", "/workspace",
		"ctgbot-test",
		"codex",
		"exec",
		"hello",
	}
	if len(got) != len(want) {
		t.Fatalf("args len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg[%d] = %q, want %q; all args: %#v", i, got[i], want[i], got)
		}
	}
}
