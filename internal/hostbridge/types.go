package hostbridge

import (
	"encoding/gob"

	"github.com/bartdeboer/ctgbot/internal/chatcommands"
)

type Request struct {
	Request chatcommands.Request
}

type Response struct {
	Result chatcommands.Result
	Error  string
}

func init() {
	gob.Register(chatcommands.RunCommand{})
	gob.Register(chatcommands.SendMedia{})
	gob.Register(chatcommands.ConfigList{})
	gob.Register(chatcommands.ConfigSet{})
	gob.Register(chatcommands.StartSession{})
	gob.Register(chatcommands.StopActiveSession{})
	gob.Register(chatcommands.RefreshActiveSession{})
	gob.Register(chatcommands.PurgeActiveSession{})
}
