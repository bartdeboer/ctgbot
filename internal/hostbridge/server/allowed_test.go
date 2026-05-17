package server

import (
	"encoding/json"
	"testing"
)

func TestAllowedCommandJSONAcceptsSnakeCaseExtraArgs(t *testing.T) {
	t.Parallel()

	var allowed map[string]AllowedCommand
	if err := json.Unmarshal([]byte(`{
		"docker": {
			"name": "docker",
			"allow_extra_args": true
		},
		"delete-branch": {
			"name": "git",
			"args_pattern": "<branch>"
		}
	}`), &allowed); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !allowed["docker"].AllowExtraArgs {
		t.Fatalf("allow_extra_args was not decoded: %#v", allowed["docker"])
	}
	if got, want := allowed["delete-branch"].ArgsPattern, "<branch>"; got != want {
		t.Fatalf("args_pattern = %q, want %q", got, want)
	}
}

func TestBuildExecutionPlanSubstitutesArgsPatternParams(t *testing.T) {
	t.Parallel()

	plan, err := BuildExecutionPlan("git-branch-delete", []string{"feature/foo"}, AllowedCommand{
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

	spec := AllowedCommand{
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

	plan, err := BuildExecutionPlan("grep-file", []string{"needle", "--line-number"}, AllowedCommand{
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

	_, err := BuildExecutionPlan("git-branch-delete", []string{"feature/foo"}, AllowedCommand{
		Name: "git",
		Args: []string{"push", "origin", "--delete", "--", "{{branch}}"},
	})
	if err == nil {
		t.Fatalf("BuildExecutionPlan() error = nil")
	}
}

func TestBuildExecutionPlanRejectsUnresolvedTemplate(t *testing.T) {
	t.Parallel()

	_, err := BuildExecutionPlan("git-branch-delete", []string{"feature/foo"}, AllowedCommand{
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

	plan, err := BuildExecutionPlan("git-push", []string{"--follow-tags"}, AllowedCommand{
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
