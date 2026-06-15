package server

import (
	"encoding/json"
	"testing"

	hostbridgepolicy "github.com/bartdeboer/ctgbot/internal/hostbridgepolicy"
)

func TestAliasJSONAcceptsSnakeCaseExtraArgs(t *testing.T) {
	t.Parallel()

	var aliases map[string]Alias
	if err := json.Unmarshal([]byte(`{
		"docker": {
			"name": "docker",
			"allow_extra_args": true
		},
		"delete-branch": {
			"name": "git",
			"args_pattern": "<branch>"
		},
		"git-ctgbot": {
			"name": "git",
			"args": ["-C", "/workspace/src/ctgbot"],
			"subcommands": {
				"fetch": {},
				"push": {"args": ["push", "--follow-tags"]}
			}
		}
	}`), &aliases); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !aliases["docker"].AllowExtraArgs {
		t.Fatalf("allow_extra_args was not decoded: %#v", aliases["docker"])
	}
	if got, want := aliases["delete-branch"].ArgsPattern, "<branch>"; got != want {
		t.Fatalf("args_pattern = %q, want %q", got, want)
	}
	if _, ok := aliases["git-ctgbot"].Subcommands["fetch"]; !ok {
		t.Fatalf("subcommands were not decoded: %#v", aliases["git-ctgbot"])
	}
}

func TestBuildExecutionPlanSubstitutesArgsPatternParams(t *testing.T) {
	t.Parallel()

	plan, err := BuildExecutionPlan("git-branch-delete", []string{"feature/foo"}, Alias{
		Name:        "git",
		ArgsPattern: "<branch>",
		Args:        []string{"push", "origin", "--delete", "--", "{{branch}}"},
		Dir:         "/repo",
	})
	if err != nil {
		t.Fatalf("BuildExecutionPlan() error = %v", err)
	}
	want := []string{"push", "origin", "--delete", "--", "feature/foo"}
	if !equalStrings(plan.Args, want) {
		t.Fatalf("Args = %#v, want %#v", plan.Args, want)
	}
	if got, want := plan.Name, "git"; got != want {
		t.Fatalf("Name = %q, want %q", got, want)
	}
	if got, want := plan.Dir, "/repo"; got != want {
		t.Fatalf("Dir = %q, want %q", got, want)
	}
}

func TestBuildExecutionPlanArgsPatternRejectsMissingAndExtraArgs(t *testing.T) {
	t.Parallel()

	spec := Alias{
		Name:        "git",
		ArgsPattern: "<branch>",
		Args:        []string{"push", "origin", "--delete", "--", "{{branch}}"},
	}
	if _, err := BuildExecutionPlan("git-branch-delete", nil, spec); err == nil {
		t.Fatalf("BuildExecutionPlan(missing arg) error = nil")
	}
	if _, err := BuildExecutionPlan("git-branch-delete", []string{"feature/foo", "extra"}, spec); err == nil {
		t.Fatalf("BuildExecutionPlan(extra arg) error = nil")
	}
}

func TestBuildExecutionPlanArgsPatternAllowsExtraArgsWhenExplicit(t *testing.T) {
	t.Parallel()

	plan, err := BuildExecutionPlan("grep-file", []string{"needle", "--line-number"}, Alias{
		Name:           "grep",
		ArgsPattern:    "<pattern>",
		Args:           []string{"{{pattern}}"},
		AllowExtraArgs: true,
	})
	if err != nil {
		t.Fatalf("BuildExecutionPlan() error = %v", err)
	}
	want := []string{"needle", "--line-number"}
	if !equalStrings(plan.Args, want) {
		t.Fatalf("Args = %#v, want %#v", plan.Args, want)
	}
}

func TestBuildExecutionPlanRejectsTemplateWithoutArgsPattern(t *testing.T) {
	t.Parallel()

	_, err := BuildExecutionPlan("git-branch-delete", []string{"feature/foo"}, Alias{
		Name: "git",
		Args: []string{"push", "origin", "--delete", "--", "{{branch}}"},
	})
	if err == nil {
		t.Fatalf("BuildExecutionPlan() error = nil")
	}
}

func TestBuildExecutionPlanRejectsUnresolvedTemplate(t *testing.T) {
	t.Parallel()

	_, err := BuildExecutionPlan("git-branch-delete", []string{"feature/foo"}, Alias{
		Name:        "git",
		ArgsPattern: "<branch>",
		Args:        []string{"push", "origin", "--delete", "--", "{{missing}}"},
	})
	if err == nil {
		t.Fatalf("BuildExecutionPlan() error = nil")
	}
}

