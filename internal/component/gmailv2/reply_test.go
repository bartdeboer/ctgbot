package gmailv2

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	gmailapi "google.golang.org/api/gmail/v1"
)

func TestParseRawMessageHeadersDecodesSelectedHeaders(t *testing.T) {
	raw := strings.Join([]string{
		"Message-ID: <orig@example.com>",
		"References: <root@example.com>",
		"In-Reply-To: <parent@example.com>",
		"Subject: =?UTF-8?Q?Hallo_=C3=BCber?=",
		"From: Sender <sender@example.com>",
		"Reply-To: Replies <reply@example.com>",
		"To: Me <me@example.com>",
		"", "body",
	}, "\r\n")

	headers, err := parseRawMessageHeaders([]byte(raw))
	if err != nil {
		t.Fatalf("parseRawMessageHeaders() error = %v", err)
	}
	if got, want := headers.MessageID, "<orig@example.com>"; got != want {
		t.Fatalf("MessageID = %q, want %q", got, want)
	}
	if got, want := headers.InReplyTo, "<parent@example.com>"; got != want {
		t.Fatalf("InReplyTo = %q, want %q", got, want)
	}
	if got, want := headers.Subject, "Hallo über"; got != want {
		t.Fatalf("Subject = %q, want %q", got, want)
	}
}

func TestBuildReplySendRequestRequiresMessageID(t *testing.T) {
	_, err := buildReplySendRequest(replyBuildInput{
		Source: rawMessageHeaders{From: "Sender <sender@example.com>", Subject: "Missing id"},
		Body:   "Thanks",
	})
	if err == nil || !strings.Contains(err.Error(), "missing Message-ID") {
		t.Fatalf("buildReplySendRequest() error = %v, want missing Message-ID", err)
	}
}

func TestBuildReplySendRequestBuildsThreadingHeaders(t *testing.T) {
	req, err := buildReplySendRequest(replyBuildInput{
		Source: rawMessageHeaders{
			MessageID:  " <orig@example.com> ",
			References: "<root@example.com> <parent@example.com>",
			Subject:    "Project update",
			From:       "Sender <sender@example.com>",
		},
		ThreadID: "gmail-thread-1",
		Body:     "Thanks",
	})
	if err != nil {
		t.Fatalf("buildReplySendRequest() error = %v", err)
	}
	if got, want := req.Subject, "Re: Project update"; got != want {
		t.Fatalf("Subject = %q, want %q", got, want)
	}
	if got, want := req.InReplyTo, "<orig@example.com>"; got != want {
		t.Fatalf("InReplyTo = %q, want %q", got, want)
	}
	if got, want := req.References, "<root@example.com> <parent@example.com> <orig@example.com>"; got != want {
		t.Fatalf("References = %q, want %q", got, want)
	}
	if got, want := req.ThreadID, "gmail-thread-1"; got != want {
		t.Fatalf("ThreadID = %q, want %q", got, want)
	}
}

func TestReplyRecipientsSelectsReplyToAndReplyAllExcludesSelf(t *testing.T) {
	req, err := buildReplySendRequest(replyBuildInput{
		Source: rawMessageHeaders{
			MessageID: "<orig@example.com>",
			Subject:   "Re: Existing",
			From:      "Sender <sender@example.com>",
			ReplyTo:   "Replies <reply@example.com>",
			To:        "Me <me@example.com>, Other <other@example.com>",
			Cc:        "CC <cc@example.com>, Me Again <me@example.com>",
		},
		Mode:         "reply-all",
		Body:         "body",
		SelfAccounts: []string{"me@example.com"},
	})
	if err != nil {
		t.Fatalf("buildReplySendRequest() error = %v", err)
	}
	if got, want := strings.Join(req.To, ";"), "Replies <reply@example.com>;Other <other@example.com>"; got != want {
		t.Fatalf("To = %q, want %q", got, want)
	}
	if got, want := strings.Join(req.Cc, ";"), "CC <cc@example.com>"; got != want {
		t.Fatalf("Cc = %q, want %q", got, want)
	}
	if got, want := req.Subject, "Re: Existing"; got != want {
		t.Fatalf("Subject = %q, want %q", got, want)
	}
}

