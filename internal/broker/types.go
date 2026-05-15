package broker

import (
	"github.com/bartdeboer/ctgbot/internal/component"
	brokercomponent "github.com/bartdeboer/ctgbot/internal/component/broker"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

// CommandSurfaceDeps are broker-owned runtime capabilities needed when app
// assembles command surfaces for a chat.
type CommandSurfaceDeps struct {
	Inbound       component.ResolvedInboundQueuer
	BrokerActions brokercomponent.Actions
}

// RuntimeSpec is the app-resolved runtime view that broker needs to build a
// chat runtime without reading storage directly.
type RuntimeSpec struct {
	Chat      coremodel.Chat
	Workspace string
	Bindings  []coremodel.ChatComponent
	Loaded    map[modeluuid.UUID]*component.Loaded
}
