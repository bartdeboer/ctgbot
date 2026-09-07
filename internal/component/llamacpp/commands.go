package llamacpp

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

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

// CompletionStdinMaxBytes bounds instruction plus document bytes, not model tokens.
const CompletionStdinMaxBytes = 1 << 20

type completionCommand struct {
	Reasoning    component.ReasoningMode
	MaxTokens    int
	MaxTokensSet bool
	Stdin        bool
	Prompt       string
	Model        string
}

type embedCommand struct {
	Text  string
	Model string
}

func RegisterGobTypes(register func(any)) {
	register(startCommand{})
	register(stopCommand{})
	register(statusCommand{})
	register(completionCommand{})
	register(embedCommand{})
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		llamacppCommand("start", startCommand{}, "Start the default llama.cpp model service", nil),
		llamacppCommand("stop", stopCommand{}, "Stop the default llama.cpp model service", nil),
		llamacppCommand("status", statusCommand{}, "Show default llama.cpp model service status", nil),
		llamacppCommand("completion <prompt>", nil, "Run a completion with the default llama.cpp model; options: --stdin, --reasoning default|enabled|disabled, --max-tokens N", buildCompletionCommand),
		llamacppCommand("embed <text>", nil, "Embed text with the default llama.cpp embedding model", buildEmbedCommand),
		llamacppCommand("model <model> completion <prompt>", nil, "Run a completion with a specific llama.cpp model; options: --stdin, --reasoning default|enabled|disabled, --max-tokens N", buildModelCompletionCommand),
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
	fs := flag.NewFlagSet("completion", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	stdin := fs.Bool("stdin", false, "Read a document from piped stdin")
	reasoning := fs.String("reasoning", "default", "default, enabled or disabled")
	maxTokens := fs.Int("max-tokens", 0, "Positive output token limit")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, fmt.Errorf("invalid completion options")
	}
	if len(fs.Args()) != 0 {
		return nil, fmt.Errorf("unexpected completion arguments")
	}
	prompt := strings.TrimSpace(req.Params["prompt"])
	if prompt == "--stdin" || prompt == "--reasoning" || prompt == "--max-tokens" {
		return nil, fmt.Errorf("completion requires a quoted instruction before options")
	}
	if prompt == "" {
		return nil, fmt.Errorf("missing prompt")
	}
	cmd := completionCommand{Prompt: prompt, Stdin: *stdin}
	switch *reasoning {
	case "default":
	case "enabled":
		cmd.Reasoning = component.ReasoningEnabled
	case "disabled":
		cmd.Reasoning = component.ReasoningDisabled
	default:
		return nil, fmt.Errorf("--reasoning must be default, enabled or disabled")
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "max-tokens" {
			cmd.MaxTokens = *maxTokens
			cmd.MaxTokensSet = true
		}
	})
	if err := cmd.validateOptions(); err != nil {
		return nil, err
	}
	return cmd, nil
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

func (c *Component) handleCompletionCommand(ctx context.Context, req commandengine.Request, cmd completionCommand) (commandengine.Result, error) {
	if err := cmd.validateOptions(); err != nil {
		return commandengine.Result{}, err
	}
	prompt, err := completionCommandPrompt(cmd, req.Stdin)
	if err != nil {
		return commandengine.Result{}, err
	}
	request := component.CompletionRequest{Model: cmd.Model, Prompt: prompt, Reasoning: cmd.Reasoning}
	if cmd.MaxTokensSet {
		request.MaxOutputTokens = cmd.MaxTokens
	}
	result, err := c.Complete(ctx, request)
	if err != nil {
		if cmd.Stdin {
			// Providers may echo submitted input in error bodies.
			if ctx.Err() != nil {
				return commandengine.Result{}, ctx.Err()
			}
			return commandengine.Result{}, fmt.Errorf("stdin completion failed; backend error details withheld to protect document contents")
		}
		return commandengine.Result{}, err
	}
	text := completionResultText(result)
	if text == "" {
		return commandengine.Result{}, fmt.Errorf("model returned no final answer; try --reasoning disabled or a larger --max-tokens limit")
	}
	return commandengine.Result{Text: text}, nil
}

func (c *Component) handleEmbedCommand(ctx context.Context, req commandengine.Request, cmd embedCommand) (commandengine.Result, error) {
	_, _ = req, c
	result, err := c.Embed(ctx, component.EmbeddingRequest{
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
	_ = normalized
	return simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser)
}

func llamacppInstructionVisibility(pattern string) commandengine.InstructionVisibility {
	if commandengine.NormalizePattern(pattern) == "status" {
		return commandengine.InstructionImportant
	}
	return commandengine.InstructionDiscoverable
}

// CompletionUsesStdin lets the Hostbridge client capture input only after typed
// catalog parsing, including named component prefixes.
func CompletionUsesStdin(command any) bool {
	cmd, ok := command.(completionCommand)
	return ok && cmd.Stdin
}

func completionCommandPrompt(cmd completionCommand, document string) (component.CompletionPrompt, error) {
	if !cmd.Stdin {
		return component.CompletionPrompt{Messages: []component.CompletionMessage{{Role: component.CompletionRoleUser, Content: cmd.Prompt}}}, nil
	}
	if len(cmd.Prompt) > CompletionStdinMaxBytes || len(document) > CompletionStdinMaxBytes-len(cmd.Prompt) {
		return component.CompletionPrompt{}, fmt.Errorf("completion instruction plus stdin exceeds %d bytes", CompletionStdinMaxBytes)
	}
	if !utf8.ValidString(cmd.Prompt) || !utf8.ValidString(document) {
		return component.CompletionPrompt{}, fmt.Errorf("completion instruction and stdin must be UTF-8")
	}
	if strings.TrimSpace(cmd.Prompt) == "" {
		return component.CompletionPrompt{}, fmt.Errorf("completion --stdin requires an instruction")
	}
	if strings.TrimSpace(document) == "" {
		return component.CompletionPrompt{}, fmt.Errorf("completion --stdin requires a nonblank piped document; this transport supplied no usable stdin")
	}
	return component.CompletionPrompt{Messages: []component.CompletionMessage{
		{Role: component.CompletionRoleSystem, Content: cmd.Prompt},
		{Role: component.CompletionRoleUser, Content: document, PreserveWhitespace: true},
	}}, nil
}

func (cmd completionCommand) validateOptions() error {
	if (cmd.MaxTokensSet && cmd.MaxTokens <= 0) || cmd.MaxTokens < 0 {
		return fmt.Errorf("--max-tokens must be positive")
	}
	switch cmd.Reasoning {
	case component.ReasoningDefault, component.ReasoningEnabled, component.ReasoningDisabled:
		return nil
	default:
		return fmt.Errorf("--reasoning must be default, enabled or disabled")
	}
}
