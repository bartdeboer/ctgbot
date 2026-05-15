package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	admin "github.com/bartdeboer/ctgbot/internal/component/admin"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

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
		{name: "model status", ref: "codex", in: []string{"model"}, want: []string{"codex", "model"}},
		{name: "model set", ref: "codex", in: []string{"model", "set", "gpt-5.5"}, want: []string{"codex", "model", "set", "gpt-5.5"}},
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

func TestHostbridgeRouterUsesCodexDefinitions(t *testing.T) {
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
		{argv: normalizedArgs([]string{"refresh"}, "codex"), want: "codex container refresh"},
		{argv: normalizedArgs([]string{"interrupt"}, "codex"), want: "codex interrupt"},
		{argv: normalizedArgs([]string{"model"}, "codex"), want: "codex model"},
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
	router, err := hostbridgeRouter()
	if err != nil {
		t.Fatalf("hostbridgeRouter() error = %v", err)
	}

	for _, tc := range []struct {
		name string
		argv []string
	}{
		{name: "scoped", argv: []string{"codex", "help"}},
		{name: "all", argv: []string{"help", "all"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := router.FPrintHelp(context.Background(), &buf, tc.argv); err != nil {
				t.Fatalf("FPrintHelp(%v) error = %v", tc.argv, err)
			}

			out := buf.String()
			if !strings.Contains(out, "codex status") {
				t.Fatalf("FPrintHelp(%v) missing visible codex command in %q", tc.argv, out)
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
	router, err := hostbridgeRouter()
	if err != nil {
		t.Fatalf("hostbridgeRouter() error = %v", err)
	}
	base := testHostbridgeRequest()

	tests := []struct {
		name     string
		argv     []string
		contains []string
	}{
		{
			name: "model group",
			argv: []string{"codex", "model", "help"},
			contains: []string{
				"codex model clear",
				"codex model list",
				"codex model set",
			},
		},
		{
			name: "model effort group",
			argv: []string{"codex", "model", "effort", "help"},
			contains: []string{
				"codex model effort clear",
				"codex model effort list",
				"codex model effort set",
			},
		},
		{
			name: "container group",
			argv: []string{"codex", "container", "help"},
			contains: []string{
				"codex container refresh",
				"codex container start",
				"codex container stop",
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
			if strings.Contains(out, "codex model:") || strings.Contains(out, "codex reasoning effort:") {
				t.Fatalf("parseOrRenderHelp(%v) rendered command result instead of help: %q", tc.argv, out)
			}
		})
	}
}

func TestHelpRequestKeepsExactExecutableRoutes(t *testing.T) {
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
			argv:    []string{"codex", "model", "effort", "set", "help"},
			pattern: "codex model effort set <effort>",
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

func TestHelpRequestUsesClirLiteralScopeBehavior(t *testing.T) {
	router, err := hostbridgeRouter()
	if err != nil {
		t.Fatalf("hostbridgeRouter() error = %v", err)
	}

	var buf bytes.Buffer
	_, handled, err := parseOrRenderHelp(
		context.Background(),
		router,
		testHostbridgeRequest(),
		[]string{"thread", "current", "help"},
		&buf,
	)
	if err != nil {
		t.Fatalf("parseOrRenderHelp(thread current help) error = %v", err)
	}
	if !handled {
		t.Fatal("parseOrRenderHelp(thread current help) handled = false, want contextual help")
	}

	out := buf.String()
	if !strings.Contains(out, "thread current status") {
		t.Fatalf("thread current help missing current status in %q", out)
	}
	for _, notWant := range []string{"thread <thread>", "thread <thread> label", "thread <thread> message", "thread <thread> status"} {
		if strings.Contains(out, notWant) {
			t.Fatalf("thread current help unexpectedly contains parameterized sibling %q in %q", notWant, out)
		}
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
	cmd, ok := req.Command.(admin.MessagesSendCommand)
	if !ok {
		t.Fatalf("Command = %T, want admin.MessagesSendCommand", req.Command)
	}
	if cmd.Component != "gmail/work" || cmd.Body != "hello" || len(cmd.To) != 1 || cmd.To[0] != "bart@example.com" || cmd.Subject != "Hi" {
		t.Fatalf("command = %#v", cmd)
	}
}
