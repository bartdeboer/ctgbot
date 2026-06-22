package gmailv2

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseReplySourceHeadersDecodesSubjectAndRequiresMessageID(t *testing.T) {
	raw := strings.Join([]string{
		"Message-ID: <orig@example.com>",
		"References: <root@example.com>",
		"Subject: =?UTF-8?Q?Hallo_=C3=BCber?=",
		"From: Sender <sender@example.com>",
		"Reply-To: Replies <reply@example.com>",
		"To: Me <me@example.com>",
		"", "body",
	}, "\r\n")

	headers, err := parseReplySourceHeaders([]byte(raw))
	if err != nil {
		t.Fatalf("parseReplySourceHeaders() error = %v", err)
	}
	if got, want := headers.MessageID, "<orig@example.com>"; got != want {
		t.Fatalf("MessageID = %q, want %q", got, want)
	}
	if got, want := headers.Subject, "Hallo über"; got != want {
		t.Fatalf("Subject = %q, want %q", got, want)
	}

	_, err = parseReplySourceHeaders([]byte("Subject: missing\r\n\r\nbody"))
	if err == nil || !strings.Contains(err.Error(), "missing Message-ID") {
		t.Fatalf("missing Message-ID error = %v", err)
	}
}

func TestBuildReplySendRequestBuildsThreadingHeaders(t *testing.T) {
	req, err := buildReplySendRequest(replyBuildInput{
		Source: replySourceHeaders{
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
		Source: replySourceHeaders{
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

func TestBuildReferencesTrimsFromLeft(t *testing.T) {
	var ids []string
	for i := 0; i < 80; i++ {
		ids = append(ids, fmt.Sprintf("<%02d-long-message-id@example.com>", i))
	}
	refs := buildReferences(strings.Join(ids, " "), "<latest@example.com>")
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
