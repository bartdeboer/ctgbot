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
	gob.Register(chatcommands.RefreshContainer{})
	gob.Register(chatcommands.PurgeChat{})
	gob.Register(chatcommands.InterruptTurn{})
	gob.Register(chatcommands.Upgrade{})
	gob.Register(chatcommands.Quit{})
	gob.Register(chatcommands.Stop{})
	gob.Register(chatcommands.Status{})
	gob.Register(chatcommands.Help{})
}
