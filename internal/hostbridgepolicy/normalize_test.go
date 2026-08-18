package hostbridgepolicy

import "testing"

func TestNormalizeAliasOwnsAllPolicyCollections(t *testing.T) {
	t.Parallel()

	spec := NormalizeAlias(Alias{
		Name:        " git ",
		Args:        []string{"status"},
		AllowedCWDs: []string{" /workspace/src/ctgbot "},
		Subcommands: map[string]AliasSubcommand{
			" status ": {Args: []string{"status", "--short"}},
		},
		Env: map[string]string{" TEST ": "value"},
	})
	if spec.Name != "git" {
		t.Fatalf("Name = %q, want git", spec.Name)
	}
	if got, want := spec.AllowedCWDs[0], "/workspace/src/ctgbot"; got != want {
		t.Fatalf("AllowedCWDs[0] = %q, want %q", got, want)
	}
	if _, ok := spec.Subcommands["status"]; !ok {
		t.Fatalf("Subcommands = %#v, want normalized status", spec.Subcommands)
	}
	if got := spec.Env["TEST"]; got != "value" {
		t.Fatalf("Env[TEST] = %q, want value", got)
	}
}

func TestValidateAliasRejectsCWDPolicyContradictions(t *testing.T) {
	t.Parallel()

	for _, spec := range []Alias{
		{Name: "git", Dir: "/workspace", AllowedCWDs: []string{"/workspace/src/ctgbot"}},
		{Name: "git", AllowedCWDs: []string{"relative"}},
		{Name: "git", AllowedCWDs: []string{""}},
	} {
		if err := ValidateAlias(NormalizeAlias(spec)); err == nil {
			t.Fatalf("ValidateAlias(%#v) error = nil", spec)
		}
	}
}
