package commands

import (
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type Install struct{}
type Quit struct{}

func ProcessCommands() []commandengine.Definition {
	return []commandengine.Definition{
		processCommand("process.install", Install{}, "Install ctgbot binaries from source", []string{"install"}),
		processCommand("process.quit", Quit{}, "Restart ctgbot", []string{"quit"}),
	}
}

func processCommand(id string, command any, help string, patterns []string) commandengine.Definition {
	routes := make([]commandengine.Route, 0, len(patterns))
	for _, pattern := range patterns {
		command := command
		routes = append(routes, commandengine.Route{
			Pattern: pattern,
			Help:    help,
			Build: func(req *clir.Request) (any, error) {
				return command, nil
			},
		})
	}
	return commandengine.Definition{
		ID:      id,
		Sources: processSources(),
		Policy:  simplerbac.Any(simplerbac.RoleRoot),
		Routes:  routes,
	}
}

func processSources() []commandengine.Source {
	return []commandengine.Source{
		commandengine.SourceMessage,
		commandengine.SourceHostbridge,
	}
}
