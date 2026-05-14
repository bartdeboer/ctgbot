package gmail

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/message"
	gmailapi "google.golang.org/api/gmail/v1"
)

const (
	maxInboundAttachments          = 20
	maxInboundAttachmentBytes      = 25 * 1024 * 1024
	maxTotalInboundAttachmentBytes = 50 * 1024 * 1024
)

func (c *Component) loadMessageAttachments(ctx context.Context, client gmailClient, gmailMessage *gmailapi.Message) ([]message.Media, error) {
	if client == nil || gmailMessage == nil || gmailMessage.Payload == nil {
		return nil, nil
	}
	var out []message.Media
	totalBytes := 0
	if err := c.collectAttachments(ctx, client, strings.TrimSpace(gmailMessage.Id), gmailMessage.Payload, &out, &totalBytes); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Component) collectAttachments(
	ctx context.Context,
	client gmailClient,
	messageID string,
	part *gmailapi.MessagePart,
	out *[]message.Media,
	totalBytes *int,
) error {
	if part == nil {
		return nil
	}
	filename := strings.TrimSpace(part.Filename)
	if filename != "" && part.Body != nil {
		if len(*out) >= maxInboundAttachments {
			c.logf("gmail attachment skipped component=%s message=%s filename=%q reason=max_attachments", c.registration.Ref(), messageID, filename)
			return nil
		}
		content, err := c.attachmentContent(ctx, client, messageID, part)
		if err != nil {
			return err
		}
		if len(content) > maxInboundAttachmentBytes {
			c.logf("gmail attachment skipped component=%s message=%s filename=%q bytes=%d reason=max_attachment_bytes", c.registration.Ref(), messageID, filename, len(content))
			return nil
		}
		if *totalBytes+len(content) > maxTotalInboundAttachmentBytes {
			c.logf("gmail attachment skipped component=%s message=%s filename=%q bytes=%d reason=max_total_attachment_bytes", c.registration.Ref(), messageID, filename, len(content))
			return nil
		}
		*totalBytes += len(content)
		*out = append(*out, message.Media{
			Kind:        "attachment",
			Filename:    filename,
			ContentType: strings.TrimSpace(part.MimeType),
			Content:     content,
		})
	}
	for _, child := range part.Parts {
		if err := c.collectAttachments(ctx, client, messageID, child, out, totalBytes); err != nil {
			return err
		}
	}
	return nil
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
			c.logf("gmail attachment no longer available component=%s message=%s attachment=%s", c.registration.Ref(), messageID, attachmentID)
			return nil, nil
		}
		return nil, fmt.Errorf("get gmail attachment %s: %w", attachmentID, err)
	}
	return content, nil
}
