package broker

import (
	"context"
	"fmt"

	component "github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/inbound"
)

func (b *Broker) admitInbound(ctx context.Context, event component.InboundEvent) (inbound.Admission, error) {
	if b == nil || b.App == nil {
		return inbound.Admission{}, fmt.Errorf("missing broker app")
	}
	return b.App.AdmitInbound(ctx, event)
}
