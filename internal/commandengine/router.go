package commandengine

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/bartdeboer/go-clir"
)

type Router struct {
	source                Source
	clir                  *clir.Router
	definitionsByPattern  map[string]Definition
	descriptionsByPattern map[string]Description
}

type parseState struct {
	Request Request
}

type parseStateKey struct{}

func NewRouter(definitions []Definition, source Source, descriptions ...Description) (*Router, error) {
	if source == "" {
		return nil, fmt.Errorf("missing command source")
	}
	router := &Router{
		source:                source,
		clir:                  clir.New(),
		definitionsByPattern:  map[string]Definition{},
		descriptionsByPattern: map[string]Description{},
	}
	router.clir.SetHelpEntryFormatter(func(w io.Writer, routes []clir.RouteInfo) {
		writeHelpCompactLines(w, routes, source)
	})
	seenRoutes := map[string]string{}
	var routes []registeredRoute
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			return nil, err
		}
		if !definition.AllowsSource(source) {
			continue
		}
		canonicalPattern := definition.CanonicalPattern()
		if _, exists := router.definitionsByPattern[canonicalPattern]; exists {
			return nil, fmt.Errorf("duplicate command pattern: %s", canonicalPattern)
		}
		router.definitionsByPattern[canonicalPattern] = definition
		for _, route := range definition.Routes() {
			pattern := NormalizePattern(route.Pattern)
			if previous, ok := seenRoutes[pattern]; ok {
				return nil, fmt.Errorf("duplicate command route %q in %s and %s", pattern, previous, canonicalPattern)
			}
			seenRoutes[pattern] = canonicalPattern
			routes = append(routes, registeredRoute{
				definition: definition,
				route:      route,
				pattern:    pattern,
			})
		}
	}
	var describedRoutes []registeredDescription
	for _, description := range descriptions {
		if err := description.Validate(); err != nil {
			return nil, err
		}
		if !description.AllowsSource(source) {
			continue
		}
		pattern := NormalizePattern(description.Pattern)
		if previous, ok := seenRoutes[pattern]; ok {
			return nil, fmt.Errorf("duplicate command route %q in %s and description", pattern, previous)
		}
		seenRoutes[pattern] = "description"
		router.descriptionsByPattern[pattern] = description
		describedRoutes = append(describedRoutes, registeredDescription{description: description, pattern: pattern})
	}

	router.clir.Routes(func(b *clir.Builder) {
		for _, described := range describedRoutes {
			b.Describe(described.pattern, described.description.Help, clirDescriptionOptions(described.description)...)
		}
		for _, registered := range routes {
			registered := registered
			b.Handle(registered.pattern, registered.definition.Help, func(req *clir.Request) error {
				state, ok := req.Context().Value(parseStateKey{}).(*parseState)
				if !ok || state == nil {
					return fmt.Errorf("missing command parse state")
				}
				command, err := registered.definition.Build(req)
				if err != nil {
					return err
				}
				if command == nil {
					return fmt.Errorf("command route %q built nil command", registered.pattern)
				}
				state.Request.Context.Source = source
				state.Request.Command = command
				state.Request.CanonicalPattern = registered.definition.CanonicalPattern()
				state.Request.Route = registered.pattern
				return nil
			}, clirRouteOptions(registered.definition, registered.route)...)
		}
	})
	return router, nil
}

func clirRouteOptions(definition Definition, route Route) []clir.RouteOption {
	var opts []clir.RouteOption
	if route.Hidden {
		opts = append(opts, clir.Hidden())
	}
	switch definition.InstructionVisibilityOrDefault() {
	case InstructionHidden:
		opts = append(opts, clir.Tag(string(InstructionHidden)))
	case InstructionDiscoverable:
		opts = append(opts, clir.Tag(string(InstructionDiscoverable)))
	case InstructionImportant:
		opts = append(opts, clir.Tag(string(InstructionImportant)))
	case InstructionEssential:
		opts = append(opts, clir.Tag(string(InstructionEssential)))
	}
	return opts
}

func clirDescriptionOptions(description Description) []clir.RouteOption {
	var opts []clir.RouteOption
	if description.Hidden {
		opts = append(opts, clir.Hidden())
	}
	return opts
}

type registeredRoute struct {
	definition Definition
	route      Route
	pattern    string
}

