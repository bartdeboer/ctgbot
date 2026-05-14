package gmail

import (
	"context"
	"fmt"
	"strings"

	gmailapi "google.golang.org/api/gmail/v1"
)

type gmailClient interface {
	GetProfile(ctx context.Context, userID string) (*gmailapi.Profile, error)
	ListHistory(ctx context.Context, userID string, startHistoryID uint64, pageToken string) (*gmailapi.ListHistoryResponse, error)
	GetMessage(ctx context.Context, userID string, messageID string) (*gmailapi.Message, error)
	GetAttachment(ctx context.Context, userID string, messageID string, attachmentID string) ([]byte, error)
	SendMessage(ctx context.Context, userID string, message *gmailapi.Message) (*gmailapi.Message, error)
}

type serviceClient struct {
	service *gmailapi.Service
}

func (c *Component) client(ctx context.Context) (gmailClient, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: missing gmail component", errMissingAuthMaterial)
	}
	if c.clientOverride != nil {
		return c.clientOverride, nil
	}
	if c.Service == nil {
		service, err := c.serviceFromStoredToken(ctx)
		if err != nil {
			return nil, err
		}
		c.Service = service
	}
	if c.Service == nil {
		return nil, fmt.Errorf("%w: gmail is not authenticated; run ctgbot component %s auth", errMissingAuthMaterial, c.registration.Ref())
	}
	return serviceClient{service: c.Service}, nil
}

func (c serviceClient) GetProfile(ctx context.Context, userID string) (*gmailapi.Profile, error) {
	if c.service == nil {
		return nil, fmt.Errorf("missing gmail service")
	}
	return c.service.Users.GetProfile(cleanUserID(userID)).Context(ctx).Do()
}

func (c serviceClient) ListHistory(ctx context.Context, userID string, startHistoryID uint64, pageToken string) (*gmailapi.ListHistoryResponse, error) {
	if c.service == nil {
		return nil, fmt.Errorf("missing gmail service")
	}
	call := c.service.Users.History.List(cleanUserID(userID)).StartHistoryId(startHistoryID).HistoryTypes("messageAdded")
	if pageToken = strings.TrimSpace(pageToken); pageToken != "" {
		call.PageToken(pageToken)
	}
	return call.Context(ctx).Do()
}

func (c serviceClient) GetMessage(ctx context.Context, userID string, messageID string) (*gmailapi.Message, error) {
	if c.service == nil {
		return nil, fmt.Errorf("missing gmail service")
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil, fmt.Errorf("missing gmail message id")
	}
	return c.service.Users.Messages.Get(cleanUserID(userID), messageID).Format("full").Context(ctx).Do()
}

func (c serviceClient) GetAttachment(ctx context.Context, userID string, messageID string, attachmentID string) ([]byte, error) {
	if c.service == nil {
		return nil, fmt.Errorf("missing gmail service")
	}
	messageID = strings.TrimSpace(messageID)
	attachmentID = strings.TrimSpace(attachmentID)
	if messageID == "" {
		return nil, fmt.Errorf("missing gmail message id")
	}
	if attachmentID == "" {
		return nil, fmt.Errorf("missing gmail attachment id")
	}
	body, err := c.service.Users.Messages.Attachments.Get(cleanUserID(userID), messageID, attachmentID).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return decodeGmailBytes(body.Data), nil
}

func (c serviceClient) SendMessage(ctx context.Context, userID string, message *gmailapi.Message) (*gmailapi.Message, error) {
	if c.service == nil {
		return nil, fmt.Errorf("missing gmail service")
	}
	if message == nil {
		return nil, fmt.Errorf("missing gmail message")
	}
	return c.service.Users.Messages.Send(cleanUserID(userID), message).Context(ctx).Do()
}

func cleanUserID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return DefaultUserID
	}
	return value
}
