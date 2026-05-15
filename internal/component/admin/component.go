package admin

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

const Type = "component"

// Component is the global component-administration command surface.
//
// Hostbridge also supports active dynamic component-ref dispatch such as:
//
//	hostbridge gmail/work message "hello"
//
// That dynamic path is the preferred agent-facing shape for commands exposed by
// bound components. The literal "component" namespace remains for explicit
// component administration and setup operations.

type Resolver interface {
	ResolveComponent(ctx context.Context, componentID modeluuid.UUID) (*component.Loaded, error)
}

type Component struct {
	Storage  repository.Storage
	Resolver Resolver
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)

type HelpCommand struct{}

type ListCommand struct{}

type ComponentHelpCommand struct {
	Component string
}

type AuthStatusCommand struct {
	Component string
}

type ManagedFileListCommand struct {
	Component string
}

type ManagedFileStatusCommand struct {
	Component string
}

type ManagedFilePutCommand struct {
	Component   string
	File        string
	ContentType string
	Content     []byte
}

// ComponentMessageCommand is the standard component-direct send command.
//
// It intentionally uses one command shape for active component refs such as
// "gmail/work message ..." and the explicit administration namespace
// "component gmail/work message ...". Component implementations may interpret
// fields that make sense for their transport; the common fields cover email-ish
// components today while keeping attachments on the shared message.Media model.
type ComponentMessageCommand struct {
	Component   string
	To          []string
	Cc          []string
	Bcc         []string
	Subject     string
	Body        string
	ContentType string
	Syntax      string
	Attachments []message.Media
	ThreadID    string
	InReplyTo   string
}

func RegisterGobTypes(register func(any)) {
	register(HelpCommand{})
	register(ListCommand{})
	register(ComponentHelpCommand{})
	register(AuthStatusCommand{})
	register(ManagedFileListCommand{})
	register(ManagedFileStatusCommand{})
	register(ManagedFilePutCommand{})
	register(ComponentMessageCommand{})
}

func New(storage repository.Storage, resolver Resolver) *Component {
	return &Component{Storage: storage, Resolver: resolver}
}

func (c *Component) Type() string { return Type }

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		componentCommand("component help", "Show component command help", func(req *clir.Request) (any, error) {
			_ = req
			return HelpCommand{}, nil
		}, componentReadSources(), commandengine.InstructionEssential),
		componentCommand("component list", "List registered components", func(req *clir.Request) (any, error) {
			_ = req
			return ListCommand{}, nil
		}, componentReadSources(), commandengine.InstructionImportant),
		componentCommand("component <component> help", "Show component-specific help", buildComponentHelp, componentReadSources(), commandengine.InstructionEssential),
		componentCommand("component <component> auth status", "Show component auth status", buildAuthStatus, []commandengine.Source{commandengine.SourceHostbridge}, commandengine.InstructionDiscoverable),
		componentCommand("component <component> managed-file list", "List declared managed files", buildManagedFileList, componentReadSources(), commandengine.InstructionDiscoverable),
		componentCommand("component <component> managed-file status", "Show managed file presence", buildManagedFileStatus, componentReadSources(), commandengine.InstructionDiscoverable),
		componentCommand("component <component> managed-file put <file>", "Write a declared managed file from stdin", buildManagedFilePut, []commandengine.Source{commandengine.SourceHostbridge}, commandengine.InstructionDiscoverable),
		hiddenComponentCommand("component <component> message <text>", "Send a message through a component", buildComponentMessage, []commandengine.Source{commandengine.SourceHostbridge}),
	}
}

func componentReadSources() []commandengine.Source {
	return []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge}
}

func componentCommand(pattern string, help string, build commandengine.BuildFunc, sources []commandengine.Source, visibility commandengine.InstructionVisibility) commandengine.Definition {
	return commandengine.Definition{
		Pattern:               pattern,
		Help:                  help,
		Build:                 build,
		Sources:               sources,
		Policy:                simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
		InstructionVisibility: visibility,
	}
}

func hiddenComponentCommand(pattern string, help string, build commandengine.BuildFunc, sources []commandengine.Source) commandengine.Definition {
	definition := componentCommand(pattern, help, build, sources, commandengine.InstructionHidden)
	definition.Hidden = true
	return definition
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := commandengine.Register[HelpCommand](registry, c.handleHelp); err != nil {
		return err
	}
	if err := commandengine.Register[ListCommand](registry, c.handleList); err != nil {
		return err
	}
	if err := commandengine.Register[ComponentHelpCommand](registry, c.handleComponentHelp); err != nil {
		return err
	}
	if err := commandengine.Register[AuthStatusCommand](registry, c.handleAuthStatus); err != nil {
		return err
	}
	if err := commandengine.Register[ManagedFileListCommand](registry, c.handleManagedFileList); err != nil {
		return err
	}
	if err := commandengine.Register[ManagedFileStatusCommand](registry, c.handleManagedFileStatus); err != nil {
		return err
	}
	if err := commandengine.Register[ManagedFilePutCommand](registry, c.handleManagedFilePut); err != nil {
		return err
	}
	return commandengine.Register[ComponentMessageCommand](registry, c.handleComponentMessage)
}

