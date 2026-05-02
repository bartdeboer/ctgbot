package broker

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/v2/component"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

func (b *Broker) runAgents(ctx context.Context, inbound coremodel.ThreadMessage, runtime *ChatRuntime) ([]coremodel.ThreadMessage, error) {
	if runtime == nil || len(runtime.Agents) == 0 {
		return nil, nil
	}

	var outbound []coremodel.ThreadMessage
	for _, agent := range runtime.Agents {
		b.logf("v2 agent invoking type=%s thread=%s", agent.Type(), inbound.ThreadID)
		message, err := agent.HandleTurn(ctx, component.AgentTurn{
			Message:       inbound,
			AgentCommands: runtime.AgentCommands,
		})
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
