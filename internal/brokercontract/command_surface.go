package brokercontract

import (
	"github.com/bartdeboer/ctgbot/internal/component"
	brokercomponent "github.com/bartdeboer/ctgbot/internal/component/broker"
)

type CommandSurfaceDeps struct {
	Inbound       component.ResolvedInboundQueuer
	BrokerActions brokercomponent.Actions
}