func (c *Component) handleHelp(ctx context.Context, req commandengine.Request, cmd HelpCommand) (commandengine.Result, error) {
	_ = ctx
	_ = cmd
	return commandengine.Result{Text: FormatHelp(definitionsForSource(c.CommandDefinitions(), req.Context.Source))}, nil
}

func (c *Component) handleList(ctx context.Context, req commandengine.Request, cmd ListCommand) (commandengine.Result, error) {
	_ = req
	_ = cmd
	if c == nil || c.Storage == nil {
		return commandengine.Result{}, fmt.Errorf("missing component storage")
	}
	components, err := c.Storage.Components().ListEnabled(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(components) == 0 {
		return commandengine.Result{Text: "no registered components"}, nil
	}
	sort.Slice(components, func(i, j int) bool { return components[i].Ref() < components[j].Ref() })
	lines := make([]string, 0, len(components))
	for _, registration := range components {
		lines = append(lines, fmt.Sprintf("%s\truntime=%s\tdefault=%t", registration.Ref(), registration.Runtime, registration.IsDefault))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (c *Component) handleComponentHelp(ctx context.Context, req commandengine.Request, cmd ComponentHelpCommand) (commandengine.Result, error) {
	_ = req
	loaded, err := c.resolveLoaded(ctx, cmd.Component)
	if err != nil {
		return commandengine.Result{}, err
	}
	if provider, ok := loaded.Component.(component.SkillProvider); ok {
		skill := provider.Skill()
		text := strings.TrimSpace(skill.Text)
		if text != "" {
			return commandengine.Result{Text: text}, nil
		}
		if strings.TrimSpace(skill.Description) != "" {
			return commandengine.Result{Text: strings.TrimSpace(skill.Description)}, nil
		}
	}
	return commandengine.Result{Text: fmt.Sprintf("component %s has no component-specific help", loaded.Registration.Ref())}, nil
}

func (c *Component) handleAuthStatus(ctx context.Context, req commandengine.Request, cmd AuthStatusCommand) (commandengine.Result, error) {
	_ = req
	loaded, err := c.resolveLoaded(ctx, cmd.Component)
	if err != nil {
		return commandengine.Result{}, err
	}
	reporter, ok := loaded.Component.(component.AuthStatusReporter)
	if !ok {
		return commandengine.Result{}, fmt.Errorf("component does not support auth status: %s", loaded.Registration.Ref())
	}
	var stdout, stderr bytes.Buffer
	if err := reporter.AuthStatus(ctx, &stdout, &stderr); err != nil {
		text := strings.TrimSpace(stderr.String())
		if text != "" {
			return commandengine.Result{}, fmt.Errorf("%s: %w", text, err)
		}
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: strings.TrimSpace(stdout.String())}, nil
}

func (c *Component) handleManagedFileList(ctx context.Context, req commandengine.Request, cmd ManagedFileListCommand) (commandengine.Result, error) {
	_ = req
	_, files, err := c.managedFiles(ctx, cmd.Component)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: formatManagedFileList(files)}, nil
}

func (c *Component) handleManagedFileStatus(ctx context.Context, req commandengine.Request, cmd ManagedFileStatusCommand) (commandengine.Result, error) {
	_ = req
	loaded, files, err := c.managedFiles(ctx, cmd.Component)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: formatManagedFileStatus(loaded.Home.Path, files)}, nil
}

func (c *Component) handleManagedFilePut(ctx context.Context, req commandengine.Request, cmd ManagedFilePutCommand) (commandengine.Result, error) {
	_ = req
	loaded, files, err := c.managedFiles(ctx, cmd.Component)
	if err != nil {
		return commandengine.Result{}, err
	}
	file, err := declaredManagedFile(files, cmd.File)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := writeManagedFile(loaded.Home.Path, file, cmd.Content); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "managed file written: " + file.RelativePath}, nil
}

