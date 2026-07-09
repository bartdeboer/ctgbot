package outlook

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type statusCommand struct{}
type queryCommand struct{ Query string }
type fetchCommand struct{ OutlookMessageID string }
type messageViewCommand struct{ MessageID string }
type messageCommand struct{}
type replyCommand struct{}

func RegisterGobTypes(register func(any)) {
	register(statusCommand{})
	register(queryCommand{})
	register(fetchCommand{})
	register(messageViewCommand{})
	register(messageCommand{})
	register(replyCommand{})
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	defs := []commandengine.Definition{
		def("status", "Show Outlook component status", func(*clir.Request) (any, error) {
			return statusCommand{}, nil
		}, commandengine.SourceCLI),
		def("query <query>", "List recent Outlook messages (minimal v1)", func(req *clir.Request) (any, error) {
			return queryCommand{Query: req.Params["query"]}, nil
		}, commandengine.SourceHostbridge),
		def("fetch <message_id>", "Fetch and store an Outlook message", func(req *clir.Request) (any, error) {
			return fetchCommand{OutlookMessageID: req.Params["message_id"]}, nil
		}, commandengine.SourceHostbridge),
		def("message view <message_id>", "View a stored Outlook message", func(req *clir.Request) (any, error) {
			return messageViewCommand{MessageID: req.Params["message_id"]}, nil
		}, commandengine.SourceHostbridge),
		def("message <text>", "Send Outlook mail (not implemented in skeleton)", func(req *clir.Request) (any, error) {
			_ = req
			return messageCommand{}, nil
		}, commandengine.SourceHostbridge),
		def("reply <message_id>", "Reply to Outlook mail (not implemented in skeleton)", func(req *clir.Request) (any, error) {
			_ = req
			return replyCommand{}, nil
		}, commandengine.SourceHostbridge),
	}
	defs = append(defs, configsurface.CommandDefinitions(configsurface.DefinitionOptions{
		Sources:       []commandengine.Source{commandengine.SourceHostbridge},
		Policy:        simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
		SupportsUnset: true,
	})...)
	return defs
}

func def(pattern string, help string, build func(*clir.Request) (any, error), sources ...commandengine.Source) commandengine.Definition {
	policy := simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent)
	if len(sources) == 1 && sources[0] == commandengine.SourceCLI {
		policy = simplerbac.Any(simplerbac.RoleRoot)
	}
	return commandengine.Definition{Pattern: pattern, Help: help, Build: build, Sources: sources, Policy: policy}
}

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	handlers := []error{
		commandengine.RegisterPattern[statusCommand](registry, "status", func(ctx context.Context, req commandengine.Request, cmd statusCommand) (commandengine.Result, error) {
			return c.status(ctx)
		}),
		commandengine.RegisterPattern[queryCommand](registry, "query <query>", c.handleQuery),
		commandengine.RegisterPattern[fetchCommand](registry, "fetch <message_id>", c.handleFetch),
		commandengine.RegisterPattern[messageViewCommand](registry, "message view <message_id>", c.handleMessageView),
		commandengine.RegisterPattern[messageCommand](registry, "message <text>", func(context.Context, commandengine.Request, messageCommand) (commandengine.Result, error) {
			return commandengine.Result{}, fmt.Errorf("outlook message send is not implemented yet")
		}),
		commandengine.RegisterPattern[replyCommand](registry, "reply <message_id>", func(context.Context, commandengine.Request, replyCommand) (commandengine.Result, error) {
			return commandengine.Result{}, fmt.Errorf("outlook reply is not implemented yet")
		}),
		configsurface.RegisterCommandHandlers(registry, c),
	}
	for _, err := range handlers {
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Component) status(ctx context.Context) (commandengine.Result, error) {
	lines := []string{
		"outlook status",
		"component: " + c.registration.Ref(),
		"profile: " + strings.TrimSpace(c.profile.Path),
		"poll_interval: " + c.componentConfig.pollInterval().String(),
		"poll_folder: " + c.componentConfig.PollFolder,
	}
	client, err := c.client(ctx)
	if err != nil {
		lines = append(lines, "auth: not authenticated")
		return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
	}
	me, err := client.Me(ctx)
	if err != nil {
		lines = append(lines, "auth: token unavailable", "error: "+err.Error())
		return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
	}
	account := strings.TrimSpace(firstNonEmpty(me.Mail, me.UserPrincipalName))
	if account != "" {
		c.mailboxEmail = account
		lines = append(lines, "auth: authenticated", "account: "+account)
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (c *Component) handleQuery(ctx context.Context, req commandengine.Request, cmd queryCommand) (commandengine.Result, error) {
	_, _ = req, cmd // The first validation slice lists recent mail; Graph search can follow later.
	client, err := c.client(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	messages, err := client.ListMessages(ctx, c.componentConfig.PollFolder, c.componentConfig.MaxPollMessages)
	if err != nil {
		return commandengine.Result{}, err
	}
	lines := []string{"outlook_message_id\treceived\tfrom\tsubject\tsnippet"}
	for _, m := range messages {
		r := c.parseMessage(m)
		lines = append(lines, strings.Join([]string{
			cleanCell(r.OutlookMessageID),
			cleanCell(r.ReceivedAt.Format("2006-01-02T15:04:05Z07:00")),
			cleanCell(r.FromLabel + emailSuffix(r.FromEmail)),
			cleanCell(r.Subject),
			cleanCell(r.BodyPreview),
		}, "\t"))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (c *Component) handleFetch(ctx context.Context, req commandengine.Request, cmd fetchCommand) (commandengine.Result, error) {
	_ = req
	client, err := c.client(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	msg, err := client.GetMessage(ctx, cmd.OutlookMessageID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if msg == nil {
		return commandengine.Result{}, fmt.Errorf("outlook message %s not found", cmd.OutlookMessageID)
	}
	record, _, err := c.storeIncomingMessage(ctx, *msg)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: c.renderStoredMessage(*record)}, nil
}

func (c *Component) handleMessageView(ctx context.Context, req commandengine.Request, cmd messageViewCommand) (commandengine.Result, error) {
	_ = req
	record, err := c.store.messageByRef(ctx, cmd.MessageID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if record == nil {
		return commandengine.Result{}, fmt.Errorf("outlook message %s not found", cmd.MessageID)
	}
	return commandengine.Result{Text: c.renderStoredMessage(*record)}, nil
}

func (c *Component) renderStoredMessage(r storedMessage) string {
	return strings.Join([]string{
		"Outlook message " + r.ID,
		"outlook_message_id: " + r.OutlookMessageID,
		"conversation_id: " + r.ConversationID,
		"internet_message_id: " + r.InternetMessageID,
		"from: " + r.FromLabel + emailSuffix(r.FromEmail),
		"subject: " + r.Subject,
		"received_at: " + r.ReceivedAt.Format("2006-01-02T15:04:05Z07:00"),
		"",
		"Body:",
		r.BodyText,
	}, "\n")
}

func cleanCell(value string) string {
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}
