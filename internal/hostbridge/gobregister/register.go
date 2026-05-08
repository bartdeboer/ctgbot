package gobregister

import (
	"encoding/gob"

	"github.com/bartdeboer/ctgbot/internal/hostbridge/cmdsurface"
)

func init() {
	cmdsurface.RegisterGobTypes(gob.Register)
}
