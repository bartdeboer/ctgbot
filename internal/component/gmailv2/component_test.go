package gmailv2

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	gmailapi "google.golang.org/api/gmail/v1"
)

func TestLoadComponentConfigDefaultsAndExplicitFalse(t *testing.T) {
	home := t.TempDir()
	config, err := loadComponentConfig(home)
	if err != nil {
		t.Fatalf("loadComponentConfig() error = %v", err)
	}
	if !config.materializeRaw() || !config.materializeHTML() || config.MaxPollMessages != DefaultMaxPollMessages {
		t.Fatalf("defaults not applied: %+v", config)
	}
	if err := os.WriteFile(filepath.Join(home, ComponentConfigFilename), []byte(`{"mailbox_email":"me@example.com","materialize_html":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err = loadComponentConfig(home)
	if err != nil {
		t.Fatalf("loadComponentConfig() explicit error = %v", err)
	}
	if config.materializeHTML() {
		t.Fatalf("explicit materialize_html=false was not preserved: %+v", config)
	}
}

func newTestComponent(t *testing.T, name string) *Component {
	t.Helper()
	home := t.TempDir()
	component, err := NewWithOptions(context.Background(), coremodel.Component{ID: modeluuid.New(), Type: Type, Name: name, Enabled: true}, nil, runtimepkg.Home{Path: home}, nil, Options{})
	if err != nil {
		t.Fatalf("NewWithOptions() error = %v", err)
	}
	return component.(*Component)
}

func TestDerivedPathsUseProfileAndSQLiteIDs(t *testing.T) {
	c := newTestComponent(t, "work")
	if got, want := c.runtimeMessagePath("msg-1", ".txt"), "/workspace/gmailv2/work/messages/msg-1.txt"; got != want {
		t.Fatalf("message path = %q, want %q", got, want)
	}
	if got, want := c.runtimeAttachmentPath("att-1"), "/workspace/gmailv2/work/attachments/att-1.bin"; got != want {
		t.Fatalf("attachment path = %q, want %q", got, want)
	}
}

func TestCommandDefinitionsUseTightInboxSurface(t *testing.T) {
	c := newTestComponent(t, "work")
	patterns := map[string]bool{}
	for _, def := range c.CommandDefinitions() {
		patterns[def.Pattern] = true
	}
	for _, pattern := range []string{"query <query>", "fetch <message_id>", "db help", "db schema", "db query <sql>", "message view <message_id>", "message display <message_id>"} {
		if !patterns[pattern] {
			t.Fatalf("missing command pattern %q in %#v", pattern, patterns)
		}
	}
	for _, pattern := range []string{"query <sql>", "sql <sql>", "message path <message_id>", "attachment path <attachment_id>"} {
		if patterns[pattern] {
			t.Fatalf("obsolete command pattern still registered: %q", pattern)
		}
	}
}

func TestDBHelpAndSchemaDescribeQuerySurface(t *testing.T) {
	c := newTestComponent(t, "work")
	help, err := c.handleDBHelp(context.Background(), commandengine.Request{}, dbHelpCommand{})
	if err != nil {
		t.Fatalf("handleDBHelp() error = %v", err)
	}
	if !strings.Contains(help.Text, `db query "select id`) || !strings.Contains(help.Text, "Only SELECT") {
		t.Fatalf("db help did not include examples/safety notes:\n%s", help.Text)
	}
	schema, err := c.handleDBSchema(context.Background(), commandengine.Request{}, dbSchemaCommand{})
	if err != nil {
		t.Fatalf("handleDBSchema() error = %v", err)
	}
	for _, want := range []string{"view messages:", "gmail_message_id", "view attachments:", "sender_policies"} {
		if !strings.Contains(schema.Text, want) {
			t.Fatalf("db schema missing %q:\n%s", want, schema.Text)
		}
	}
}

func TestRenderMessageDisplayIncludesBodyAndAttachmentPaths(t *testing.T) {
	c := newTestComponent(t, "work")
	text := c.renderMessageDisplay(
		storedMessage{ID: "msg-1", FromEmail: "hello@example.com", FromLabel: "Hello", Subject: "Hi"},
		[]storedAttachment{{ID: "att-1", Filename: "report.pdf", ContentType: "application/pdf", Size: 123}},
		"Hello operator",
	)
	for _, want := range []string{
		"From: Hello <hello@example.com>",
		"Subject: Hi",
		"Hello operator",
		"/workspace/gmailv2/work/attachments/att-1.bin",
		"Displayed from stored Gmail message msg-1.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("display text missing %q:\n%s", want, text)
		}
	}
}

