package commands

import (
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type ChatListUnregistered struct{}

type ChatApplyPreset struct {
	ChatID string
	Preset string
}

func ChatCommands() []commandengine.Definition {
	return []commandengine.Definition{
		{
			ID:      "broker.chat.list_unregistered",
			Sources: []commandengine.Source{commandengine.SourceCLI},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
			Routes: []commandengine.Route{{
				Pattern: "chat list-unregistered",
				Help:    "List chats that are known but not enabled",
				Build: func(req *clir.Request) (any, error) {
					return ChatListUnregistered{}, nil
				},
			}},
		},
		{
			ID:      "broker.chat.apply_preset",
			Sources: []commandengine.Source{commandengine.SourceCLI},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
			Routes: []commandengine.Route{{
				Pattern: "chat <chat_id> preset <preset>",
				Help:    "Apply a chat component preset",
				Build: func(req *clir.Request) (any, error) {
					return ChatApplyPreset{
						ChatID: req.Params["chat_id"],
						Preset: req.Params["preset"],
					}, nil
				},
			}},
		},
	}
}
