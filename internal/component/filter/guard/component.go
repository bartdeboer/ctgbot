package guard

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/inbound"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

type ComponentResolver interface {
	ResolveComponentRef(ctx context.Context, ref string) (*coremodel.Component, error)
	ResolveComponent(ctx context.Context, componentID modeluuid.UUID) (*component.Loaded, error)
}

type Component struct {
	registration coremodel.Component
	resolver     ComponentResolver
	config       ComponentConfig
	logf         func(format string, args ...any)
}

var _ component.Component = (*Component)(nil)
var _ component.ProfileOwner = (*Component)(nil)
var _ component.SkillProvider = (*Component)(nil)
var _ inbound.Filterer = (*Component)(nil)

func New(
	ctx context.Context,
	registration coremodel.Component,
	runtime runtimepkg.Factory,
	profile runtimepkg.Profile,
	storage repository.Storage,
	resolver ComponentResolver,
	logf func(format string, args ...any),
) (component.Component, error) {
	_, _, _ = ctx, runtime, storage
	config, err := loadComponentConfig(profile.Path)
	if err != nil {
		return nil, err
	}
	return &Component{
		registration: registration,
		resolver:     resolver,
		config:       config,
		logf:         logf,
	}, nil
}

func (c *Component) Type() string { return Type }

func (c *Component) InboundFilterPrecedence() int { return filterPrecedence }

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{{RelativePath: ComponentConfigFilename, Required: true, Sensitive: false}}
}

func (c *Component) FilterInbound(ctx context.Context, input inbound.ChannelEvent) (inbound.FilterResult, error) {
	engine, ref, err := c.resolveCompletionEngine(ctx)
	if err != nil {
		c.log("inbound guard unavailable guard=%s err=%v", c.ref(), err)
		return inbound.Quarantine(input, "guard-quarantine", "guard_error="+logValue(err.Error())), nil
	}

	result, err := engine.Complete(ctx, component.CompletionRequest{
		Prompt:          inboundGuardPrompt(filterEventToGuardInput(input)),
		MaxOutputTokens: c.config.MaxOutputTokens,
		ResponseFormat:  "json",
		Reasoning:       component.ReasoningDisabled,
	})
	if err != nil {
		c.log("inbound guard failed guard=%s completion=%s err=%v", c.ref(), ref, err)
		return inbound.Quarantine(input, "guard-quarantine", "guard="+logValue(ref), "guard_error="+logValue(err.Error())), nil
	}

	parsed, err := parseGuardResult(completionResultText(result))
	if err != nil {
		c.log("inbound guard returned invalid output guard=%s completion=%s err=%v", c.ref(), ref, err)
		return inbound.Quarantine(input, "guard-quarantine", "guard="+logValue(ref), "guard_error=invalid-output"), nil
	}

	return parsed.filterResult(input, ref, c.config.HighRiskScore), nil
}

func (c *Component) resolveCompletionEngine(ctx context.Context) (component.CompletionEngine, string, error) {
	if c == nil {
		return nil, "", fmt.Errorf("missing guard component")
	}
	completionRef := strings.TrimSpace(c.config.Completion)
	if completionRef == "" {
		return nil, "", fmt.Errorf("missing guard completion config")
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
	engine, ok := loaded.Component.(component.CompletionEngine)
	if !ok {
		return nil, loaded.Registration.Ref(), fmt.Errorf("component %s does not implement completion engine", loaded.Registration.Ref())
	}
	return engine, loaded.Registration.Ref(), nil
}

func (c *Component) ref() string {
	if c == nil {
		return ""
	}
	return c.registration.Ref()
}

func (c *Component) log(format string, args ...any) {
	if c != nil && c.logf != nil {
		c.logf(format, args...)
	}
}
