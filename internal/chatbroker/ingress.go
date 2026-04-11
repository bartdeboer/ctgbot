package chatbroker

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appstate"
)

func (b *Broker) HandleIncomingMessage(ctx context.Context, msg IncomingMessage) (IncomingResult, error) {
	text := strings.TrimSpace(msg.Message)
	if text == "" {
		return IncomingResult{}, nil
	}

	chatCfg, thread, err := b.resolveIncomingThread(ctx, msg, true)
	if err != nil {
		return IncomingResult{}, err
	}
	if chatCfg == nil {
		return IncomingResult{}, fmt.Errorf("missing chat mapping")
	}
	if !chatCfg.Enabled {
		b.logf("ignoring update from disabled chat provider=%q chat=%q title=%q", msg.ProviderType, msg.ProviderChatID, chatCfg.ProviderChatTitle)
		return IncomingResult{}, nil
	}

	if strings.HasPrefix(text, "/") {
		args := normalizeIncomingCommand(msg.ProviderType, text)
		if len(args) == 0 {
			return IncomingResult{}, nil
		}
		reply, err := b.handleCommand(ctx, chatCfg.ID, thread, args[0], args[1:])
		if err != nil {
			return IncomingResult{
				Messages: []OutboundMessage{{Text: fmt.Sprintf("command error: %v", err)}},
			}, nil
		}
		if strings.TrimSpace(reply) == "" {
			return IncomingResult{}, nil
		}
		return IncomingResult{
			Messages: []OutboundMessage{{Text: reply}},
		}, nil
	}

	outcome, err := b.handlePrompt(ctx, chatCfg.ID, thread, text)
	if err != nil {
		return IncomingResult{
			Messages: []OutboundMessage{{Text: fmt.Sprintf("conversation error: %v", err)}},
		}, nil
	}

	var messages []OutboundMessage
	if outcome.Started && outcome.Thread != nil {
		messages = append(messages, OutboundMessage{
			Text: fmt.Sprintf("conversation started\ncontainer: %s\nworkspace: %s", outcome.Thread.ContainerName, outcome.Thread.WorkspaceHost),
		})
	}
	if strings.TrimSpace(outcome.Reply) != "" {
		messages = append(messages, OutboundMessage{Text: outcome.Reply})
	}
	return IncomingResult{Messages: messages}, nil
}

func (b *Broker) ResolveIncomingThread(ctx context.Context, msg IncomingMessage, create bool) (*appstate.ChatConfigEntry, *Thread, error) {
	return b.resolveIncomingThread(ctx, msg, create)
}

func (b *Broker) resolveIncomingThread(ctx context.Context, msg IncomingMessage, create bool) (*appstate.ChatConfigEntry, *Thread, error) {
	if b.Config == nil {
		return nil, nil, fmt.Errorf("missing config")
	}
	if b.Sessions == nil {
		return nil, nil, fmt.Errorf("missing session store")
	}

	providerType := strings.TrimSpace(msg.ProviderType)
	providerChatID := strings.TrimSpace(msg.ProviderChatID)
	providerThreadID := strings.TrimSpace(msg.ProviderThreadID)

	if providerType == "" {
		return nil, nil, fmt.Errorf("missing provider type")
	}
	if providerChatID == "" {
		return nil, nil, fmt.Errorf("missing provider chat id")
	}
	if providerThreadID == "" {
		return nil, nil, fmt.Errorf("missing provider thread id")
	}

	chatLabel := strings.TrimSpace(msg.ChatLabel)
	if chatLabel == "" {
		chatLabel = strings.TrimSpace(msg.UserLabel)
	}

	var (
		chatCfg *appstate.ChatConfigEntry
		err     error
	)
	if create {
		chatCfg, err = b.Config.EnsureProviderChat(providerType, providerChatID, chatLabel)
	} else {
		chatCfg, err = b.Config.FindProviderChat(providerType, providerChatID)
	}
	if err != nil || chatCfg == nil {
		return chatCfg, nil, err
	}

	var thread *Thread
	if create {
		thread, err = b.Sessions.EnsureThread(ctx, chatCfg.ID, providerThreadID)
	} else {
		thread, err = b.Sessions.FindThread(ctx, chatCfg.ID, providerThreadID)
	}
	if err != nil {
		return nil, nil, err
	}
	return chatCfg, thread, nil
}

func normalizeIncomingCommand(providerType string, text string) []string {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return nil
	}

	fields[0] = strings.TrimPrefix(fields[0], "/")
	if providerType == "telegram" {
		if i := strings.Index(fields[0], "@"); i >= 0 {
			fields[0] = fields[0][:i]
		}
	}
	return fields
}
