package chatcommands

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type fakeRunner struct {
	requests []Request
	result   Result
}

func (f *fakeRunner) Execute(_ context.Context, req Request) (Result, error) {
	f.requests = append(f.requests, req)
	return f.result, nil
}

type fakeHostCommandRunner struct {
	requests []Request
	commands []RunCommand
	result   Result
}

func (f *fakeHostCommandRunner) ExecuteRunCommand(_ context.Context, req Request, cmd RunCommand) (Result, error) {
	f.requests = append(f.requests, req)
	f.commands = append(f.commands, cmd)
	return f.result, nil
}

type fakeProvider struct {
	sentPayloads        []messenger.OutboundPayload
	stoppedThreadID     modeluuid.UUID
	statusThreadID      modeluuid.UUID
	listedThreadID      modeluuid.UUID
	listedContext       CommandContext
	setThreadID         modeluuid.UUID
	setContext          CommandContext
	setKey              string
	setValue            string
	resolvedFromSandbox modeluuid.UUID
	resolvedThreadID    *modeluuid.UUID
	listResult          string
	setResult           string
	statusResult        string
	stopResult          string
}

func (f *fakeProvider) SendPayload(_ context.Context, sandboxID modeluuid.UUID, payload messenger.OutboundPayload) error {
	payload.ProviderChatID = sandboxID.String()
	f.sentPayloads = append(f.sentPayloads, payload)
	return nil
}

func (f *fakeProvider) Stop(_ context.Context, threadID modeluuid.UUID) (string, error) {
	f.stoppedThreadID = threadID
	if strings.TrimSpace(f.stopResult) != "" {
		return f.stopResult, nil
	}
	return "conversation stopped", nil
}

func (f *fakeProvider) ResolveThreadIDBySandboxID(_ context.Context, sandboxID modeluuid.UUID) (*modeluuid.UUID, error) {
	f.resolvedFromSandbox = sandboxID
	return f.resolvedThreadID, nil
}

func (f *fakeProvider) List(_ context.Context, threadID modeluuid.UUID, cmdctx CommandContext) (string, error) {
	f.listedThreadID = threadID
	f.listedContext = cmdctx
	return f.listResult, nil
}

func (f *fakeProvider) Set(_ context.Context, threadID modeluuid.UUID, cmdctx CommandContext, key, value string) (string, error) {
	f.setThreadID = threadID
	f.setContext = cmdctx
	f.setKey = key
	f.setValue = value
	return f.setResult, nil
}

func (f *fakeProvider) Status(_ context.Context, threadID modeluuid.UUID) (string, error) {
	f.statusThreadID = threadID
	return f.statusResult, nil
}

func withStdin(t *testing.T, data string, fn func()) {
	t.Helper()
	old := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	if _, err := w.WriteString(data); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = old
		_ = r.Close()
	}()
	fn()
}

func TestParseBuildsRunCommandRequest(t *testing.T) {
	runner := &fakeRunner{}
	cmds := New(runner)
	base := Request{SandboxID: modeluuid.New()}

	var req Request
	withStdin(t, "hello", func() {
		var err error
		req, err = cmds.Parse(context.Background(), base, []string{"run", "echo", "a", "b"})
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
	})

	if req.SandboxID != base.SandboxID {
		t.Fatalf("SandboxID changed")
	}
	cmd, ok := req.Command.(RunCommand)
	if !ok {
		t.Fatalf("command type = %T, want RunCommand", req.Command)
	}
	if cmd.Command != "echo" || len(cmd.Args) != 2 || string(cmd.Stdin) != "hello" {
		t.Fatalf("run command = %#v", cmd)
	}
}

