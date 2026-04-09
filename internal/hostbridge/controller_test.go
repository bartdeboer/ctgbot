package hostbridge

import (
	"reflect"
	"testing"
)

func TestBuildExecutionPlanUsesFixedCommandShape(t *testing.T) {
	plan, err := buildExecutionPlan(Request{
		Command: "git-pull-ctgbot",
		Args:    []string{"origin"},
		Cwd:     "/tmp/ignored",
		Env:     map[string]string{"IGNORED": "1"},
	}, AllowedCommand{
		Name: "git",
		Args: []string{"pull", "--ff-only"},
		Dir:  "/workspace/src/ctgbot",
	})
	if err == nil {
		t.Fatalf("expected extra args rejection")
	}
	if plan.Name != "" {
		t.Fatalf("unexpected plan on error: %+v", plan)
	}
}

func TestBuildExecutionPlanAllowsConfiguredExtraArgs(t *testing.T) {
	plan, err := buildExecutionPlan(Request{
		Command: "docker",
		Args:    []string{"ps", "-a"},
		Cwd:     "/tmp/ignored",
		Env:     map[string]string{"IGNORED": "1"},
	}, AllowedCommand{
		Name:           "docker",
		Args:           []string{"container"},
		Dir:            "/host/project",
		Env:            map[string]string{"DOCKER_HOST": "unix:///var/run/docker.sock"},
		AllowExtraArgs: true,
	})
	if err != nil {
		t.Fatalf("build execution plan: %v", err)
	}
	if plan.Name != "docker" {
		t.Fatalf("plan.Name = %q, want docker", plan.Name)
	}
	if !reflect.DeepEqual(plan.Args, []string{"container", "ps", "-a"}) {
		t.Fatalf("plan.Args = %#v", plan.Args)
	}
	if plan.Dir != "/host/project" {
		t.Fatalf("plan.Dir = %q, want /host/project", plan.Dir)
	}
	if len(plan.Env) == 0 {
		t.Fatalf("expected inherited env entries")
	}
	foundDockerHost := false
	for _, entry := range plan.Env {
		if entry == "DOCKER_HOST=unix:///var/run/docker.sock" {
			foundDockerHost = true
		}
		if entry == "IGNORED=1" {
			t.Fatalf("request env should not be propagated")
		}
	}
	if !foundDockerHost {
		t.Fatalf("expected DOCKER_HOST override in env")
	}
}

func TestMergeNamedAllowedCommandsNormalizesEntries(t *testing.T) {
	allowed := MergeNamedAllowedCommands(map[string]AllowedCommand{
		"git-push-ctgbot": {
			Name: " git ",
			Dir:  " /workspace/src/ctgbot ",
			Args: []string{"push"},
		},
	})
	spec, ok := allowed["git-push-ctgbot"]
	if !ok {
		t.Fatalf("expected merged command")
	}
	if spec.Name != "git" {
		t.Fatalf("spec.Name = %q, want git", spec.Name)
	}
	if spec.Dir != "/workspace/src/ctgbot" {
		t.Fatalf("spec.Dir = %q, want /workspace/src/ctgbot", spec.Dir)
	}
}
