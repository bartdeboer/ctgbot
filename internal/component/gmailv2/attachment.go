package gmailv2

import (
	"context"
	"fmt"
	"strings"

	gmailapi "google.golang.org/api/gmail/v1"
)

const (
	maxInboundAttachments          = 20
	maxInboundAttachmentBytes      = 25 * 1024 * 1024
	maxTotalInboundAttachmentBytes = 50 * 1024 * 1024
)

func (c *Component) loadStoredAttachments(ctx context.Context, client gmailClient, messageID string, root *gmailapi.MessagePart) ([]parsedAttachment, error) {
	if client == nil || root == nil {
		return nil, nil
	}
	var out []parsedAttachment
	totalBytes := 0
	if err := c.collectStoredAttachments(ctx, client, strings.TrimSpace(messageID), root, &out, &totalBytes); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Component) collectStoredAttachments(ctx context.Context, client gmailClient, messageID string, part *gmailapi.MessagePart, out *[]parsedAttachment, totalBytes *int) error {
	if part == nil {
		return nil
	}
	filename := strings.TrimSpace(part.Filename)
	if filename != "" && part.Body != nil {
		if len(*out) >= maxInboundAttachments {
			c.logf("gmailv2 attachment skipped component=%s message=%s filename=%q reason=max_attachments", c.registration.Ref(), messageID, filename)
			return nil
		}
		content, err := c.attachmentContent(ctx, client, messageID, part)
		if err != nil {
			return err
		}
		if len(content) > maxInboundAttachmentBytes {
			c.logf("gmailv2 attachment skipped component=%s message=%s filename=%q bytes=%d reason=max_attachment_bytes", c.registration.Ref(), messageID, filename, len(content))
			return nil
		}
		if *totalBytes+len(content) > maxTotalInboundAttachmentBytes {
			c.logf("gmailv2 attachment skipped component=%s message=%s filename=%q bytes=%d reason=max_total_attachment_bytes", c.registration.Ref(), messageID, filename, len(content))
			return nil
		}
		*totalBytes += len(content)
		attachmentID := newStoreID()
		*out = append(*out, parsedAttachment{
			Record: storedAttachment{
				ID:                attachmentID,
				GmailAttachmentID: strings.TrimSpace(part.Body.AttachmentId),
				Kind:              attachmentKind(part),
				Filename:          filename,
				ContentType:       strings.TrimSpace(part.MimeType),
				Disposition:       messagePartDisposition(part),
				ContentID:         headerValuePart(part, "Content-ID"),
				Size:              int64(len(content)),
			},
			Content: content,
		})
	}
	for _, child := range part.Parts {
		if err := c.collectStoredAttachments(ctx, client, messageID, child, out, totalBytes); err != nil {
			return err
		}
	}
	return nil
}

func attachmentKind(part *gmailapi.MessagePart) string {
	if strings.EqualFold(messagePartDisposition(part), "inline") {
		return "inline"
	}
	return "attachment"
}

func messagePartDisposition(part *gmailapi.MessagePart) string {
	value := strings.ToLower(headerValuePart(part, "Content-Disposition"))
	switch {
	case strings.Contains(value, "inline"):
		return "inline"
	case strings.Contains(value, "attachment"):
		return "attachment"
	default:
		return "unknown"
	}
}

func headerValuePart(part *gmailapi.MessagePart, name string) string {
	if part == nil {
		return ""
	}
	for _, header := range part.Headers {
		if header != nil && strings.EqualFold(strings.TrimSpace(header.Name), strings.TrimSpace(name)) {
			return strings.Trim(strings.TrimSpace(header.Value), "<>")
		}
	}
	return ""
}

func (c *Component) attachmentContent(ctx context.Context, client gmailClient, messageID string, part *gmailapi.MessagePart) ([]byte, error) {
	if part == nil || part.Body == nil {
		return nil, nil
	}
	if data := strings.TrimSpace(part.Body.Data); data != "" {
		return decodeGmailBytes(data), nil
	}
	attachmentID := strings.TrimSpace(part.Body.AttachmentId)
	if attachmentID == "" {
		return nil, nil
	}
	content, err := client.GetAttachment(ctx, c.userID(), messageID, attachmentID)
	if err != nil {
		if isGmailNotFound(err) {
			c.logf("gmailv2 attachment no longer available component=%s message=%s attachment=%s", c.registration.Ref(), messageID, attachmentID)
			return nil, nil
		}
		return nil, fmt.Errorf("get gmail attachment %s: %w", attachmentID, err)
	}
	return content, nil
}
