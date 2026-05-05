package commandengine

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type Router struct {
	source      Source
	clir        *clir.Router
	definitions map[string]Definition
}

type parseState struct {
	Request Request
}

type parseStateKey struct{}

func NewRouter(definitions []Definition, source Source) (*Router, error) {
	if source == "" {
		return nil, fmt.Errorf("missing command source")
	}
	router := &Router{
		source:      source,
		clir:        clir.New(),
		definitions: map[string]Definition{},
	}
	seenRoutes := map[string]string{}
	var routes []registeredRoute
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			return nil, err
		}
		if !definition.AllowsSource(source) {
			continue
		}
		if _, exists := router.definitions[definition.ID]; exists {
			return nil, fmt.Errorf("duplicate command definition: %s", definition.ID)
		}
		router.definitions[definition.ID] = definition
		for _, route := range definition.Routes {
			pattern := NormalizePattern(route.Pattern)
			if previous, ok := seenRoutes[pattern]; ok {
				return nil, fmt.Errorf("duplicate command route %q in %s and %s", pattern, previous, definition.ID)
			}
			seenRoutes[pattern] = definition.ID
			routes = append(routes, registeredRoute{
				definition: definition,
				route:      route,
				pattern:    pattern,
			})
		}
	}
	router.clir.Routes(func(b *clir.Builder) {
		for _, registered := range routes {
			registered := registered
			b.Handle(registered.pattern, registered.route.Help, func(req *clir.Request) error {
				state, ok := req.Context().Value(parseStateKey{}).(*parseState)
				if !ok || state == nil {
					return fmt.Errorf("missing command parse state")
				}
				command, err := registered.route.Build(req)
				if err != nil {
					return err
				}
				if command == nil {
					return fmt.Errorf("command route %q built nil command", registered.pattern)
				}
				state.Request.Context.Source = source
				state.Request.Command = command
				state.Request.DefinitionID = registered.definition.ID
				state.Request.Route = registered.pattern
				return nil
			})
		}
	})
	return router, nil
}

type registeredRoute struct {
	definition Definition
	route      Route
	pattern    string
}

func (r *Router) Parse(ctx context.Context, base Request, argv []string) (Request, error) {
	if r == nil || r.clir == nil {
		return Request{}, fmt.Errorf("missing command router")
	}
	state := &parseState{Request: base}
	parseCtx := context.WithValue(ctx, parseStateKey{}, state)
	if err := r.clir.Run(parseCtx, argv); err != nil {
		return Request{}, err
	}
	if state.Request.Command == nil {
		return Request{}, fmt.Errorf("missing command")
	}
	if err := r.Authorize(state.Request); err != nil {
		return Request{}, err
	}
	return state.Request, nil
}

func (r *Router) Authorize(req Request) error {
	if r == nil || r.clir == nil {
		return fmt.Errorf("missing command router")
	}
	definition, ok := r.definitionFor(req.DefinitionID)
	if !ok {
		return fmt.Errorf("missing command definition: %s", req.DefinitionID)
	}
	if !definition.AllowsSource(req.Context.Source) {
		return fmt.Errorf("command %s unavailable from source %s", definition.ID, req.Context.Source)
	}
	actor := simplerbac.Actor{Roles: req.Context.Actor.Roles}
	if err := definition.Policy.Check(actor); err != nil {
		return fmt.Errorf("command %s denied: %w", definition.ID, err)
	}
	return nil
}

func (r *Router) definitionFor(id string) (Definition, bool) {
	if r == nil || r.clir == nil {
		return Definition{}, false
	}
	definition, ok := r.definitions[id]
	return definition, ok
}

func (r *Router) Definitions() []Definition {
	if r == nil {
		return nil
	}
	out := make([]Definition, 0, len(r.definitions))
	for _, definition := range r.definitions {
		out = append(out, definition)
	}
	return out
}
