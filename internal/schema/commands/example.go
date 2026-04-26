package commands

import (
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type Echo struct {
	Text string
}

func ExampleCommands() []commandengine.Definition {
	return []commandengine.Definition{
		{
			ID: "example.echo",
			Sources: []commandengine.Source{
				commandengine.SourceCLI,
				commandengine.SourceMessage,
				commandengine.SourceHostbridge,
			},
			Policy: simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser),
			Routes: []commandengine.Route{
				{
					Pattern: "echo <text>",
					Help:    "Echo text",
					Build:   buildEcho,
				},
			},
		},
	}
}

func RegisterGobTypes(register func(any)) {
	register(Echo{})
	register(ConfigList{})
	register(ConfigGet{})
	register(ConfigSet{})
	register(ConfigHostbridgeScaffold{})
	register(RunCommand{})
	register(SendMedia{})
	register(RefreshContainer{})
	register(PurgeChat{})
	register(InterruptTurn{})
	register(Upgrade{})
	register(Quit{})
	register(Stop{})
	register(Status{})
}

func buildEcho(req *clir.Request) (any, error) {
	text := strings.TrimSpace(req.Params["text"])
	if text == "" {
		return nil, fmt.Errorf("missing echo text")
	}
	return Echo{Text: text}, nil
}
