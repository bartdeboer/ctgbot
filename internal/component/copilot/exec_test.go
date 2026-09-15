package copilot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
)

type execFunc func(context.Context, io.Writer, io.Writer, string, ...string) error

func (f execFunc) Exec(ctx context.Context, out, errout io.Writer, name string, args ...string) error {
	return f(ctx, out, errout, name, args...)
}

func TestRunnerIdentityAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name, mode   string
		execErr      error
		wantID, fail bool
	}{
		{name: "success", wantID: true},
		{name: "provider-failure", mode: "failed", wantID: true, fail: true},
		{name: "process-failure-after-validated-result", execErr: errors.New("exit 1"), wantID: true, fail: true},
		{name: "mismatch", mode: "mismatch", fail: true},
		{name: "empty", mode: "empty", fail: true},
		{name: "cancelled-without-terminal", mode: "empty", execErr: context.Canceled, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := execFunc(func(_ context.Context, out, errout io.Writer, _ string, args ...string) error {
				if !slices.Contains(args, "--resume="+fixtureID) {
					t.Fatal(args)
				}
				io.WriteString(errout, "secret employer detail")
				switch tc.mode {
				case "empty":
				case "failed":
					io.WriteString(out, terminal(fixtureID, 1))
				case "mismatch":
					io.WriteString(out, fullMessage("ok")+terminal(otherID, 0))
				default:
					io.WriteString(out, fullMessage("ok")+terminal(fixtureID, 0))
				}
				return tc.execErr
			})
			r, err := (Runner{}).RunTurn(t.Context(), rt, TurnRequest{ProviderThreadID: fixtureID, PromptPath: "/profile/prompt", Workspace: "/workspace"})
			if (err != nil) != tc.fail || (r.ProviderThreadID != "") != tc.wantID {
				t.Fatalf("result=%+v err=%v", r, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret employer") {
				t.Fatal("stderr leaked")
			}
		})
	}
}

