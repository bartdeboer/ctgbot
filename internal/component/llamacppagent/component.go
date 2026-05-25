package llamacppagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
	"github.com/bartdeboer/ctgbot/internal/toolloop"
)

type ComponentResolver interface {
	ResolveComponentRef(ctx context.Context, ref string) (*coremodel.Component, error)
	ResolveComponent(ctx context.Context, id modeluuid.UUID) (*component.Loaded, error)
}

type Component struct {
	registration     coremodel.Component
	runtime          runtimepkg.ThreadRuntime
	runtimeConfig    runtimepkg.BindConfig
	home             runtimepkg.Home
	storage          repository.Storage
	resolveWorkspace func(context.Context, coremodel.Chat) (string, error)
	config           ComponentConfig
	resolver         ComponentResolver
	logger           *log.Logger
}

var _ component.Agent = (*Component)(nil)
var _ component.ProfileOwner = (*Component)(nil)
var _ component.RuntimeImageProvider = (*Component)(nil)

func New(ctx context.Context, registration coremodel.Component, runtimeFactory runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage, resolver ComponentResolver, resolveWorkspace func(context.Context, coremodel.Chat) (string, error), logger *log.Logger) (component.Component, error) {
	_ = ctx
	if storage == nil {
		return nil, fmt.Errorf("missing storage")
	}
	if resolveWorkspace == nil {
		return nil, fmt.Errorf("missing workspace resolver")
	}
	threadFactory, ok := runtimeFactory.(runtimepkg.ThreadRuntimeFactory)
	if !ok {
		return nil, fmt.Errorf("llamacppagent requires thread runtime, got %T", runtimeFactory)
	}
	runtimeConfig, err := loadRuntimeConfig(home.Path)
	if err != nil {
		return nil, err
	}
	config, err := loadComponentConfig(home.Path)
	if err != nil {
		return nil, err
	}
	runtimeHomePath := runtimeFactory.RuntimeComponentHomePath(registration, home)
	bindConfig := componentBindConfig(runtimeConfig, runtimeHomePath)
	return &Component{
		registration:     registration,
		runtime:          threadFactory.Bind(registration, home, bindConfig),
		runtimeConfig:    bindConfig,
		home:             home,
		storage:          storage,
		resolveWorkspace: resolveWorkspace,
		config:           config,
		resolver:         resolver,
		logger:           logger,
	}, nil
}

func componentBindConfig(config runtimepkg.BindConfig, runtimeHomePath string) runtimepkg.BindConfig {
	runtimeHomePath = strings.TrimSpace(runtimeHomePath)
	return config.Clean().WithEnvOverride(
		"HOME="+runtimeHomePath,
		"GOCACHE="+runtimeHomePath+"/.cache/go-build",
		"GOPATH="+runtimeHomePath+"/go",
		"GOMODCACHE="+runtimeHomePath+"/go/pkg/mod",
	)
}

func (c *Component) Type() string { return Type }

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{
		{RelativePath: ComponentConfigFilename, Required: false, Sensitive: false},
		{RelativePath: runtimepkg.ConfigFilename, Required: false, Sensitive: false},
	}
}

func (c *Component) RuntimeImageTargets(ctx context.Context) ([]runtimeimage.Target, error) {
	_ = ctx
	if c == nil {
		return nil, nil
	}
	target := runtimeimage.Target{
		Name:       Type,
		Image:      firstNonEmpty(c.runtimeConfig.Image, DefaultImage),
		Dockerfile: firstNonEmpty(c.runtimeConfig.Dockerfile, DefaultDockerfile),
		NoCache:    c.runtimeConfig.NoCache,
		Uses:       c.runtimeConfig.Uses,
	}
	if target.Uses != nil {
		if !target.NoCache {
			target.NoCache = true
		}
		return []runtimeimage.Target{target}, nil
	}
	if target.Dockerfile != DefaultDockerfile {
		return []runtimeimage.Target{target}, nil
	}
	target.Uses = &runtimeimage.Target{
		Name:       "go-node-python-base",
		Image:      DefaultBaseImage,
		Dockerfile: DefaultBaseDockerfile,
	}
	target.NoCache = true
	return []runtimeimage.Target{target}, nil
}

