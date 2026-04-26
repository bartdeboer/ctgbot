package routers

import (
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configengine"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

func MessageDefinitions() []commandengine.Definition {
	out := append([]commandengine.Definition{}, schemacommands.ConfigCommands()...)
	out = append(out, schemacommands.ThreadCommands()...)
	return definitionsForSource(out, commandengine.SourceMessage)
}

func NewMessageRouter() (*commandengine.Router, error) {
	return commandengine.NewRouter(MessageDefinitions(), commandengine.SourceMessage)
}

func NewMessageCommandEngine(manager *configengine.Manager, handlers ThreadHandlers, configHandlers ...ConfigHandlers) (*commandengine.Engine, error) {
	router, err := NewMessageRouter()
	if err != nil {
		return nil, err
	}
	registry := commandengine.NewRegistry()
	if err := RegisterConfigHandlers(registry, manager, configHandlers...); err != nil {
		return nil, err
	}
	if err := RegisterThreadHandlers(registry, handlers); err != nil {
		return nil, err
	}
	return commandengine.NewEngine(router, registry), nil
}
