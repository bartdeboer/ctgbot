package server

import (
	"encoding/json"
	"os"
	"testing"

	hostbridgepolicy "github.com/bartdeboer/ctgbot/internal/hostbridgepolicy"
)

func TestAliasJSONAcceptsSnakeCaseExtraArgs(t *testing.T) {
	t.Parallel()

	var aliases map[string]Alias
	if err := json.Unmarshal([]byte(`{
		"docker": {
			"name": "docker",
			"allow_extra_args": true,
			"serialization_key": "docker-daemon",
			"stdin_max_bytes": 1048576
		},
		"delete-branch": {
			"name": "git",
			"args_pattern": "<branch>"
		},
		"git-ctgbot": {
			"name": "git",
			"allowed_cwds": ["/workspace/src/ctgbot"],
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
	if got, want := aliases["docker"].StdinMaxBytes, int64(1048576); got != want {
		t.Fatalf("stdin_max_bytes = %d, want %d", got, want)
	}
	if got, want := aliases["docker"].SerializationKey, "docker-daemon"; got != want {
		t.Fatalf("serialization_key = %q, want %q", got, want)
	}
	if got, want := aliases["delete-branch"].ArgsPattern, "<branch>"; got != want {
		t.Fatalf("args_pattern = %q, want %q", got, want)
	}
	if _, ok := aliases["git-ctgbot"].Subcommands["fetch"]; !ok {
		t.Fatalf("subcommands were not decoded: %#v", aliases["git-ctgbot"])
	}
	if got, want := aliases["git-ctgbot"].AllowedCWDs, []string{"/workspace/src/ctgbot"}; !equalStrings(got, want) {
		t.Fatalf("allowed_cwds = %#v, want %#v", got, want)
	}
}

func TestBuildExecutionPlanConsumesAllowedCWD(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	plan, err := BuildExecutionPlan("git", []string{"--cwd", repo, "status"}, Alias{
		Name:        "git",
		AllowedCWDs: []string{repo},
		Subcommands: map[string]hostbridgepolicy.AliasSubcommand{
			"status": {Args: []string{"status", "--short"}},
		},
	})
	if err != nil {
		t.Fatalf("BuildExecutionPlan() error = %v", err)
	}
	if got, want := plan.Dir, repo; got != want {
		t.Fatalf("Dir = %q, want %q", got, want)
	}
	if got, want := plan.Args, []string{"status", "--short"}; !equalStrings(got, want) {
		t.Fatalf("Args = %#v, want %#v", got, want)
	}
}

func TestBuildExecutionPlanRejectsUnlistedCWD(t *testing.T) {
	t.Parallel()

	allowed := t.TempDir()
	unlisted := t.TempDir()
	_, err := BuildExecutionPlan("git", []string{"--cwd", unlisted, "status"}, Alias{
		Name:        "git",
		AllowedCWDs: []string{allowed},
		Subcommands: map[string]hostbridgepolicy.AliasSubcommand{
			"status": {},
		},
	})
	if err == nil {
		t.Fatal("BuildExecutionPlan() error = nil")
	}
}

func TestBuildExecutionPlanDoesNotDiscloseRequestedPathState(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	allowed := t.TempDir()
	file := root + "/file"
	if err := os.WriteFile(file, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	want := "command git working directory is not allowed"
	for _, requested := range []string{root + "/missing", file, root} {
		_, err := BuildExecutionPlan("git", []string{"--cwd", requested, "status"}, Alias{
			Name:        "git",
			AllowedCWDs: []string{allowed},
			Subcommands: map[string]hostbridgepolicy.AliasSubcommand{"status": {}},
		})
		if err == nil || err.Error() != want {
			t.Fatalf("BuildExecutionPlan(%q) error = %v, want %q", requested, err, want)
		}
	}
}

func TestBuildExecutionPlanSkipsUnavailableAllowedCWD(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repo := t.TempDir()
	plan, err := BuildExecutionPlan("git", []string{"--cwd", repo, "status"}, Alias{
		Name:        "git",
		AllowedCWDs: []string{root + "/not-cloned-yet", repo},
		Subcommands: map[string]hostbridgepolicy.AliasSubcommand{
			"status": {},
		},
	})
	if err != nil {
		t.Fatalf("BuildExecutionPlan() error = %v", err)
	}
	if plan.Dir != repo {
		t.Fatalf("Dir = %q, want %q", plan.Dir, repo)
	}
}

func TestBuildExecutionPlanRejectsMissingOrRelativeCWD(t *testing.T) {
	t.Parallel()

	allowed := t.TempDir()
	spec := Alias{Name: "git", AllowedCWDs: []string{allowed}, AllowExtraArgs: true}
	for _, args := range [][]string{{"status"}, {"--cwd"}, {"--cwd", "relative", "status"}} {
		if _, err := BuildExecutionPlan("git", args, spec); err == nil {
			t.Fatalf("BuildExecutionPlan(%#v) error = nil", args)
		}
	}
}

func TestBuildExecutionPlanAllowsSymlinkResolvingToListedCWD(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repo := t.TempDir()
	link := root + "/repo"
	if err := os.Symlink(repo, link); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	plan, err := BuildExecutionPlan("git", []string{"--cwd", link}, Alias{
		Name:           "git",
		AllowedCWDs:    []string{repo},
		AllowExtraArgs: true,
	})
	if err != nil {
		t.Fatalf("BuildExecutionPlan() error = %v", err)
	}
	if got, want := plan.Dir, repo; got != want {
		t.Fatalf("Dir = %q, want %q", got, want)
	}
}

func TestBuildExecutionPlanRejectsFixedAndDynamicCWD(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	_, err := BuildExecutionPlan("git", []string{"--cwd", repo}, Alias{
		Name:           "git",
		Dir:            repo,
		AllowedCWDs:    []string{repo},
		AllowExtraArgs: true,
	})
	if err == nil {
		t.Fatal("BuildExecutionPlan() error = nil")
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
			Name:        "git",
			AllowedCWDs: []string{"/workspace/src/ctgbot"},
			Subcommands: map[string]hostbridgepolicy.AliasSubcommand{
				"push":   {},
				"fetch":  {},
				"status": {},
			},
		},
	})
	want := []string{"docker", "git-ctgbot --cwd <path> [ fetch | push | status ]"}
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
