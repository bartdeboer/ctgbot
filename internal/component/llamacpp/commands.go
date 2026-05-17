package llamacpp

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

var _ component.CommandSurface = (*Component)(nil)
var _ component.LocalCommandSurface = (*Component)(nil)

type startCommand struct{}
type stopCommand struct{}
type statusCommand struct{}
type modelListCommand struct{}

type completionCommand struct {
	Prompt string
	Model  string
}

type embedCommand struct {
	Text  string
	Model string
}

type modelInstallCommand struct {
	Name        string
	URL         string
	Path        string
	Mode        string
	Filename    string
	SHA256      string
	HostPort    int
	ContextSize int
	UBatchSize  int
	GPULayers   int
	Pooling     string
	Normalize   *bool
	Default     bool
}

func RegisterGobTypes(register func(any)) {
	register(startCommand{})
	register(stopCommand{})
	register(statusCommand{})
	register(modelListCommand{})
	register(completionCommand{})
	register(embedCommand{})
	register(modelInstallCommand{})
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		llamacppCommand("start", startCommand{}, "Start the default llama.cpp model service", nil),
		llamacppCommand("stop", stopCommand{}, "Stop the default llama.cpp model service", nil),
		llamacppCommand("status", statusCommand{}, "Show default llama.cpp model service status", nil),
		llamacppCommand("completion <prompt>", nil, "Run a completion with the default llama.cpp model", buildCompletionCommand),
		llamacppCommand("embed <text>", nil, "Embed text with the default llama.cpp embedding model", buildEmbedCommand),
		llamacppCommand("model list", modelListCommand{}, "List installed llama.cpp models", nil),
		llamacppCommand("model install <name> <url>", nil, "Download and register a llama.cpp model", buildModelInstallCommand),
		llamacppCommand("model register <name> <path>", nil, "Register an existing local llama.cpp model", buildModelRegisterCommand),
		llamacppCommand("model <model> completion <prompt>", nil, "Run a completion with a specific llama.cpp model", buildModelCompletionCommand),
		llamacppCommand("model <model> embed <text>", nil, "Embed text with a specific llama.cpp embedding model", buildModelEmbedCommand),
	}
}

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := commandengine.RegisterPattern[startCommand](registry, "start", func(ctx context.Context, req commandengine.Request, cmd startCommand) (commandengine.Result, error) {
		_, _ = req, cmd
		return c.start(ctx)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[stopCommand](registry, "stop", func(ctx context.Context, req commandengine.Request, cmd stopCommand) (commandengine.Result, error) {
		_, _ = req, cmd
		return c.stop(ctx)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[statusCommand](registry, "status", func(ctx context.Context, req commandengine.Request, cmd statusCommand) (commandengine.Result, error) {
		_, _ = req, cmd
		return c.status(ctx)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[completionCommand](registry, "completion <prompt>", c.handleCompletionCommand); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[embedCommand](registry, "embed <text>", c.handleEmbedCommand); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[modelListCommand](registry, "model list", func(ctx context.Context, req commandengine.Request, cmd modelListCommand) (commandengine.Result, error) {
		_, _, _ = ctx, req, cmd
		return c.modelList(), nil
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[modelInstallCommand](registry, "model install <name> <url>", func(ctx context.Context, req commandengine.Request, cmd modelInstallCommand) (commandengine.Result, error) {
		_, _ = ctx, req
		return c.modelInstall(cmd)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[modelInstallCommand](registry, "model register <name> <path>", func(ctx context.Context, req commandengine.Request, cmd modelInstallCommand) (commandengine.Result, error) {
		_, _ = ctx, req
		return c.modelRegister(cmd)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[completionCommand](registry, "model <model> completion <prompt>", c.handleCompletionCommand); err != nil {
		return err
	}
	return commandengine.RegisterPattern[embedCommand](registry, "model <model> embed <text>", c.handleEmbedCommand)
}

func (c *Component) start(ctx context.Context) (commandengine.Result, error) {
	runtime, model, err := c.runtimeForModel("")
	if err != nil {
		return commandengine.Result{}, err
	}
	status, err := runtime.Start(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: c.formatStatus("llama.cpp started", model, status)}, nil
}

func (c *Component) stop(ctx context.Context) (commandengine.Result, error) {
	runtime, model, err := c.runtimeForModel("")
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := runtime.Stop(ctx); err != nil {
		return commandengine.Result{}, err
	}
	status, err := runtime.Status(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: c.formatStatus("llama.cpp stopped", model, status)}, nil
}

func (c *Component) status(ctx context.Context) (commandengine.Result, error) {
	runtime, model, err := c.runtimeForModel("")
	if err != nil {
		return commandengine.Result{}, err
	}
	status, err := runtime.Status(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: c.formatStatus("llama.cpp status", model, status)}, nil
}

func (c *Component) formatStatus(title string, model resolvedModel, status runtimepkg.Status) string {
	lines := []string{strings.TrimSpace(title)}
	lines = append(lines,
		"state: "+status.State,
		"container: "+status.Name,
	)
	lines = append(lines,
		"model: "+model.Name,
		"path: "+model.ModelPath,
		fmt.Sprintf("port: %d", model.HostPort),
	)
	if strings.TrimSpace(model.MMProjPath) != "" {
		lines = append(lines, "mmproj: "+model.MMProjPath)
	}
	return strings.Join(lines, "\n")
}

func buildCompletionCommand(req *clir.Request) (any, error) {
	prompt := strings.TrimSpace(req.Params["prompt"])
	if prompt == "" {
		return nil, fmt.Errorf("missing prompt")
	}
	return completionCommand{Prompt: prompt}, nil
}

func buildModelCompletionCommand(req *clir.Request) (any, error) {
	cmd, err := buildCompletionCommand(req)
	if err != nil {
		return nil, err
	}
	typed := cmd.(completionCommand)
	typed.Model = cleanModelName(req.Params["model"])
	if typed.Model == "" {
		return nil, fmt.Errorf("missing model")
	}
	return typed, nil
}

func buildEmbedCommand(req *clir.Request) (any, error) {
	text := strings.TrimSpace(req.Params["text"])
	if text == "" {
		return nil, fmt.Errorf("missing text")
	}
	return embedCommand{Text: text}, nil
}

func buildModelEmbedCommand(req *clir.Request) (any, error) {
	cmd, err := buildEmbedCommand(req)
	if err != nil {
		return nil, err
	}
	typed := cmd.(embedCommand)
	typed.Model = cleanModelName(req.Params["model"])
	if typed.Model == "" {
		return nil, fmt.Errorf("missing model")
	}
	return typed, nil
}

func buildModelInstallCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("llamacpp model install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	filename := fs.String("filename", "", "Downloaded model filename")
	sha := fs.String("sha256", "", "Expected model sha256")
	hostPort := fs.Int("host-port", 0, "Host port for this model service")
	ctxSize := fs.Int("ctx-size", 0, "llama.cpp context size")
	gpuLayers := fs.Int("gpu-layers", 0, "llama.cpp GPU layers")
	embedding := fs.Bool("embedding", false, "Register this model for embedding mode")
	pooling := fs.String("pooling", "", "llama.cpp embedding pooling mode")
	ubatch := fs.Int("ubatch-size", 0, "llama.cpp physical batch size")
	normalize := fs.Bool("normalize", true, "L2-normalize embedding vectors client-side")
	makeDefault := fs.Bool("default", false, "Use this model as the component default")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	if len(fs.Args()) > 0 {
		return nil, fmt.Errorf("unexpected install arguments: %s", strings.Join(fs.Args(), " "))
	}
	name := cleanModelName(req.Params["name"])
	url := strings.TrimSpace(req.Params["url"])
	if name == "" || url == "" {
		return nil, fmt.Errorf("missing model name or url")
	}
	return modelInstallCommand{Name: name, URL: url, Mode: modelModeFromFlag(*embedding), Filename: *filename, SHA256: *sha, HostPort: *hostPort, ContextSize: *ctxSize, UBatchSize: *ubatch, GPULayers: *gpuLayers, Pooling: *pooling, Normalize: normalizeOption(*embedding, *normalize), Default: *makeDefault}, nil
}

func buildModelRegisterCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("llamacpp model register", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	hostPort := fs.Int("host-port", 0, "Host port for this model service")
	ctxSize := fs.Int("ctx-size", 0, "llama.cpp context size")
	gpuLayers := fs.Int("gpu-layers", 0, "llama.cpp GPU layers")
	embedding := fs.Bool("embedding", false, "Register this model for embedding mode")
	pooling := fs.String("pooling", "", "llama.cpp embedding pooling mode")
	ubatch := fs.Int("ubatch-size", 0, "llama.cpp physical batch size")
	normalize := fs.Bool("normalize", true, "L2-normalize embedding vectors client-side")
	makeDefault := fs.Bool("default", false, "Use this model as the component default")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	if len(fs.Args()) > 0 {
		return nil, fmt.Errorf("unexpected register arguments: %s", strings.Join(fs.Args(), " "))
	}
	name := cleanModelName(req.Params["name"])
	path := strings.TrimSpace(req.Params["path"])
	if name == "" || path == "" {
		return nil, fmt.Errorf("missing model name or path")
	}
	return modelInstallCommand{Name: name, Path: path, Mode: modelModeFromFlag(*embedding), HostPort: *hostPort, ContextSize: *ctxSize, UBatchSize: *ubatch, GPULayers: *gpuLayers, Pooling: *pooling, Normalize: normalizeOption(*embedding, *normalize), Default: *makeDefault}, nil
}

func modelModeFromFlag(embedding bool) string {
	if embedding {
		return "embedding"
	}
	return "completion"
}

func normalizeOption(embedding bool, normalize bool) *bool {
	if !embedding {
		return nil
	}
	return &normalize
}

func (c *Component) handleCompletionCommand(ctx context.Context, req commandengine.Request, cmd completionCommand) (commandengine.Result, error) {
	_, _ = req, c
	result, err := c.HandleCompletion(ctx, component.CompletionRequest{
		Model: cmd.Model,
		Prompt: component.CompletionPrompt{Messages: []component.CompletionMessage{{
			Role:    component.CompletionRoleUser,
			Content: cmd.Prompt,
		}}},
	})
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: completionResultText(result)}, nil
}

func (c *Component) handleEmbedCommand(ctx context.Context, req commandengine.Request, cmd embedCommand) (commandengine.Result, error) {
	_, _ = req, c
	result, err := c.Embed(ctx, component.EmbedRequest{
		Model: cmd.Model,
		Inputs: []component.EmbeddingInput{{
			ID:   "input",
			Text: cmd.Text,
			Kind: component.EmbeddingKindQuery,
		}},
	})
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(result.Embeddings) == 0 {
		return commandengine.Result{Text: "no embedding returned"}, nil
	}
	embedding := result.Embeddings[0]
	return commandengine.Result{Text: fmt.Sprintf("embedding model=%s dim=%d normalized=%t", embedding.Model, embedding.Dim, embedding.Normalized)}, nil
}

func completionResultText(result *component.CompletionResult) string {
	if result == nil || result.Final == nil {
		return ""
	}
	return strings.TrimSpace(result.Final.Text)
}

func (c *Component) modelList() commandengine.Result {
	var lines []string
	lines = append(lines, "llama.cpp models")
	if strings.TrimSpace(c.componentConfig.DefaultModel) != "" {
		lines = append(lines, "default_model: "+c.componentConfig.DefaultModel)
	}
	if len(c.models.Models) == 0 {
		if strings.TrimSpace(c.componentConfig.ModelPath) != "" {
			lines = append(lines, "default legacy path: "+c.componentConfig.ModelPath)
		} else {
			lines = append(lines, "(no models installed)")
		}
		return commandengine.Result{Text: strings.Join(lines, "\n")}
	}
	for _, name := range sortedModelNames(c.models.Models) {
		model := c.models.Models[name]
		suffix := ""
		if name == c.componentConfig.DefaultModel {
			suffix = " default=true"
		}
		lines = append(lines, fmt.Sprintf("- %s%s mode=%s path=%s port=%d", name, suffix, cleanModelMode(model.Mode), c.modelPath(name, model), firstPositive(model.HostPort, c.componentConfig.HostPort)))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}
}

func (c *Component) modelInstall(cmd modelInstallCommand) (commandengine.Result, error) {
	model, err := c.installModel(cmd.Name, ModelConfig{
		URL:         cmd.URL,
		Mode:        cmd.Mode,
		Filename:    cmd.Filename,
		SHA256:      cmd.SHA256,
		HostPort:    cmd.HostPort,
		ContextSize: cmd.ContextSize,
		UBatchSize:  cmd.UBatchSize,
		GPULayers:   cmd.GPULayers,
		Pooling:     cmd.Pooling,
		Normalize:   cmd.Normalize,
	})
	if err != nil {
		return commandengine.Result{}, err
	}
	if cmd.Default {
		c.componentConfig.DefaultModel = cmd.Name
		if err := saveComponentConfig(c.home.Path, c.componentConfig); err != nil {
			return commandengine.Result{}, err
		}
	}
	return commandengine.Result{Text: fmt.Sprintf("model installed: %s\npath: %s", cmd.Name, c.modelPath(cmd.Name, model))}, nil
}

func (c *Component) modelRegister(cmd modelInstallCommand) (commandengine.Result, error) {
	model := ModelConfig{
		Path:        cmd.Path,
		Mode:        cmd.Mode,
		HostPort:    cmd.HostPort,
		ContextSize: cmd.ContextSize,
		UBatchSize:  cmd.UBatchSize,
		GPULayers:   cmd.GPULayers,
		Pooling:     cmd.Pooling,
		Normalize:   cmd.Normalize,
	}
	if c.models.Models == nil {
		c.models.Models = map[string]ModelConfig{}
	}
	c.models.Models[cmd.Name] = model
	if err := saveModelRegistry(c.home.Path, c.models); err != nil {
		return commandengine.Result{}, err
	}
	if cmd.Default {
		c.componentConfig.DefaultModel = cmd.Name
		if err := saveComponentConfig(c.home.Path, c.componentConfig); err != nil {
			return commandengine.Result{}, err
		}
	}
	return commandengine.Result{Text: fmt.Sprintf("model registered: %s\npath: %s", cmd.Name, c.modelPath(cmd.Name, model))}, nil
}

func llamacppCommand(pattern string, command any, help string, build func(req *clir.Request) (any, error), aliases ...commandengine.Route) commandengine.Definition {
	commandAliases := make([]commandengine.Route, 0, len(aliases))
	for _, alias := range aliases {
		commandAliases = append(commandAliases, commandengine.Route{Pattern: alias.Pattern, Absolute: alias.Absolute, Hidden: alias.Hidden})
	}
	if build == nil {
		build = func(req *clir.Request) (any, error) { _ = req; return command, nil }
	}
	return commandengine.Definition{
		Pattern: pattern,
		Help:    help,
		Build:   build,
		Sources: []commandengine.Source{
			commandengine.SourceCLI,
			commandengine.SourceMessage,
			commandengine.SourceHostbridge,
		},
		Policy:                llamacppCommandPolicy(pattern),
		Aliases:               commandAliases,
		InstructionVisibility: llamacppInstructionVisibility(pattern),
	}
}

func llamacppCommandPolicy(pattern string) simplerbac.Rule {
	normalized := commandengine.NormalizePattern(pattern)
	if strings.HasPrefix(normalized, "model install") || strings.HasPrefix(normalized, "model register") {
		return simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent)
	}
	return simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser)
}

func llamacppInstructionVisibility(pattern string) commandengine.InstructionVisibility {
	if commandengine.NormalizePattern(pattern) == "status" {
		return commandengine.InstructionImportant
	}
	return commandengine.InstructionDiscoverable
}
