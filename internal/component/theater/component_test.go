package theater

import (
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestTheaterCreateSubscribePostReadFlow(t *testing.T) {
	ctx := context.Background()
	component := newTestComponent(t)
	engine := newTestEngine(t, component)
	threadID := modeluuid.New()
	base := testRequest(threadID)

	if result, err := engine.Run(ctx, base, []string{Type, "create", "qwen-parser-lab", "--workspace", "/tmp/qwen-parser-lab"}); err != nil || result.Text != "theater created: qwen-parser-lab" {
		t.Fatalf("create result=%q err=%v", result.Text, err)
	}
	if result, err := engine.Run(ctx, base, []string{Type, "qwen-parser-lab", "subscribe"}); err != nil || result.Text != "subscribed: qwen-parser-lab" {
		t.Fatalf("subscribe result=%q err=%v", result.Text, err)
	}
	if _, err := engine.Run(ctx, base, []string{Type, "qwen-parser-lab", "post", "parser", "image", "ready"}); err != nil {
		t.Fatalf("post error = %v", err)
	}

	updates, err := component.NewUpdates(ctx, componentUpdateRequest(threadID))
	if err != nil {
		t.Fatalf("NewUpdates() error = %v", err)
	}
	if len(updates) != 1 || updates[0].Source != Type || updates[0].Label != "qwen-parser-lab" || updates[0].Count != 1 {
		t.Fatalf("updates = %#v, want one theater update", updates)
	}
	status, err := engine.Run(ctx, base, []string{Type, "qwen-parser-lab", "status"})
	if err != nil {
		t.Fatalf("named status error = %v", err)
	}
	if !strings.Contains(status.Text, "unread messages: 1") {
		t.Fatalf("named status = %q, want pending count", status.Text)
	}
	if !strings.Contains(status.Text, "workspace: /tmp/qwen-parser-lab") {
		t.Fatalf("named status = %q, want workspace path", status.Text)
	}

	read, err := engine.Run(ctx, base, []string{Type, "qwen-parser-lab", "read"})
	if err != nil {
		t.Fatalf("read error = %v", err)
	}
	if !strings.Contains(read.Text, "tester: parser image ready") {
		t.Fatalf("read = %q, want posted message", read.Text)
	}
	updates, err = component.NewUpdates(ctx, componentUpdateRequest(threadID))
	if err != nil {
		t.Fatalf("NewUpdates after read error = %v", err)
	}
	if len(updates) != 0 {
		t.Fatalf("updates after read = %#v, want none", updates)
	}
}

func TestTheaterPostUsesStdin(t *testing.T) {
	ctx := context.Background()
	component := newTestComponent(t)
	engine := newTestEngine(t, component)
	threadID := modeluuid.New()
	base := testRequest(threadID)
	if _, err := engine.Run(ctx, base, []string{Type, "create", "lab"}); err != nil {
		t.Fatal(err)
	}
	postReq := base
	postReq.Stdin = "hello from stdin"
	if _, err := engine.Run(ctx, postReq, []string{Type, "lab", "post"}); err != nil {
		t.Fatalf("post stdin error = %v", err)
	}
	read, err := engine.Run(ctx, base, []string{Type, "lab", "read"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read.Text, "hello from stdin") {
		t.Fatalf("read = %q, want stdin message", read.Text)
	}
}

func TestTheaterThreadBindingReceivesTurnsAndRelaysToOtherBoards(t *testing.T) {
	ctx := context.Background()
	component := newTestComponent(t)
	sender := &fakeSender{}
	component.SetChatPayloadSender(sender)
	engine := newTestEngine(t, component)
	originThreadID := modeluuid.New()
	peerThreadID := modeluuid.New()
	base := testRequest(originThreadID)

	if _, err := engine.Run(ctx, base, []string{Type, "create", "lab"}); err != nil {
		t.Fatal(err)
	}
	if result, err := engine.Run(ctx, base, []string{Type, "lab", "thread", "bind"}); err != nil || result.Text != "thread bound to theater: lab" {
		t.Fatalf("bind origin result=%q err=%v", result.Text, err)
	}
	if _, err := engine.Run(ctx, testRequest(peerThreadID), []string{Type, "lab", "thread", "bind"}); err != nil {
		t.Fatalf("bind peer error = %v", err)
	}

	result, err := component.HandleTurn(ctx, componentTurn(originThreadID, "parser image ready"))
	if err != nil {
		t.Fatalf("HandleTurn() error = %v", err)
	}
	if result != nil {
		t.Fatalf("HandleTurn() result = %#v, want nil", result)
	}
	read, err := engine.Run(ctx, base, []string{Type, "lab", "read"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read.Text, "tester: parser image ready") {
		t.Fatalf("read = %q, want agent-posted message", read.Text)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sent = %#v, want one relay to peer", sender.sent)
	}
	if sender.sent[0].threadID != peerThreadID {
		t.Fatalf("relay thread = %s, want %s", sender.sent[0].threadID, peerThreadID)
	}
	if !strings.Contains(sender.sent[0].payload.Text.Text, "theater: lab\n") || !strings.Contains(sender.sent[0].payload.Text.Text, "tester: parser image ready") {
		t.Fatalf("relay text = %q, want theater message", sender.sent[0].payload.Text.Text)
	}
}

func newTestComponent(t *testing.T) *Component {
	t.Helper()
	store, err := openStore(t.TempDir())
	if err != nil {
		t.Fatalf("openStore() error = %v", err)
	}
	return &Component{registration: coremodel.Component{Type: Type, Name: Type}, store: store}
}

func newTestEngine(t *testing.T, c *Component) *commandengine.Engine {
	t.Helper()
	engine, err := commandset.NewBoundEngineForSource(commandengine.SourceMessage, []commandset.BoundSurface{{Surface: c, ComponentRef: Type, ComponentType: Type}})
	if err != nil {
		t.Fatalf("NewBoundEngineForSource() error = %v", err)
	}
	return engine
}

func testRequest(threadID modeluuid.UUID) commandengine.Request {
	return commandengine.Request{Context: commandengine.Context{Source: commandengine.SourceMessage, ThreadID: threadID, Actor: coremodel.Actor{ID: "tester", Label: "tester", Roles: []simplerbac.Role{simplerbac.RoleAgent}}}}
}

func componentUpdateRequest(threadID modeluuid.UUID) component.UpdateRequest {
	return component.UpdateRequest{ThreadID: threadID}
}

func componentTurn(threadID modeluuid.UUID, text string) component.Turn {
	return component.Turn{
		Thread: coremodel.Thread{ID: threadID},
		Inbound: coremodel.ThreadMessage{
			ThreadID:   threadID,
			ActorID:    "tester",
			ActorLabel: "tester",
			Text:       text,
		},
	}
}

type sentPayload struct {
	threadID modeluuid.UUID
	payload  message.OutboundPayload
}

type fakeSender struct{ sent []sentPayload }

func (f *fakeSender) SendPayload(ctx context.Context, threadID modeluuid.UUID, payload message.OutboundPayload) error {
	_ = ctx
	f.sent = append(f.sent, sentPayload{threadID: threadID, payload: payload})
	return nil
}
