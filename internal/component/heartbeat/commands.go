package heartbeat

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/ctgbot/internal/timedintent"
	"github.com/bartdeboer/go-clir"
)

type nowCommand struct{}
type startCommand struct{ Every string }
type startCronCommand struct {
	Expr     string
	Timezone string
	Reason   string
}
type setThreadHeartbeatCommand struct {
	Schedule string
	Reason   string
}
type wakeOnceCommand struct {
	Delay  string
	Reason string
}
type wakeScheduleCommand struct {
	Expr     string
	Timezone string
	Reason   string
}
type wakeListCommand struct{}
type wakeHeartbeatClearCommand struct{}
type wakeOnceClearCommand struct{}
type wakeScheduleClearCommand struct {
	Target string
}
type stopCommand struct{}
type statusCommand struct{}
type tickCommand struct{ ThreadID string }

func RegisterGobTypes(register func(any)) {
	register(nowCommand{})
	register(startCommand{})
	register(startCronCommand{})
	register(setThreadHeartbeatCommand{})
	register(wakeOnceCommand{})
	register(wakeScheduleCommand{})
	register(wakeListCommand{})
	register(wakeHeartbeatClearCommand{})
	register(wakeOnceClearCommand{})
	register(wakeScheduleClearCommand{})
	register(stopCommand{})
	register(statusCommand{})
	register(tickCommand{})
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	userSources := []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge}
	userPolicy := simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent)
	return []commandengine.Definition{
		{
			Pattern:               "now",
			Help:                  "Show the current heartbeat update",
			Build:                 func(req *clir.Request) (any, error) { _ = req; return nowCommand{}, nil },
			Sources:               userSources,
			Policy:                userPolicy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "thread-heartbeat <schedule>",
			Help:    "Set the current thread heartbeat schedule",
			Build:   buildSetThreadHeartbeatCommand,
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  userPolicy,
			Hidden:  true,
			Aliases: []commandengine.Route{
				{Pattern: "thread heartbeat <schedule>", Absolute: true},
			},
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "thread-wake once <delay>",
			Help:    "Set a one-shot wake for the current thread",
			Build:   buildWakeOnceCommand,
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  userPolicy,
			Hidden:  true,
			Aliases: []commandengine.Route{
				{Pattern: "thread wake once <delay>", Absolute: true},
			},
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "thread-wake schedule <expr>",
			Help:    "Set a recurring scheduled wake for the current thread",
			Build:   buildWakeScheduleCommand,
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  userPolicy,
			Hidden:  true,
			Aliases: []commandengine.Route{
				{Pattern: "thread wake schedule <expr>", Absolute: true},
			},
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "thread-wake list",
			Help:    "List current thread wake intents",
			Build:   func(req *clir.Request) (any, error) { _ = req; return wakeListCommand{}, nil },
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  userPolicy,
			Hidden:  true,
			Aliases: []commandengine.Route{
				{Pattern: "thread wake list", Absolute: true},
			},
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "thread-wake heartbeat clear",
			Help:    "Clear the current thread heartbeat",
			Build:   func(req *clir.Request) (any, error) { _ = req; return wakeHeartbeatClearCommand{}, nil },
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  userPolicy,
			Hidden:  true,
			Aliases: []commandengine.Route{
				{Pattern: "thread wake heartbeat clear", Absolute: true},
			},
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "thread-wake once clear",
			Help:    "Clear the current thread one-shot wake",
			Build:   func(req *clir.Request) (any, error) { _ = req; return wakeOnceClearCommand{}, nil },
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  userPolicy,
			Hidden:  true,
			Aliases: []commandengine.Route{
				{Pattern: "thread wake once clear", Absolute: true},
			},
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "thread-wake schedule clear <target>",
			Help:    "Clear current thread scheduled wakes by all or reason",
			Build:   buildWakeScheduleClearCommand,
			Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
			Policy:  userPolicy,
			Hidden:  true,
			Aliases: []commandengine.Route{
				{Pattern: "thread wake schedule clear <target>", Absolute: true},
			},
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "start <interval>",
			Help:                  "Start an interval heartbeat for this thread, for example 30m, 1h, or 2h",
			Build:                 buildStartCommand,
			Sources:               userSources,
			Policy:                userPolicy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "start cron <expr>",
			Help:                  "Start a cron heartbeat for this thread, for example CRON_TZ=Europe/Amsterdam 0 9-17/2 * * 1-5",
			Build:                 buildStartCronCommand,
			Sources:               userSources,
			Policy:                userPolicy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "stop",
			Help:                  "Stop recurring heartbeat messages for this thread",
			Build:                 func(req *clir.Request) (any, error) { _ = req; return stopCommand{}, nil },
			Sources:               userSources,
			Policy:                userPolicy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern:               "status",
			Help:                  "Show the heartbeat schedule for this thread",
			Build:                 func(req *clir.Request) (any, error) { _ = req; return statusCommand{}, nil },
			Sources:               userSources,
			Policy:                userPolicy,
			InstructionVisibility: commandengine.InstructionImportant,
		},
		{
			Pattern: "tick <thread>",
			Help:    "Send a scheduled heartbeat to a thread",
			Build: func(req *clir.Request) (any, error) {
				thread := strings.TrimSpace(req.Params["thread"])
				if thread == "" {
					return nil, fmt.Errorf("missing thread")
				}
				return tickCommand{ThreadID: thread}, nil
			},
			Sources: []commandengine.Source{commandengine.SourceScheduler, commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
			Hidden:  true,
		},
	}
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if err := commandengine.RegisterPattern[nowCommand](registry, "now", c.handleNow); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[startCommand](registry, "start <interval>", c.handleStart); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[startCronCommand](registry, "start cron <expr>", c.handleStartCron); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[setThreadHeartbeatCommand](registry, "thread-heartbeat <schedule>", c.handleSetThreadHeartbeat); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[wakeOnceCommand](registry, "thread-wake once <delay>", c.handleWakeOnce); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[wakeScheduleCommand](registry, "thread-wake schedule <expr>", c.handleWakeSchedule); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[wakeListCommand](registry, "thread-wake list", c.handleWakeList); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[wakeHeartbeatClearCommand](registry, "thread-wake heartbeat clear", c.handleWakeHeartbeatClear); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[wakeOnceClearCommand](registry, "thread-wake once clear", c.handleWakeOnceClear); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[wakeScheduleClearCommand](registry, "thread-wake schedule clear <target>", c.handleWakeScheduleClear); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[stopCommand](registry, "stop", c.handleStop); err != nil {
		return err
	}
	if err := commandengine.RegisterPattern[statusCommand](registry, "status", c.handleStatus); err != nil {
		return err
	}
	return commandengine.RegisterPattern[tickCommand](registry, "tick <thread>", c.handleTick)
}

func buildStartCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("heartbeat start", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	extra := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if extra != "" {
		return nil, fmt.Errorf("unexpected heartbeat start arguments: %s", extra)
	}
	every := strings.TrimSpace(req.Params["interval"])
	if every == "" {
		return nil, fmt.Errorf("missing interval")
	}
	if _, err := time.ParseDuration(every); err != nil {
		return nil, fmt.Errorf("parse interval: %w", err)
	}
	return startCommand{Every: every}, nil
}

func buildStartCronCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("heartbeat start cron", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	timezone := fs.String("tz", "", "cron timezone, for example Europe/Amsterdam")
	reason := fs.String("reason", "", "heartbeat reason shown in wake messages")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	extra := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if extra != "" {
		return nil, fmt.Errorf("unexpected heartbeat start cron arguments: %s", extra)
	}
	expr := strings.TrimSpace(req.Params["expr"])
	if expr == "" {
		return nil, fmt.Errorf("missing cron expression")
	}
	return startCronCommand{Expr: expr, Timezone: strings.TrimSpace(*timezone), Reason: strings.TrimSpace(*reason)}, nil
}

func buildSetThreadHeartbeatCommand(req *clir.Request) (any, error) {
	schedule := strings.TrimSpace(req.Params["schedule"])
	if schedule == "" {
		return nil, fmt.Errorf("missing heartbeat schedule")
	}
	return setThreadHeartbeatCommand{
		Schedule: schedule,
		Reason:   strings.TrimSpace(strings.Join(req.Extra, " ")),
	}, nil
}

func buildWakeOnceCommand(req *clir.Request) (any, error) {
	delay := strings.TrimSpace(req.Params["delay"])
	if delay == "" {
		return nil, fmt.Errorf("missing wake delay")
	}
	reason := strings.TrimSpace(strings.Join(req.Extra, " "))
	if reason == "" {
		return nil, fmt.Errorf("missing wake reason")
	}
	return wakeOnceCommand{Delay: delay, Reason: reason}, nil
}

func buildWakeScheduleCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("thread wake schedule", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	timezone := fs.String("tz", "", "cron timezone, for example Europe/Amsterdam")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	expr := strings.TrimSpace(req.Params["expr"])
	if expr == "" {
		return nil, fmt.Errorf("missing cron expression")
	}
	reason := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if reason == "" {
		return nil, fmt.Errorf("missing scheduled wake reason")
	}
	return wakeScheduleCommand{Expr: expr, Timezone: strings.TrimSpace(*timezone), Reason: reason}, nil
}

func buildWakeScheduleClearCommand(req *clir.Request) (any, error) {
	target := strings.TrimSpace(req.Params["target"])
	if target == "" {
		return nil, fmt.Errorf("missing scheduled wake target")
	}
	if extra := strings.TrimSpace(strings.Join(req.Extra, " ")); extra != "" {
		target = strings.TrimSpace(target + " " + extra)
	}
	return wakeScheduleClearCommand{Target: target}, nil
}

func (c *Component) handleNow(ctx context.Context, req commandengine.Request, cmd nowCommand) (commandengine.Result, error) {
	_ = cmd
	threadID := requestThreadID(req)
	text, err := c.heartbeatText(ctx, threadID, time.Now().UTC())
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: text}, nil
}

func (c *Component) handleStart(ctx context.Context, req commandengine.Request, cmd startCommand) (commandengine.Result, error) {
	if c == nil || c.intents == nil {
		return commandengine.Result{}, fmt.Errorf("missing timed intent repository")
	}
	threadID := requestThreadID(req)
	if threadID.IsNull() {
		return commandengine.Result{}, fmt.Errorf("heartbeat start requires a current thread")
	}
	intent, err := c.timed().StartHeartbeat(ctx, threadID, cmd.Every, req.Context.Actor.Resolved())
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("heartbeat started: every=%s thread_id=%s", intent.Every, threadID)}, nil
}

func (c *Component) handleStartCron(ctx context.Context, req commandengine.Request, cmd startCronCommand) (commandengine.Result, error) {
	if c == nil || c.intents == nil {
		return commandengine.Result{}, fmt.Errorf("missing timed intent repository")
	}
	threadID := requestThreadID(req)
	if threadID.IsNull() {
		return commandengine.Result{}, fmt.Errorf("heartbeat start requires a current thread")
	}
	intent, err := c.timed().StartCronHeartbeat(ctx, threadID, cmd.Expr, cmd.Timezone, cmd.Reason, req.Context.Actor.Resolved())
	if err != nil {
		return commandengine.Result{}, err
	}
	parts := []string{fmt.Sprintf("heartbeat started: cron=%q", intent.Cron)}
	if strings.TrimSpace(intent.Timezone) != "" {
		parts = append(parts, fmt.Sprintf("timezone=%s", intent.Timezone))
	}
	if strings.TrimSpace(intent.Label) != "" && intent.Label != "heartbeat" {
		parts = append(parts, fmt.Sprintf("reason=%q", intent.Label))
	}
	parts = append(parts, fmt.Sprintf("thread_id=%s", threadID))
	return commandengine.Result{Text: strings.Join(parts, " ")}, nil
}

func (c *Component) handleSetThreadHeartbeat(ctx context.Context, req commandengine.Request, cmd setThreadHeartbeatCommand) (commandengine.Result, error) {
	if c == nil || c.intents == nil {
		return commandengine.Result{}, fmt.Errorf("missing timed intent repository")
	}
	threadID := requestThreadID(req)
	if threadID.IsNull() {
		return commandengine.Result{}, fmt.Errorf("thread heartbeat requires a current thread")
	}
	if _, err := time.ParseDuration(cmd.Schedule); err == nil {
		intent, err := c.timed().StartHeartbeat(ctx, threadID, cmd.Schedule, req.Context.Actor.Resolved())
		if err != nil {
			return commandengine.Result{}, err
		}
		return commandengine.Result{Text: fmt.Sprintf("thread heartbeat set: every=%s thread_id=%s", intent.Every, threadID)}, nil
	}
	intent, err := c.timed().StartCronHeartbeat(ctx, threadID, cmd.Schedule, "", cmd.Reason, req.Context.Actor.Resolved())
	if err != nil {
		return commandengine.Result{}, err
	}
	parts := []string{fmt.Sprintf("thread heartbeat set: cron=%q", intent.Cron)}
	if strings.TrimSpace(intent.Label) != "" && intent.Label != "heartbeat" {
		parts = append(parts, fmt.Sprintf("reason=%q", intent.Label))
	}
	parts = append(parts, fmt.Sprintf("thread_id=%s", threadID))
	return commandengine.Result{Text: strings.Join(parts, " ")}, nil
}

