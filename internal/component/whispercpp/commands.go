package whispercpp

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type transcribeCommand struct {
	Media    message.Media
	Model    string
	Language string
}

func RegisterGobTypes(register func(any)) {
	register(transcribeCommand{})
}

func commandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{{
		Pattern: "transcribe <path>",
		Help:    "Transcribe an audio file using whisper.cpp",
		Sources: []commandengine.Source{commandengine.SourceCLI, commandengine.SourceHostbridge},
		Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
		Build:   buildTranscribeCommand,
	}}
}

func buildTranscribeCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("whispercpp transcribe", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	contentType := fs.String("type", "", "Audio content type")
	model := fs.String("model", "", "Model name from the model component")
	language := fs.String("language", "", "Whisper language hint")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	if len(fs.Args()) > 0 {
		return nil, fmt.Errorf("unexpected transcribe arguments: %s", strings.Join(fs.Args(), " "))
	}
	path := strings.TrimSpace(req.Params["path"])
	if path == "" {
		return nil, fmt.Errorf("missing audio path")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return transcribeCommand{
		Media: message.Media{
			Filename:    filepath.Base(path),
			ContentType: strings.TrimSpace(*contentType),
			Content:     append([]byte(nil), content...),
		},
		Model:    strings.TrimSpace(*model),
		Language: strings.TrimSpace(*language),
	}, nil
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	return commandengine.RegisterPattern[transcribeCommand](registry, "transcribe <path>", c.handleTranscribe)
}

func (c *Component) handleTranscribe(ctx context.Context, req commandengine.Request, cmd transcribeCommand) (commandengine.Result, error) {
	threadID := req.Context.ThreadID
	if threadID.IsNull() {
		threadID = req.Context.SandboxID
	}
	result, err := c.Transcribe(ctx, component.TranscriptionRequest{
		Media:    cmd.Media,
		Model:    cmd.Model,
		Language: cmd.Language,
		ThreadID: threadID,
	})
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: strings.TrimSpace(result.Text)}, nil
}
