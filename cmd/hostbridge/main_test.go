package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	gmailcomponent "github.com/bartdeboer/ctgbot/internal/component/gmail"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestWriteCommandResultPreservesExecutionStreamsAndExitCode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	result := commandengine.Result{Execution: &commandengine.ExecutionResult{
		Stdout:   "{\"status\":\"degraded\"}\n\x00",
		Stderr:   "health diagnostic\n\x80",
		ExitCode: 2,
	}}
	if got := writeCommandResult(&stdout, &stderr, result); got != 2 {
		t.Fatalf("exit code = %d, want 2", got)
	}
	if got, want := stdout.String(), result.Execution.Stdout; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), result.Execution.Stderr; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestWriteCommandResponseUsesExecutionOutcomeDespiteCompatibilityError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	resp := hostbridge.CommandResponse{Result: commandengine.Result{Execution: &commandengine.ExecutionResult{
		Stdout:   "{\"status\":\"degraded\"}\n\x00",
		Stderr:   "health diagnostic\n\x80",
		ExitCode: 2,
	}}}
	if got := writeCommandResponse(&stdout, &stderr, resp, errors.New("exit status 2: health diagnostic")); got != 2 {
		t.Fatalf("exit code = %d, want 2", got)
	}
	if got, want := stdout.String(), resp.Result.Execution.Stdout; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), resp.Result.Execution.Stderr; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestWriteCommandResponseKeepsOrdinaryErrorsAtExitOne(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if got := writeCommandResponse(&stdout, &stderr, hostbridge.CommandResponse{}, errors.New("admission denied")); got != 1 {
		t.Fatalf("exit code = %d, want 1", got)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if got, want := stderr.String(), "error: admission denied\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestWriteCommandResultKeepsOrdinaryTextBehavior(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if got := writeCommandResult(&stdout, &stderr, commandengine.Result{Text: "ok"}); got != 0 {
		t.Fatalf("exit code = %d, want 0", got)
	}
	if got, want := stdout.String(), "ok\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestReadRunStdinReadsPipedBytesExactly(t *testing.T) {
	want := "{\"text\":\"line one\\n`backticks` $dollar \\\\ tail\"}\n"
	stdin := &testStdin{Reader: strings.NewReader(want)}
	got, err := readRunStdin([]string{"run", "leaseweb", "domain", "apply", "--file", "-"}, stdin, int64(len(want)))
	if err != nil {
		t.Fatalf("readRunStdin() error = %v", err)
	}
	if got != want {
		t.Fatalf("readRunStdin() = %q, want %q", got, want)
	}
}

func TestReadRunStdinLeavesTerminalUntouched(t *testing.T) {
	stdin := &testStdin{Reader: strings.NewReader("must remain unread"), Mode: os.ModeCharDevice}
	got, err := readRunStdin([]string{"run", "leaseweb"}, stdin, 1024)
	if err != nil {
		t.Fatalf("readRunStdin() error = %v", err)
	}
	if got != "" {
		t.Fatalf("readRunStdin() = %q, want empty", got)
	}
	if stdin.Reads != 0 {
		t.Fatalf("terminal stdin was read %d times", stdin.Reads)
	}
}

func TestReadRunStdinTreatsUnavailableDescriptorAsEmpty(t *testing.T) {
	stdin := &testStdin{Reader: strings.NewReader("must remain unread"), StatErr: errors.New("bad file descriptor")}
	got, err := readRunStdin([]string{"run", "leaseweb"}, stdin, 1024)
	if err != nil {
		t.Fatalf("readRunStdin() error = %v", err)
	}
	if got != "" || stdin.Reads != 0 {
		t.Fatalf("unavailable stdin = %q, reads = %d; want untouched", got, stdin.Reads)
	}
}

func TestReadRunStdinRejectsPayloadOverGlobalLimit(t *testing.T) {
	stdin := &testStdin{Reader: strings.NewReader("12345")}
	_, err := readRunStdin([]string{"run", "leaseweb"}, stdin, 4)
	if err == nil || !strings.Contains(err.Error(), "exceeds 4 bytes") {
		t.Fatalf("readRunStdin() error = %v, want global limit", err)
	}
}

func TestReadRunStdinIgnoresOtherCommands(t *testing.T) {
	stdin := &testStdin{Reader: strings.NewReader("must remain unread")}
	got, err := readRunStdin([]string{"send", "hello"}, stdin, 1024)
	if err != nil {
		t.Fatalf("readRunStdin() error = %v", err)
	}
	if got != "" || stdin.Reads != 0 {
		t.Fatalf("non-run stdin = %q, reads = %d; want untouched", got, stdin.Reads)
	}
}

