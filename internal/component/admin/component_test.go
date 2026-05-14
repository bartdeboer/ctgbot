package admin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	componentpkg "github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

type fakeResolver struct {
	loaded map[modeluuid.UUID]*componentpkg.Loaded
}

func (r fakeResolver) ResolveComponent(ctx context.Context, componentID modeluuid.UUID) (*componentpkg.Loaded, error) {
	_ = ctx
	return r.loaded[componentID], nil
}

type fakeProfileComponent struct {
	typeName string
	files    []componentpkg.ManagedFile
	skill    componentpkg.Skill
}

func (c *fakeProfileComponent) Type() string { return c.typeName }
func (c *fakeProfileComponent) ManagedFiles() []componentpkg.ManagedFile {
	return append([]componentpkg.ManagedFile(nil), c.files...)
}
func (c *fakeProfileComponent) Skill() componentpkg.Skill { return c.skill }

type fakeMessageSenderComponent struct {
	fakeProfileComponent
	requests []componentpkg.MessageSendRequest
}

func (c *fakeMessageSenderComponent) SendMessage(ctx context.Context, request componentpkg.MessageSendRequest) (componentpkg.MessageSendResult, error) {
	_ = ctx
	c.requests = append(c.requests, request)
	return componentpkg.MessageSendResult{ID: "sent-1", ThreadID: request.ThreadID}, nil
}

func TestComponentHelpReturnsSkillText(t *testing.T) {
	engine, _ := newTestEngine(t, &fakeProfileComponent{
		typeName: "gmail",
		skill: componentpkg.Skill{
			Name: "gmail",
			Text: "gmail setup docs\nhostbridge component gmail/work auth",
		},
	})

	result, err := engine.Run(context.Background(), testRequest(), []string{"component", "gmail/work", "help"})
	if err != nil {
		t.Fatalf("Run(component help) error = %v", err)
	}
	if !strings.Contains(result.Text, "gmail setup docs") {
		t.Fatalf("help text = %q, want skill text", result.Text)
	}
}

func TestHostbridgeAuthCommandIsNotRegistered(t *testing.T) {
	engine, _ := newTestEngine(t, &fakeProfileComponent{typeName: "gmail"})

	if _, err := engine.Run(context.Background(), testRequest(), []string{"component", "gmail/work", "auth"}); err == nil {
		t.Fatal("Run(component auth) error = nil, want unavailable command")
	}

	result, err := engine.Run(context.Background(), testRequest(), []string{"component", "help"})
	if err != nil {
		t.Fatalf("Run(component help) error = %v", err)
	}
	if strings.Contains(result.Text, "component <component> auth - Authenticate") {
		t.Fatalf("component help still exposes blocking auth command: %q", result.Text)
	}
	if !strings.Contains(result.Text, "component <component> auth status") {
		t.Fatalf("component help = %q, want auth status route", result.Text)
	}
	if !strings.Contains(result.Text, "component <component> message <text>") {
		t.Fatalf("component help = %q, want message route", result.Text)
	}
	if !strings.Contains(result.Text, "component <component> messages send") {
		t.Fatalf("component help = %q, want legacy messages send route", result.Text)
	}
}

