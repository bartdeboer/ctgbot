package gmail

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"net/mail"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	gmailapi "google.golang.org/api/gmail/v1"
)

var _ component.MessageSender = (*Component)(nil)

func (c *Component) SendMessage(ctx context.Context, request component.MessageSendRequest) (component.MessageSendResult, error) {
	client, err := c.client(ctx)
	if err != nil {
		return component.MessageSendResult{}, err
	}
	message, err := buildGmailSendMessage(request)
	if err != nil {
		return component.MessageSendResult{}, err
	}
	sent, err := client.SendMessage(ctx, c.userID(), message)
	if err != nil {
		return component.MessageSendResult{}, err
	}
	if sent == nil {
		return component.MessageSendResult{}, nil
	}
	return component.MessageSendResult{
		ID:       strings.TrimSpace(sent.Id),
		ThreadID: strings.TrimSpace(sent.ThreadId),
	}, nil
}

func buildGmailSendMessage(request component.MessageSendRequest) (*gmailapi.Message, error) {
	to, err := cleanAddressList(request.To)
	if err != nil {
		return nil, fmt.Errorf("to: %w", err)
	}
	if len(to) == 0 {
		return nil, fmt.Errorf("missing --to")
	}
	cc, err := cleanAddressList(request.Cc)
	if err != nil {
		return nil, fmt.Errorf("cc: %w", err)
	}
	bcc, err := cleanAddressList(request.Bcc)
	if err != nil {
		return nil, fmt.Errorf("bcc: %w", err)
	}
	subject, err := safeHeaderValue(request.Subject)
	if err != nil {
		return nil, fmt.Errorf("subject: %w", err)
	}
	if subject == "" {
		return nil, fmt.Errorf("missing --subject")
	}
	body := request.Body
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("missing message body on stdin")
	}
	inReplyTo, err := safeHeaderValue(request.InReplyTo)
	if err != nil {
		return nil, fmt.Errorf("in-reply-to: %w", err)
	}

	raw, err := buildRFC822Message(rfc822Message{
		To:         to,
		Cc:         cc,
		Bcc:        bcc,
		Subject:    subject,
		Body:       body,
		InReplyTo:  inReplyTo,
		References: inReplyTo,
	})
	if err != nil {
		return nil, err
	}

	return &gmailapi.Message{
		Raw:      base64.RawURLEncoding.EncodeToString(raw),
		ThreadId: strings.TrimSpace(request.ThreadID),
	}, nil
}

type rfc822Message struct {
	To         []string
	Cc         []string
	Bcc        []string
	Subject    string
	Body       string
	InReplyTo  string
	References string
}

func buildRFC822Message(message rfc822Message) ([]byte, error) {
	var out bytes.Buffer
	writeHeader(&out, "To", strings.Join(message.To, ", "))
	writeHeader(&out, "Cc", strings.Join(message.Cc, ", "))
	writeHeader(&out, "Bcc", strings.Join(message.Bcc, ", "))
	writeHeader(&out, "Subject", mime.QEncoding.Encode("utf-8", message.Subject))
	writeHeader(&out, "In-Reply-To", message.InReplyTo)
	writeHeader(&out, "References", message.References)
	writeHeader(&out, "MIME-Version", "1.0")
	writeHeader(&out, "Content-Type", `text/plain; charset="UTF-8"`)
	writeHeader(&out, "Content-Transfer-Encoding", "8bit")
	out.WriteString("\r\n")
	body := strings.ReplaceAll(message.Body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")
	out.WriteString(body)
	out.WriteString("\r\n")
	return out.Bytes(), nil
}

func writeHeader(out *bytes.Buffer, name string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	out.WriteString(name)
	out.WriteString(": ")
	out.WriteString(value)
	out.WriteString("\r\n")
}

func cleanAddressList(values []string) ([]string, error) {
	var out []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			safe, err := safeHeaderValue(part)
			if err != nil {
				return nil, err
			}
			if _, err := mail.ParseAddress(safe); err != nil {
				return nil, err
			}
			out = append(out, safe)
		}
	}
	return out, nil
}

func safeHeaderValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if strings.ContainsAny(value, "\r\n") {
		return "", fmt.Errorf("must not contain newlines")
	}
	return value, nil
}
