package broker

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

type fakeActions struct {
	threadID modeluuid.UUID
	payload  message.OutboundPayload
}

func (f *fakeActions) SendPayload(ctx context.Context, threadID modeluuid.UUID, payload message.OutboundPayload) error {
	_ = ctx
	f.threadID = threadID
	f.payload = payload
	return nil
}

func (f *fakeActions) RunHostbridgeCommand(ctx context.Context, req commandengine.Request, cmd schemacommands.RunCommand) (commandengine.Result, error) {
	_, _, _ = ctx, req, cmd
	return commandengine.Result{}, nil
}

func (f *fakeActions) MessageHelp(ctx context.Context, chatID modeluuid.UUID, actor commandengine.Actor) (string, error) {
	_ = actor
	_, _ = ctx, chatID
	return "", nil
}

func (f *fakeActions) RefreshThreadRuntime(ctx context.Context, threadID modeluuid.UUID) (string, error) {
	_, _ = ctx, threadID
	return "runtime refreshed", nil
}

func (f *fakeActions) DroppedList(ctx context.Context, limit int) (string, error) {
	_, _ = ctx, limit
	return "dropped messages", nil
}

func (f *fakeActions) DroppedView(ctx context.Context, ref string) (string, error) {
	_, _ = ctx, ref
	return "dropped message", nil
}

func (f *fakeActions) DroppedAllow(ctx context.Context, ref string) (string, error) {
	_, _ = ctx, ref
	return "dropped message replayed", nil
}

func TestSendPayloadUsesCurrentThread(t *testing.T) {
	actions := &fakeActions{}
	component := New(actions)
	threadID := modeluuid.New()
	payload := message.OutboundPayload{
		Text: message.TextMessage{Text: "hello"},
		Attachments: []message.Media{{
			Filename: "note.txt",
			Content:  []byte("note"),
		}},
	}

	err := component.sendPayload(context.Background(), commandengine.Request{
		Context: commandengine.Context{ThreadID: threadID},
	}, payload)
	if err != nil {
		t.Fatalf("sendPayload() error = %v", err)
	}
	if actions.threadID != threadID {
		t.Fatalf("threadID = %s, want %s", actions.threadID, threadID)
	}
	if actions.payload.Text.Text != "hello" || len(actions.payload.Attachments) != 1 || string(actions.payload.Attachments[0].Content) != "note" {
		t.Fatalf("payload = %#v, want copied payload", actions.payload)
	}
}

func TestSendPayloadUsesRequestStdinWhenPayloadEmpty(t *testing.T) {
	actions := &fakeActions{}
	component := New(actions)
	threadID := modeluuid.New()

	err := component.sendPayload(context.Background(), commandengine.Request{
		Context: commandengine.Context{ThreadID: threadID},
		Stdin:   "hello from stdin\n",
	}, message.OutboundPayload{})
	if err != nil {
		t.Fatalf("sendPayload() error = %v", err)
	}
	if actions.threadID != threadID {
		t.Fatalf("threadID = %s, want %s", actions.threadID, threadID)
	}
	if got, want := actions.payload.Text.Text, "hello from stdin"; got != want {
		t.Fatalf("payload text = %q, want %q", got, want)
	}
}