type registeredDescription struct {
	description Description
	pattern     string
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

// Match resolves argv to clir's best route without executing the command builder.
func (r *Router) Match(ctx context.Context, argv []string) (RouteMatch, error) {
	if r == nil || r.clir == nil {
		return RouteMatch{}, fmt.Errorf("missing command router")
	}
	resolution, err := r.clir.Resolve(ctx, argv)
	if err != nil {
		// Match is an inspection helper. A missing route is represented as
		// Matched=false so callers can decide whether to render help, fall back,
		// or report the parse error themselves.
		return RouteMatch{}, nil
	}
	return RouteMatch{
		Matched:    true,
		Executable: resolution.Executable,
		Exact:      resolution.Exact,
	}, nil
}

func (r *Router) FPrintHelp(ctx context.Context, w io.Writer, argv []string, actors ...Actor) error {
	return r.FPrintHelpWithOptions(ctx, w, argv, nil, actors...)
}

type HelpOption = clir.HelpOption

func HelpDepth(n int) HelpOption {
	return clir.Depth(n)
}

func HelpLitDepth(n int) HelpOption {
	return clir.LitDepth(n)
}

func HelpIncludeTags(tags ...string) HelpOption {
	return clir.IncludeTags(tags...)
}

func (r *Router) FPrintHelpWithOptions(ctx context.Context, w io.Writer, argv []string, opts []HelpOption, actors ...Actor) error {
	if r == nil || r.clir == nil {
		return fmt.Errorf("missing command router")
	}
	helpOptions := r.helpOptionsForActor(opts, actors...)
	return r.clir.FPrintHelp(ctx, w, argv, helpOptions...)
}

func (r *Router) FPrintHelpIndex(ctx context.Context, w io.Writer, actors ...Actor) error {
	_ = ctx
	if r == nil || r.clir == nil {
		return fmt.Errorf("missing command router")
	}
	routes := r.HelpRoutes(nil, nil, actors...)
	writeHelpCompactLines(w, routes, r.source)
	return nil
}

func (r *Router) HelpRoutes(scope []string, opts []HelpOption, actors ...Actor) []clir.RouteInfo {
	if r == nil || r.clir == nil {
		return nil
	}
	return r.clir.HelpRoutes(scope, r.helpOptionsForActor(opts, actors...)...)
}

func (r *Router) helpOptionsForActor(opts []HelpOption, actors ...Actor) []clir.FilterOption {
	helpOptions := append([]clir.FilterOption(nil), opts...)
	if len(actors) == 0 {
		return helpOptions
	}
	helpOptions = append(helpOptions, clir.FilterHelp(r.helpAllowedForActor(actors[0])))
	return helpOptions
}

func (r *Router) helpAllowedForActor(actor Actor) func(clir.RouteInfo) bool {
	return func(info clir.RouteInfo) bool {
		pattern := NormalizePattern(info.Pattern)
		if pattern == "" {
			return false
		}
		for _, description := range r.descriptionsByPattern {
			if description.Hidden {
				continue
			}
			if err := description.Policy.Check(actor); err != nil {
				continue
			}
			if helpPatternCoversRoute(pattern, description.Pattern) {
				return true
			}
		}
		for _, definition := range r.definitionsByPattern {
			if err := definition.Policy.Check(actor); err != nil {
				continue
			}
			for _, route := range definition.Routes() {
				if route.Hidden {
					continue
				}
				if helpPatternCoversRoute(pattern, route.Pattern) {
					return true
				}
			}
		}
		return false
	}
}

func helpPatternCoversRoute(helpPattern string, routePattern string) bool {
	helpParts := strings.Fields(NormalizePattern(helpPattern))
	routeParts := strings.Fields(NormalizePattern(routePattern))
	if len(helpParts) == 0 || len(routeParts) < len(helpParts) {
		return false
	}
	for i, part := range helpParts {
		if routeParts[i] != part {
			return false
		}
	}
	return true
}

func (r *Router) Descriptions() []Description {
	if r == nil {
		return nil
	}
	out := make([]Description, 0, len(r.descriptionsByPattern))
	for _, description := range r.descriptionsByPattern {
		out = append(out, description)
	}
	sort.Slice(out, func(i, j int) bool {
		return NormalizePattern(out[i].Pattern) < NormalizePattern(out[j].Pattern)
	})
	return out
}

func (r *Router) Definitions() []Definition {
	if r == nil {
		return nil
	}
	out := make([]Definition, 0, len(r.definitionsByPattern))
	for _, definition := range r.definitionsByPattern {
		out = append(out, definition)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CanonicalPattern() < out[j].CanonicalPattern()
	})
	return out
}

func (r *Router) Authorize(req Request) error {
	if r == nil || r.clir == nil {
		return fmt.Errorf("missing command router")
	}
	definition, ok := r.definitionForPattern(req.CanonicalPattern)
	if !ok {
		return fmt.Errorf("missing command pattern: %s", req.CanonicalPattern)
	}
	if !definition.AllowsSource(req.Context.Source) {
		return fmt.Errorf("command %s unavailable from source %s", definition.CanonicalPattern(), req.Context.Source)
	}
	if err := definition.Policy.Check(req.Context.Actor); err != nil {
		return fmt.Errorf("command %s denied: %w", definition.CanonicalPattern(), err)
	}
	return nil
}

func (r *Router) definitionForPattern(pattern string) (Definition, bool) {
	if r == nil || r.clir == nil {
		return Definition{}, false
	}
	definition, ok := r.definitionsByPattern[pattern]
	return definition, ok
}
