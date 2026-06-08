package agentcommon

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type RefreshContainer struct{}
type StartContainer struct{}
type StopContainer struct{}
type PurgeChat struct{}
type InterruptTurn struct{}
type Status struct{}
type Compact struct{ Text string }
type Goal struct{ Text string }

func RegisterGobTypes(register func(any)) {
	register(RefreshContainer{})
	register(StartContainer{})
	register(StopContainer{})
	register(PurgeChat{})
	register(InterruptTurn{})
	register(Status{})
	register(Compact{})
	register(Goal{})
}

type KeepRunningSetter interface {
	SetKeepRunning(ctx context.Context, thread *coremodel.Thread, keepRunning *bool) error
}

type AgentCommandOptions struct {
	Name          string
	HiddenAliases map[string]string
}

func AgentCommandSources() []commandengine.Source {
	return []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge}
}

func AgentCommandPolicy() simplerbac.Rule {
	return simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser)
}

func AgentCommandDescriptions(name string) []commandengine.Description {
	return []commandengine.Description{{
		Pattern: "",
		Help:    "agent lifecycle and config",
		Sources: AgentCommandSources(),
		Policy:  AgentCommandPolicy(),
	}}
}

func AgentCommandDefinitions(opts AgentCommandOptions) []commandengine.Definition {
	type entry struct {
		pattern string
		command any
		help    string
		build   func(*clir.Request) (any, error)
	}
	entries := []entry{
		{pattern: "container refresh", command: RefreshContainer{}, help: fmt.Sprintf("Delete and recreate the %s runtime on next turn", opts.Name)},
		{pattern: "container start", command: StartContainer{}, help: fmt.Sprintf("Start the %s runtime container", opts.Name)},
		{pattern: "container stop", command: StopContainer{}, help: fmt.Sprintf("Stop the %s runtime container but keep its data", opts.Name)},
		{pattern: "chat purge", command: PurgeChat{}, help: fmt.Sprintf("Reset the %s conversation and delete the runtime container", opts.Name)},
		{pattern: "compact", command: Compact{}, help: fmt.Sprintf("Ask %s to compact its current provider conversation", opts.Name), build: buildCompactCommand},
		{pattern: "goal", command: Goal{}, help: fmt.Sprintf("Ask %s to show or update its current provider goal", opts.Name), build: buildGoalCommand},
		{pattern: "interrupt", command: InterruptTurn{}, help: fmt.Sprintf("Interrupt the active %s turn", opts.Name)},
		{pattern: "status", command: Status{}, help: fmt.Sprintf("Show %s conversation and runtime status", opts.Name)},
	}
	definitions := make([]commandengine.Definition, 0, len(entries))
	for _, e := range entries {
		e := e
		build := e.build
		if build == nil {
			build = func(_ *clir.Request) (any, error) { return e.command, nil }
		}
		def := commandengine.Definition{
			Pattern:               e.pattern,
			Help:                  e.help,
			Build:                 build,
			Sources:               agentCommandSourcesFor(e.pattern),
			Policy:                AgentCommandPolicy(),
			InstructionVisibility: agentInstructionVisibility(e.pattern),
		}
		if alias, ok := opts.HiddenAliases[e.pattern]; ok {
			def.Aliases = []commandengine.Route{{Pattern: alias, Hidden: true}}
		}
		definitions = append(definitions, def)
	}
	return definitions
}

func buildCompactCommand(req *clir.Request) (any, error) {
	return Compact{Text: strings.TrimSpace(strings.Join(req.Extra, " "))}, nil
}

func buildGoalCommand(req *clir.Request) (any, error) {
	return Goal{Text: strings.TrimSpace(strings.Join(req.Extra, " "))}, nil
}

