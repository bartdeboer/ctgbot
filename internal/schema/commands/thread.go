package commands

import (
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type RefreshContainer struct{}
type StartContainer struct{}
type StopContainer struct{}
type PurgeChat struct{}
type InterruptTurn struct{}
type Install struct{}
type Upgrade struct{}
type Quit struct{}
type Status struct{}
type ModelStatus struct{}
type ModelList struct{}
type ModelSet struct {
	Model string
}
type ModelClear struct{}
type ModelEffortStatus struct{}
type ModelEffortList struct{}
type ModelEffortSet struct {
	Effort string
}
type ModelEffortClear struct{}

func ThreadCommands() []commandengine.Definition {
	return []commandengine.Definition{
		threadCommand("thread.refresh", RefreshContainer{}, "Delete and recreate the container on next turn", []string{"refresh", "container refresh"}),
		threadCommand("thread.container-start", StartContainer{}, "Start the active container and keep it running", []string{"container start"}),
		threadCommand("thread.container-stop", StopContainer{}, "Stop the container but keep its data", []string{"container stop"}),
		threadCommand("thread.purge", PurgeChat{}, "Reset the conversation and delete the container", []string{"purge", "chat purge"}),
		threadCommand("thread.interrupt", InterruptTurn{}, "Interrupt the active turn", []string{"interrupt"}),
		threadCommand("thread.install", Install{}, "Install ctgbot binaries from source", []string{"install"}),
		threadCommand("thread.upgrade", Upgrade{}, "Upgrade ctgbot", []string{"upgrade"}),
		threadCommand("thread.quit", Quit{}, "Restart ctgbot", []string{"quit"}),
		threadCommand("thread.status", Status{}, "Show conversation and container status", []string{"status"}),
		threadCommand("thread.model-status", ModelStatus{}, "Show the Codex model for this thread", []string{"model"}),
		threadCommand("thread.model-list", ModelList{}, "List suggested Codex models", []string{"model list"}),
		{
			ID:      "thread.model-set",
			Sources: allSources(),
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser),
			Routes: []commandengine.Route{{
				Pattern: "model set <model>",
				Help:    "Set the Codex model for this thread",
				Build: func(req *clir.Request) (any, error) {
					model := strings.TrimSpace(req.Params["model"])
					if model == "" {
						return nil, fmt.Errorf("missing model")
					}
					return ModelSet{Model: model}, nil
				},
			}},
		},
		threadCommand("thread.model-clear", ModelClear{}, "Clear the thread model override", []string{"model clear"}),
		threadCommand("thread.model-effort-status", ModelEffortStatus{}, "Show the Codex reasoning effort for this thread", []string{"model effort"}),
		threadCommand("thread.model-effort-list", ModelEffortList{}, "List suggested Codex reasoning efforts", []string{"model effort list"}),
		{
			ID:      "thread.model-effort-set",
			Sources: allSources(),
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser),
			Routes: []commandengine.Route{{
				Pattern: "model effort set <effort>",
				Help:    "Set the Codex reasoning effort for this thread",
				Build: func(req *clir.Request) (any, error) {
					effort := strings.TrimSpace(req.Params["effort"])
					if effort == "" {
						return nil, fmt.Errorf("missing reasoning effort")
					}
					return ModelEffortSet{Effort: effort}, nil
				},
			}},
		},
		threadCommand("thread.model-effort-clear", ModelEffortClear{}, "Clear the thread reasoning effort override", []string{"model effort clear"}),
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
