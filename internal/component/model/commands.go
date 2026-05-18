package model

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

type listCommand struct{}

type installCommand struct {
	Name        string
	URL         string
	Path        string
	Mode        component.ModelMode
	Filename    string
	SHA256      string
	HostPort    int
	ContextSize int
	UBatchSize  int
	GPULayers   int
	Pooling     string
	Normalize   bool
	Default     bool
}

func RegisterGobTypes(register func(any)) {
	register(listCommand{})
	register(installCommand{})
}

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		modelCommand("list", listCommand{}, "List installed AI models", nil),
		modelCommand("install <name> <url>", nil, "Download and register an AI model", buildInstallCommand),
		modelCommand("register <name> <path>", nil, "Register an existing local AI model", buildRegisterCommand),
	}
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := commandengine.RegisterPattern[listCommand](registry, "list", func(ctx context.Context, req commandengine.Request, cmd listCommand) (commandengine.Result, error) {
		_, _ = req, cmd
		return c.handleList(ctx)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[installCommand](registry, "install <name> <url>", func(ctx context.Context, req commandengine.Request, cmd installCommand) (commandengine.Result, error) {
		_, _ = req, cmd
		return c.handleInstall(ctx, cmd)
	}); err != nil {
		return err
	}
	return commandengine.RegisterPattern[installCommand](registry, "register <name> <path>", func(ctx context.Context, req commandengine.Request, cmd installCommand) (commandengine.Result, error) {
		_, _ = req, cmd
		return c.handleRegister(ctx, cmd)
	})
}

func buildInstallCommand(req *clir.Request) (any, error) {
	cmd, err := buildModelCommand(req, "model install")
	if err != nil {
		return nil, err
	}
	cmd.URL = strings.TrimSpace(req.Params["url"])
	if cmd.URL == "" {
		return nil, fmt.Errorf("missing model url")
	}
	return cmd, nil
}

func buildRegisterCommand(req *clir.Request) (any, error) {
	cmd, err := buildModelCommand(req, "model register")
	if err != nil {
		return nil, err
	}
	cmd.Path = strings.TrimSpace(req.Params["path"])
	if cmd.Path == "" {
		return nil, fmt.Errorf("missing model path")
	}
	return cmd, nil
}

func buildModelCommand(req *clir.Request, name string) (installCommand, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	filename := fs.String("filename", "", "Downloaded model filename")
	sha := fs.String("sha256", "", "Expected model sha256")
	hostPort := fs.Int("host-port", 0, "Host port for this model service")
	ctxSize := fs.Int("ctx-size", 0, "llama.cpp context size")
	gpuLayers := fs.Int("gpu-layers", 0, "llama.cpp GPU layers")
	modeFlag := fs.String("mode", "", "Model mode: completion, embedding, asr, tts")
	embedding := fs.Bool("embedding", false, "Register this model for embedding mode")
	asr := fs.Bool("asr", false, "Register this model for ASR/transcription mode")
	tts := fs.Bool("tts", false, "Register this model for TTS/speech synthesis mode")
	pooling := fs.String("pooling", "", "llama.cpp embedding pooling mode")
	ubatch := fs.Int("ubatch-size", 0, "llama.cpp physical batch size")
	normalize := fs.Bool("normalize", true, "L2-normalize embedding vectors client-side")
	makeDefault := fs.Bool("default", false, "Use this model as the store default")
	if err := fs.Parse(req.Extra); err != nil {
		return installCommand{}, err
	}
	if len(fs.Args()) > 0 {
		return installCommand{}, fmt.Errorf("unexpected model arguments: %s", strings.Join(fs.Args(), " "))
	}
	modelName := cleanModelName(req.Params["name"])
	if modelName == "" {
		return installCommand{}, fmt.Errorf("missing model name")
	}
	mode := component.ModelModeCompletion
	if *embedding {
		mode = component.ModelModeEmbedding
	}
	if *asr {
		mode = component.ModelModeASR
	}
	if *tts {
		mode = component.ModelModeTTS
	}
	if strings.TrimSpace(*modeFlag) != "" && parseModelMode(*modeFlag) == "" {
		return installCommand{}, fmt.Errorf("unsupported model mode: %s", *modeFlag)
	}
	if parsed := parseModelMode(*modeFlag); parsed != "" {
		mode = parsed
	}
	return installCommand{Name: modelName, Mode: mode, Filename: *filename, SHA256: *sha, HostPort: *hostPort, ContextSize: *ctxSize, UBatchSize: *ubatch, GPULayers: *gpuLayers, Pooling: *pooling, Normalize: *normalize, Default: *makeDefault}, nil
}

func parseModelMode(mode string) component.ModelMode {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "embedding", "embed":
		return component.ModelModeEmbedding
	case "asr", "transcription", "transcribe", "speech-to-text", "stt":
		return component.ModelModeASR
	case "tts", "speech", "synthesis", "speech-synthesis", "text-to-speech":
		return component.ModelModeTTS
	case "completion", "complete", "chat", "llm":
		return component.ModelModeCompletion
	default:
		return ""
	}
}