type testStdin struct {
	Reader  io.Reader
	Mode    os.FileMode
	Reads   int
	StatErr error
}

func (s *testStdin) Read(p []byte) (int, error) {
	s.Reads++
	return s.Reader.Read(p)
}

func (s *testStdin) Stat() (os.FileInfo, error) {
	if s.StatErr != nil {
		return nil, s.StatErr
	}
	return testFileInfo{mode: s.Mode}, nil
}

type testFileInfo struct{ mode os.FileMode }

func (i testFileInfo) Name() string       { return "stdin" }
func (i testFileInfo) Size() int64        { return 0 }
func (i testFileInfo) Mode() os.FileMode  { return i.mode }
func (i testFileInfo) ModTime() time.Time { return time.Time{} }
func (i testFileInfo) IsDir() bool        { return false }
func (i testFileInfo) Sys() any           { return nil }

func TestNormalizedArgsLegacyCodexShorthand(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		in   []string
		want []string
	}{
		{name: "status", ref: "codex", in: []string{"status"}, want: []string{"status"}},
		{name: "refresh", ref: "codex", in: []string{"refresh"}, want: []string{"codex", "refresh"}},
		{name: "interrupt", ref: "codex", in: []string{"interrupt"}, want: []string{"codex", "interrupt"}},
		{name: "model is direct model component", ref: "codex", in: []string{"model"}, want: []string{"model"}},
		{name: "codex config is explicit", ref: "codex", in: []string{"codex", "config", "set", "model", "gpt-5.5"}, want: []string{"codex", "config", "set", "model", "gpt-5.5"}},
		{name: "llamacpp status is explicit", ref: "llamacpp/default", in: []string{"llamacpp", "status"}, want: []string{"llamacpp", "status"}},
		{name: "status is global", ref: "llamacpp/default", in: []string{"status"}, want: []string{"status"}},
		{name: "full current ref is direct", ref: "llamacpp/default", in: []string{"llamacpp/default", "status"}, want: []string{"llamacpp/default", "status"}},
		{name: "run alias", ref: "codex", in: []string{"whoami"}, want: []string{"run", "whoami"}},
		{name: "direct hostbridge message", ref: "codex", in: []string{"message", "hello"}, want: []string{"message", "hello"}},
		{name: "direct hostbridge sendstdin", ref: "codex", in: []string{"sendstdin"}, want: []string{"sendstdin"}},
		{name: "config", ref: "codex", in: []string{"config", "list"}, want: []string{"config", "list"}},
		{name: "version", ref: "codex", in: []string{"version"}, want: []string{"version"}},
		{name: "component global direct", ref: "codex", in: []string{"component", "help"}, want: []string{"component", "help"}},
		{name: "status global direct", ref: "codex", in: []string{"status"}, want: []string{"status"}},
		{name: "thread global direct", ref: "codex", in: []string{"thread", "list"}, want: []string{"thread", "list"}},
		{name: "turn global direct", ref: "codex", in: []string{"turn", "config", "list"}, want: []string{"turn", "config", "list"}},
		{name: "sql direct", ref: "codex", in: []string{"sql", "SELECT 1"}, want: []string{"sql", "SELECT 1"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizedArgs(tc.in, tc.ref)
			if len(got) != len(tc.want) {
				t.Fatalf("normalizedArgs(%v) length = %d, want %d (%v)", tc.in, len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("normalizedArgs(%v)[%d] = %q, want %q (%v)", tc.in, i, got[i], tc.want[i], got)
				}
			}
		})
	}
}

