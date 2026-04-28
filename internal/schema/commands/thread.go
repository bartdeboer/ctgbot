package commands

import (
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type RefreshContainer struct{}
type StartContainer struct{}
type StopContainer struct{}
type PurgeChat struct{}
type InterruptTurn struct{}
type Upgrade struct{}
type Quit struct{}
type Status struct{}

func ThreadCommands() []commandengine.Definition {
	return []commandengine.Definition{
		threadCommand("thread.refresh", RefreshContainer{}, "Refresh the active container", []string{"refresh", "container refresh"}),
		threadCommand("thread.container-start", StartContainer{}, "Start the active container and keep it running", []string{"container start"}),
		threadCommand("thread.container-stop", StopContainer{}, "Stop the active container and disable keep-running", []string{"container stop"}),
		threadCommand("thread.purge", PurgeChat{}, "Purge the active chat state", []string{"purge", "chat purge"}),
		threadCommand("thread.interrupt", InterruptTurn{}, "Interrupt the active turn", []string{"interrupt"}),
		threadCommand("thread.upgrade", Upgrade{}, "Upgrade ctgbot", []string{"upgrade"}),
		threadCommand("thread.quit", Quit{}, "Quit ctgbot", []string{"quit"}),
		threadCommand("thread.status", Status{}, "Show active conversation status", []string{"status"}),
	}
}

func threadCommand(id string, command any, help string, patterns []string) commandengine.Definition {
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
		Sources: allSources(),
		Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser),
		Routes:  routes,
	}
}
