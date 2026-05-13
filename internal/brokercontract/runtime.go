package brokercontract

import (
	component "github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type RuntimeSpec struct {
	Chat      coremodel.Chat
	Workspace string
	Bindings  []coremodel.ChatComponent
	Loaded    map[modeluuid.UUID]*component.Loaded
}