func TestRunExecutesParsedRequest(t *testing.T) {
	runner := &fakeRunner{result: Result{Text: "ok"}}
	cmds := New(runner)

	result, err := cmds.RunRequest(context.Background(), Request{Context: CommandContext{IsRoot: true}}, []string{"config", "list"})
	if err != nil {
		t.Fatalf("RunRequest() error = %v", err)
	}
	if result.Text != "ok" {
		t.Fatalf("result.Text = %q, want ok", result.Text)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(runner.requests))
	}
	if _, ok := runner.requests[0].Command.(ConfigList); !ok {
		t.Fatalf("command type = %T, want ConfigList", runner.requests[0].Command)
	}
	if !runner.requests[0].Context.IsRoot {
		t.Fatal("expected IsRoot to be preserved")
	}
}

func TestProviderRunnerConfigListUsesResolvedThreadID(t *testing.T) {
	threadID := modeluuid.New()
	sandboxID := modeluuid.New()
	provider := &fakeProvider{resolvedThreadID: &threadID, listResult: "settings"}
	runner := NewProviderRunner(provider)

	result, err := runner.Execute(context.Background(), Request{
		SandboxID: sandboxID,
		Context:   CommandContext{IsRoot: true},
		Command:   ConfigList{},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Text != "settings" {
		t.Fatalf("result.Text = %q, want settings", result.Text)
	}
	if provider.resolvedFromSandbox != sandboxID {
		t.Fatal("expected sandbox lookup")
	}
	if provider.listedThreadID != threadID {
		t.Fatal("expected resolved thread id")
	}
	if !provider.listedContext.IsRoot {
		t.Fatal("expected IsRoot context")
	}
}

func TestProviderRunnerConfigSetUsesExplicitThreadID(t *testing.T) {
	threadID := modeluuid.New()
	provider := &fakeProvider{setResult: "set chat.enabled = true"}
	runner := NewProviderRunner(provider)

	result, err := runner.Execute(context.Background(), Request{
		ThreadID: threadID,
		Command:  ConfigSet{Setting: "chat.enabled", Value: "true"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Text == "" {
		t.Fatal("expected result text")
	}
	if provider.setThreadID != threadID || provider.setKey != "chat.enabled" || provider.setValue != "true" {
		t.Fatalf("set call = %#v %#v %#v", provider.setThreadID, provider.setKey, provider.setValue)
	}
}

func TestProviderRunnerSendMediaUsesSandboxID(t *testing.T) {
	sandboxID := modeluuid.New()
	provider := &fakeProvider{}
	runner := NewProviderRunner(provider)

	if _, err := runner.Execute(context.Background(), Request{SandboxID: sandboxID, Command: SendMedia{Filename: "a.txt", ContentType: "text/plain", Syntax: "diff", Content: []byte("abc")}}); err != nil {
		t.Fatalf("send file error = %v", err)
	}

	if len(provider.sentPayloads) != 1 {
		t.Fatalf("sentPayloads = %#v", provider.sentPayloads)
	}
	payload := provider.sentPayloads[0]
	if payload.ProviderChatID != sandboxID.String() {
		t.Fatalf("providerChatID = %q, want %q", payload.ProviderChatID, sandboxID)
	}
	if len(payload.Attachments) != 1 {
		t.Fatalf("attachments = %#v", payload.Attachments)
	}
	attachment := payload.Attachments[0]
	if string(attachment.Content) != "abc" || attachment.Syntax != "diff" {
		t.Fatalf("attachment = %#v", attachment)
	}
}

func TestProviderRunnerStatusCommand(t *testing.T) {
	threadID := modeluuid.New()
	provider := &fakeProvider{statusResult: "conversation active"}
	runner := NewProviderRunner(provider)

	result, err := runner.Execute(context.Background(), Request{
		ThreadID: threadID,
		Command:  Status{},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Text != "conversation active" {
		t.Fatalf("result.Text = %q, want conversation active", result.Text)
	}
	if provider.statusThreadID != threadID {
		t.Fatalf("status thread id = %v, want %v", provider.statusThreadID, threadID)
	}
}

func TestProviderRunnerStopCommandUsesProviderStopResult(t *testing.T) {
	threadID := modeluuid.New()
	provider := &fakeProvider{}
	runner := NewProviderRunner(provider)

	result, err := runner.Execute(context.Background(), Request{
		ThreadID: threadID,
		Command:  Stop{},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Text != "conversation stopped" {
		t.Fatalf("result.Text = %q, want conversation stopped", result.Text)
	}
	if provider.stoppedThreadID != threadID {
		t.Fatalf("stopped thread id = %v, want %v", provider.stoppedThreadID, threadID)
	}
}

func TestProviderRunnerStopCommandReportsNoActiveConversation(t *testing.T) {
	threadID := modeluuid.New()
	provider := &fakeProvider{stopResult: "no active conversation"}
	runner := NewProviderRunner(provider)

	result, err := runner.Execute(context.Background(), Request{
		ThreadID: threadID,
		Command:  Stop{},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Text != "no active conversation" {
		t.Fatalf("result.Text = %q, want no active conversation", result.Text)
	}
	if provider.stoppedThreadID != threadID {
		t.Fatalf("stopped thread id = %v, want %v", provider.stoppedThreadID, threadID)
	}
}

func TestDispatchRunnerUsesHostRunnerForRunCommand(t *testing.T) {
	host := &fakeHostCommandRunner{result: Result{Text: "ran"}}
	provider := &fakeRunner{result: Result{Text: "provider"}}
	runner := NewDispatchRunner(host, provider)
	request := Request{SandboxID: modeluuid.New(), Command: RunCommand{Command: "echo"}}

	result, err := runner.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Text != "ran" {
		t.Fatalf("result.Text = %q, want ran", result.Text)
	}
	if len(host.commands) != 1 || host.commands[0].Command != "echo" {
		t.Fatalf("host commands = %#v", host.commands)
	}
	if len(provider.requests) != 0 {
		t.Fatal("provider runner should not be used for run commands")
	}
}

func TestDispatchRunnerUsesProviderForNonRunCommands(t *testing.T) {
	host := &fakeHostCommandRunner{}
	provider := &fakeRunner{result: Result{Text: "provider"}}
	runner := NewDispatchRunner(host, provider)

	result, err := runner.Execute(context.Background(), Request{Command: ConfigList{}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Text != "provider" {
		t.Fatalf("result.Text = %q, want provider", result.Text)
	}
	if len(provider.requests) != 1 {
		t.Fatal("expected provider runner to be used")
	}
}

func TestParseBuildsSendMediaCommands(t *testing.T) {
	runner := &fakeRunner{}
	cmds := New(runner)
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	withStdin(t, "hello", func() {
		req, err := cmds.Parse(context.Background(), Request{}, []string{"sendstdin", "--language", "go"})
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		cmd, ok := req.Command.(SendMedia)
		if !ok || cmd.Filename != "stdin.txt" || string(cmd.Content) != "hello" || cmd.Syntax != "go" || cmd.ContentType != "text/plain" {
			t.Fatalf("send media stdin = %#v", req.Command)
		}
	})

	req, err := cmds.Parse(context.Background(), Request{}, []string{"sendfile", path, "--caption", "hi", "--syntax", "go"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	cmd, ok := req.Command.(SendMedia)
	if !ok || cmd.Filename != "note.txt" || string(cmd.Content) != "abc" || cmd.Syntax != "go" || cmd.ContentType != "text/plain" {
		t.Fatalf("send media file = %#v", req.Command)
	}
}

func TestProviderRunnerRunCommandIsRejected(t *testing.T) {
	runner := NewProviderRunner(&fakeProvider{})
	_, err := runner.Execute(context.Background(), Request{Command: RunCommand{Command: "echo"}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunRequestRequiresRunner(t *testing.T) {
	cmds := New(nil)
	_, err := cmds.Run(context.Background(), []string{"config", "list"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func (f *fakeProvider) RefreshContainer(_ context.Context, _ modeluuid.UUID) (string, error) {
	return "conversation runtime refreshed", nil
}

func (f *fakeProvider) PurgeChat(_ context.Context, _ modeluuid.UUID) (string, error) {
	return "conversation purged", nil
}

func (f *fakeProvider) InterruptTurn(_ context.Context, _ modeluuid.UUID) (string, error) {
	return "interrupt requested", nil
}

func (f *fakeProvider) Upgrade(_ context.Context, _ modeluuid.UUID) (string, error) {
	return "upgrade completed\ntype /quit to restart", nil
}

func (f *fakeProvider) Quit(_ context.Context, _ modeluuid.UUID) (string, error) {
	return "shutting down ctgbot", nil
}

func TestParseBuildsGroupedChatCommands(t *testing.T) {
	cmds := New(nil)

	tests := []struct {
		argv []string
		want any
	}{
		{argv: []string{"container", "refresh"}, want: RefreshContainer{}},
		{argv: []string{"chat", "purge"}, want: PurgeChat{}},
		{argv: []string{"interrupt"}, want: InterruptTurn{}},
		{argv: []string{"upgrade"}, want: Upgrade{}},
		{argv: []string{"quit"}, want: Quit{}},
	}

	for _, tc := range tests {
		req, err := cmds.Parse(context.Background(), Request{}, tc.argv)
		if err != nil {
			t.Fatalf("Parse(%v) error = %v", tc.argv, err)
		}
		if got, want := req.Command, tc.want; reflect.TypeOf(got) != reflect.TypeOf(want) {
			t.Fatalf("Parse(%v) command = %T, want %T", tc.argv, got, want)
		}
	}
}

func TestParseBuildsHelpStatusStopAndNewCommands(t *testing.T) {
	cmds := New(nil)

	tests := []struct {
		argv []string
		want any
	}{
		{argv: []string{"help"}, want: Help{}},
		{argv: []string{"status"}, want: Status{}},
		{argv: []string{"stop"}, want: Stop{}},
		{argv: []string{"new"}, want: DeprecatedNew{}},
	}

	for _, tc := range tests {
		req, err := cmds.ParseUser(context.Background(), Request{}, tc.argv)
		if err != nil {
			t.Fatalf("ParseUser(%v) error = %v", tc.argv, err)
		}
		if got, want := req.Command, tc.want; reflect.TypeOf(got) != reflect.TypeOf(want) {
			t.Fatalf("ParseUser(%v) command = %T, want %T", tc.argv, got, want)
		}
	}
}

func TestUserHelpTextPrefixesSlashAndHidesBridgeOnlyCommands(t *testing.T) {
	cmds := New(nil)
	help := cmds.UserHelpText()
	if !strings.Contains(help, "/container refresh") {
		t.Fatalf("user help missing /container refresh: %q", help)
	}
	if strings.Contains(help, "sendstdin") || strings.Contains(help, "run <command>") {
		t.Fatalf("user help leaked bridge-only commands: %q", help)
	}
}

func TestRunUserRequestHelpReturnsUserHelp(t *testing.T) {
	cmds := New(nil)

	result, err := cmds.RunUserRequest(context.Background(), Request{}, []string{"help"})
	if err != nil {
		t.Fatalf("RunUserRequest() error = %v", err)
	}
	if result.Text != cmds.UserHelpText() {
		t.Fatalf("result.Text = %q, want user help", result.Text)
	}
}

func TestRunUserRequestNewReturnsGuidance(t *testing.T) {
	cmds := New(nil)

	result, err := cmds.RunUserRequest(context.Background(), Request{}, []string{"new"})
	if err != nil {
		t.Fatalf("RunUserRequest() error = %v", err)
	}
	if !strings.Contains(result.Text, "/container refresh") || !strings.Contains(result.Text, "/chat purge") {
		t.Fatalf("result.Text = %q, want /new guidance", result.Text)
	}
}

func TestParseUserRejectsBridgeOnlyCommand(t *testing.T) {
	cmds := New(nil)
	_, err := cmds.ParseUser(context.Background(), Request{}, []string{"sendstdin"})
	if err == nil {
		t.Fatal("expected error")
	}
}
