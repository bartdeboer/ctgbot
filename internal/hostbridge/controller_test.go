package hostbridge

import (
	"context"
	"encoding/gob"
	"io"
	"log"
	"net"
	"reflect"
	"testing"
	"time"
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

func TestHandleConnDispatchesSendFileRequests(t *testing.T) {
	t.Parallel()

	server, client := net.Pipe()
	defer client.Close()

	var got SendFileRequest
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleConn(server, StaticAllowedCommandResolver(nil), func(ctx context.Context, req SendFileRequest) error {
			got = req
			return nil
		}, nil, nil, nil, 30, log.New(io.Discard, "", 0))
	}()

	enc := gob.NewEncoder(client)
	dec := gob.NewDecoder(client)
	if err := enc.Encode(Request{
		Op:        OpSendFile,
		SandboxID: "thread-2",
		Filename:  "report.pdf",
		Caption:   "Weekly report",
		Content:   []byte("hello"),
	}); err != nil {
		t.Fatalf("encode request: %v", err)
	}

	var frame Frame
	if err := dec.Decode(&frame); err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	if frame.Kind != StreamStdout {
		t.Fatalf("frame.Kind = %d, want stdout", frame.Kind)
	}
	if string(frame.Data) != "sent file: report.pdf\n" {
		t.Fatalf("frame.Data = %q", string(frame.Data))
	}

	if err := dec.Decode(&frame); err != nil {
		t.Fatalf("decode exit frame: %v", err)
	}
	if frame.Kind != StreamExit {
		t.Fatalf("frame.Kind = %d, want exit", frame.Kind)
	}
	if frame.ExitCode != 0 {
		t.Fatalf("frame.ExitCode = %d, want 0", frame.ExitCode)
	}
	<-done

	if got.SandboxID != "thread-2" {
		t.Fatalf("unexpected sandbox id: %+v", got)
	}
	if got.Filename != "report.pdf" || got.Caption != "Weekly report" {
		t.Fatalf("unexpected file metadata: %+v", got)
	}
	if string(got.Content) != "hello" {
		t.Fatalf("unexpected content: %q", string(got.Content))
	}
}

func TestHandleConnDispatchesSendTextRequests(t *testing.T) {
	t.Parallel()

	server, client := net.Pipe()
	defer client.Close()

	var got SendTextRequest
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleConn(server, StaticAllowedCommandResolver(nil), nil, func(ctx context.Context, req SendTextRequest) error {
			got = req
			return nil
		}, nil, nil, 30, log.New(io.Discard, "", 0))
	}()

	enc := gob.NewEncoder(client)
	dec := gob.NewDecoder(client)
	if err := enc.Encode(Request{
		Op:        OpSendText,
		SandboxID: "thread-2",
		Text:      "hello world",
	}); err != nil {
		t.Fatalf("encode request: %v", err)
	}

	var frame Frame
	if err := dec.Decode(&frame); err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	if frame.Kind != StreamStdout {
		t.Fatalf("frame.Kind = %d, want stdout", frame.Kind)
	}
	if string(frame.Data) != "sent text\n" {
		t.Fatalf("frame.Data = %q", string(frame.Data))
	}

	if err := dec.Decode(&frame); err != nil {
		t.Fatalf("decode exit frame: %v", err)
	}
	if frame.Kind != StreamExit {
		t.Fatalf("frame.Kind = %d, want exit", frame.Kind)
	}
	if frame.ExitCode != 0 {
		t.Fatalf("frame.ExitCode = %d, want 0", frame.ExitCode)
	}
	<-done

	if got.SandboxID != "thread-2" || got.Text != "hello world" {
		t.Fatalf("unexpected sendtext request: %+v", got)
	}
}

func TestBuildExecutionPlanParsesDelayMillisecondsByDefault(t *testing.T) {
	plan, err := buildExecutionPlan(Request{Command: "git-push-ctgbot"}, AllowedCommand{
		Name:  "git",
		Args:  []string{"push"},
		Dir:   "/workspace/src/ctgbot",
		Delay: "250",
	})
	if err != nil {
		t.Fatalf("build execution plan: %v", err)
	}
	if plan.Delay != 250*time.Millisecond {
		t.Fatalf("plan.Delay = %s, want 250ms", plan.Delay)
	}
}

func TestBuildExecutionPlanRejectsInvalidDelay(t *testing.T) {
	_, err := buildExecutionPlan(Request{Command: "git-push-ctgbot"}, AllowedCommand{
		Name:  "git",
		Delay: "soon",
	})
	if err == nil {
		t.Fatalf("expected invalid delay error")
	}
}