func TestAppendReferenceMessageIDTrimsFromLeft(t *testing.T) {
	var ids []string
	for i := 0; i < 80; i++ {
		ids = append(ids, fmt.Sprintf("<%02d-long-message-id@example.com>", i))
	}
	refs := appendReferenceMessageID(strings.Join(ids, " "), "<latest@example.com>")
	if !strings.Contains(refs, "<latest@example.com>") {
		t.Fatalf("References = %q, want latest id preserved", refs)
	}
	if len(refs) > maxReferencesHeaderBytes {
		t.Fatalf("References length = %d, want <= %d", len(refs), maxReferencesHeaderBytes)
	}
	if strings.Contains(refs, ids[0]) {
		t.Fatalf("References = %q, want oldest ids trimmed", refs)
	}
}

type replyFakeClient struct {
	message *gmailapi.Message
	raw     []byte
	sent    *gmailapi.Message
}

func (f *replyFakeClient) GetProfile(ctx context.Context, userID string) (*gmailapi.Profile, error) {
	return &gmailapi.Profile{EmailAddress: "me@example.com"}, nil
}

func (f *replyFakeClient) ListHistory(ctx context.Context, userID string, startHistoryID uint64, pageToken string) (*gmailapi.ListHistoryResponse, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f *replyFakeClient) SearchMessages(ctx context.Context, userID string, query string, limit int64) ([]*gmailapi.Message, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f *replyFakeClient) GetMessage(ctx context.Context, userID string, messageID string) (*gmailapi.Message, error) {
	return f.message, nil
}

func (f *replyFakeClient) GetRawMessage(ctx context.Context, userID string, messageID string) ([]byte, error) {
	return append([]byte(nil), f.raw...), nil
}

func (f *replyFakeClient) GetAttachment(ctx context.Context, userID string, messageID string, attachmentID string) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f *replyFakeClient) SendMessage(ctx context.Context, userID string, message *gmailapi.Message) (*gmailapi.Message, error) {
	f.sent = message
	return &gmailapi.Message{Id: "sent-1", ThreadId: "thread-1"}, nil
}

func TestHandleReplyFetchesOriginalAndSendsThreadedReply(t *testing.T) {
	raw := strings.Join([]string{
		"Message-ID: <orig@example.com>",
		"References: <root@example.com>",
		"In-Reply-To: <parent@example.com>",
		"Subject: Original subject",
		"From: Sender <sender@example.com>",
		"To: Me <me@example.com>",
		"", "body",
	}, "\r\n")
	fake := &replyFakeClient{
		message: &gmailapi.Message{Id: "gmail-1", ThreadId: "thread-1"},
		raw:     []byte(raw),
	}
	component := &Component{clientOverride: fake, mailboxEmail: "me@example.com"}

	result, err := component.handleReply(context.Background(), commandengine.Request{}, ReplyCommand{
		GmailMessageID: "gmail-1",
		Body:           "Thanks",
	})
	if err != nil {
		t.Fatalf("handleReply() error = %v", err)
	}
	if got := result.Text; !strings.Contains(got, "reply sent") || !strings.Contains(got, "id: sent-1") || !strings.Contains(got, "thread_id: thread-1") {
		t.Fatalf("result text = %q", got)
	}
	if fake.sent == nil {
		t.Fatalf("SendMessage was not called")
	}
	if got, want := fake.sent.ThreadId, "thread-1"; got != want {
		t.Fatalf("sent ThreadId = %q, want %q", got, want)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(fake.sent.Raw)
	if err != nil {
		t.Fatalf("decode sent raw: %v", err)
	}
	text := string(decoded)
	for _, want := range []string{
		"To: Sender <sender@example.com>\r\n",
		"Subject: Re: Original subject\r\n",
		"In-Reply-To: <orig@example.com>\r\n",
		"References: <root@example.com> <orig@example.com>\r\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("sent raw = %q, want contains %q", text, want)
		}
	}
}

func TestHandleReplyRejectsMissingOriginalThread(t *testing.T) {
	fake := &replyFakeClient{message: &gmailapi.Message{Id: "gmail-1"}}
	component := &Component{clientOverride: fake}
	_, err := component.handleReply(context.Background(), commandengine.Request{}, ReplyCommand{GmailMessageID: "gmail-1", Body: "Thanks"})
	if err == nil || !strings.Contains(err.Error(), "no Gmail thread id") {
		t.Fatalf("handleReply() error = %v, want missing thread id", err)
	}
}

func TestHandleReplyRejectsNilOriginalMessage(t *testing.T) {
	fake := &replyFakeClient{}
	component := &Component{clientOverride: fake}
	_, err := component.handleReply(context.Background(), commandengine.Request{}, ReplyCommand{GmailMessageID: "gmail-1", Body: "Thanks"})
	if err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Fatalf("handleReply() error = %v, want empty response", err)
	}
}
