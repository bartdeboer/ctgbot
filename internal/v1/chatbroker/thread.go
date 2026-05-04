package chatbroker

import (
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/dbmodel"
)

type Thread = dbmodel.Thread

func ThreadContainerName(cfg *appstate.Config, thread *Thread) string {
	if thread == nil {
		return ""
	}
	if cfg != nil && !thread.ID.IsNull() {
		if name := strings.TrimSpace(cfg.Thread(thread.ChatID, thread.ID).ContainerName()); name != "" {
			return name
		}
	}
	return strings.TrimSpace(thread.RuntimeName)
}
