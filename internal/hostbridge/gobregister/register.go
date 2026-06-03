package gobregister

import (
	"encoding/gob"

	"github.com/bartdeboer/ctgbot/internal/app"
	"github.com/bartdeboer/ctgbot/internal/hostbridge/cmdsurface"
)

func init() {
	app.RegisterGobTypes(gob.Register)
	cmdsurface.RegisterGobTypes(gob.Register)
}
