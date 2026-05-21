package llamacppagent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
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
	return &Component{
		registration:     registration,
		runtime:          threadFactory.Bind(registration, home, runtimeConfig),
		runtimeConfig:    runtimeConfig,
		home:             home,
		storage:          storage,
		resolveWorkspace: resolveWorkspace,
		config:           config,
		resolver:         resolver,
		logger:           logger,
	}, nil
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
		Name:       Type + "-base",
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
	backend, err := c.backend(ctx)
	if err != nil {
		return nil, err
	}
	session, err := backend.BeginOpenAIChatSession(ctx, component.CompletionSessionOptions{Model: c.config.Model, IdleTimeout: c.config.backendIdleTimeout()})
	if err != nil {
		return nil, err
	}
	defer session.Close()

	stopTyping, err := turn.Runtime.StartChatAction(ctx, message.ChatActionTyping)
	if err == nil && stopTyping != nil {
		defer stopTyping()
	}

	requestHost, outputHost, cleanup, err := c.writeRequest(turn, session, prompt)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	runtimeHome := c.runtime.RuntimeComponentHomePath()
	requestRuntime := filepath.ToSlash(filepath.Join(runtimeHome, "toolloop", filepath.Base(filepath.Dir(requestHost)), filepath.Base(requestHost)))
	outputRuntime := filepath.ToSlash(filepath.Join(runtimeHome, "toolloop", filepath.Base(filepath.Dir(outputHost)), filepath.Base(outputHost)))
	out, err := c.runtime.CombinedOutput(ctx, turn.Runtime.WorkspacePath(), turn.Thread.ID, turn.Runtime.Commands(), "toolloop", "--request", requestRuntime, "--output", outputRuntime)
	if err != nil {
		return nil, fmt.Errorf("toolloop: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	data, err := os.ReadFile(outputHost)
	if err != nil {
		return nil, fmt.Errorf("read toolloop result: %w", err)
	}
	var result toolloop.Result
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode toolloop result: %w", err)
	}
	reply := strings.TrimSpace(result.Text)
	if reply == "" {
		return nil, nil
	}
	return &component.TurnResult{Final: &coremodel.ThreadMessage{Kind: coremodel.MessageKindAgent, ComponentID: c.registration.ID, ActorID: c.registration.Ref(), ActorLabel: "llama.cpp agent", Text: reply}}, nil
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

func (c *Component) writeRequest(turn component.Turn, session component.OpenAIChatSession, prompt string) (string, string, func(), error) {
	hostDir := filepath.Join(c.runtime.ComponentHome().Path, "toolloop", turn.Thread.ID.String()+"-"+modeluuid.New().String())
	if err := os.MkdirAll(hostDir, 0o700); err != nil {
		return "", "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(hostDir) }
	requestHost := filepath.Join(hostDir, "request.json")
	outputHost := filepath.Join(hostDir, "result.json")
	messages := toolloopMessages(turn.History, turn.Inbound)
	req := toolloop.Request{
		BaseURL:        firstNonEmpty(c.config.BaseURL, sandboxBaseURL(session.BaseURL())),
		APIKey:         firstNonEmpty(c.config.APIKey, session.APIKey()),
		Model:          session.Model(),
		System:         c.systemPrompt(turn),
		Messages:       messages,
		Prompt:         textPromptFromMessages(messages, prompt),
		Workspace:      c.runtime.RuntimeWorkspacePath(turn.Runtime.WorkspacePath()),
		MaxIterations:  c.config.MaxIterations,
		MaxTokens:      c.config.MaxTokens,
		Temperature:    c.config.Temperature,
		EnableThinking: boolPtr(c.config.EnableThinking),
	}
	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		cleanup()
		return "", "", nil, err
	}
	if err := os.WriteFile(requestHost, data, 0o600); err != nil {
		cleanup()
		return "", "", nil, err
	}
	return requestHost, outputHost, cleanup, nil
}

func textPromptFromMessages(messages []toolloop.Message, fallback string) string {
	if len(messages) == 0 {
		return strings.TrimSpace(fallback)
	}
	var b strings.Builder
	b.WriteString("Conversation history:\n")
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		content := strings.TrimSpace(message.Content)
		if role == "" || content == "" {
			continue
		}
		switch role {
		case "assistant":
			role = "Assistant"
		case "user":
			role = "User"
		case "system":
			role = "System"
		}
		b.WriteString(role)
		b.WriteString(": ")
		b.WriteString(content)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func toolloopMessages(history []coremodel.ThreadMessage, inbound coremodel.ThreadMessage) []toolloop.Message {
	if len(history) == 0 {
		return nil
	}
	out := make([]toolloop.Message, 0, len(history))
	for _, message := range history {
		if !inbound.ID.IsNull() && message.ID == inbound.ID {
			message = inbound
		}
		content := strings.TrimSpace(message.Text)
		if content == "" {
			continue
		}
		role, ok := toolloopRole(message)
		if !ok {
			continue
		}
		out = append(out, toolloop.Message{Role: role, Content: content})
	}
	return out
}

func toolloopRole(message coremodel.ThreadMessage) (string, bool) {
	switch message.Kind {
	case coremodel.MessageKindSystem:
		return "system", true
	case coremodel.MessageKindAgent:
		return "assistant", true
	case coremodel.MessageKindUser:
		return "user", true
	}
	switch message.Direction {
	case coremodel.MessageDirectionInbound:
		return "user", true
	case coremodel.MessageDirectionOutbound:
		return "assistant", true
	default:
		return "", false
	}
}

func (c *Component) systemPrompt(turn component.Turn) string {
	if strings.TrimSpace(c.config.SystemPrompt) != "" {
		return c.config.SystemPrompt
	}
	instructions := turn.Runtime.Instructions()
	return fmt.Sprintf(`You are a coding agent running inside ctgbot.

Use shell for workspace inspection and normal coding commands. Useful patterns include rg -n "name" path, nl -ba path | sed -n '120,180p', and sed -n '120,180p' path.

Use the hostbridge tool when you need ctgbot commands or hostbridge-specific actions. Before using hostbridge commands, call hostbridge help if you are unsure which commands are available.

Use apply_patch to edit workspace files. Read files before editing them. Keep patches small and focused.

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

func boolPtr(value bool) *bool { return &value }
