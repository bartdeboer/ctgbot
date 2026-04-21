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
	sentText            []messenger.OutgoingMessage
	sentFiles           []messenger.OutgoingFile
	startedChatID       modeluuid.UUID
	startedWorkspace    string
	startedReplace      bool
	startedSession      SessionInfo
	stoppedThreadID     modeluuid.UUID
	refreshedThreadID   modeluuid.UUID
	purgedThreadID      modeluuid.UUID
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
}

func (f *fakeProvider) SendText(_ context.Context, msg messenger.OutgoingMessage) error {
	f.sentText = append(f.sentText, msg)
	return nil
}

func (f *fakeProvider) SendFile(_ context.Context, file messenger.OutgoingFile) error {
	f.sentFiles = append(f.sentFiles, file)
	return nil
}

func (f *fakeProvider) StartSession(_ context.Context, chatID modeluuid.UUID, workspace string, replace bool) (SessionInfo, error) {
	f.startedChatID = chatID
	f.startedWorkspace = workspace
	f.startedReplace = replace
	return f.startedSession, nil
}

func (f *fakeProvider) StopActiveSession(_ context.Context, threadID modeluuid.UUID) error {
	f.stoppedThreadID = threadID
	return nil
}

func (f *fakeProvider) RefreshActiveSession(_ context.Context, threadID modeluuid.UUID) error {
	f.refreshedThreadID = threadID
	return nil
}

func (f *fakeProvider) PurgeActiveSession(_ context.Context, threadID modeluuid.UUID) error {
	f.purgedThreadID = threadID
	return nil
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

func TestProviderRunnerSendTextAndFileUseSandboxID(t *testing.T) {
	sandboxID := modeluuid.New()
	provider := &fakeProvider{}
	runner := NewProviderRunner(provider)

	if _, err := runner.Execute(context.Background(), Request{SandboxID: sandboxID, Command: SendText{Text: "hello", ContentType: "text/plain"}}); err != nil {
		t.Fatalf("send text error = %v", err)
	}
	if _, err := runner.Execute(context.Background(), Request{SandboxID: sandboxID, Command: SendFile{Filename: "a.txt", ContentType: "text/plain", Content: []byte("abc")}}); err != nil {
		t.Fatalf("send file error = %v", err)
	}

	if len(provider.sentText) != 1 || provider.sentText[0].SandboxID != sandboxID || provider.sentText[0].Text != "hello" {
		t.Fatalf("sentText = %#v", provider.sentText)
	}
	if len(provider.sentFiles) != 1 || provider.sentFiles[0].SandboxID != sandboxID || string(provider.sentFiles[0].Content) != "abc" {
		t.Fatalf("sentFiles = %#v", provider.sentFiles)
	}
}

func TestProviderRunnerSessionCommands(t *testing.T) {
	chatID := modeluuid.New()
	threadID := modeluuid.New()
	provider := &fakeProvider{startedSession: SessionInfo{ThreadID: threadID, Container: "ctgbot-1", Workspace: "/workspace"}}
	runner := NewProviderRunner(provider)

	started, err := runner.Execute(context.Background(), Request{Command: StartSession{ChatID: chatID, Workspace: "/tmp/work", Replace: true}})
	if err != nil {
		t.Fatalf("start session error = %v", err)
	}
	if started.Session == nil || started.Session.ThreadID != threadID {
		t.Fatalf("session result = %#v", started)
	}
	if provider.startedChatID != chatID || provider.startedWorkspace != "/tmp/work" || !provider.startedReplace {
		t.Fatalf("start call = %#v %#v %#v", provider.startedChatID, provider.startedWorkspace, provider.startedReplace)
	}

	if _, err := runner.Execute(context.Background(), Request{ThreadID: threadID, Command: StopActiveSession{}}); err != nil {
		t.Fatalf("stop session error = %v", err)
	}
	if _, err := runner.Execute(context.Background(), Request{ThreadID: threadID, Command: RefreshActiveSession{}}); err != nil {
		t.Fatalf("refresh session error = %v", err)
	}
	if _, err := runner.Execute(context.Background(), Request{ThreadID: threadID, Command: PurgeActiveSession{}}); err != nil {
		t.Fatalf("purge session error = %v", err)
	}

	if provider.stoppedThreadID != threadID || provider.refreshedThreadID != threadID || provider.purgedThreadID != threadID {
		t.Fatalf("session calls = %#v %#v %#v", provider.stoppedThreadID, provider.refreshedThreadID, provider.purgedThreadID)
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

func TestParseBuildsSendTextAndSendFile(t *testing.T) {
	runner := &fakeRunner{}
	cmds := New(runner)
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	withStdin(t, "hello", func() {
		req, err := cmds.Parse(context.Background(), Request{}, []string{"sendstdin", "--fenced", "--language", "go"})
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		cmd, ok := req.Command.(SendText)
		if !ok || cmd.Text != "hello" || !cmd.Fenced || cmd.Language != "go" {
			t.Fatalf("send text = %#v", req.Command)
		}
	})

	req, err := cmds.Parse(context.Background(), Request{}, []string{"sendfile", path, "--caption", "hi", "--type", "text/plain"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	cmd, ok := req.Command.(SendFile)
	if !ok || cmd.Filename != "note.txt" || string(cmd.Content) != "abc" {
		t.Fatalf("send file = %#v", req.Command)
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

func TestParseUserRejectsBridgeOnlyCommand(t *testing.T) {
	cmds := New(nil)
	_, err := cmds.ParseUser(context.Background(), Request{}, []string{"sendstdin"})
	if err == nil {
		t.Fatal("expected error")
	}
}