func TestExpandStdinArgsThreadMessageSend(t *testing.T) {
	got, err := expandStdinArgs(
		[]string{"thread", "abc", "message", "send"},
		strings.NewReader("hello `world`\nline two\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"thread", "abc", "message", "send", "hello `world`\nline two\n"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExpandStdinArgsThreadMessageSendLiteralStdin(t *testing.T) {
	got, err := expandStdinArgs(
		[]string{"thread", "abc", "message", "send", "stdin"},
		strings.NewReader("hello `world`\nline two\n"),
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"thread", "abc", "message", "send", "hello `world`\nline two\n"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExpandStdinArgsSendLiteralStdin(t *testing.T) {
	got, err := expandStdinArgs([]string{"send", "stdin"}, strings.NewReader("hello `world`\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"send", "hello `world`\n"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExpandStdinArgsSendfileLiteralStdin(t *testing.T) {
	got, err := expandStdinArgs([]string{"sendfile", "stdin", "--caption", "note"}, strings.NewReader("ignored by expand"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"sendstdin", "--caption", "note"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExpandStdinArgsThreadMessageSendEmptyStdinKeepsArgs(t *testing.T) {
	args := []string{"thread", "abc", "message", "send"}
	got, err := expandStdinArgs(args, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(args) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(args), got)
	}
	for i := range args {
		if got[i] != args[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], args[i])
		}
	}
}

func TestExpandStdinArgsSendReadsStdin(t *testing.T) {
	got, err := expandStdinArgs([]string{"send"}, strings.NewReader("hello `world`\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"send", "hello `world`\n"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExpandStdinArgsSendfileWithoutPathUsesSendstdin(t *testing.T) {
	got, err := expandStdinArgs([]string{"sendfile", "--caption", "note"}, strings.NewReader("ignored by expand"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"sendstdin", "--caption", "note"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestHostbridgeRouterUsesCodexDefinitions(t *testing.T) {
	t.Setenv("CTGBOT_ACTIVE_COMPONENTS", "")
	t.Setenv("CTGBOT_COMPONENT_REF", "codex")
	router, err := hostbridgeRouter()
	if err != nil {
		t.Fatalf("hostbridgeRouter() error = %v", err)
	}

	base := commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{
				ID:    "hostbridge",
				Roles: []simplerbac.Role{simplerbac.RoleAgent},
			},
		},
	}

	tests := []struct {
		argv []string
		want string
	}{
		{argv: normalizedArgs([]string{"status"}, "codex"), want: "status"},
		{argv: normalizedArgs([]string{"interrupt"}, "codex"), want: "codex interrupt"},
		{argv: normalizedArgs([]string{"codex", "config", "get", "model"}, "codex"), want: "codex config get <key>"},
		{argv: normalizedArgs([]string{"sql", "SELECT 1"}, "codex"), want: "sql"},
	}

	for _, tc := range tests {
		req, err := router.Parse(context.Background(), base, tc.argv)
		if err != nil {
			t.Fatalf("Parse(%v) error = %v", tc.argv, err)
		}
		if req.CanonicalPattern != tc.want {
			t.Fatalf("Parse(%v) canonical pattern = %q, want %q", tc.argv, req.CanonicalPattern, tc.want)
		}
	}
}

func TestHostbridgeRouterSupportsLlamacppSurface(t *testing.T) {
	t.Setenv("CTGBOT_ACTIVE_COMPONENTS", "")
	t.Setenv("CTGBOT_COMPONENT_REF", "llamacpp/default")
	router, err := hostbridgeRouter()
	if err != nil {
		t.Fatalf("hostbridgeRouter() error = %v", err)
	}

	base := commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{
				ID:    "hostbridge",
				Roles: []simplerbac.Role{simplerbac.RoleAgent},
			},
		},
	}

	req, err := router.Parse(context.Background(), base, []string{"llamacpp", "status"})
	if err != nil {
		t.Fatalf("Parse(llamacpp status) error = %v", err)
	}
	if got, want := req.CanonicalPattern, "llamacpp/default status"; got != want {
		t.Fatalf("Parse(llamacpp status) canonical pattern = %q, want %q", got, want)
	}
}

func TestHostbridgeRouterFallsBackToGlobalsForUnsupportedComponent(t *testing.T) {
	t.Setenv("CTGBOT_ACTIVE_COMPONENTS", "")
	t.Setenv("CTGBOT_COMPONENT_REF", "gmail/work")
	router, err := hostbridgeRouter()
	if err != nil {
		t.Fatalf("hostbridgeRouter() error = %v", err)
	}

	base := commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{
				ID:    "hostbridge",
				Roles: []simplerbac.Role{simplerbac.RoleAgent},
			},
		},
	}

	req, err := router.Parse(context.Background(), base, []string{"run", "whoami"})
	if err != nil {
		t.Fatalf("Parse(run whoami) error = %v", err)
	}
	if got, want := req.CanonicalPattern, "run <command>"; got != want {
		t.Fatalf("Parse(run whoami) canonical pattern = %q, want %q", got, want)
	}
	if _, err := router.Parse(context.Background(), base, []string{"gmail", "sendmessage"}); err == nil {
		t.Fatal("Parse(gmail sendmessage) error = nil, want no matching command")
	}
}

func TestHostbridgeScopedHelpHidesHiddenCodexAliases(t *testing.T) {
	t.Setenv("CTGBOT_ACTIVE_COMPONENTS", "")
	t.Setenv("CTGBOT_COMPONENT_REF", "codex")
	router, err := hostbridgeRouter()
	if err != nil {
		t.Fatalf("hostbridgeRouter() error = %v", err)
	}

	for _, tc := range []struct {
		name string
		argv []string
	}{
		{name: "scoped", argv: []string{"codex"}},
		{name: "root", argv: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := router.FPrintHelp(context.Background(), &buf, tc.argv); err != nil {
				t.Fatalf("FPrintHelp(%v) error = %v", tc.argv, err)
			}

			out := buf.String()
			if !strings.Contains(out, "codex [ chat | compact | config | container | goal | interrupt | status | help ]") {
				t.Fatalf("FPrintHelp(%v) missing compact codex group in %q", tc.argv, out)
			}
			for _, notWant := range []string{"codex purge", "codex refresh"} {
				if strings.Contains(out, notWant) {
					t.Fatalf("FPrintHelp(%v) unexpectedly contains hidden alias %q in %q", tc.argv, notWant, out)
				}
			}
		})
	}
}

func TestHelpRequestRendersContextualHelpBeforePrefixCommandExecution(t *testing.T) {
	t.Setenv("CTGBOT_ACTIVE_COMPONENTS", "")
	t.Setenv("CTGBOT_COMPONENT_REF", "codex")
	router, err := hostbridgeRouter()
	if err != nil {
		t.Fatalf("hostbridgeRouter() error = %v", err)
	}
	base := testHostbridgeRequest()

	tests := []struct {
		name        string
		argv        []string
		contains    []string
		notContains []string
		occursOnce  []string
	}{
		{
			name: "root help is navigation index",
			argv: []string{"help"},
			contains: []string{
				"codex [ chat | compact | config | container | goal | interrupt | status | help ] - agent lifecycle and config",
				"thread [ <thread> | config | label | list | status | help ] - thread messaging, config, and attention controls",
				"status - Show current thread status",
			},
		},
		{
			name: "config group",
			argv: []string{"codex", "config", "help"},
			contains: []string{
				"codex config list",
				"codex config get <key>",
				"codex config set <key> <value>",
				"codex config unset <key>",
			},
		},
		{
			name: "container group",
			argv: []string{"codex", "container", "help"},
			contains: []string{
				"codex container start",
				"codex container stop",
			},
		},
		{
			name: "send group help beats text payload",
			argv: []string{"send", "help"},
			contains: []string{
				"hostbridge send [--type <mime>]",
				"hostbridge send --type text/plain",
			},
		},
		{
			name: "send flag help beats text payload",
			argv: []string{"send", "--help"},
			contains: []string{
				"hostbridge send [--type <mime>]",
				"hostbridge send --type text/plain",
			},
		},
		{
			name: "thread root shows compact command family",
			argv: []string{"thread", "help"},
			contains: []string{
				"thread messaging, config, and attention controls",
				"thread [ <thread> | config | label | list | status | help ] - Thread commands",
			},
			occursOnce: []string{
				"thread [ <thread> | config | label | list | status | help ] - Thread commands",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			req, handled, err := parseOrRenderHelp(context.Background(), router, base, tc.argv, &buf)
			if err != nil {
				t.Fatalf("parseOrRenderHelp(%v) error = %v", tc.argv, err)
			}
			if !handled {
				t.Fatalf("parseOrRenderHelp(%v) handled = false, req = %#v", tc.argv, req)
			}

			out := buf.String()
			for _, want := range tc.contains {
				if !strings.Contains(out, want) {
					t.Fatalf("parseOrRenderHelp(%v) output missing %q in %q", tc.argv, want, out)
				}
			}
			for _, notWant := range tc.notContains {
				if strings.Contains(out, notWant) {
					t.Fatalf("parseOrRenderHelp(%v) output unexpectedly contains %q in %q", tc.argv, notWant, out)
				}
			}
			for _, wantOnce := range tc.occursOnce {
				if got := strings.Count(out, wantOnce); got != 1 {
					t.Fatalf("parseOrRenderHelp(%v) output contains %q %d times, want once in %q", tc.argv, wantOnce, got, out)
				}
			}
			if strings.Contains(out, "codex model:") || strings.Contains(out, "codex reasoning effort:") {
				t.Fatalf("parseOrRenderHelp(%v) rendered command result instead of help: %q", tc.argv, out)
			}
		})
	}
}

func TestHelpRequestKeepsExactExecutableRoutes(t *testing.T) {
	t.Setenv("CTGBOT_ACTIVE_COMPONENTS", "")
	t.Setenv("CTGBOT_COMPONENT_REF", "codex")
	router, err := hostbridgeRouter()
	if err != nil {
		t.Fatalf("hostbridgeRouter() error = %v", err)
	}

	tests := []struct {
		name    string
		argv    []string
		pattern string
	}{
		{
			name:    "component skill help",
			argv:    []string{"component", "gmail/work", "help"},
			pattern: "component <component> help",
		},
		{
			name:    "param value named help",
			argv:    []string{"codex", "config", "set", "model", "help"},
			pattern: "codex config set <key> <value>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			req, handled, err := parseOrRenderHelp(context.Background(), router, testHostbridgeRequest(), tc.argv, &buf)
			if err != nil {
				t.Fatalf("parseOrRenderHelp(%v) error = %v", tc.argv, err)
			}
			if handled {
				t.Fatalf("parseOrRenderHelp(%v) rendered local help, want executable route; output = %q", tc.argv, buf.String())
			}
			if got := req.CanonicalPattern; got != tc.pattern {
				t.Fatalf("canonical pattern = %q, want %q", got, tc.pattern)
			}
			if strings.TrimSpace(buf.String()) != "" {
				t.Fatalf("unexpected local help output for executable route: %q", buf.String())
			}
		})
	}
}

func TestHiddenThreadCurrentStatusAliasStillParses(t *testing.T) {
	router, err := hostbridgeRouter()
	if err != nil {
		t.Fatalf("hostbridgeRouter() error = %v", err)
	}
	req, err := router.Parse(context.Background(), testHostbridgeRequest(), []string{"thread", "current", "status"})
	if err != nil {
		t.Fatalf("Parse(thread current status) error = %v", err)
	}
	if req.CanonicalPattern != "status" {
		t.Fatalf("canonical pattern = %q, want status", req.CanonicalPattern)
	}
}

func testHostbridgeRequest() commandengine.Request {
	return commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{
				ID:    "hostbridge",
				Roles: []simplerbac.Role{simplerbac.RoleAgent},
			},
		},
	}
}

func TestHostbridgeRouterSupportsExplicitKnownComponentRef(t *testing.T) {
	t.Setenv("CTGBOT_ACTIVE_COMPONENTS", "llamacpp/work")
	argv := []string{"llamacpp/work", "status"}
	router, err := hostbridgeRouter(argv)
	if err != nil {
		t.Fatalf("hostbridgeRouter() error = %v", err)
	}
	base := commandengine.Request{Context: commandengine.Context{Actor: commandengine.Actor{ID: "hostbridge", Roles: []simplerbac.Role{simplerbac.RoleAgent}}}}
	req, err := router.Parse(context.Background(), base, argv)
	if err != nil {
		t.Fatalf("Parse(%v) error = %v", argv, err)
	}
	if got, want := req.CanonicalPattern, "llamacpp/work status"; got != want {
		t.Fatalf("CanonicalPattern = %q, want %q", got, want)
	}
}

func TestHostbridgeRouterSupportsExplicitComponentMessageSurface(t *testing.T) {
	t.Setenv("CTGBOT_ACTIVE_COMPONENTS", "gmail/work")
	argv := []string{"gmail/work", "message", "hello", "--to", "bart@example.com", "--subject", "Hi"}
	router, err := hostbridgeRouter(argv)
	if err != nil {
		t.Fatalf("hostbridgeRouter() error = %v", err)
	}
	base := commandengine.Request{Context: commandengine.Context{Actor: commandengine.Actor{ID: "hostbridge", Roles: []simplerbac.Role{simplerbac.RoleAgent}}}}
	req, err := router.Parse(context.Background(), base, argv)
	if err != nil {
		t.Fatalf("Parse(%v) error = %v", argv, err)
	}
	if got, want := req.CanonicalPattern, "gmail/work message <text>"; got != want {
		t.Fatalf("CanonicalPattern = %q, want %q", got, want)
	}
	cmd, ok := req.Command.(gmailcomponent.MessageCommand)
	if !ok {
		t.Fatalf("Command = %T, want gmail.MessageCommand", req.Command)
	}
	if cmd.Body != "hello" || len(cmd.To) != 1 || cmd.To[0] != "bart@example.com" || cmd.Subject != "Hi" {
		t.Fatalf("command = %#v", cmd)
	}
}