func (c *Core) RegisterAgentCommandHandlers(
	registry *commandengine.Registry,
	componentType string,
	ks KeepRunningSetter,
	statusFn func(context.Context, commandengine.Request) (commandengine.Result, error),
) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := commandengine.RegisterPattern[RefreshContainer](registry, "container refresh", func(ctx context.Context, req commandengine.Request, _ RefreshContainer) (commandengine.Result, error) {
		return c.agentRefresh(ctx, req, componentType)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[StartContainer](registry, "container start", func(ctx context.Context, req commandengine.Request, _ StartContainer) (commandengine.Result, error) {
		return c.agentStart(ctx, req, componentType, ks)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[StopContainer](registry, "container stop", func(ctx context.Context, req commandengine.Request, _ StopContainer) (commandengine.Result, error) {
		return c.agentStop(ctx, req, componentType, ks)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[PurgeChat](registry, "chat purge", func(ctx context.Context, req commandengine.Request, _ PurgeChat) (commandengine.Result, error) {
		return c.agentPurge(ctx, req, componentType, ks)
	}); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[InterruptTurn](registry, "interrupt", func(ctx context.Context, req commandengine.Request, _ InterruptTurn) (commandengine.Result, error) {
		return c.agentInterrupt(ctx, req, componentType)
	}); err != nil {
		return err
	}
	return commandengine.RegisterPattern[Status](registry, "status", func(ctx context.Context, req commandengine.Request, _ Status) (commandengine.Result, error) {
		return statusFn(ctx, req)
	})
}

func (c *Core) agentRefresh(ctx context.Context, req commandengine.Request, componentType string) (commandengine.Result, error) {
	thread, workspacePath, err := ThreadWorkspace(ctx, c.Storage, c.ResolveWorkspace, req, componentType)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := c.Runtime.Refresh(ctx, workspacePath, thread.ID); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "runtime refreshed"}, nil
}

func (c *Core) agentStart(ctx context.Context, req commandengine.Request, componentType string, ks KeepRunningSetter) (commandengine.Result, error) {
	thread, workspacePath, err := ThreadWorkspace(ctx, c.Storage, c.ResolveWorkspace, req, componentType)
	if err != nil {
		return commandengine.Result{}, err
	}
	status, err := c.Runtime.Start(ctx, workspacePath, thread.ID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := ks.SetKeepRunning(ctx, thread, boolPtr(true)); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("container started\nkeep_running: true\ncontainer: %s\nstate: %s", status.Name, status.State)}, nil
}

func (c *Core) agentStop(ctx context.Context, req commandengine.Request, componentType string, ks KeepRunningSetter) (commandengine.Result, error) {
	thread, workspacePath, err := ThreadWorkspace(ctx, c.Storage, c.ResolveWorkspace, req, componentType)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := c.Runtime.Stop(ctx, workspacePath, thread.ID); err != nil {
		return commandengine.Result{}, err
	}
	if err := ks.SetKeepRunning(ctx, thread, nil); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "container stopped\nkeep_running: false"}, nil
}

func (c *Core) agentPurge(ctx context.Context, req commandengine.Request, componentType string, ks KeepRunningSetter) (commandengine.Result, error) {
	thread, workspacePath, err := ThreadWorkspace(ctx, c.Storage, c.ResolveWorkspace, req, componentType)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := c.Runtime.Refresh(ctx, workspacePath, thread.ID); err != nil {
		return commandengine.Result{}, err
	}
	if err := ks.SetKeepRunning(ctx, thread, nil); err != nil {
		return commandengine.Result{}, err
	}
	if err := c.Storage.ThreadComponentMappings().DeleteByThreadAndComponent(ctx, thread.ID, c.Registration.ID); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "conversation purged"}, nil
}

func (c *Core) agentInterrupt(ctx context.Context, req commandengine.Request, componentType string) (commandengine.Result, error) {
	thread, workspacePath, err := ThreadWorkspace(ctx, c.Storage, c.ResolveWorkspace, req, componentType)
	if err != nil {
		return commandengine.Result{}, err
	}
	ok, err := c.Runtime.Interrupt(ctx, workspacePath, thread.ID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if !ok {
		return commandengine.Result{Text: "no active run to interrupt"}, nil
	}
	return commandengine.Result{Text: "interrupt requested"}, nil
}

func agentCommandSourcesFor(pattern string) []commandengine.Source {
	switch commandengine.NormalizePattern(pattern) {
	case "container refresh":
		return []commandengine.Source{commandengine.SourceMessage}
	default:
		return AgentCommandSources()
	}
}

func agentInstructionVisibility(pattern string) commandengine.InstructionVisibility {
	switch commandengine.NormalizePattern(pattern) {
	case "container refresh", "container start", "container stop", "chat purge", "compact", "goal", "interrupt", "status":
		return commandengine.InstructionImportant
	default:
		return commandengine.InstructionDiscoverable
	}
}

func boolPtr(v bool) *bool { return &v }
