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
		threadCommand(RefreshContainer{}, "Delete and recreate the container on next turn", "refresh", "container refresh"),
		threadCommand(StartContainer{}, "Start the active container and keep it running", "container start"),
		threadCommand(StopContainer{}, "Stop the container but keep its data", "container stop"),
		threadCommand(PurgeChat{}, "Reset the conversation and delete the container", "purge", "chat purge"),
		threadCommand(InterruptTurn{}, "Interrupt the active turn", "interrupt"),
		threadCommand(Install{}, "Install ctgbot binaries from source", "install"),
		threadCommand(Upgrade{}, "Upgrade ctgbot", "upgrade"),
		threadCommand(Quit{}, "Restart ctgbot", "quit"),
		threadCommand(Status{}, "Show conversation and container status", "status"),
		threadCommand(ModelStatus{}, "Show the Codex model for this thread", "model"),
		threadCommand(ModelList{}, "List suggested Codex models", "model list"),
		{
			Pattern: "model set <model>",
			Help:    "Set the Codex model for this thread",
			Build: func(req *clir.Request) (any, error) {
				model := strings.TrimSpace(req.Params["model"])
				if model == "" {
					return nil, fmt.Errorf("missing model")
				}
				return ModelSet{Model: model}, nil
			},
			Sources: allSources(),
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser),
		},
		threadCommand(ModelClear{}, "Clear the thread model override", "model clear"),
		threadCommand(ModelEffortStatus{}, "Show the Codex reasoning effort for this thread", "model effort"),
		threadCommand(ModelEffortList{}, "List suggested Codex reasoning efforts", "model effort list"),
		{
			Pattern: "model effort set <effort>",
			Help:    "Set the Codex reasoning effort for this thread",
			Build: func(req *clir.Request) (any, error) {
				effort := strings.TrimSpace(req.Params["effort"])
				if effort == "" {
					return nil, fmt.Errorf("missing reasoning effort")
				}
				return ModelEffortSet{Effort: effort}, nil
			},
			Sources: allSources(),
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser),
		},
		threadCommand(ModelEffortClear{}, "Clear the thread reasoning effort override", "model effort clear"),
	}
}

func threadCommand(command any, help string, patterns ...string) commandengine.Definition {
	aliases := make([]commandengine.Route, 0)
	for _, pattern := range patterns[1:] {
		aliases = append(aliases, commandengine.Route{Pattern: pattern})
	}
	return commandengine.Definition{
		Pattern: patterns[0],
		Help:    help,
		Build: func(req *clir.Request) (any, error) {
			_ = req
			return command, nil
		},
		Sources: allSources(),
		Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser),
		Aliases: aliases,
	}
}
