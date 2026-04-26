package hostbridge

import (
	"encoding/gob"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

type CommandRequest struct {
	Request commandengine.Request
}

type CommandResponse struct {
	Result commandengine.Result
	Error  string
}

func init() {
	schemacommands.RegisterGobTypes(gob.Register)
}