func (c *Component) handleList(ctx context.Context) (commandengine.Result, error) {
	models, err := c.ListModels(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	var lines []string
	lines = append(lines, "models")
	if strings.TrimSpace(c.registry.DefaultModel) != "" {
		lines = append(lines, "default_model: "+c.registry.DefaultModel)
	}
	for _, mode := range []component.ModelMode{component.ModelModeCompletion, component.ModelModeEmbedding, component.ModelModeASR, component.ModelModeTTS} {
		if name := c.defaultModelForMode(mode); name != "" {
			lines = append(lines, fmt.Sprintf("default_%s_model: %s", mode, name))
		}
	}
	if len(models) == 0 {
		lines = append(lines, "(no models installed)")
		return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
	}
	for _, model := range models {
		var suffixes []string
		if model.Name == c.registry.DefaultModel {
			suffixes = append(suffixes, "legacy_default=true")
		}
		if defaultMode := defaultModeForModel(c, model.Name); defaultMode != "" {
			suffixes = append(suffixes, "default_"+defaultMode+"=true")
		}
		suffix := ""
		if len(suffixes) > 0 {
			suffix = " " + strings.Join(suffixes, " ")
		}
		lines = append(lines, fmt.Sprintf("- %s%s mode=%s path=%s port=%d", model.Name, suffix, model.Mode, model.Path, model.HostPort))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func defaultModeForModel(c *Component, modelName string) string {
	if c == nil {
		return ""
	}
	for _, mode := range []component.ModelMode{component.ModelModeCompletion, component.ModelModeEmbedding, component.ModelModeASR, component.ModelModeTTS} {
		if c.defaultModelForMode(mode) == modelName {
			return string(mode)
		}
	}
	return ""
}

func (c *Component) handleInstall(ctx context.Context, cmd installCommand) (commandengine.Result, error) {
	model, err := c.InstallModel(ctx, installRequest(cmd))
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("model installed: %s\npath: %s", model.Name, model.Path)}, nil
}

func (c *Component) handleRegister(ctx context.Context, cmd installCommand) (commandengine.Result, error) {
	model, err := c.RegisterModel(ctx, installRequest(cmd))
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("model registered: %s\npath: %s", model.Name, model.Path)}, nil
}

func installRequest(cmd installCommand) component.ModelInstallRequest {
	return component.ModelInstallRequest{
		Model: component.Model{
			Name:        cmd.Name,
			URL:         cmd.URL,
			Filename:    cmd.Filename,
			Path:        cmd.Path,
			Mode:        cmd.Mode,
			SHA256:      cmd.SHA256,
			HostPort:    cmd.HostPort,
			ContextSize: cmd.ContextSize,
			UBatchSize:  cmd.UBatchSize,
			GPULayers:   cmd.GPULayers,
			Pooling:     cmd.Pooling,
			Normalize:   cmd.Normalize,
		},
		Default: cmd.Default,
	}
}

func modelCommand(pattern string, command any, help string, build func(req *clir.Request) (any, error)) commandengine.Definition {
	if build == nil {
		build = func(req *clir.Request) (any, error) { _ = req; return command, nil }
	}
	return commandengine.Definition{
		Pattern: pattern,
		Help:    help,
		Build:   build,
		Sources: []commandengine.Source{commandengine.SourceCLI, commandengine.SourceMessage, commandengine.SourceHostbridge},
		Policy:  modelCommandPolicy(pattern),
	}
}

func modelCommandPolicy(pattern string) simplerbac.Rule {
	normalized := commandengine.NormalizePattern(pattern)
	if normalized == "install <name> <url>" || normalized == "register <name> <path>" {
		return simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent)
	}
	return simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser)
}
