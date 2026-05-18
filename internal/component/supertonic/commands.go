package supertonic

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type speakCommand struct {
	Text     string
	Model    string
	Voice    string
	Language string
}

func RegisterGobTypes(register func(any)) {
	register(speakCommand{})
}

func commandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{{
		Pattern: "speak <text>",
		Help:    "Synthesize speech using Supertonic",
		Sources: []commandengine.Source{commandengine.SourceCLI, commandengine.SourceHostbridge},
		Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
		Build:   buildSpeakCommand,
	}}
}

func buildSpeakCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("supertonic speak", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	model := fs.String("model", "", "Model name from the model component")
	voice := fs.String("voice", "", "Supertonic voice style, such as F1 or M1")
	language := fs.String("language", "", "Language code, such as en, nl, or ru")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	if len(fs.Args()) > 0 {
		return nil, fmt.Errorf("unexpected speak arguments: %s", strings.Join(fs.Args(), " "))
	}
	text := strings.TrimSpace(req.Params["text"])
	if text == "" {
		return nil, fmt.Errorf("missing speech text")
	}
	return speakCommand{Text: text, Model: strings.TrimSpace(*model), Voice: strings.TrimSpace(*voice), Language: strings.TrimSpace(*language)}, nil
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	return commandengine.RegisterPattern[speakCommand](registry, "speak <text>", c.handleSpeak)
}

func (c *Component) handleSpeak(ctx context.Context, req commandengine.Request, cmd speakCommand) (commandengine.Result, error) {
	threadID := req.Context.ThreadID
	if threadID.IsNull() {
		threadID = req.Context.SandboxID
	}
	result, err := c.Synthesize(ctx, component.SpeechRequest{
		Text:     cmd.Text,
		Model:    cmd.Model,
		Voice:    cmd.Voice,
		Language: cmd.Language,
		ThreadID: threadID,
	})
	if err != nil {
		return commandengine.Result{}, err
	}
	lines := []string{
		"speech synthesized",
		fmt.Sprintf("bytes: %d", len(result.Media.Content)),
	}
	if result.DurationSeconds > 0 {
		lines = append(lines, fmt.Sprintf("duration_seconds: %.2f", result.DurationSeconds))
	}
	if result.SynthesisSeconds > 0 {
		lines = append(lines, fmt.Sprintf("synthesis_seconds: %.2f", result.SynthesisSeconds))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}
