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
	"github.com/bartdeboer/ctgbot/internal/message"
	gmailapi "google.golang.org/api/gmail/v1"
)

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
		return nil, fmt.Errorf("missing message body")
	}
	inReplyTo, err := safeHeaderValue(request.InReplyTo)
	if err != nil {
		return nil, fmt.Errorf("in-reply-to: %w", err)
	}

	raw, err := buildRFC822Message(rfc822Message{
		To:          to,
		Cc:          cc,
		Bcc:         bcc,
		Subject:     subject,
		Body:        body,
		ContentType: request.ContentType,
		InReplyTo:   inReplyTo,
		References:  inReplyTo,
		Attachments: request.Attachments,
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
	To          []string
	Cc          []string
	Bcc         []string
	Subject     string
	Body        string
	ContentType string
	InReplyTo   string
	References  string
	Attachments []message.Media
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
	if len(message.Attachments) == 0 {
		writeBodyPart(&out, message.Body, normalizedBodyContentType(message.ContentType))
		return out.Bytes(), nil
	}

	boundary := "ctgbot-gmail-boundary"
	writeHeader(&out, "Content-Type", `multipart/mixed; boundary="`+boundary+`"`)
	out.WriteString("\r\n")
	writeMultipartBodyPart(&out, boundary, message.Body, normalizedBodyContentType(message.ContentType))
	for _, attachment := range message.Attachments {
		if err := writeMultipartAttachment(&out, boundary, attachment); err != nil {
			return nil, err
		}
	}
	out.WriteString("--")
	out.WriteString(boundary)
	out.WriteString("--\r\n")
	return out.Bytes(), nil
}

func writeBodyPart(out *bytes.Buffer, body string, contentType string) {
	writeHeader(out, "Content-Type", contentType+`; charset="UTF-8"`)
	writeHeader(out, "Content-Transfer-Encoding", "8bit")
	out.WriteString("\r\n")
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")
	out.WriteString(body)
	out.WriteString("\r\n")
}

func writeMultipartBodyPart(out *bytes.Buffer, boundary string, body string, contentType string) {
	out.WriteString("--")
	out.WriteString(boundary)
	out.WriteString("\r\n")
	writeBodyPart(out, body, contentType)
}

func writeMultipartAttachment(out *bytes.Buffer, boundary string, attachment message.Media) error {
	filename, err := safeHeaderValue(attachment.Filename)
	if err != nil {
		return fmt.Errorf("attachment filename: %w", err)
	}
	if filename == "" {
		filename = "attachment"
	}
	contentType, err := safeHeaderValue(attachment.ContentType)
	if err != nil {
		return fmt.Errorf("attachment content type: %w", err)
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	contentID, err := safeHeaderValue(attachment.ContentID)
	if err != nil {
		return fmt.Errorf("attachment content id: %w", err)
	}
	disposition, err := attachmentDisposition(attachment.Disposition, contentID)
	if err != nil {
		return err
	}
	out.WriteString("--")
	out.WriteString(boundary)
	out.WriteString("\r\n")
	writeHeader(out, "Content-Type", contentType+`; name="`+mime.QEncoding.Encode("utf-8", filename)+`"`)
	writeHeader(out, "Content-Disposition", disposition+`; filename="`+mime.QEncoding.Encode("utf-8", filename)+`"`)
	if contentID != "" {
		writeHeader(out, "Content-ID", bracketedContentID(contentID))
	}
	writeHeader(out, "Content-Transfer-Encoding", "base64")
	out.WriteString("\r\n")
	writeBase64Lines(out, attachment.Content)
	out.WriteString("\r\n")
	return nil
}

func attachmentDisposition(value string, contentID string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "":
		if strings.TrimSpace(contentID) != "" {
			return "inline", nil
		}
		return "attachment", nil
	case "attachment", "inline":
		return value, nil
	default:
		return "", fmt.Errorf("invalid attachment disposition: %s", value)
	}
}

func bracketedContentID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "<")
	value = strings.TrimSuffix(value, ">")
	if value == "" {
		return ""
	}
	return "<" + value + ">"
}

func writeBase64Lines(out *bytes.Buffer, content []byte) {
	encoded := base64.StdEncoding.EncodeToString(content)
	for len(encoded) > 76 {
		out.WriteString(encoded[:76])
		out.WriteString("\r\n")
		encoded = encoded[76:]
	}
	if encoded != "" {
		out.WriteString(encoded)
		out.WriteString("\r\n")
	}
}

func normalizedBodyContentType(contentType string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "text/html":
		return "text/html"
	default:
		return "text/plain"
	}
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