func TestManagedFilePutRejectsUnknownFile(t *testing.T) {
	engine, _ := newTestEngine(t, &fakeProfileComponent{
		typeName: "gmail",
		files:    []componentpkg.ManagedFile{{RelativePath: "oauth_client.json", Sensitive: true}},
	})

	err := runWithStdin(t, "secret", func() error {
		_, err := engine.Run(context.Background(), testRequest(), []string{"component", "gmail/work", "managed-file", "put", "unknown.json"})
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("Run(put unknown) error = %v, want not declared", err)
	}
}

func TestManagedFilePutRejectsTraversal(t *testing.T) {
	engine, _ := newTestEngine(t, &fakeProfileComponent{
		typeName: "gmail",
		files:    []componentpkg.ManagedFile{{RelativePath: "oauth_client.json", Sensitive: true}},
	})

	err := runWithStdin(t, "secret", func() error {
		_, err := engine.Run(context.Background(), testRequest(), []string{"component", "gmail/work", "managed-file", "put", "../oauth_client.json"})
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "invalid managed file path") {
		t.Fatalf("Run(put traversal) error = %v, want invalid path", err)
	}
}

func TestManagedFilePutWritesDeclaredFileUnderComponentHome(t *testing.T) {
	engine, home := newTestEngine(t, &fakeProfileComponent{
		typeName: "gmail",
		files:    []componentpkg.ManagedFile{{RelativePath: "secrets/oauth_client.json", Sensitive: true}},
	})

	err := runWithStdin(t, "secret", func() error {
		result, err := engine.Run(context.Background(), testRequest(), []string{"component", "gmail/work", "managed-file", "put", "secrets/oauth_client.json", "--type", "application/json"})
		if err != nil {
			return err
		}
		if got, want := strings.TrimSpace(result.Text), "managed file written: secrets/oauth_client.json"; got != want {
			t.Fatalf("result text = %q, want %q", got, want)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Run(put declared) error = %v", err)
	}

	path := filepath.Join(home, "secrets", "oauth_client.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if string(data) != "secret" {
		t.Fatalf("file content = %q, want secret", string(data))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", path, err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("file mode = %o, want %o", got, want)
	}
}

func TestManagedFileStatusDoesNotExposeSensitiveContents(t *testing.T) {
	engine, home := newTestEngine(t, &fakeProfileComponent{
		typeName: "gmail",
		files:    []componentpkg.ManagedFile{{RelativePath: "token.json", Required: true, Sensitive: true}},
	})
	if err := os.WriteFile(filepath.Join(home, "token.json"), []byte("super-secret-token"), 0o600); err != nil {
		t.Fatalf("WriteFile(token) error = %v", err)
	}

	result, err := engine.Run(context.Background(), testRequest(), []string{"component", "gmail/work", "managed-file", "status"})
	if err != nil {
		t.Fatalf("Run(status) error = %v", err)
	}
	if strings.Contains(result.Text, "super-secret-token") {
		t.Fatalf("status exposed sensitive content: %q", result.Text)
	}
	if !strings.Contains(result.Text, "token.json\tpresent\trequired\tsensitive") {
		t.Fatalf("status = %q, want present sensitive token line", result.Text)
	}
}

func TestMessagesSendCallsComponentSenderWithStdinBody(t *testing.T) {
	sender := &fakeMessageSenderComponent{
		fakeProfileComponent: fakeProfileComponent{typeName: "gmail"},
	}
	engine, _ := newTestEngine(t, sender)

	body := " \nHi there!\n "
	err := runWithStdin(t, body, func() error {
		result, err := engine.Run(context.Background(), testRequest(), []string{
			"component", "gmail/work", "messages", "send",
			"--to", "sender@example.com",
			"--subject", "Re: Test",
			"--thread-id", "thread-1",
			"--in-reply-to", "<message@example.com>",
		})
		if err != nil {
			return err
		}
		if !strings.Contains(result.Text, "message sent") || !strings.Contains(result.Text, "id: sent-1") {
			t.Fatalf("result text = %q, want sent id", result.Text)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Run(messages send) error = %v", err)
	}
	if got, want := len(sender.requests), 1; got != want {
		t.Fatalf("requests = %d, want %d", got, want)
	}
	request := sender.requests[0]
	if got, want := request.To, []string{"sender@example.com"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("To = %#v, want %#v", got, want)
	}
	if got, want := request.Subject, "Re: Test"; got != want {
		t.Fatalf("Subject = %q, want %q", got, want)
	}
	if got, want := request.Body, body; got != want {
		t.Fatalf("Body = %q, want %q", got, want)
	}
	if got, want := request.ThreadID, "thread-1"; got != want {
		t.Fatalf("ThreadID = %q, want %q", got, want)
	}
	if got, want := request.InReplyTo, "<message@example.com>"; got != want {
		t.Fatalf("InReplyTo = %q, want %q", got, want)
	}
}

func TestComponentMessageCallsComponentSenderWithAttachments(t *testing.T) {
	sender := &fakeMessageSenderComponent{
		fakeProfileComponent: fakeProfileComponent{typeName: "gmail"},
	}
	engine, _ := newTestEngine(t, sender)
	dir := t.TempDir()
	path := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(path, []byte("pdf"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result, err := engine.Run(context.Background(), testRequest(), []string{
		"component", "gmail/work", "message", "Monthly report",
		"--to", "sender@example.com",
		"--subject", "Report",
		"--type", "text/html",
		"--attach", path + ";type=application/pdf;name=monthly.pdf",
	})
	if err != nil {
		t.Fatalf("Run(component message) error = %v", err)
	}
	if !strings.Contains(result.Text, "message sent") || !strings.Contains(result.Text, "id: sent-1") {
		t.Fatalf("result text = %q, want sent id", result.Text)
	}
	if got, want := len(sender.requests), 1; got != want {
		t.Fatalf("requests = %d, want %d", got, want)
	}
	request := sender.requests[0]
	if got, want := request.Body, "Monthly report"; got != want {
		t.Fatalf("Body = %q, want %q", got, want)
	}
	if got, want := request.ContentType, "text/html"; got != want {
		t.Fatalf("ContentType = %q, want %q", got, want)
	}
	if got, want := len(request.Attachments), 1; got != want {
		t.Fatalf("Attachments = %d, want %d", got, want)
	}
	if got := request.Attachments[0]; got.Filename != "monthly.pdf" || got.ContentType != "application/pdf" || string(got.Content) != "pdf" {
		t.Fatalf("Attachment = %#v, want parsed attachment", got)
	}
}
func newTestEngine(t *testing.T, fake componentpkg.Component) (*commandengine.Engine, string) {
	t.Helper()
	ctx := context.Background()
	storage := repository.NewMemory()
	registration := &coremodel.Component{Type: "gmail", Name: "work", Runtime: "local", Enabled: true}
	if err := storage.Components().Save(ctx, registration); err != nil {
		t.Fatalf("Save(component) error = %v", err)
	}
	home := t.TempDir()
	resolver := fakeResolver{loaded: map[modeluuid.UUID]*componentpkg.Loaded{
		registration.ID: {
			Registration: *registration,
			Home:         runtimepkg.Home{Path: home},
			Component:    fake,
		},
	}}
	engine, err := commandset.NewEngineForSource(commandengine.SourceHostbridge, New(storage, resolver))
	if err != nil {
		t.Fatalf("NewEngineForSource() error = %v", err)
	}
	return engine, home
}

func testRequest() commandengine.Request {
	return commandengine.Request{Context: commandengine.Context{
		Source: commandengine.SourceHostbridge,
		Actor:  commandengine.Actor{ID: "agent", Roles: []simplerbac.Role{simplerbac.RoleAgent}},
	}}
}

func runWithStdin(t *testing.T, input string, fn func() error) error {
	t.Helper()
	old := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	if _, err := writer.WriteString(input); err != nil {
		t.Fatalf("WriteString(stdin) error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close(stdin writer) error = %v", err)
	}
	os.Stdin = reader
	defer func() {
		os.Stdin = old
		_ = reader.Close()
	}()
	return fn()
}
