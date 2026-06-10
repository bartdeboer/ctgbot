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
type stopCommand struct{}
type statusCommand struct{}
type tickCommand struct{ ThreadID string }

func RegisterGobTypes(register func(any)) {
	register(nowCommand{})
	register(startCommand{})
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
			Pattern:               "start <interval>",
			Help:                  "Start recurring heartbeat messages for this thread",
			Build:                 buildStartCommand,
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
	service := timedintent.New(c.intents, nil, nil, nil)
	intent, err := service.StartHeartbeat(ctx, threadID, cmd.Every, req.Context.Actor.Resolved())
	if err != nil {
		return commandengine.Result{}, err
	}
	if c.jobs != nil {
		_, _ = c.jobs.DeleteByName(ctx, jobName(threadID))
	}
	return commandengine.Result{Text: fmt.Sprintf("heartbeat started: every=%s thread_id=%s", intent.Every, threadID)}, nil
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
	service := timedintent.New(c.intents, nil, nil, nil)
	deleted, err := service.StopHeartbeat(ctx, threadID)
	if err != nil {
		return commandengine.Result{}, err
	}
	legacyDeleted := false
	if c.jobs != nil {
		legacyDeleted, err = c.jobs.DeleteByName(ctx, jobName(threadID))
		if err != nil {
			return commandengine.Result{}, err
		}
	}
	if !deleted && !legacyDeleted {
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
	service := timedintent.New(c.intents, nil, nil, nil)
	intent, found, err := service.Heartbeat(ctx, threadID)
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

func jobName(threadID modeluuid.UUID) string {
	return "heartbeat:" + threadID.String()
}

func formatIntentStatus(intent coremodel.TimedIntent) string {
	lines := []string{
		"heartbeat is running",
		"every: " + intent.Every,
		"enabled: " + fmt.Sprintf("%t", intent.Enabled),
		"status: " + firstNonEmpty(intent.LastStatus, coremodel.TimedIntentStatusNever),
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
