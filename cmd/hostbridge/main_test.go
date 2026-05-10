package main

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
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
		{name: "direct hostbridge", ref: "codex", in: []string{"sendstdin"}, want: []string{"sendstdin"}},
		{name: "config", ref: "codex", in: []string{"config", "list"}, want: []string{"config", "list"}},
		{name: "component global direct", ref: "codex", in: []string{"component", "help"}, want: []string{"component", "help"}},
		{name: "status global direct", ref: "codex", in: []string{"status"}, want: []string{"status"}},
		{name: "thread global direct", ref: "codex", in: []string{"thread", "list"}, want: []string{"thread", "list"}},
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
