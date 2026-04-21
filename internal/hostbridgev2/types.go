package hostbridgev2

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
	gob.Register(chatcommands.SendFile{})
	gob.Register(chatcommands.SendText{})
	gob.Register(chatcommands.ConfigList{})
	gob.Register(chatcommands.ConfigSet{})
	gob.Register(chatcommands.StartSession{})
	gob.Register(chatcommands.StopActiveSession{})
	gob.Register(chatcommands.RefreshActiveSession{})
	gob.Register(chatcommands.PurgeActiveSession{})
}