func (c *Component) handleComponentMessage(ctx context.Context, req commandengine.Request, cmd ComponentMessageCommand) (commandengine.Result, error) {
	_ = req
	loaded, err := c.resolveLoaded(ctx, cmd.Component)
	if err != nil {
		return commandengine.Result{}, err
	}
	sender, ok := loaded.Component.(component.MessageSender)
	if !ok {
		return commandengine.Result{}, fmt.Errorf("component does not support message sending: %s", loaded.Registration.Ref())
	}
	result, err := sender.SendMessage(ctx, component.MessageSendRequest{
		To:          append([]string(nil), cmd.To...),
		Cc:          append([]string(nil), cmd.Cc...),
		Bcc:         append([]string(nil), cmd.Bcc...),
		Subject:     strings.TrimSpace(cmd.Subject),
		Body:        cmd.Body,
		ContentType: strings.TrimSpace(cmd.ContentType),
		Syntax:      strings.TrimSpace(cmd.Syntax),
		Attachments: append([]message.Media(nil), cmd.Attachments...),
		ThreadID:    strings.TrimSpace(cmd.ThreadID),
		InReplyTo:   strings.TrimSpace(cmd.InReplyTo),
	})
	if err != nil {
		return commandengine.Result{}, err
	}
	var lines []string
	lines = append(lines, "message sent")
	if strings.TrimSpace(result.ID) != "" {
		lines = append(lines, "id: "+strings.TrimSpace(result.ID))
	}
	if strings.TrimSpace(result.ThreadID) != "" {
		lines = append(lines, "thread_id: "+strings.TrimSpace(result.ThreadID))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (c *Component) managedFiles(ctx context.Context, ref string) (*component.Loaded, []component.ManagedFile, error) {
	loaded, err := c.resolveLoaded(ctx, ref)
	if err != nil {
		return nil, nil, err
	}
	owner, ok := loaded.Component.(component.ProfileOwner)
	if !ok {
		return nil, nil, fmt.Errorf("component has no managed files: %s", loaded.Registration.Ref())
	}
	return loaded, owner.ManagedFiles(), nil
}

func (c *Component) resolveLoaded(ctx context.Context, ref string) (*component.Loaded, error) {
	if c == nil || c.Storage == nil {
		return nil, fmt.Errorf("missing component storage")
	}
	if c.Resolver == nil {
		return nil, fmt.Errorf("missing component resolver")
	}
	registration, err := resolveRegistrationRef(ctx, c.Storage, ref)
	if err != nil {
		return nil, err
	}
	loaded, err := c.Resolver.ResolveComponent(ctx, registration.ID)
	if err != nil {
		return nil, err
	}
	if loaded == nil {
		return nil, fmt.Errorf("component unavailable: %s", registration.Ref())
	}
	return loaded, nil
}

func resolveRegistrationRef(ctx context.Context, storage repository.Storage, ref string) (*coremodel.Component, error) {
	if storage == nil {
		return nil, fmt.Errorf("missing storage")
	}
	parsed, err := coremodel.ParseComponentRef(ref)
	if err != nil {
		return nil, err
	}
	if !parsed.ExplicitName {
		registration, err := storage.Components().GetDefaultByType(ctx, parsed.Type)
		if err != nil {
			return nil, err
		}
		if registration != nil {
			return registration, nil
		}
	}
	registration, err := storage.Components().GetByTypeAndName(ctx, parsed.Type, parsed.ResolvedName())
	if err != nil {
		return nil, err
	}
	if registration == nil {
		return nil, fmt.Errorf("component not registered: %s", parsed.Ref())
	}
	return registration, nil
}

// MessageSenderSurface exposes the standard component-direct message command as
// a local command surface for a concrete component ref. It provides the active
// ref UX, e.g. "gmail/work message ..."; the global component admin surface
// keeps the explicit "component gmail/work message ..." fallback.
type MessageSenderSurface struct {
	ComponentRef string
}

var _ component.Component = (*MessageSenderSurface)(nil)
var _ component.CommandSurface = (*MessageSenderSurface)(nil)
var _ component.LocalCommandSurface = (*MessageSenderSurface)(nil)

func NewMessageSenderSurface(componentRef string) *MessageSenderSurface {
	return &MessageSenderSurface{ComponentRef: strings.TrimSpace(componentRef)}
}

func (s *MessageSenderSurface) Type() string { return "component-message" }

func (s *MessageSenderSurface) UsesLocalCommandRoutes() bool { return true }

func (s *MessageSenderSurface) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		componentCommand("message <text>", "Send a message through this component", s.buildComponentMessage, []commandengine.Source{commandengine.SourceHostbridge}, commandengine.InstructionDiscoverable),
	}
}

func (s *MessageSenderSurface) RegisterCommandHandlers(registry *commandengine.Registry) error {
	// ComponentMessageCommand is handled by the global component admin surface. This
	// local surface only adds active, prefixed routes such as gmail/work message.
	_ = registry
	return nil
}

func (s *MessageSenderSurface) buildComponentMessage(req *clir.Request) (any, error) {
	command, err := buildComponentMessageForRef(req, s.ComponentRef)
	if err != nil {
		return nil, err
	}
	cmd, ok := command.(ComponentMessageCommand)
	if !ok {
		return nil, fmt.Errorf("message command type mismatch: %T", command)
	}
	cmd.Component = strings.TrimSpace(s.ComponentRef)
	return cmd, nil
}
