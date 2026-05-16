package gmailv2

import (
	"context"
	"fmt"
	"strings"

	gmailapi "google.golang.org/api/gmail/v1"
)

type incomingRecord struct {
	Record   storedMessage
	TextBody string
}

func (c *Component) storeIncomingMessage(ctx context.Context, client gmailClient, gmailMessage *gmailapi.Message) (*incomingRecord, bool, error) {
	if gmailMessage == nil {
		return nil, false, fmt.Errorf("missing gmail message")
	}
	gmailID := strings.TrimSpace(gmailMessage.Id)
	if gmailID == "" {
		return nil, false, fmt.Errorf("missing gmail message id")
	}
	existing, err := c.store.messageByGmailID(ctx, gmailID)
	if err != nil || existing != nil {
		if existing == nil {
			return nil, false, err
		}
		return &incomingRecord{Record: *existing}, false, nil
	}
	parsed := c.parseMessage(gmailMessage)
	parsed.Record.ID = newStoreID()
	raw, err := c.rawMessageBytes(ctx, client, gmailID)
	if err != nil {
		return nil, false, err
	}
	parsed.Record.HasText = strings.TrimSpace(parsed.TextBody) != ""
	parsed.Record.HasRaw = c.componentConfig.materializeRaw() && len(raw) > 0
	parsed.Record.HasHTML = c.componentConfig.materializeHTML() && strings.TrimSpace(parsed.HTMLBody) != ""
	attachments, err := c.loadStoredAttachments(ctx, client, gmailID, gmailMessage.Payload)
	if err != nil {
		return nil, false, err
	}
	for i := range attachments {
		attachments[i].Record.MessageID = parsed.Record.ID
	}
	if err := c.store.saveMessage(ctx, &parsed.Record); err != nil {
		return nil, false, err
	}
	for i := range attachments {
		if err := c.store.saveAttachment(ctx, &attachments[i].Record); err != nil {
			return nil, false, err
		}
	}
	workspaces, err := c.workspacePaths(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, workspace := range workspaces {
		if err := c.materializeMessage(workspace, parsed.Record, parsed.TextBody, parsed.HTMLBody, raw, attachments); err != nil {
			return nil, false, err
		}
	}
	return &incomingRecord{Record: parsed.Record, TextBody: parsed.TextBody}, true, nil
}

func (c *Component) rawMessageBytes(ctx context.Context, client gmailClient, gmailID string) ([]byte, error) {
	if !c.componentConfig.materializeRaw() {
		return nil, nil
	}
	raw, err := client.GetRawMessage(ctx, c.userID(), gmailID)
	if err != nil {
		if isGmailNotFound(err) {
			c.logf("gmailv2 raw message no longer available component=%s message=%s", c.registration.Ref(), gmailID)
		} else {
			c.logf("gmailv2 raw message fetch failed component=%s message=%s err=%v", c.registration.Ref(), gmailID, err)
		}
		return nil, nil
	}
	return raw, nil
}
