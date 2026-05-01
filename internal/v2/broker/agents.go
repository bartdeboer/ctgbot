package broker

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

func (b *Broker) runAgents(ctx context.Context, inbound coremodel.ThreadMessage, bindings []coremodel.ChatComponent) ([]coremodel.ThreadMessage, error) {
	if b.components == nil {
		return nil, nil
	}

	var outbound []coremodel.ThreadMessage
	for _, agent := range b.components.Agents() {
		if !matchesAnyBinding(agent, bindings) {
			continue
		}
		b.logf("v2 agent invoking type=%s thread=%s", agent.Type(), inbound.ThreadID)
		message, err := agent.HandleMessage(ctx, inbound)
		if err != nil {
			return outbound, fmt.Errorf("agent %s: %w", agent.Type(), err)
		}
		if message == nil || message.Text == "" {
			b.logf("v2 agent empty output type=%s thread=%s", agent.Type(), inbound.ThreadID)
			continue
		}
		b.logf("v2 agent output type=%s chars=%d", agent.Type(), len(message.Text))
		outbound = append(outbound, *message)
	}
	return outbound, nil
}
