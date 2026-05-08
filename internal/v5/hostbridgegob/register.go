package hostbridgegob

import (
	"encoding/gob"

	"github.com/bartdeboer/ctgbot/internal/v5/hostbridgecmd"
)

func init() {
	hostbridgecmd.RegisterGobTypes(gob.Register)
}
