package commands

import (
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/go-clir"
)

type TurnConfigList struct{}

type TurnInfo struct{}

type TurnConfigGet struct {
	Key string
}

type TurnConfigSet struct {
	Key   string
	Value string
}

type TurnConfigUnset struct {
	Key string
}

func TurnCommands() []commandengine.Definition {
	return []commandengine.Definition{
		{
			Pattern: "turn info",
			Help:    "Show current-turn input metadata and files",
			Build: func(req *clir.Request) (any, error) {
				_ = req
				return TurnInfo{}, nil
			},
			Sources:               []commandengine.Source{commandengine.SourceHostbridge},
			Policy:                agentPolicy(),
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "turn config list",
			Help:    "List current turn config keys, values, defaults, and options",
			Build: func(req *clir.Request) (any, error) {
				_ = req
				return TurnConfigList{}, nil
			},
			Sources:               []commandengine.Source{commandengine.SourceHostbridge},
			Policy:                agentPolicy(),
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "turn config get <key>",
			Help:    "Show a current turn config value, default, and options",
			Build: func(req *clir.Request) (any, error) {
				key := strings.TrimSpace(req.Params["key"])
				if key == "" {
					return nil, fmt.Errorf("missing turn config key")
				}
				return TurnConfigGet{Key: key}, nil
			},
			Sources:               []commandengine.Source{commandengine.SourceHostbridge},
			Policy:                agentPolicy(),
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "turn config set <key> <value>",
			Help:    "Set a config value for the current turn only",
			Build: func(req *clir.Request) (any, error) {
				key := strings.TrimSpace(req.Params["key"])
				if key == "" {
					return nil, fmt.Errorf("missing turn config key")
				}
				return TurnConfigSet{Key: key, Value: strings.TrimSpace(req.Params["value"])}, nil
			},
			Sources:               []commandengine.Source{commandengine.SourceHostbridge},
			Policy:                agentPolicy(),
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "turn config unset <key>",
			Help:    "Remove a config override for the current turn only",
			Build: func(req *clir.Request) (any, error) {
				key := strings.TrimSpace(req.Params["key"])
				if key == "" {
					return nil, fmt.Errorf("missing turn config key")
				}
				return TurnConfigUnset{Key: key}, nil
			},
			Sources:               []commandengine.Source{commandengine.SourceHostbridge},
			Policy:                agentPolicy(),
			InstructionVisibility: commandengine.InstructionImportant,
		},
	}
}