func (c *Component) HandleTurn(ctx context.Context, turn component.Turn) (*component.TurnResult, error) {
	if c == nil || c.runtime == nil {
		return nil, fmt.Errorf("missing llamacppagent runtime")
	}
	prompt := strings.TrimSpace(turn.Inbound.Text)
	if prompt == "" {
		return nil, nil
	}

	session, err := c.beginBackendSession(ctx)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	stopTyping, err := turn.Runtime.StartChatAction(ctx, message.ChatActionTyping)
	if err == nil && stopTyping != nil {
		defer stopTyping()
	}

	profile := c.modelToolloopProfile(ctx, session.Model())
	files, err := c.prepareToolloopRun(turn, session, profile, prompt)
	if err != nil {
		return nil, err
	}
	keepDebugFiles := false
	defer func() {
		if !keepDebugFiles {
			files.Cleanup()
		}
	}()

	providerThreadID, err := c.providerThreadID(turn.Runtime)
	if err != nil {
		return nil, err
	}
	result, runErr := c.runToolloop(ctx, turn, session, profile, files, providerThreadID, prompt)
	if bindErr := c.bindProviderThreadID(turn.Runtime, result.ConversationID); bindErr != nil && runErr == nil {
		runErr = bindErr
	}
	if runErr != nil {
		keepDebugFiles = true
		c.logToolloopTrace(turn.Thread.ID, result.Trace)
		return nil, fmt.Errorf("toolloop: %w\n%s", runErr, files.DebugFiles())
	}
	c.logToolloopTrace(turn.Thread.ID, result.Trace)

	reply := strings.TrimSpace(result.Text)
	if reply == "" {
		if result.Status == "error" && strings.TrimSpace(result.Error) != "" {
			keepDebugFiles = true
			return nil, fmt.Errorf("toolloop error: %s\n%s", result.Error, files.DebugFiles())
		}
		return nil, nil
	}
	return &component.TurnResult{Final: &coremodel.ThreadMessage{Role: coremodel.MessageRoleAgent, Kind: coremodel.MessageKindAgent, ComponentID: c.registration.ID, ActorID: c.registration.Ref(), ActorLabel: "llama.cpp agent", Text: reply}}, nil
}

func (c *Component) beginBackendSession(ctx context.Context) (component.OpenAIChatSession, error) {
	backend, err := c.backend(ctx)
	if err != nil {
		return nil, err
	}
	return backend.BeginOpenAIChatSession(ctx, component.CompletionSessionOptions{Model: c.config.Model, IdleTimeout: c.config.backendIdleTimeout()})
}

func (c *Component) prepareToolloopRun(turn component.Turn, session component.OpenAIChatSession, profile component.ModelToolloopProfile, prompt string) (*toolloopRunFiles, error) {
	files, err := newToolloopRunFiles(c.runtime.ComponentHome().Path, c.runtime.RuntimeComponentHomePath(), turn.Thread.ID)
	if err != nil {
		return nil, err
	}
	invocation := toolloopInvocation{
		BaseURL:                 firstNonEmpty(c.config.BaseURL, sandboxBaseURL(session.BaseURL())),
		Model:                   session.Model(),
		Prompt:                  prompt,
		Workspace:               c.runtime.RuntimeWorkspacePath(turn.Runtime.WorkspacePath()),
		MaxIterations:           c.config.MaxIterations,
		MaxTokens:               c.config.MaxTokens,
		Temperature:             c.config.Temperature,
		ModelPromptInstructions: profile.PromptInstructions,
		ModelToolInstructions:   profile.ToolInstructions,
		ModelReasoningFormat:    profile.ReasoningFormat,
		ModelToolCallFormat:     profile.ToolCallFormat,
	}
	if err := files.WriteInvocation(invocation); err != nil {
		files.Cleanup()
		return nil, err
	}
	return files, nil
}

