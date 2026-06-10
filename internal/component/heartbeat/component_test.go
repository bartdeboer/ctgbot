package heartbeat

import (
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	componentpkg "github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestHeartbeatStartCreatesTimedIntentForCurrentThread(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	c := newTestComponent(storage, nil)
	engine := newTestEngine(t, c, commandengine.SourceMessage)
	threadID := modeluuid.New()

	result, err := engine.Run(ctx, testRequest(threadID), []string{Type, "start", "15m"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(result.Text, "heartbeat started") {
		t.Fatalf("result = %q, want started", result.Text)
	}

	intents, err := storage.TimedIntents().ListByTarget(ctx, threadID)
	if err != nil {
		t.Fatalf("ListByTarget() error = %v", err)
	}
	if len(intents) != 1 {
		t.Fatalf("intents len = %d, want 1", len(intents))
	}
	if got, want := intents[0].Kind+":"+intents[0].Key, "heartbeat:default"; got != want {
		t.Fatalf("intent identity = %q, want %q", got, want)
	}
	if got, want := intents[0].Every, "15m"; got != want {
		t.Fatalf("intent every = %q, want %q", got, want)
	}
	if intents[0].NextDueAt == nil {
		t.Fatalf("intent next due is nil")
	}
}

func TestHeartbeatTickSendsPayloadToThread(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	sender := &fakeSender{}
	c := newTestComponent(storage, sender)
	engine := newTestEngine(t, c, commandengine.SourceScheduler)
	threadID := modeluuid.New()

	result, err := engine.Run(ctx, commandengine.Request{Context: commandengine.Context{Source: commandengine.SourceScheduler, Actor: coremodel.Actor{ID: "scheduler", Roles: []simplerbac.Role{simplerbac.RoleRoot}}}}, []string{Type, "tick", threadID.String()})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Text != "heartbeat sent" {
		t.Fatalf("result = %q, want heartbeat sent", result.Text)
	}
	if sender.threadID != threadID {
		t.Fatalf("sent thread = %s, want %s", sender.threadID, threadID)
	}
	if !strings.HasPrefix(sender.payload.Text.Text, "Heartbeat\n") {
		t.Fatalf("payload = %q, want heartbeat", sender.payload.Text.Text)
	}
}

func TestHeartbeatNowIncludesUpdateFeeds(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	c := newTestComponent(storage, nil)
	c.updateFeeds = []componentpkg.UpdateFeed{fakeFeed{notice: componentpkg.UpdateNotice{Source: "theater", Label: "qwen-parser-lab", Kind: "message", Count: 2}}}
	engine := newTestEngine(t, c, commandengine.SourceMessage)
	threadID := modeluuid.New()

	result, err := engine.Run(ctx, testRequest(threadID), []string{Type, "now"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !strings.Contains(result.Text, "Updates:\n- theater: qwen-parser-lab (2 messages)") {
		t.Fatalf("result = %q, want theater update", result.Text)
	}
}

func newTestComponent(storage *repository.MemoryStorage, sender componentpkg.ChatPayloadSender) *Component {
	return &Component{registration: coremodel.Component{Type: Type, Name: Type}, intents: storage.TimedIntents(), jobs: storage.ScheduledJobs(), chatPayloadSender: sender}
}

func newTestEngine(t *testing.T, c *Component, source commandengine.Source) *commandengine.Engine {
	t.Helper()
	engine, err := commandset.NewBoundEngineForSource(source, []commandset.BoundSurface{{Surface: c, ComponentRef: Type, ComponentType: Type}})
	if err != nil {
		t.Fatalf("NewBoundEngineForSource() error = %v", err)
	}
	return engine
}

func testRequest(threadID modeluuid.UUID) commandengine.Request {
	return commandengine.Request{Context: commandengine.Context{Source: commandengine.SourceMessage, ThreadID: threadID, Actor: coremodel.Actor{ID: "agent", Roles: []simplerbac.Role{simplerbac.RoleAgent}}}}
}

type fakeSender struct {
	threadID modeluuid.UUID
	payload  message.OutboundPayload
}

func (f *fakeSender) SendPayload(ctx context.Context, threadID modeluuid.UUID, payload message.OutboundPayload) error {
	_ = ctx
	f.threadID = threadID
	f.payload = payload
	return nil
}

type fakeFeed struct{ notice componentpkg.UpdateNotice }

func (f fakeFeed) NewUpdates(ctx context.Context, req componentpkg.UpdateRequest) ([]componentpkg.UpdateNotice, error) {
	_ = ctx
	if req.ThreadID.IsNull() {
		return nil, nil
	}
	return []componentpkg.UpdateNotice{f.notice}, nil
}
