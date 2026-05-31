package app

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
)

// ControllerCommandEngine exposes the local operational command surface to
// trusted ctgbot controllers. V1 intentionally mirrors the CLI surface: trust is
// directional and controlled by the node's trusted_controllers table. Per-command
// scopes can narrow this later without changing the transport contract.
func (s *service) ControllerCommandEngine(ctx context.Context) (*commandengine.Engine, error) {
	surfaces, err := s.CLICommandSurfaces(ctx)
	if err != nil {
		return nil, err
	}
	if len(surfaces) == 0 {
		return nil, fmt.Errorf("missing controller command surfaces")
	}
	definitions := commandset.DefinitionsForSource(commandengine.SourceCLI, surfaces...)
	for i := range definitions {
		definitions[i].Sources = []commandengine.Source{commandengine.SourceController}
	}
	if len(definitions) == 0 {
		return nil, fmt.Errorf("no controller command definitions")
	}
	router, err := commandengine.NewRouter(definitions, commandengine.SourceController)
	if err != nil {
		return nil, err
	}
	registry := commandengine.NewRegistry()
	for _, surface := range surfaces {
		if surface == nil {
			continue
		}
		if err := surface.RegisterCommandHandlers(registry); err != nil {
			return nil, err
		}
	}
	return commandengine.NewEngine(router, registry), nil
}
