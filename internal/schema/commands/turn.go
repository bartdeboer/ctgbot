package commands

import (
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/go-clir"
)

type TurnSet struct {
	Key   string
	Value string
}

type TurnGet struct {
	Key string
}

type TurnClear struct {
	Key string
}

func TurnCommands() []commandengine.Definition {
	return []commandengine.Definition{
		{
			Pattern: "turn set <key> <value>",
			Help:    "Set a value for the current turn only",
			Build: func(req *clir.Request) (any, error) {
				key := strings.TrimSpace(req.Params["key"])
				if key == "" {
					return nil, fmt.Errorf("missing turn setting key")
				}
				return TurnSet{Key: key, Value: strings.TrimSpace(req.Params["value"])}, nil
			},
			Sources:               []commandengine.Source{commandengine.SourceHostbridge},
			Policy:                agentPolicy(),
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "turn get",
			Help:    "Show current turn settings",
			Build: func(req *clir.Request) (any, error) {
				_ = req
				return TurnGet{}, nil
			},
			Sources:               []commandengine.Source{commandengine.SourceHostbridge},
			Policy:                agentPolicy(),
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "turn get <key>",
			Help:    "Show a current turn setting",
			Build: func(req *clir.Request) (any, error) {
				key := strings.TrimSpace(req.Params["key"])
				if key == "" {
					return nil, fmt.Errorf("missing turn setting key")
				}
				return TurnGet{Key: key}, nil
			},
			Sources:               []commandengine.Source{commandengine.SourceHostbridge},
			Policy:                agentPolicy(),
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "turn clear <key>",
			Help:    "Clear a current turn setting",
			Build: func(req *clir.Request) (any, error) {
				key := strings.TrimSpace(req.Params["key"])
				if key == "" {
					return nil, fmt.Errorf("missing turn setting key")
				}
				return TurnClear{Key: key}, nil
			},
			Sources:               []commandengine.Source{commandengine.SourceHostbridge},
			Policy:                agentPolicy(),
			InstructionVisibility: commandengine.InstructionImportant,
		},
	}
}
