package commandengine

import (
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type Source string

const (
	SourceCLI        Source = "cli"
	SourceMessage    Source = "message"
	SourceHostbridge Source = "hostbridge"
)

type Actor struct {
	ID    string
	Roles []simplerbac.Role
}

type Context struct {
	Source    Source
	Actor     Actor
	ChatID    modeluuid.UUID
	ThreadID  modeluuid.UUID
	SandboxID modeluuid.UUID
}

type Request struct {
	Context      Context
	Command      any
	DefinitionID string
	Route        string
}

type Result struct {
	Text string
}

type BuildFunc func(req *clir.Request) (any, error)

type Route struct {
	Pattern string
	Help    string
	Build   BuildFunc
}

type Definition struct {
	ID      string
	Sources []Source
	Policy  simplerbac.Rule
	Routes  []Route
}

func (d Definition) Validate() error {
	if strings.TrimSpace(d.ID) == "" {
		return fmt.Errorf("missing command definition id")
	}
	if len(d.Sources) == 0 {
		return fmt.Errorf("command definition %s has no sources", d.ID)
	}
	if len(d.Routes) == 0 {
		return fmt.Errorf("command definition %s has no routes", d.ID)
	}
	for _, route := range d.Routes {
		if NormalizePattern(route.Pattern) == "" {
			return fmt.Errorf("command definition %s has an empty route pattern", d.ID)
		}
		if route.Build == nil {
			return fmt.Errorf("command definition %s route %q has no builder", d.ID, route.Pattern)
		}
	}
	return nil
}

func (d Definition) AllowsSource(source Source) bool {
	for _, candidate := range d.Sources {
		if candidate == source {
			return true
		}
	}
	return false
}

func NormalizePattern(pattern string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(pattern)), " ")
}