func (c *Component) runToolloop(ctx context.Context, turn component.Turn, session component.OpenAIChatSession, profile component.ModelToolloopProfile, files *toolloopRunFiles, providerThreadID string, prompt string) (toolloop.Result, error) {
	runtime := commandRuntime{
		runtime:       c.runtime,
		workspacePath: turn.Runtime.WorkspacePath(),
		threadID:      turn.Thread.ID,
		commands:      turn.Runtime.Commands(),
	}
	result, err := NewRunner(c.logger).RunTurn(ctx, runtime, outputHandler{runtime: turn.Runtime}, ToolloopTurnRequest{
		ProviderThreadID: providerThreadID,
		Prompt:           prompt,
		Env:              c.toolloopEnv(session, turn, profile),
		ResultRuntime:    files.ResultRuntime(),
		ResultHost:       files.ResultHost,
		EventsRuntime:    files.EventsRuntime(),
		EventsHost:       files.EventsHost,
	})
	return result.Result, err
}

func (c *Component) toolloopEnv(session component.OpenAIChatSession, turn component.Turn, profile component.ModelToolloopProfile) []string {
	return []string{
		"TOOLLOOP_BASE_URL=" + firstNonEmpty(c.config.BaseURL, sandboxBaseURL(session.BaseURL())),
		"TOOLLOOP_API_KEY=" + firstNonEmpty(c.config.APIKey, session.APIKey()),
		"TOOLLOOP_MODEL=" + session.Model(),
		"TOOLLOOP_SYSTEM=" + c.systemPrompt(turn),
		"TOOLLOOP_WORKSPACE=" + c.runtime.RuntimeWorkspacePath(turn.Runtime.WorkspacePath()),
		"TOOLLOOP_MAX_ITERATIONS=" + fmt.Sprintf("%d", c.config.MaxIterations),
		"TOOLLOOP_MAX_TOKENS=" + fmt.Sprintf("%d", c.config.MaxTokens),
		"TOOLLOOP_TEMPERATURE=" + fmt.Sprintf("%g", c.config.Temperature),
		"TOOLLOOP_CONVERSATION_DIR=" + c.runtime.RuntimeComponentHomePath() + "/toolloop/conversations",
		"TOOLLOOP_MODEL_PROMPT_INSTRUCTIONS=" + profile.PromptInstructions,
		"TOOLLOOP_MODEL_TOOL_INSTRUCTIONS=" + profile.ToolInstructions,
		"TOOLLOOP_MODEL_REASONING_FORMAT=" + profile.ReasoningFormat,
		"TOOLLOOP_MODEL_TOOL_CALL_FORMAT=" + profile.ToolCallFormat,
	}
}

func readToolloopResult(path string) (toolloop.Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return toolloop.Result{}, fmt.Errorf("read result: %w", err)
	}
	var result toolloop.Result
	if err := json.Unmarshal(data, &result); err != nil {
		return toolloop.Result{}, fmt.Errorf("decode result: %w", err)
	}
	return result, nil
}

func (c *Component) modelToolloopProfile(ctx context.Context, modelName string) component.ModelToolloopProfile {
	if c == nil || c.resolver == nil || strings.TrimSpace(modelName) == "" {
		return component.ModelToolloopProfile{}
	}
	registration, err := c.resolver.ResolveComponentRef(ctx, c.config.ModelRegistry)
	if err != nil {
		c.logf("model profile registry resolve failed model=%s err=%v", modelName, err)
		return component.ModelToolloopProfile{}
	}
	loaded, err := c.resolver.ResolveComponent(ctx, registration.ID)
	if err != nil || loaded == nil {
		c.logf("model profile component resolve failed model=%s err=%v", modelName, err)
		return component.ModelToolloopProfile{}
	}
	registry, ok := loaded.Component.(component.ModelRegistry)
	if !ok {
		c.logf("model profile component %s is not model registry", loaded.Registration.Ref())
		return component.ModelToolloopProfile{}
	}
	profile, err := registry.ModelToolloopProfile(ctx, modelName)
	if err != nil {
		c.logf("model toolloop profile unavailable model=%s err=%v", modelName, err)
		return component.ModelToolloopProfile{}
	}
	return profile
}

