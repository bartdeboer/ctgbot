package gmailv2

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/message/mediafile"
	"github.com/bartdeboer/go-clir"
)

// MessageCommand is Gmail's hostbridge send command.
//
// It is intentionally owned by the Gmail v2 component: when an agent runs
// `hostbridge gmailv2/work message ...`, ctgbot invokes this command surface on
// the gmailv2/work component and the handler calls Gmail's SendMessage method.
type MessageCommand struct {
	To          []string
	Cc          []string
	Bcc         []string
	Subject     string
	Body        string
	ContentType string
	Syntax      string
	Attachments []message.Media
	ThreadID    string
	InReplyTo   string
}

type repeatStringFlag []string

func (f *repeatStringFlag) String() string {
	if f == nil {
		return ""
	}
	return strings.Join(*f, ",")
}

func (f *repeatStringFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	*f = append(*f, value)
	return nil
}

func buildMessageCommand(req *clir.Request) (any, error) {
	fields, err := parseMessageFields("gmailv2 message", req.Extra)
	if err != nil {
		return nil, err
	}
	fields.Body = strings.TrimSpace(req.Params["text"])
	if fields.Body == "" && len(fields.Attachments) == 0 {
		return nil, fmt.Errorf("message requires text or --attach")
	}
	return fields, nil
}

func parseMessageFields(name string, args []string) (MessageCommand, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var to repeatStringFlag
	var cc repeatStringFlag
	var bcc repeatStringFlag
	var attach repeatStringFlag
	fs.Var(&to, "to", "Recipient email address; repeat for multiple recipients")
	fs.Var(&cc, "cc", "CC email address; repeat for multiple recipients")
	fs.Var(&bcc, "bcc", "BCC email address; repeat for multiple recipients")
	fs.Var(&attach, "attach", "Attachment descriptor; repeat for multiple attachments")
	subject := fs.String("subject", "", "Message subject")
	contentType := fs.String("type", "", "Optional message body content type")
	language := fs.String("language", "", "Optional legacy syntax hint")
	syntax := fs.String("syntax", "", "Optional syntax hint")
	threadID := fs.String("thread-id", "", "Gmail thread id for replies")
	inReplyTo := fs.String("in-reply-to", "", "RFC Message-ID being replied to")
	if err := fs.Parse(args); err != nil {
		return MessageCommand{}, err
	}
	if len(fs.Args()) > 0 {
		return MessageCommand{}, fmt.Errorf("unexpected message arguments: %s", strings.Join(fs.Args(), " "))
	}
	attachments := make([]message.Media, 0, len(attach))
	for _, raw := range attach {
		media, err := mediafile.ReadDescriptor(raw)
		if err != nil {
			return MessageCommand{}, err
		}
		attachments = append(attachments, media)
	}
	return MessageCommand{
		To:          append([]string(nil), to...),
		Cc:          append([]string(nil), cc...),
		Bcc:         append([]string(nil), bcc...),
		Subject:     strings.TrimSpace(*subject),
		ContentType: strings.TrimSpace(*contentType),
		Syntax:      resolveMessageSyntax(*language, *syntax),
		Attachments: attachments,
		ThreadID:    strings.TrimSpace(*threadID),
		InReplyTo:   strings.TrimSpace(*inReplyTo),
	}, nil
}

func resolveMessageSyntax(legacyLanguage string, syntax string) string {
	syntax = strings.TrimSpace(syntax)
	if syntax != "" {
		return syntax
	}
	return strings.TrimSpace(legacyLanguage)
}

func (c *Component) handleMessage(ctx context.Context, req commandengine.Request, cmd MessageCommand) (commandengine.Result, error) {
	_ = req
	result, err := c.SendMessage(ctx, component.MessageSendRequest{
		To:          append([]string(nil), cmd.To...),
		Cc:          append([]string(nil), cmd.Cc...),
		Bcc:         append([]string(nil), cmd.Bcc...),
		Subject:     strings.TrimSpace(cmd.Subject),
		Body:        cmd.Body,
		ContentType: strings.TrimSpace(cmd.ContentType),
		Syntax:      strings.TrimSpace(cmd.Syntax),
		Attachments: append([]message.Media(nil), cmd.Attachments...),
		ThreadID:    strings.TrimSpace(cmd.ThreadID),
		InReplyTo:   strings.TrimSpace(cmd.InReplyTo),
	})
	if err != nil {
		return commandengine.Result{}, err
	}
	var lines []string
	lines = append(lines, "message sent")
	if strings.TrimSpace(result.ID) != "" {
		lines = append(lines, "id: "+strings.TrimSpace(result.ID))
	}
	if strings.TrimSpace(result.ThreadID) != "" {
		lines = append(lines, "thread_id: "+strings.TrimSpace(result.ThreadID))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}
