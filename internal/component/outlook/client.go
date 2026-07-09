package outlook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type outlookClient interface {
	Me(ctx context.Context) (*graphUser, error)
	ListMessages(ctx context.Context, folder string, limit int) ([]graphMessage, error)
	GetMessage(ctx context.Context, id string) (*graphMessage, error)
}

type graphClient struct{ http *http.Client }

func newGraphClient(client *http.Client) *graphClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &graphClient{http: client}
}

type graphUser struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	Mail              string `json:"mail"`
	UserPrincipalName string `json:"userPrincipalName"`
}
type emailAddress struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}
type recipient struct {
	EmailAddress emailAddress `json:"emailAddress"`
}
type itemBody struct {
	ContentType string `json:"contentType"`
	Content     string `json:"content"`
}
type graphMessage struct {
	ID                string      `json:"id"`
	ConversationID    string      `json:"conversationId"`
	InternetMessageID string      `json:"internetMessageId"`
	Subject           string      `json:"subject"`
	BodyPreview       string      `json:"bodyPreview"`
	Body              itemBody    `json:"body"`
	From              recipient   `json:"from"`
	Sender            recipient   `json:"sender"`
	ToRecipients      []recipient `json:"toRecipients"`
	CcRecipients      []recipient `json:"ccRecipients"`
	ReplyTo           []recipient `json:"replyTo"`
	ReceivedDateTime  time.Time   `json:"receivedDateTime"`
	HasAttachments    bool        `json:"hasAttachments"`
}
type graphListMessagesResponse struct {
	Value []graphMessage `json:"value"`
}

func (c *graphClient) Me(ctx context.Context) (*graphUser, error) {
	var out graphUser
	if err := c.get(ctx, "https://graph.microsoft.com/v1.0/me?$select=id,displayName,mail,userPrincipalName", &out); err != nil {
		return nil, err
	}
	return &out, nil
}
func (c *graphClient) ListMessages(ctx context.Context, folder string, limit int) ([]graphMessage, error) {
	if limit <= 0 {
		limit = DefaultMaxPollMessages
	}
	if folder = strings.TrimSpace(folder); folder == "" {
		folder = DefaultPollFolder
	}
	values := url.Values{}
	values.Set("$top", fmt.Sprint(limit))
	values.Set("$orderby", "receivedDateTime desc")
	values.Set("$select", "id,conversationId,internetMessageId,subject,bodyPreview,body,from,sender,toRecipients,ccRecipients,replyTo,receivedDateTime,hasAttachments")
	endpoint := "https://graph.microsoft.com/v1.0/me/mailFolders/" + url.PathEscape(folder) + "/messages?" + values.Encode()
	var out graphListMessagesResponse
	if err := c.get(ctx, endpoint, &out); err != nil {
		return nil, err
	}
	return out.Value, nil
}
func (c *graphClient) GetMessage(ctx context.Context, id string) (*graphMessage, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("missing outlook message id")
	}
	values := url.Values{}
	values.Set("$select", "id,conversationId,internetMessageId,subject,bodyPreview,body,from,sender,toRecipients,ccRecipients,replyTo,receivedDateTime,hasAttachments")
	var out graphMessage
	if err := c.get(ctx, "https://graph.microsoft.com/v1.0/me/messages/"+url.PathEscape(id)+"?"+values.Encode(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}
func (c *graphClient) get(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Prefer", `IdType="ImmutableId"`)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("graph GET %s: %s: %s", endpoint, resp.Status, strings.TrimSpace(string(body)))
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	return dec.Decode(out)
}
