// Package component defines ctgbot's capability-based extension seam.
//
// Components are registered units that may expose one or more capabilities.
// For example, a Gmail component may own profile files, emit inbound events,
// and expose hostbridge commands while sharing one authenticated client.
package component

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

// Component is the smallest registered unit in ctgbot.
//
// Type should be a stable identifier such as "gmail", "telegram", or "codex".
type Component interface {
	Type() string
}

type Profiled interface {
	Component
	ProfileName() string
}

func MatchesBinding(component Component, binding coremodel.ChatComponent) bool {
	if component == nil {
		return false
	}
	if strings.TrimSpace(component.Type()) != strings.TrimSpace(binding.ComponentType) {
		return false
	}
	profileName := ComponentProfileName(component)
	return profileName == "" || profileName == strings.TrimSpace(binding.ProfileName)
}

func ComponentProfileName(component Component) string {
	profiled, ok := component.(Profiled)
	if !ok {
		return ""
	}
	return strings.TrimSpace(profiled.ProfileName())
}

// Registry stores components and exposes views by capability.
type Registry struct {
	components []Component
}

func NewRegistry(components ...Component) *Registry {
	r := &Registry{}
	for _, component := range components {
		r.Add(component)
	}
	return r
}

func (r *Registry) Add(component Component) {
	if r == nil || component == nil {
		return
	}
	r.components = append(r.components, component)
}

func (r *Registry) Components() []Component {
	if r == nil || len(r.components) == 0 {
		return nil
	}
	return append([]Component(nil), r.components...)
}

func (r *Registry) EventSources() []EventSource {
	return Capabilities[EventSource](r)
}

func (r *Registry) CommandSurfaces() []CommandSurface {
	return Capabilities[CommandSurface](r)
}

func (r *Registry) ProfileOwners() []ProfileOwner {
	return Capabilities[ProfileOwner](r)
}

func (r *Registry) Authenticators() []Authenticator {
	return Capabilities[Authenticator](r)
}

func (r *Registry) Agents() []Agent {
	return Capabilities[Agent](r)
}

func (r *Registry) OutboundRelays() []OutboundRelay {
	return Capabilities[OutboundRelay](r)
}

// Capabilities returns the registered components that satisfy a capability
// interface.
//
// The capability does not need to be declared in this package. Existing ctgbot
// interfaces, such as messenger.InboundChatProvider, can also be discovered.
func Capabilities[T any](r *Registry) []T {
	if r == nil {
		return nil
	}
	var out []T
	for _, component := range r.components {
		capability, ok := component.(T)
		if ok {
			out = append(out, capability)
		}
	}
	return out
}

// Actor describes the external actor that caused an inbound event.
type Actor struct {
	ID      string
	Label   string
	IsAdmin bool
	Roles   []simplerbac.Role
}

// InboundEvent is an external stimulus that ctgbot can route into a
// canonical conversation thread.
type InboundEvent struct {
	SourceType        string
	SourceProfileName string
	EventType         string
	ExternalID        string

	ProviderChatID   string
	ProviderThreadID string

	ChatID   modeluuid.UUID
	ThreadID modeluuid.UUID

	Actor    Actor
	Text     string
	Metadata map[string]string
}

type InboundEventEmitter func(ctx context.Context, event InboundEvent) error

// EventSource emits external events, such as an incoming Gmail message.
type EventSource interface {
	Component
	RunEvents(ctx context.Context, emit InboundEventEmitter) error
}

type AuthRequest struct {
	ComponentType string
	ProfileName   string

	ProfileHostPath      string
	ProfileContainerPath string

	Image           string
	CallbackPort    int
	CallbackTimeout time.Duration

	SandboxManager sandboxengine.Manager
	Stdout         io.Writer
	Stderr         io.Writer
}

// Authenticator prepares or refreshes a component profile's authentication.
type Authenticator interface {
	Component
	Auth(ctx context.Context, req AuthRequest) error
}

// Agent processes canonical thread messages and may produce an outbound reply.
type Agent interface {
	Component
	HandleMessage(ctx context.Context, message coremodel.ThreadMessage) (*coremodel.ThreadMessage, error)
}

// OutboundRelay projects canonical outbound messages to an external system.
type OutboundRelay interface {
	Component
	SendMessage(ctx context.Context, message coremodel.ThreadMessage) error
}

// CommandSurface contributes typed commands to a commandengine registry.
type CommandSurface interface {
	Component
	CommandDefinitions() []commandengine.Definition
	RegisterCommandHandlers(registry *commandengine.Registry) error
}

// ManagedFile describes a file under a component profile that ctgbot manages.
type ManagedFile struct {
	RelativePath string
	Required     bool
	Sensitive    bool
}

// ProfileOwner declares files that a profile manager should own for a component.
type ProfileOwner interface {
	Component
	ManagedFiles() []ManagedFile
}