func TestRunnerJoinsOutputBeforeReturn(t *testing.T) {
	wrote := false
	rt := execFunc(func(ctx context.Context, out, _ io.Writer, _ string, _ ...string) error {
		<-ctx.Done()
		io.WriteString(out, terminal(fixtureID, 1))
		wrote = true
		return ctx.Err()
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	r, err := (Runner{}).RunTurn(ctx, rt, TurnRequest{ProviderThreadID: fixtureID, PromptPath: "/prompt", Workspace: "/workspace"})
	if !wrote || !errors.Is(err, context.Canceled) || r.ProviderThreadID != fixtureID {
		t.Fatalf("joined=%v result=%+v error=%v", wrote, r, err)
	}
}

// This helper is a fake executable, never a real Copilot call. It verifies the
// actual shell stdin-redirection/argv contract and runs only under go test.
func TestCopilotExecutable(t *testing.T) {
	if os.Getenv("CTGBOT_COPILOT_FAKE") != "1" {
		return
	}
	start := slices.Index(os.Args, "--")
	if start < 0 {
		os.Exit(3)
	}
	args := os.Args[start+1:]
	body, err := io.ReadAll(os.Stdin)
	expected, readErr := os.ReadFile(os.Getenv("CTGBOT_EXPECT_PROMPT_FILE"))
	if err != nil || readErr != nil || string(body) != string(expected) {
		os.Exit(4)
	}
	if i := slices.Index(args, "--model"); i < 0 || i+1 >= len(args) || args[i+1] != os.Getenv("CTGBOT_EXPECT_MODEL") {
		os.Exit(5)
	}
	id := ""
	for _, a := range args {
		if strings.HasPrefix(a, "--session-id=") {
			id = strings.TrimPrefix(a, "--session-id=")
		}
	}
	if _, err := canonicalSessionID(id); err != nil {
		os.Exit(6)
	}
	fmt.Print(fullMessage("fake executable reply") + terminal(id, 0))
	os.Exit(0)
}

func TestStagedPromptWithFakeExecutable(t *testing.T) {
	dir := t.TempDir()
	prompt := "Literal 'quote' \"double\" $HOME $(touch NEVER) ;\n✓\n" + strings.Repeat("large input\n", 20000)
	model := "model ; $(touch NEVER) ' \""
	path, cleanup, err := stagePrompt(dir, dir, prompt)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
	// Avoid touching the real container's /tmp/ctgbot-active-command.pid in tests.
	// Verify the shared wrapper's exact prefix, then run only its inner command.
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(dir, "copilot")
	if err := os.WriteFile(shim, []byte("#!/bin/sh\nexec \"$CTGBOT_TEST_EXECUTABLE\" -test.run=TestCopilotExecutable -- \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	rt := execFunc(func(ctx context.Context, out, errout io.Writer, name string, args ...string) error {
		all := append([]string{name}, args...)
		if !slices.Equal(all[:4], agentcommon.WrapWithPIDFile(nil)) {
			t.Fatal(all[:4])
		}
		for _, a := range all {
			if strings.Contains(a, prompt) {
				t.Fatal("prompt leaked to argv")
			}
		}
		inner := all[4:]
		cmd := exec.CommandContext(ctx, inner[0], inner[1:]...)
		cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "CTGBOT_COPILOT_FAKE=1", "CTGBOT_TEST_EXECUTABLE="+executable, "CTGBOT_EXPECT_PROMPT_FILE="+path, "CTGBOT_EXPECT_MODEL="+model)
		cmd.Stdout, cmd.Stderr = out, errout
		return cmd.Run()
	})
	result, err := (Runner{}).RunTurn(t.Context(), rt, TurnRequest{PromptPath: path, Workspace: dir, Model: model})
	if err != nil || result.Reply != "fake executable reply" || result.ProviderThreadID == "" {
		t.Fatalf("%+v %v", result, err)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("staged file retained", err)
	}
}

func TestArgumentsAreNarrowAndExplicit(t *testing.T) {
	req := TurnRequest{PromptPath: "/profile/a b/prompt", Workspace: "/workspace", Model: "-model-with-leading-dash"}
	for _, resume := range []bool{false, true} {
		args, err := buildExecArgs(req, fixtureID, resume)
		if err != nil {
			t.Fatal(err)
		}
		for _, flag := range []string{"--continue", "--allow-all", "--allow-all-tools", "--allow-all-paths", "--allow-all-urls", "--enable-memory", "--autopilot", "--no-sandbox"} {
			if slices.Contains(args, flag) {
				t.Fatal(flag, args)
			}
		}
		for _, flag := range []string{"--no-auto-update", "--disable-builtin-mcps", "--no-remote-export", "--no-ask-user", "--output-format=json", "--stream=off", "--allow-tool=read,write,shell"} {
			if !slices.Contains(args, flag) {
				t.Fatal(flag, args)
			}
		}
	}
	req.Model = "bad\x00model"
	if _, err := buildExecArgs(req, fixtureID, false); err == nil {
		t.Fatal("NUL accepted")
	}
	called := false
	_, err := (Runner{}).RunTurn(t.Context(), execFunc(func(context.Context, io.Writer, io.Writer, string, ...string) error { called = true; return nil }), TurnRequest{ProviderThreadID: "latest"})
	if err == nil || called {
		t.Fatal("invalid ID spawned runtime")
	}
}

func TestTimeoutHasDeadline(t *testing.T) {
	rt := execFunc(func(ctx context.Context, _, _ io.Writer, _ string, _ ...string) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 2*time.Second {
			t.Fatal("deadline", deadline)
		}
		return nil
	})
	(Runner{}).RunTurn(t.Context(), rt, TurnRequest{PromptPath: "/prompt", Workspace: "/workspace", SessionTimeoutSec: 1})
}