func (c *Component) providerThreadID(turnRuntime component.TurnRuntime) (string, error) {
	if turnRuntime == nil {
		return "", fmt.Errorf("missing turn runtime")
	}
	providerThreadID, ok, err := turnRuntime.ComponentThreadID(c.registration.ID)
	if err != nil || !ok {
		return "", err
	}
	return strings.TrimSpace(providerThreadID), nil
}

func (c *Component) bindProviderThreadID(turnRuntime component.TurnRuntime, providerThreadID string) error {
	providerThreadID = strings.TrimSpace(providerThreadID)
	if providerThreadID == "" {
		return nil
	}
	if turnRuntime == nil {
		return fmt.Errorf("missing turn runtime")
	}
	return turnRuntime.BindComponentThreadID(c.registration.ID, providerThreadID)
}

func (c *Component) logToolloopTrace(threadID modeluuid.UUID, trace []toolloop.TraceStep) {
	if trace := toolloop.FormatTrace(trace, 4000); trace != "" {
		c.logf("llamacppagent toolloop trace thread=%s\n%s", threadID.String(), trace)
	}
}

func (c *Component) backend(ctx context.Context) (component.OpenAIChatSessionProvider, error) {
	if c == nil || c.resolver == nil {
		return nil, fmt.Errorf("missing component resolver")
	}
	ref := firstNonEmpty(c.config.Backend, "llamacpp")
	registration, err := c.resolver.ResolveComponentRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	loaded, err := c.resolver.ResolveComponent(ctx, registration.ID)
	if err != nil {
		return nil, err
	}
	if loaded == nil {
		return nil, fmt.Errorf("backend component not found: %s", ref)
	}
	provider, ok := loaded.Component.(component.OpenAIChatSessionProvider)
	if !ok {
		return nil, fmt.Errorf("component %s does not provide OpenAI chat sessions", loaded.Registration.Ref())
	}
	return provider, nil
}

func (c *Component) systemPrompt(turn component.Turn) string {
	if strings.TrimSpace(c.config.SystemPrompt) != "" {
		return c.config.SystemPrompt
	}
	instructions := turn.Runtime.Instructions()
	return fmt.Sprintf(`You are a coding agent running inside ctgbot.

Use shell for normal coding commands and workspace inspection. Useful patterns include rg -n "name" path, nl -ba path | sed -n '120,180p', and sed -n '120,180p' path. Do not use shell redirection for file edits unless the dedicated file tools are insufficient.

Use the hostbridge tool when you need ctgbot commands or hostbridge-specific actions. Before using hostbridge commands, call hostbridge help if you are unsure which commands are available.

Use read before editing existing files. Use edit for localized exact-string replacements. Use write for new files or deliberate full-file rewrites. Existing files must be read before write overwrites them.

Use apply_patch for multi-file or multi-hunk structured edits. apply_patch uses Codex patch grammar, not unified diff:
*** Begin Patch
*** Add File: hello.txt
+hello
*** End Patch
In apply_patch, file paths are relative, Add File content lines start with +, and you must not use --- /dev/null or +++ b/file unified-diff headers.

Be concise. Start every final response with %q.

Current date: %s
Workspace: %s`, instructions.MessagePrefix, time.Now().Format("2006-01-02"), c.runtime.RuntimeWorkspacePath(turn.Runtime.WorkspacePath()))
}

func sandboxBaseURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return raw
	}
	host := parsed.Hostname()
	if host == "127.0.0.1" || host == "localhost" {
		port := parsed.Port()
		if port != "" {
			parsed.Host = "host.docker.internal:" + port
		} else {
			parsed.Host = "host.docker.internal"
		}
	}
	return parsed.String()
}

func (c *Component) logf(format string, args ...any) {
	if c != nil && c.logger != nil {
		c.logger.Printf(format, args...)
	}
}
