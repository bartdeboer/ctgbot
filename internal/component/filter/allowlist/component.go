package allowlist

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/inbound"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

const (
	Type = "filters"
	Name = "allowlist"
)

const dropIDPlaceholder = "{{drop_id}}"

const filterPrecedence = 1000

type Component struct {
	Storage repository.Storage
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)
var _ inbound.Filterer = (*Component)(nil)

type droppedViewCommand struct {
	DropRef string
}

type whitelistAddCommand struct {
	Sender string
}

type whitelistListCommand struct{}

type whitelistRemoveCommand struct {
	Sender string
}

func New(storage repository.Storage) *Component {
	return &Component{Storage: storage}
}

func (c *Component) Type() string { return Type }

func (c *Component) InboundFilterPrecedence() int { return filterPrecedence }

func RegisterGobTypes(register func(any)) {
	register(droppedViewCommand{})
	register(whitelistAddCommand{})
	register(whitelistListCommand{})
	register(whitelistRemoveCommand{})
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		{
			Pattern: "allowlist dropped view <dropID>",
			Help:    "View a dropped inbound event",
			Build: func(req *clir.Request) (any, error) {
				return droppedViewCommand{DropRef: strings.TrimSpace(req.Params["dropID"])}, nil
			},
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		},
		{
			Pattern: "allowlist whitelist list",
			Help:    "List senders in the current allowlist",
			Build: func(req *clir.Request) (any, error) {
				if extra := strings.TrimSpace(strings.Join(req.Extra, " ")); extra != "" {
					return nil, fmt.Errorf("unexpected allowlist whitelist list arguments: %s", extra)
				}
				return whitelistListCommand{}, nil
			},
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		},
		{
			Pattern: "allowlist whitelist remove <sender>",
			Help:    "Remove a sender from the current allowlist",
			Build: func(req *clir.Request) (any, error) {
				return whitelistRemoveCommand{Sender: strings.TrimSpace(req.Params["sender"])}, nil
			},
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		},
		{
			Pattern: "allowlist whitelist <sender>",
			Help:    "Add a sender to the current allowlist",
			Build: func(req *clir.Request) (any, error) {
				return whitelistAddCommand{Sender: strings.TrimSpace(req.Params["sender"])}, nil
			},
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		},
	}
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := commandengine.Register[droppedViewCommand](registry, c.handleDroppedView); err != nil {
		return err
	}
	if err := commandengine.Register[whitelistAddCommand](registry, c.handleWhitelistAdd); err != nil {
		return err
	}
	if err := commandengine.Register[whitelistListCommand](registry, c.handleWhitelistList); err != nil {
		return err
	}
	return commandengine.Register[whitelistRemoveCommand](registry, c.handleWhitelistRemove)
}

func (c *Component) FilterInbound(ctx context.Context, input inbound.ChannelEvent) (inbound.FilterResult, error) {
	if c == nil || c.Storage == nil {
		return inbound.Quarantine(input, "allowlist-missing-storage"), nil
	}
	if input.Channel.SourceBinding.ID.IsNull() {
		return inbound.Quarantine(input, "allowlist-missing-source-binding"), nil
	}
	senderKey, senderLabel := inbound.SenderIdentity(input.Event.Payload)
	allowed, err := c.Storage.AllowlistSenders().GetBySourceBindingAndSenderKey(ctx, input.Channel.SourceBinding.ID, senderKey)
	if err != nil {
		return inbound.FilterResult{}, err
	}
	if allowed != nil {
		return inbound.Pass(input), nil
	}
	notice := unknownSenderNotice(input.Event, senderKey, senderLabel)
	return inbound.DropWithNotice(
		input,
		"allowlist-unknown-sender",
		notice,
		"sender_key="+senderKey,
		"sender_label="+senderLabel,
	), nil
}

func IsRegistration(registration *coremodel.Component) bool {
	return registration != nil &&
		strings.TrimSpace(registration.Type) == Type &&
		strings.TrimSpace(registration.Name) == Name
}
