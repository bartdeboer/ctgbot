package gmailv2

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/message"
)

func TestBuildGmailV2SendMessageUsesRelatedMultipartForInlineCID(t *testing.T) {
	message, err := buildGmailSendMessage(component.MessageSendRequest{
		To:          []string{"sender@example.com"},
		Subject:     "Inline image",
		Body:        `<h1>Hello</h1><img src="cid:logo">`,
		ContentType: "text/html",
		Attachments: []message.Media{{
			Filename:    "logo.png",
			ContentType: "image/png",
			ContentID:   "logo",
			Disposition: "inline",
			Content:     []byte("png bytes"),
		}},
	})
	if err != nil {
		t.Fatalf("buildGmailSendMessage() error = %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(message.Raw)
	if err != nil {
		t.Fatalf("decode raw error = %v", err)
	}
	text := string(raw)
	for _, want := range []string{
		"Content-Type: multipart/related; boundary=\"ctgbot-gmail-related-boundary\"\r\n",
		"Content-Type: text/html; charset=\"UTF-8\"\r\n",
		`<h1>Hello</h1><img src="cid:logo">`,
		"Content-Type: image/png; name=\"logo.png\"\r\n",
		"Content-Disposition: inline; filename=\"logo.png\"\r\n",
		"Content-ID: <logo>\r\n",
		"--ctgbot-gmail-related-boundary--\r\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("raw message = %q, want contains %q", text, want)
		}
	}
}

func TestBuildGmailV2SendMessageNestsInlineCIDBeforeRegularAttachments(t *testing.T) {
	message, err := buildGmailSendMessage(component.MessageSendRequest{
		To:          []string{"sender@example.com"},
		Subject:     "Mixed",
		Body:        `<img src="cid:logo">`,
		ContentType: "text/html",
		Attachments: []message.Media{
			{Filename: "logo.png", ContentType: "image/png", ContentID: "logo", Disposition: "inline", Content: []byte("png")},
			{Filename: "report.pdf", ContentType: "application/pdf", Content: []byte("pdf")},
		},
	})
	if err != nil {
		t.Fatalf("buildGmailSendMessage() error = %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(message.Raw)
	if err != nil {
		t.Fatalf("decode raw error = %v", err)
	}
	text := string(raw)
	for _, want := range []string{
		"Content-Type: multipart/mixed; boundary=\"ctgbot-gmail-mixed-boundary\"\r\n",
		"Content-Type: multipart/related; boundary=\"ctgbot-gmail-related-boundary\"\r\n",
		"Content-Disposition: inline; filename=\"logo.png\"\r\n",
		"--ctgbot-gmail-related-boundary--\r\n",
		"Content-Disposition: attachment; filename=\"report.pdf\"\r\n",
		"--ctgbot-gmail-mixed-boundary--\r\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("raw message = %q, want contains %q", text, want)
		}
	}
}