func (c *Component) handleWakeOnce(ctx context.Context, req commandengine.Request, cmd wakeOnceCommand) (commandengine.Result, error) {
	if c == nil || c.intents == nil {
		return commandengine.Result{}, fmt.Errorf("missing timed intent repository")
	}
	threadID := requestThreadID(req)
	if threadID.IsNull() {
		return commandengine.Result{}, fmt.Errorf("thread wake requires a current thread")
	}
	intent, err := c.timed().ScheduleWakeOnce(ctx, threadID, cmd.Delay, cmd.Reason, req.Context.Actor.Resolved())
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("thread wake set: once=%s reason=%q thread_id=%s", cmd.Delay, intent.Label, threadID)}, nil
}

func (c *Component) handleWakeSchedule(ctx context.Context, req commandengine.Request, cmd wakeScheduleCommand) (commandengine.Result, error) {
	if c == nil || c.intents == nil {
		return commandengine.Result{}, fmt.Errorf("missing timed intent repository")
	}
	threadID := requestThreadID(req)
	if threadID.IsNull() {
		return commandengine.Result{}, fmt.Errorf("thread wake schedule requires a current thread")
	}
	intent, err := c.timed().ScheduleWakeCron(ctx, threadID, cmd.Expr, cmd.Timezone, cmd.Reason, req.Context.Actor.Resolved())
	if err != nil {
		return commandengine.Result{}, err
	}
	parts := []string{fmt.Sprintf("thread wake schedule set: cron=%q", intent.Cron)}
	if strings.TrimSpace(intent.Timezone) != "" {
		parts = append(parts, fmt.Sprintf("timezone=%s", intent.Timezone))
	}
	parts = append(parts, fmt.Sprintf("reason=%q", intent.Label), fmt.Sprintf("thread_id=%s", threadID))
	return commandengine.Result{Text: strings.Join(parts, " ")}, nil
}

func (c *Component) handleWakeList(ctx context.Context, req commandengine.Request, cmd wakeListCommand) (commandengine.Result, error) {
	_ = cmd
	if c == nil || c.intents == nil {
		return commandengine.Result{}, fmt.Errorf("missing timed intent repository")
	}
	threadID := requestThreadID(req)
	if threadID.IsNull() {
		return commandengine.Result{}, fmt.Errorf("thread wake list requires a current thread")
	}
	intents, err := c.timed().ThreadWakes(ctx, threadID)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: formatWakeList(intents, time.Now().UTC())}, nil
}

