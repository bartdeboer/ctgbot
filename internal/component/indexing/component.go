package indexing

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

const Type = "indexing"

type ComponentResolver interface {
	ResolveComponentRef(ctx context.Context, ref string) (*coremodel.Component, error)
	ResolveComponent(ctx context.Context, componentID modeluuid.UUID) (*component.Loaded, error)
}

type Component struct {
	registration coremodel.Component
	profilePath  string
	store        *store
	resolver     ComponentResolver
	messages     component.SearchMessageSource
	logf         func(format string, args ...any)
}

var _ component.Component = (*Component)(nil)
var _ component.ProfileOwner = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)
var _ component.LocalCommandSurface = (*Component)(nil)
var _ component.SearchMessageSourceReceiver = (*Component)(nil)

func New(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, profile runtimepkg.Profile, storage repository.Storage, resolver ComponentResolver, logf func(format string, args ...any)) (component.Component, error) {
	_, _, _ = ctx, runtime, storage
	store, err := openStore(profile.Path)
	if err != nil {
		return nil, err
	}
	return &Component{registration: registration, profilePath: profile.Path, store: store, resolver: resolver, logf: logf}, nil
}

func (c *Component) Type() string { return Type }

func (c *Component) SetSearchMessageSource(source component.SearchMessageSource) {
	if c != nil {
		c.messages = source
	}
}

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{{RelativePath: StoreFilename, Required: false, Sensitive: false}}
}

func (c *Component) resolveEmbeddingEngine(ctx context.Context, ref string) (component.EmbeddingEngine, string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, "", fmt.Errorf("missing embedder ref")
	}
	if c == nil || c.resolver == nil {
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
	embedder, ok := loaded.Component.(component.EmbeddingEngine)
	if !ok {
		return nil, loaded.Registration.Ref(), fmt.Errorf("component %s does not implement embedder", loaded.Registration.Ref())
	}
	return embedder, loaded.Registration.Ref(), nil
}

func (c *Component) resolveCompletionEngine(ctx context.Context, ref string) (component.CompletionEngine, string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, "", fmt.Errorf("missing completion ref")
	}
	if c == nil || c.resolver == nil {
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
		return nil, registration.Ref(), fmt.Errorf("completion component not found: %s", registration.Ref())
	}
	engine, ok := loaded.Component.(component.CompletionEngine)
	if !ok {
		return nil, loaded.Registration.Ref(), fmt.Errorf("component %s does not implement completion engine", loaded.Registration.Ref())
	}
	return engine, loaded.Registration.Ref(), nil
}

func (c *Component) log(format string, args ...any) {
	if c != nil && c.logf != nil {
		c.logf(format, args...)
	}
}
