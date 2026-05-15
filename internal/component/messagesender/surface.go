package messagesender

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
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

const Type = "component-message"

// SendCommand is the standard command DTO for component-direct messages.
//
// It backs active component routes such as:
//
//	hostbridge gmail/work message "hello" --to you@example.com
//
// Component implementations decide which transport-specific fields apply. The
// shared shape keeps text and attachments consistent while allowing email-like
// components to use recipients, subject, and reply metadata.
type SendCommand struct {
	Component   string
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

func RegisterGobTypes(register func(any)) {
	register(SendCommand{})
}

// Surface exposes a component.MessageSender as a local command surface. Broker
// constructs it with a live sender; hostbridge constructs it without a sender so
// it can parse the same command shape before sending the DTO over the bridge.
type Surface struct {
	ComponentRef string
	Sender       component.MessageSender
}

var _ component.Component = (*Surface)(nil)
var _ component.CommandSurface = (*Surface)(nil)
var _ component.LocalCommandSurface = (*Surface)(nil)

func NewSurface(componentRef string, sender component.MessageSender) *Surface {
	return &Surface{ComponentRef: strings.TrimSpace(componentRef), Sender: sender}
}

func (s *Surface) Type() string { return Type }

func (s *Surface) UsesLocalCommandRoutes() bool { return true }

func (s *Surface) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{{
		Pattern:               "message <text>",
		Help:                  "Send a message through this component",
		Build:                 s.buildMessage,
		Sources:               []commandengine.Source{commandengine.SourceHostbridge},
		Policy:                simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
		InstructionVisibility: commandengine.InstructionDiscoverable,
	}}
}

func (s *Surface) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if s == nil || s.Sender == nil {
		return nil
	}
	return commandengine.RegisterPattern[SendCommand](registry, "message <text>", s.handleMessage)
}

func (s *Surface) buildMessage(req *clir.Request) (any, error) {
	componentRef := strings.TrimSpace(s.ComponentRef)
	if componentRef == "" {
		return nil, fmt.Errorf("missing component")
	}
	fields, err := parseFields("component message", req.Extra)
	if err != nil {
		return nil, err
	}
	fields.Component = componentRef
	fields.Body = strings.TrimSpace(req.Params["text"])
	if fields.Body == "" && len(fields.Attachments) == 0 {
		return nil, fmt.Errorf("message requires text or --attach")
	}
	return fields, nil
}

func (s *Surface) handleMessage(ctx context.Context, req commandengine.Request, cmd SendCommand) (commandengine.Result, error) {
	_ = req
	if s == nil || s.Sender == nil {
		return commandengine.Result{}, fmt.Errorf("missing message sender")
	}
	result, err := s.Sender.SendMessage(ctx, component.MessageSendRequest{
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

func parseFields(name string, args []string) (SendCommand, error) {
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
		return SendCommand{}, err
	}
	if len(fs.Args()) > 0 {
		return SendCommand{}, fmt.Errorf("unexpected message arguments: %s", strings.Join(fs.Args(), " "))
	}
	attachments := make([]message.Media, 0, len(attach))
	for _, raw := range attach {
		media, err := mediafile.ReadDescriptor(raw)
		if err != nil {
			return SendCommand{}, err
		}
		attachments = append(attachments, media)
	}
	return SendCommand{
		To:          append([]string(nil), to...),
		Cc:          append([]string(nil), cc...),
		Bcc:         append([]string(nil), bcc...),
		Subject:     strings.TrimSpace(*subject),
		ContentType: strings.TrimSpace(*contentType),
		Syntax:      resolveSyntax(*language, *syntax),
		Attachments: attachments,
		ThreadID:    strings.TrimSpace(*threadID),
		InReplyTo:   strings.TrimSpace(*inReplyTo),
	}, nil
}

func resolveSyntax(legacyLanguage string, syntax string) string {
	syntax = strings.TrimSpace(syntax)
	if syntax != "" {
		return syntax
	}
	return strings.TrimSpace(legacyLanguage)
}
