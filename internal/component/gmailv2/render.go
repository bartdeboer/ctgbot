package gmailv2

import (
	"encoding/json"
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	gmailapi "google.golang.org/api/gmail/v1"
)

type parsedMessage struct {
	Record      storedMessage
	TextBody    string
	HTMLBody    string
	Attachments []parsedAttachment
}

type parsedAttachment struct {
	Record  storedAttachment
	Content []byte
}

func (c *Component) parseMessage(message *gmailapi.Message) parsedMessage {
	from := headerValue(message, "From")
	fromEmail, fromLabel := parseAddress(from)
	plain, htmlBody := collectBodyText(message.Payload)
	text := strings.TrimSpace(plain)
	if text == "" {
		text = stripHTML(htmlBody)
	}
	headers := selectedHeaderMap(message)
	date, _ := mail.ParseDate(headerValue(message, "Date"))
	record := storedMessage{
		GmailMessageID: strings.TrimSpace(message.Id),
		GmailThreadID:  strings.TrimSpace(message.ThreadId),
		RFCMessageID:   headerValue(message, "Message-ID"),
		MailboxEmail:   c.providerChannelID(),
		FromEmail:      normalizeEmail(fromEmail),
		FromLabel:      fromLabel,
		ToJSON:         mustJSON(addressList(headerValue(message, "To"))),
		CcJSON:         mustJSON(addressList(headerValue(message, "Cc"))),
		BccJSON:        mustJSON(addressList(headerValue(message, "Bcc"))),
		Subject:        renderedSubjectValue(headerValue(message, "Subject")),
		Date:           date,
		Snippet:        strings.TrimSpace(message.Snippet),
		HeadersJSON:    mustJSON(headers),
	}
	return parsedMessage{Record: record, TextBody: text, HTMLBody: strings.TrimSpace(htmlBody)}
}

func selectedHeaderMap(message *gmailapi.Message) map[string]string {
	out := map[string]string{}
	for _, name := range selectedPromptMetadataHeaders {
		if value := renderedHeaderValue(headerValue(message, name)); value != "" {
			out[name] = value
		}
	}
	return out
}

func parseAddress(value string) (email string, label string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	addr, err := mail.ParseAddress(value)
	if err != nil {
		return value, value
	}
	label = strings.TrimSpace(addr.Name)
	if label == "" {
		label = strings.TrimSpace(addr.Address)
	}
	return strings.TrimSpace(addr.Address), label
}

func addressList(value string) []string {
	list, err := mail.ParseAddressList(value)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, addr := range list {
		if addr != nil && strings.TrimSpace(addr.Address) != "" {
			out = append(out, strings.TrimSpace(addr.Address))
		}
	}
	return out
}

func normalizeEmail(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func mustJSON(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(body)
}

func (c *Component) renderText(record storedMessage, textBody string, attachments []storedAttachment) string {
	var lines []string
	lines = append(lines,
		"Gmail message "+record.ID,
		"From: "+record.FromLabel+emailSuffix(record.FromEmail),
		"To: "+record.ToJSON,
		"Subject: "+record.Subject,
	)
	if !record.Date.IsZero() {
		lines = append(lines, "Date: "+record.Date.Format(time.RFC3339))
	}
	lines = append(lines, "", "Selected headers:")
	var headers map[string]string
	_ = json.Unmarshal([]byte(record.HeadersJSON), &headers)
	for _, name := range selectedPromptMetadataHeaders {
		if value := strings.TrimSpace(headers[name]); value != "" {
			lines = append(lines, name+": "+value)
		}
	}
	if len(attachments) > 0 {
		lines = append(lines, "", "Attachments:")
		for _, a := range attachments {
			lines = append(lines, fmt.Sprintf("- %s filename=%q type=%s size=%d disposition=%s", a.ID, a.Filename, a.ContentType, a.Size, a.Disposition))
		}
	}
	lines = append(lines, "", "Body:", "```text", sanitizePromptBody(textBody), "```")
	return strings.TrimSpace(strings.Join(lines, "\n")) + "\n"
}

func emailSuffix(email string) string {
	email = strings.TrimSpace(email)
	if email == "" {
		return ""
	}
	return " <" + email + ">"
}

func (c *Component) materializeMessage(workspace string, record storedMessage, textBody string, htmlBody string, raw []byte, attachments []parsedAttachment) error {
	if strings.TrimSpace(workspace) == "" {
		return nil
	}
	stored := make([]storedAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		stored = append(stored, attachment.Record)
	}
	if err := writeFile(c.hostMessagePath(workspace, record.ID, ".txt"), []byte(c.renderText(record, textBody, stored))); err != nil {
		return err
	}
	if record.HasRaw && len(raw) > 0 {
		if err := writeFile(c.hostMessagePath(workspace, record.ID, ".eml"), raw); err != nil {
			return err
		}
	}
	if record.HasHTML && strings.TrimSpace(htmlBody) != "" {
		if err := writeFile(c.hostMessagePath(workspace, record.ID, ".html"), []byte(htmlBody)); err != nil {
			return err
		}
	}
	for _, attachment := range attachments {
		if err := writeFile(c.hostAttachmentPath(workspace, attachment.Record.ID), attachment.Content); err != nil {
			return err
		}
	}
	return nil
}

func writeFile(path string, body []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