func (c *Component) handleWakeHeartbeatClear(ctx context.Context, req commandengine.Request, cmd wakeHeartbeatClearCommand) (commandengine.Result, error) {
	_ = cmd
	if c == nil || c.intents == nil {
		return commandengine.Result{}, fmt.Errorf("missing timed intent repository")
	}
	threadID := requestThreadID(req)
	if threadID.IsNull() {
		return commandengine.Result{}, fmt.Errorf("thread wake heartbeat clear requires a current thread")
	}
	deleted, err := c.timed().StopHeartbeat(ctx, threadID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if !deleted {
		return commandengine.Result{Text: "thread heartbeat is not running"}, nil
	}
	return commandengine.Result{Text: "thread heartbeat cleared"}, nil
}

func (c *Component) handleWakeOnceClear(ctx context.Context, req commandengine.Request, cmd wakeOnceClearCommand) (commandengine.Result, error) {
	_ = cmd
	if c == nil || c.intents == nil {
		return commandengine.Result{}, fmt.Errorf("missing timed intent repository")
	}
	threadID := requestThreadID(req)
	if threadID.IsNull() {
		return commandengine.Result{}, fmt.Errorf("thread wake once clear requires a current thread")
	}
	deleted, err := c.timed().ClearWakeOnce(ctx, threadID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if !deleted {
		return commandengine.Result{Text: "thread one-shot wake is not set"}, nil
	}
	return commandengine.Result{Text: "thread one-shot wake cleared"}, nil
}

func (c *Component) handleWakeScheduleClear(ctx context.Context, req commandengine.Request, cmd wakeScheduleClearCommand) (commandengine.Result, error) {
	if c == nil || c.intents == nil {
		return commandengine.Result{}, fmt.Errorf("missing timed intent repository")
	}
	threadID := requestThreadID(req)
	if threadID.IsNull() {
		return commandengine.Result{}, fmt.Errorf("thread wake schedule clear requires a current thread")
	}
	target := strings.TrimSpace(cmd.Target)
	if strings.EqualFold(target, "all") {
		removed, err := c.timed().ClearAllScheduledWakes(ctx, threadID)
		if err != nil {
			return commandengine.Result{}, err
		}
		return commandengine.Result{Text: fmt.Sprintf("thread scheduled wakes cleared: %d", removed)}, nil
	}
	deleted, err := c.timed().ClearScheduledWake(ctx, threadID, target)
	if err != nil {
		return commandengine.Result{}, err
	}
	if !deleted {
		return commandengine.Result{Text: fmt.Sprintf("thread scheduled wake not found: %s", target)}, nil
	}
	return commandengine.Result{Text: fmt.Sprintf("thread scheduled wake cleared: %s", target)}, nil
}

func (c *Component) handleStop(ctx context.Context, req commandengine.Request, cmd stopCommand) (commandengine.Result, error) {
	_ = cmd
	if c == nil || c.intents == nil {
		return commandengine.Result{}, fmt.Errorf("missing timed intent repository")
	}
	threadID := requestThreadID(req)
	if threadID.IsNull() {
		return commandengine.Result{}, fmt.Errorf("heartbeat stop requires a current thread")
	}
	deleted, err := c.timed().StopHeartbeat(ctx, threadID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if !deleted {
		return commandengine.Result{Text: "heartbeat is not running"}, nil
	}
	return commandengine.Result{Text: "heartbeat stopped"}, nil
}

func (c *Component) handleStatus(ctx context.Context, req commandengine.Request, cmd statusCommand) (commandengine.Result, error) {
	_ = cmd
	if c == nil || c.intents == nil {
		return commandengine.Result{}, fmt.Errorf("missing timed intent repository")
	}
	threadID := requestThreadID(req)
	if threadID.IsNull() {
		return commandengine.Result{}, fmt.Errorf("heartbeat status requires a current thread")
	}
	intent, found, err := c.timed().Heartbeat(ctx, threadID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if !found {
		return commandengine.Result{Text: "heartbeat is not running"}, nil
	}
	return commandengine.Result{Text: formatIntentStatus(*intent)}, nil
}

func (c *Component) handleTick(ctx context.Context, req commandengine.Request, cmd tickCommand) (commandengine.Result, error) {
	_ = req
	threadID, err := parseRequiredThreadID(cmd.ThreadID)
	if err != nil {
		return commandengine.Result{}, err
	}
	text, err := c.heartbeatText(ctx, threadID, time.Now().UTC())
	if err != nil {
		return commandengine.Result{}, err
	}
	if c == nil || c.chatPayloadSender == nil {
		return commandengine.Result{Text: text}, nil
	}
	if err := c.chatPayloadSender.SendPayload(ctx, threadID, message.OutboundPayload{Text: message.TextMessage{Text: text}}); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "heartbeat sent"}, nil
}

func (c *Component) heartbeatText(ctx context.Context, threadID modeluuid.UUID, now time.Time) (string, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	lines := []string{
		"Heartbeat",
		now.UTC().Format("Monday January 2, 2006 15:04 UTC"),
	}
	updates := c.collectUpdates(ctx, threadID)
	if len(updates) == 0 {
		return strings.Join(lines, "\n"), nil
	}
	lines = append(lines, "", "Updates:")
	for _, update := range updates {
		lines = append(lines, "- "+formatUpdate(update))
	}
	return strings.Join(lines, "\n"), nil
}

func (c *Component) collectUpdates(ctx context.Context, threadID modeluuid.UUID) []component.UpdateNotice {
	if c == nil || threadID.IsNull() {
		return nil
	}
	var out []component.UpdateNotice
	for _, feed := range c.updateFeeds {
		if feed == nil {
			continue
		}
		notices, err := feed.NewUpdates(ctx, component.UpdateRequest{ThreadID: threadID})
		if err != nil {
			out = append(out, component.UpdateNotice{Source: "heartbeat", Label: err.Error(), Kind: "error", Count: 1})
			continue
		}
		out = append(out, notices...)
	}
	return out
}

func formatUpdate(update component.UpdateNotice) string {
	label := strings.TrimSpace(update.Label)
	if label == "" {
		label = strings.TrimSpace(update.Ref)
	}
	if label == "" {
		label = strings.TrimSpace(update.Source)
	}
	kind := strings.TrimSpace(update.Kind)
	if kind == "" {
		kind = "update"
	}
	count := update.Count
	if count <= 0 {
		count = 1
	}
	if source := strings.TrimSpace(update.Source); source != "" && label != source {
		return fmt.Sprintf("%s: %s (%d %s)", source, label, count, plural(kind, count))
	}
	return fmt.Sprintf("%s (%d %s)", label, count, plural(kind, count))
}

func plural(word string, count int) string {
	if count == 1 || strings.HasSuffix(word, "s") {
		return word
	}
	return word + "s"
}

func requestThreadID(req commandengine.Request) modeluuid.UUID {
	if !req.Context.ThreadID.IsNull() {
		return req.Context.ThreadID
	}
	return req.Context.SandboxID
}

func parseRequiredThreadID(value string) (modeluuid.UUID, error) {
	id, err := modeluuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return modeluuid.Nil, fmt.Errorf("parse thread id: %w", err)
	}
	if id.IsNull() {
		return modeluuid.Nil, fmt.Errorf("missing thread id")
	}
	return id, nil
}

func formatIntentStatus(intent coremodel.TimedIntent) string {
	lines := []string{
		"heartbeat is running",
		"enabled: " + fmt.Sprintf("%t", intent.Enabled),
		"status: " + firstNonEmpty(intent.LastStatus, coremodel.TimedIntentStatusNever),
	}
	if strings.TrimSpace(intent.Every) != "" {
		lines = append(lines, "every: "+intent.Every)
	}
	if strings.TrimSpace(intent.Cron) != "" {
		lines = append(lines, "cron: "+strings.TrimSpace(intent.Cron))
	}
	if strings.TrimSpace(intent.Timezone) != "" {
		lines = append(lines, "timezone: "+strings.TrimSpace(intent.Timezone))
	}
	if strings.TrimSpace(intent.Label) != "" && intent.Label != "heartbeat" {
		lines = append(lines, "reason: "+strings.TrimSpace(intent.Label))
	}
	if intent.NextDueAt != nil {
		lines = append(lines, "next: "+intent.NextDueAt.UTC().Format(time.RFC3339))
	}
	if intent.LastRunAt != nil {
		lines = append(lines, "last: "+intent.LastRunAt.UTC().Format(time.RFC3339))
	}
	if strings.TrimSpace(intent.LastError) != "" {
		lines = append(lines, "error: "+strings.TrimSpace(intent.LastError))
	}
	return strings.Join(lines, "\n")
}

func formatWakeList(intents []coremodel.TimedIntent, now time.Time) string {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if len(intents) == 0 {
		return "thread wakes\n(no wake intents)"
	}
	lines := []string{"thread wakes"}
	for _, intent := range intents {
		lines = append(lines, "- "+formatWakeIntent(intent, now))
	}
	return strings.Join(lines, "\n")
}

func formatWakeIntent(intent coremodel.TimedIntent, now time.Time) string {
	kind := strings.TrimSpace(intent.Kind)
	switch kind {
	case timedintent.KindHeartbeat:
		kind = "heartbeat"
	case timedintent.KindWake:
		kind = "once"
	case timedintent.KindCron:
		kind = "schedule"
	}
	label := strings.TrimSpace(intent.Label)
	if label == "" {
		label = strings.TrimSpace(intent.Key)
	}
	var parts []string
	parts = append(parts, kind)
	if label != "" && label != kind && label != "heartbeat" {
		parts = append(parts, fmt.Sprintf("%q", label))
	}
	if strings.TrimSpace(intent.Every) != "" {
		parts = append(parts, "every="+strings.TrimSpace(intent.Every))
	}
	if strings.TrimSpace(intent.Cron) != "" {
		parts = append(parts, "cron="+fmt.Sprintf("%q", strings.TrimSpace(intent.Cron)))
	}
	if intent.NextDueAt != nil {
		parts = append(parts, "next="+intent.NextDueAt.UTC().Format(time.RFC3339))
		if d := intent.NextDueAt.UTC().Sub(now.UTC()); d > 0 {
			parts = append(parts, "in="+formatDurationCompact(d))
		}
	}
	if !intent.Enabled {
		parts = append(parts, "disabled")
	}
	return strings.Join(parts, " ")
}

func formatDurationCompact(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	d = d.Round(time.Minute)
	if d < time.Minute {
		return "now"
	}
	hours := int(d / time.Hour)
	minutes := int((d % time.Hour) / time.Minute)
	if hours > 0 && minutes > 0 {
		return fmt.Sprintf("%dh%02dm", hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dm", minutes)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