func TestMaterializeMessageWritesDerivedWorkspaceFiles(t *testing.T) {
	c := newTestComponent(t, "work")
	workspace := t.TempDir()
	record := storedMessage{ID: "msg-1", HasRaw: true, HasHTML: true, FromEmail: "hello@example.com", FromLabel: "Hello", Subject: "Hi", ToJSON: `[]`, HeadersJSON: `{}`}
	attachments := []parsedAttachment{{Record: storedAttachment{ID: "att-1", Filename: "report.pdf", ContentType: "application/pdf", Disposition: "attachment", Size: 4}, Content: []byte("body")}}
	if err := c.materializeMessage(workspace, record, "Plain body", "<p>Plain body</p>", []byte("raw"), attachments); err != nil {
		t.Fatalf("materializeMessage() error = %v", err)
	}
	for _, path := range []string{
		filepath.Join(workspace, "gmailv2", "work", "messages", "msg-1.txt"),
		filepath.Join(workspace, "gmailv2", "work", "messages", "msg-1.eml"),
		filepath.Join(workspace, "gmailv2", "work", "messages", "msg-1.html"),
		filepath.Join(workspace, "gmailv2", "work", "attachments", "att-1.bin"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected materialized file %s: %v", path, err)
		}
	}
}

func TestWriteFileReplacesAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "message.txt")
	if err := writeFile(path, []byte("first")); err != nil {
		t.Fatalf("writeFile(first) error = %v", err)
	}
	if err := writeFile(path, []byte("second")); err != nil {
		t.Fatalf("writeFile(second) error = %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(body) != "second" {
		t.Fatalf("file body = %q, want second", string(body))
	}
	leftovers, err := filepath.Glob(filepath.Join(dir, ".*.tmp"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("temporary files left behind: %v", leftovers)
	}
}

func TestParseMessageStoresMetadataWithoutPaths(t *testing.T) {
	c := newTestComponent(t, "work")
	parsed := c.parseMessage(&gmailapi.Message{Id: "gmail-1", ThreadId: "thread-1", Snippet: "snippet", Payload: &gmailapi.MessagePart{Headers: []*gmailapi.MessagePartHeader{
		{Name: "From", Value: "Example Sender <hello@example.com>"},
		{Name: "Subject", Value: "Hello"},
		{Name: "Message-ID", Value: "<rfc@example.com>"},
	}, Body: &gmailapi.MessagePartBody{Data: "SGVsbG8="}, MimeType: "text/plain"}})
	if parsed.Record.GmailMessageID != "gmail-1" || parsed.Record.FromEmail != "hello@example.com" || parsed.Record.Subject != "Hello" {
		t.Fatalf("unexpected parsed record: %+v", parsed.Record)
	}
	if strings.Contains(parsed.Record.HeadersJSON, "path") {
		t.Fatalf("headers unexpectedly contain path state: %s", parsed.Record.HeadersJSON)
	}
}

func TestSenderPolicyAffectsInboundPrompt(t *testing.T) {
	c := newTestComponent(t, "work")
	if err := c.store.saveSenderPolicy(context.Background(), "hello@example.com", func(p *senderPolicy) { p.Trusted = false }); err != nil {
		t.Fatalf("saveSenderPolicy() error = %v", err)
	}
	prompt := c.inboundPrompt(storedMessage{ID: "msg-1", GmailMessageID: "gmail-1", GmailThreadID: "thread-1", FromEmail: "hello@example.com", FromLabel: "Hello", Subject: "Hi"}, "")
	if !strings.Contains(prompt, "Sender policy: untrusted") || !strings.Contains(prompt, "pinned as untrusted") {
		t.Fatalf("prompt did not render untrusted policy:\n%s", prompt)
	}
}

func TestInboundPromptShowsBodyWhenShowFullEnabled(t *testing.T) {
	c := newTestComponent(t, "work")
	if err := c.store.saveSenderPolicy(context.Background(), "hello@example.com", func(p *senderPolicy) { p.ShowFull = true }); err != nil {
		t.Fatalf("saveSenderPolicy() error = %v", err)
	}
	prompt := c.inboundPrompt(
		storedMessage{ID: "msg-1", GmailMessageID: "gmail-1", GmailThreadID: "thread-1", FromEmail: "hello@example.com", FromLabel: "Hello", Subject: "Hi"},
		"Hello from email\n```attempted fence break```",
	)
	if !strings.Contains(prompt, "Email body is shown because show_full is enabled") {
		t.Fatalf("prompt did not explain show_full body rendering:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Hello from email") {
		t.Fatalf("prompt did not include body:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Email body:\n```text\nHello from email") {
		t.Fatalf("prompt did not put text fence on its own line:\n%s", prompt)
	}
	if !strings.Contains(prompt, "\n```\n\nThis sender has been pinned as untrusted.") {
		t.Fatalf("prompt did not resume trusted guidance after a blank line:\n%s", prompt)
	}
	if strings.Contains(prompt, "```attempted fence break```") {
		t.Fatalf("prompt did not sanitize body fence markers:\n%s", prompt)
	}
}

func TestInboundPromptHidesBodyWhenShowFullDisabled(t *testing.T) {
	c := newTestComponent(t, "work")
	prompt := c.inboundPrompt(
		storedMessage{ID: "msg-1", GmailMessageID: "gmail-1", GmailThreadID: "thread-1", FromEmail: "hello@example.com", FromLabel: "Hello", Subject: "Hi"},
		"Hidden body",
	)
	if strings.Contains(prompt, "Hidden body") {
		t.Fatalf("prompt included body while show_full is disabled:\n%s", prompt)
	}
	if !strings.Contains(prompt, "was stored as files and was not injected automatically") {
		t.Fatalf("prompt did not explain hidden body:\n%s", prompt)
	}
}

func TestMessageViewOnlyShowsMaterializedKinds(t *testing.T) {
	c := newTestComponent(t, "work")
	message := &storedMessage{
		ID:             "msg-1",
		GmailMessageID: "gmail-1",
		FromEmail:      "hello@example.com",
		Subject:        "Hello",
		HeadersJSON:    `{}`,
		HasText:        true,
		HasHTML:        true,
	}
	if err := c.store.saveMessage(context.Background(), message); err != nil {
		t.Fatalf("saveMessage() error = %v", err)
	}
	result, err := c.handleMessageView(context.Background(), commandengine.Request{}, messageViewCommand{MessageID: "msg-1"})
	if err != nil {
		t.Fatalf("handleMessageView() error = %v", err)
	}
	if strings.Contains(result.Text, ".eml") {
		t.Fatalf("message view advertised non-materialized raw file: %s", result.Text)
	}
	if !strings.Contains(result.Text, ".txt") || !strings.Contains(result.Text, ".html") {
		t.Fatalf("message view missing materialized files: %s", result.Text)
	}
	result, err = c.handleMessageView(context.Background(), commandengine.Request{}, messageViewCommand{MessageID: "gmail-1"})
	if err != nil {
		t.Fatalf("handleMessageView(gmail id) error = %v", err)
	}
	if !strings.Contains(result.Text, "Gmail message msg-1") {
		t.Fatalf("message view did not resolve gmail id to stored message:\n%s", result.Text)
	}
}

func TestQueryRejectsMutatingSQL(t *testing.T) {
	c := newTestComponent(t, "work")
	if _, err := c.store.query(context.Background(), "delete from gmail_messages"); err == nil {
		t.Fatalf("query accepted mutating SQL")
	}
	if _, err := c.store.query(context.Background(), "with doomed as (delete from gmail_messages) select 1"); err == nil {
		t.Fatalf("query accepted mutating CTE SQL")
	}
}

func TestQueryLimitDetectionOnlyUsesOuterLimit(t *testing.T) {
	if !hasLimit("select id from messages limit 10") {
		t.Fatalf("outer LIMIT was not detected")
	}
	if hasLimit("select * from (select id from messages limit 1)") {
		t.Fatalf("nested LIMIT should not satisfy outer query limit")
	}
	if hasLimit("select daily_limit from messages") {
		t.Fatalf("identifier containing limit should not satisfy query limit")
	}
}

func TestQueryUsesStableMessagesView(t *testing.T) {
	c := newTestComponent(t, "work")
	message := &storedMessage{ID: "msg-1", GmailMessageID: "gmail-1", FromEmail: "hello@example.com", Subject: "Hello", HeadersJSON: `{}`}
	if err := c.store.saveMessage(context.Background(), message); err != nil {
		t.Fatalf("saveMessage() error = %v", err)
	}
	text, err := c.store.query(context.Background(), "select id, from_email, subject, headers_json, trusted from messages")
	if err != nil {
		t.Fatalf("query() error = %v", err)
	}
	if !strings.Contains(text, "msg-1") || !strings.Contains(text, "hello@example.com") || !strings.Contains(text, "headers_json") {
		t.Fatalf("query did not read messages view:\n%s", text)
	}
}

func TestRawMessageFetchFailureIsNonFatal(t *testing.T) {
	c := newTestComponent(t, "work")
	raw, err := c.rawMessageBytes(context.Background(), fakeGmailClient{rawErr: errors.New("temporary gmail failure")}, "gmail-1")
	if err != nil {
		t.Fatalf("rawMessageBytes() error = %v", err)
	}
	if raw != nil {
		t.Fatalf("rawMessageBytes() = %q, want nil after non-fatal failure", string(raw))
	}
}

type fakeGmailClient struct {
	rawErr error
}

func (f fakeGmailClient) GetProfile(context.Context, string) (*gmailapi.Profile, error) {
	return nil, nil
}

func (f fakeGmailClient) ListHistory(context.Context, string, uint64, string) (*gmailapi.ListHistoryResponse, error) {
	return nil, nil
}

func (f fakeGmailClient) SearchMessages(context.Context, string, string, int64) ([]*gmailapi.Message, error) {
	return nil, nil
}

func (f fakeGmailClient) GetMessage(context.Context, string, string) (*gmailapi.Message, error) {
	return nil, nil
}

func (f fakeGmailClient) GetRawMessage(context.Context, string, string) ([]byte, error) {
	return nil, f.rawErr
}

func (f fakeGmailClient) GetAttachment(context.Context, string, string, string) ([]byte, error) {
	return nil, nil
}

func (f fakeGmailClient) SendMessage(context.Context, string, *gmailapi.Message) (*gmailapi.Message, error) {
	return nil, nil
}
