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
		in   []string
		want []string
	}{
		{name: "status", in: []string{"status"}, want: []string{"codex", "status"}},
		{name: "refresh", in: []string{"refresh"}, want: []string{"codex", "refresh"}},
		{name: "interrupt", in: []string{"interrupt"}, want: []string{"codex", "interrupt"}},
		{name: "model status", in: []string{"model"}, want: []string{"codex", "model"}},
		{name: "model set", in: []string{"model", "set", "gpt-5.5"}, want: []string{"codex", "model", "set", "gpt-5.5"}},
		{name: "run alias", in: []string{"whoami"}, want: []string{"run", "whoami"}},
		{name: "direct hostbridge", in: []string{"sendstdin"}, want: []string{"sendstdin"}},
		{name: "config", in: []string{"config", "list"}, want: []string{"config", "list"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizedArgs(tc.in)
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

func TestHostbridgeRouterUsesV5CodexDefinitions(t *testing.T) {
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
		{argv: normalizedArgs([]string{"status"}), want: "codex status"},
		{argv: normalizedArgs([]string{"refresh"}), want: "codex container refresh"},
		{argv: normalizedArgs([]string{"interrupt"}), want: "codex interrupt"},
		{argv: normalizedArgs([]string{"model"}), want: "codex model"},
	}

	for _, tc := range tests {
		req, err := router.Parse(context.Background(), base, tc.argv)
		if err != nil {
			t.Fatalf("Parse(%v) error = %v", tc.argv, err)
		}
		if req.DefinitionID != tc.want {
			t.Fatalf("Parse(%v) definition = %q, want %q", tc.argv, req.DefinitionID, tc.want)
		}
	}
}
