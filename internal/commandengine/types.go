package commandengine

import (
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type Source string

const (
	SourceCLI        Source = "cli"
	SourceMessage    Source = "message"
	SourceHostbridge Source = "hostbridge"
	// SourceRemoteHostbridge is reserved for explicitly authenticated remote
	// hostbridgev2 callers. It intentionally does not inherit SourceHostbridge
	// policy; commands must opt into the remote surface deliberately.
	SourceRemoteHostbridge Source = "remote_hostbridge"
	SourceScheduler        Source = "scheduler"
)

type Actor = coremodel.Actor

type Context struct {
	Source    Source
	Actor     Actor
	ChatID    modeluuid.UUID
	ThreadID  modeluuid.UUID
	SandboxID modeluuid.UUID
}

type Request struct {
	Context          Context
	Command          any
	CanonicalPattern string
	Route            string
}

// RouteMatch describes clir's best route match without executing a command builder.
type RouteMatch struct {
	Matched    bool
	Executable bool
	Exact      bool
}

// HelpRequest is commandengine's wrapper around clir's trailing help token
// convention. ctgbot decides how to handle help requests explicitly; clir only
// provides the token detection and scope stripping.
type HelpRequest struct {
	Scope []string
}

type Result struct {
	Text string
}

type BuildFunc func(req *clir.Request) (any, error)

type Route struct {
	Pattern  string
	Absolute bool
	Hidden   bool
}

type InstructionVisibility string

const (
	InstructionHidden       InstructionVisibility = "hidden"
	InstructionDiscoverable InstructionVisibility = "discoverable"
	InstructionImportant    InstructionVisibility = "important"
	InstructionEssential    InstructionVisibility = "essential"
)

type Definition struct {
	Pattern               string
	Help                  string
	Build                 BuildFunc
	Absolute              bool
	Hidden                bool
	Sources               []Source
	Policy                simplerbac.Rule
	Aliases               []Route
	InstructionVisibility InstructionVisibility
}

func (d Definition) Validate() error {
	if NormalizePattern(d.Pattern) == "" {
		return fmt.Errorf("missing command definition pattern")
	}
	if d.Build == nil {
		return fmt.Errorf("command definition %s has no builder", d.CanonicalPattern())
	}
	if len(d.Sources) == 0 {
		return fmt.Errorf("command definition %s has no sources", d.CanonicalPattern())
	}
	for _, route := range d.Aliases {
		if NormalizePattern(route.Pattern) == "" {
			return fmt.Errorf("command definition %s has an empty alias pattern", d.CanonicalPattern())
		}
	}
	switch d.InstructionVisibilityOrDefault() {
	case InstructionHidden, InstructionDiscoverable, InstructionImportant, InstructionEssential:
	default:
		return fmt.Errorf("command definition %s has invalid instruction visibility: %s", d.CanonicalPattern(), d.InstructionVisibility)
	}
	return nil
}

func (d Definition) CanonicalPattern() string {
	return NormalizePattern(d.Pattern)
}

func (d Definition) Routes() []Route {
	routes := make([]Route, 0, len(d.Aliases)+1)
	routes = append(routes, Route{
		Pattern:  d.Pattern,
		Absolute: d.Absolute,
		Hidden:   d.Hidden,
	})
	routes = append(routes, d.Aliases...)
	return routes
}

func (d Definition) AllowsSource(source Source) bool {
	for _, candidate := range d.Sources {
		if candidate == source {
			return true
		}
	}
	return false
}

func (d Definition) InstructionVisibilityOrDefault() InstructionVisibility {
	if d.InstructionVisibility == "" {
		return InstructionDiscoverable
	}
	return d.InstructionVisibility
}

// ParseHelpRequest parses clir's trailing help token convention.
func ParseHelpRequest(args []string) (HelpRequest, bool) {
	if !clir.IsHelpRequest(args) {
		return HelpRequest{}, false
	}
	return HelpRequest{
		Scope: clir.StripHelpToken(args),
	}, true
}

func NormalizePattern(pattern string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(pattern)), " ")
}

func JoinPattern(prefix string, pattern string) string {
	prefix = NormalizePattern(prefix)
	pattern = NormalizePattern(pattern)
	switch {
	case prefix == "":
		return pattern
	case pattern == "":
		return prefix
	default:
		return NormalizePattern(prefix + " " + pattern)
	}
}