func TestBuildExecutionPlanPreservesExistingExtraArgBehavior(t *testing.T) {
	t.Parallel()

	plan, err := BuildExecutionPlan("git-push", []string{"--follow-tags"}, Alias{
		Name:           "git",
		Args:           []string{"push"},
		AllowExtraArgs: true,
	})
	if err != nil {
		t.Fatalf("BuildExecutionPlan() error = %v", err)
	}
	want := []string{"push", "--follow-tags"}
	if !equalStrings(plan.Args, want) {
		t.Fatalf("Args = %#v, want %#v", plan.Args, want)
	}
}

func TestBuildExecutionPlanUsesNamedSubcommand(t *testing.T) {
	t.Parallel()

	plan, err := BuildExecutionPlan("git-ctgbot", []string{"fetch"}, Alias{
		Name: "git",
		Args: []string{"-C", "/workspace/src/ctgbot"},
		Subcommands: map[string]hostbridgepolicy.AliasSubcommand{
			"fetch": {},
			"pull":  {},
		},
	})
	if err != nil {
		t.Fatalf("BuildExecutionPlan() error = %v", err)
	}
	want := []string{"-C", "/workspace/src/ctgbot", "fetch"}
	if !equalStrings(plan.Args, want) {
		t.Fatalf("Args = %#v, want %#v", plan.Args, want)
	}
}

func TestBuildExecutionPlanUsesCustomSubcommandArgs(t *testing.T) {
	t.Parallel()

	plan, err := BuildExecutionPlan("git-ctgbot", []string{"push"}, Alias{
		Name: "git",
		Args: []string{"-C", "/workspace/src/ctgbot"},
		Subcommands: map[string]hostbridgepolicy.AliasSubcommand{
			"push": {Args: []string{"push", "--follow-tags"}},
		},
	})
	if err != nil {
		t.Fatalf("BuildExecutionPlan() error = %v", err)
	}
	want := []string{"-C", "/workspace/src/ctgbot", "push", "--follow-tags"}
	if !equalStrings(plan.Args, want) {
		t.Fatalf("Args = %#v, want %#v", plan.Args, want)
	}
}

func TestBuildExecutionPlanSubcommandArgsPattern(t *testing.T) {
	t.Parallel()

	plan, err := BuildExecutionPlan("git-ctgbot", []string{"delete-branch", "feature/foo"}, Alias{
		Name: "git",
		Args: []string{"-C", "/workspace/src/ctgbot"},
		Subcommands: map[string]hostbridgepolicy.AliasSubcommand{
			"delete-branch": {
				ArgsPattern: "<branch>",
				Args:        []string{"push", "origin", "--delete", "--", "{{branch}}"},
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildExecutionPlan() error = %v", err)
	}
	want := []string{"-C", "/workspace/src/ctgbot", "push", "origin", "--delete", "--", "feature/foo"}
	if !equalStrings(plan.Args, want) {
		t.Fatalf("Args = %#v, want %#v", plan.Args, want)
	}
}

func TestBuildExecutionPlanSubcommandsRejectUnknownAndExtraArgs(t *testing.T) {
	t.Parallel()

	spec := Alias{
		Name: "git",
		Subcommands: map[string]hostbridgepolicy.AliasSubcommand{
			"fetch": {},
		},
	}
	if _, err := BuildExecutionPlan("git-ctgbot", nil, spec); err == nil {
		t.Fatalf("BuildExecutionPlan(missing subcommand) error = nil")
	}
	if _, err := BuildExecutionPlan("git-ctgbot", []string{"status"}, spec); err == nil {
		t.Fatalf("BuildExecutionPlan(unknown subcommand) error = nil")
	}
	if _, err := BuildExecutionPlan("git-ctgbot", []string{"fetch", "--all"}, spec); err == nil {
		t.Fatalf("BuildExecutionPlan(extra args) error = nil")
	}
}

func TestBuildExecutionPlanSubcommandAllowsExtraArgs(t *testing.T) {
	t.Parallel()

	plan, err := BuildExecutionPlan("git-ctgbot", []string{"status", "--short"}, Alias{
		Name: "git",
		Subcommands: map[string]hostbridgepolicy.AliasSubcommand{
			"status": {AllowExtraArgs: true},
		},
	})
	if err != nil {
		t.Fatalf("BuildExecutionPlan() error = %v", err)
	}
	want := []string{"status", "--short"}
	if !equalStrings(plan.Args, want) {
		t.Fatalf("Args = %#v, want %#v", plan.Args, want)
	}
}

func TestAliasUsagesShowsSubcommands(t *testing.T) {
	t.Parallel()

	usages := AliasUsages(map[string]Alias{
		"docker": {Name: "docker", AllowExtraArgs: true},
		"git-ctgbot": {
			Name: "git",
			Subcommands: map[string]hostbridgepolicy.AliasSubcommand{
				"push":   {},
				"fetch":  {},
				"status": {},
			},
		},
	})
	want := []string{"docker", "git-ctgbot [ fetch | push | status ]"}
	if !equalStrings(usages, want) {
		t.Fatalf("AliasUsages() = %#v, want %#v", usages, want)
	}
}

func equalStrings(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
