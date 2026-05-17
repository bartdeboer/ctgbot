package semantic

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

const Type = "semantic"

type ComponentResolver interface {
	ResolveComponentRef(ctx context.Context, ref string) (*coremodel.Component, error)
	ResolveComponent(ctx context.Context, componentID modeluuid.UUID) (*component.Loaded, error)
}

type Component struct {
	registration coremodel.Component
	config       ComponentConfig
	homePath     string
	store        *store
	resolver     ComponentResolver
	messages     component.SearchMessageSource
	logf         func(format string, args ...any)
}

var _ component.Component = (*Component)(nil)
var _ component.ProfileOwner = (*Component)(nil)
var _ component.SkillProvider = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)
var _ component.LocalCommandSurface = (*Component)(nil)
var _ component.Searcher = (*Component)(nil)
var _ component.SearchMessageSourceReceiver = (*Component)(nil)

func New(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage, resolver ComponentResolver, logf func(format string, args ...any)) (component.Component, error) {
	_, _, _ = ctx, runtime, storage
	config, err := loadComponentConfig(home.Path)
	if err != nil {
		return nil, err
	}
	store, err := openStore(home.Path)
	if err != nil {
		return nil, err
	}
	return &Component{registration: registration, config: config, homePath: home.Path, store: store, resolver: resolver, logf: logf}, nil
}

func (c *Component) Type() string { return Type }

func (c *Component) SetSearchMessageSource(source component.SearchMessageSource) {
	if c != nil {
		c.messages = source
	}
}

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{{RelativePath: ComponentConfigFilename, Required: true, Sensitive: false}}
}

func (c *Component) Skill() component.Skill {
	return component.Skill{
		Name:        "semantic-search",
		Description: "Index and search current thread history by meaning using named strategies.",
		Text: strings.TrimSpace(`Use semantic search when exact keywords may not match prior discussion.

Examples:
  hostbridge semantic/local strategy add embedding qwen-embed --embedder llamacpp/local --model qwen3-embed-0.6b
  hostbridge semantic/local index qwen-embed
  hostbridge semantic/local search qwen-embed "database abstraction layer"
  hostbridge semantic/local search qwen-embed "prompt injection email safety" --limit 5
  hostbridge semantic/local stats

Results include exact ctgbot message IDs, scores, and excerpts.`),
	}
}

func (c *Component) resolveEmbedder(ctx context.Context, ref string) (component.Embedder, string, error) {
	if c == nil {
		return nil, "", fmt.Errorf("missing semantic component")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, "", fmt.Errorf("missing embedder ref")
	}
	if c.resolver == nil {
		return nil, ref, fmt.Errorf("missing component resolver")
	}
	registration, err := c.resolver.ResolveComponentRef(ctx, ref)
	if err != nil {
		return nil, ref, err
	}
	loaded, err := c.resolver.ResolveComponent(ctx, registration.ID)
	if err != nil {
		return nil, registration.Ref(), err
	}
	if loaded == nil {
		return nil, registration.Ref(), fmt.Errorf("embedder component not found: %s", registration.Ref())
	}
	embedder, ok := loaded.Component.(component.Embedder)
	if !ok {
		return nil, loaded.Registration.Ref(), fmt.Errorf("component %s does not implement embedder", loaded.Registration.Ref())
	}
	return embedder, loaded.Registration.Ref(), nil
}

func (c *Component) resolveCompletionProvider(ctx context.Context) (component.CompletionProvider, string, error) {
	if c == nil {
		return nil, "", fmt.Errorf("missing semantic component")
	}
	completionRef := strings.TrimSpace(c.config.Completion)
	if completionRef == "" {
		return nil, "", fmt.Errorf("missing semantic completion config")
	}
	if c.resolver == nil {
		return nil, completionRef, fmt.Errorf("missing component resolver")
	}
	registration, err := c.resolver.ResolveComponentRef(ctx, completionRef)
	if err != nil {
		return nil, completionRef, err
	}
	loaded, err := c.resolver.ResolveComponent(ctx, registration.ID)
	if err != nil {
		return nil, registration.Ref(), err
	}
	if loaded == nil {
		return nil, registration.Ref(), fmt.Errorf("completion component not found: %s", registration.Ref())
	}
	provider, ok := loaded.Component.(component.CompletionProvider)
	if !ok {
		return nil, loaded.Registration.Ref(), fmt.Errorf("component %s does not implement completion provider", loaded.Registration.Ref())
	}
	return provider, loaded.Registration.Ref(), nil
}

func (c *Component) log(format string, args ...any) {
	if c != nil && c.logf != nil {
		c.logf(format, args...)
	}
}

func completionResultText(result *component.CompletionResult) string {
	if result == nil || result.Final == nil {
		return ""
	}
	return strings.TrimSpace(result.Final.Text)
}
