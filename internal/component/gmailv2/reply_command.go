package gmailv2

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/message/mediafile"
	"github.com/bartdeboer/go-clir"
)

type ReplyCommand struct {
	GmailMessageID string
	To             []string
	Cc             []string
	Subject        string
	Body           string
	ContentType    string
	Syntax         string
	Mode           string
	Attachments    []message.Media
}

func buildReplyCommand(req *clir.Request) (any, error) {
	cmd, err := parseReplyFields("gmailv2 reply", req.Extra)
	if err != nil {
		return nil, err
	}
	cmd.GmailMessageID = strings.TrimSpace(req.Params["message_id"])
	if cmd.GmailMessageID == "" {
		return nil, fmt.Errorf("missing gmail message id")
	}
	if strings.TrimSpace(cmd.Body) == "" && len(cmd.Attachments) == 0 {
		return nil, fmt.Errorf("reply requires --body, --body-file, or --attach")
	}
	return cmd, nil
}

func parseReplyFields(name string, args []string) (ReplyCommand, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var to repeatStringFlag
	var cc repeatStringFlag
	var attach repeatStringFlag
	fs.Var(&to, "to", "Override recipient email address; repeat for multiple recipients")
	fs.Var(&cc, "cc", "Override CC email address; repeat for multiple recipients")
	fs.Var(&attach, "attach", "Attachment descriptor; repeat for multiple attachments")
	subject := fs.String("subject", "", "Override reply subject")
	body := fs.String("body", "", "Reply body")
	bodyFile := fs.String("body-file", "", "Path to reply body file")
	mode := fs.String("mode", "reply", "Recipient mode: reply or reply-all")
	contentType := fs.String("type", "", "Optional message body content type")
	language := fs.String("language", "", "Optional legacy syntax hint")
	syntax := fs.String("syntax", "", "Optional syntax hint")
	if err := fs.Parse(args); err != nil {
		return ReplyCommand{}, err
	}
	if len(fs.Args()) > 0 {
		return ReplyCommand{}, fmt.Errorf("unexpected reply arguments: %s", strings.Join(fs.Args(), " "))
	}
	bodyText := strings.TrimSpace(*body)
	if path := strings.TrimSpace(*bodyFile); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return ReplyCommand{}, fmt.Errorf("read --body-file: %w", err)
		}
		bodyText = string(data)
	}
	attachments := make([]message.Media, 0, len(attach))
	for _, raw := range attach {
		media, err := mediafile.ReadDescriptor(raw)
		if err != nil {
			return ReplyCommand{}, err
		}
		attachments = append(attachments, media)
	}
	return ReplyCommand{
		To:          append([]string(nil), to...),
		Cc:          append([]string(nil), cc...),
		Subject:     strings.TrimSpace(*subject),
		Body:        bodyText,
		ContentType: strings.TrimSpace(*contentType),
		Syntax:      resolveMessageSyntax(*language, *syntax),
		Mode:        strings.TrimSpace(*mode),
		Attachments: attachments,
	}, nil
}

func (c *Component) handleReply(ctx context.Context, req commandengine.Request, cmd ReplyCommand) (commandengine.Result, error) {
	_ = req
	client, err := c.client(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	original, err := client.GetMessage(ctx, c.userID(), cmd.GmailMessageID)
	if err != nil {
		return commandengine.Result{}, fmt.Errorf("get original gmail message %s: %w", cmd.GmailMessageID, err)
	}
	raw, err := client.GetRawMessage(ctx, c.userID(), cmd.GmailMessageID)
	if err != nil {
		return commandengine.Result{}, fmt.Errorf("get original raw gmail message %s: %w", cmd.GmailMessageID, err)
	}
	headers, err := parseReplySourceHeaders(raw)
	if err != nil {
		return commandengine.Result{}, err
	}
	self := c.replySelfEmails(ctx, client)
	reply, err := buildReplySendRequest(replyBuildInput{
		Source:       headers,
		ThreadID:     strings.TrimSpace(original.ThreadId),
		Mode:         cmd.Mode,
		OverrideTo:   cmd.To,
		OverrideCc:   cmd.Cc,
		Subject:      cmd.Subject,
		Body:         cmd.Body,
		ContentType:  cmd.ContentType,
		Attachments:  cmd.Attachments,
		SelfAccounts: self,
	})
	if err != nil {
		return commandengine.Result{}, err
	}
	result, err := c.SendMessage(ctx, reply)
	if err != nil {
		return commandengine.Result{}, err
	}
	var lines []string
	lines = append(lines, "reply sent")
	if strings.TrimSpace(result.ID) != "" {
		lines = append(lines, "id: "+strings.TrimSpace(result.ID))
	}
	if strings.TrimSpace(result.ThreadID) != "" {
		lines = append(lines, "thread_id: "+strings.TrimSpace(result.ThreadID))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (c *Component) replySelfEmails(ctx context.Context, client gmailClient) []string {
	var out []string
	if c != nil {
		out = append(out, c.providerChannelID())
		if !strings.EqualFold(c.userID(), DefaultUserID) {
			out = append(out, c.userID())
		}
	}
	if client != nil {
		if profile, err := client.GetProfile(ctx, DefaultUserID); err == nil && profile != nil {
			out = append(out, profile.EmailAddress)
		}
	}
	return out
}
