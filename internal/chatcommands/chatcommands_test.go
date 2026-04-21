package chatcommands

import (
	"context"
	"testing"

	clir "github.com/bartdeboer/go-clir"
)

func TestHandleRunsRegisteredCommand(t *testing.T) {
	cmds := New()
	cmds.Routes(func(b *clir.Builder) {
		b.Handle("ping", "", func(req *clir.Request) error {
			return Reply(req, "pong")
		})
	})

	result, err := cmds.Handle(context.Background(), Request{Command: "ping"})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Text != "pong" {
		t.Fatalf("result.Text = %q, want pong", result.Text)
	}
}

func TestHandleExposesOriginalRequestInContext(t *testing.T) {
	cmds := New()
	cmds.Routes(func(b *clir.Builder) {
		b.Handle("config set <key> <value>", "", func(req *clir.Request) error {
			chatReq, ok := RequestFrom(req.Context())
			if !ok {
				t.Fatalf("RequestFrom() = false")
			}
			if chatReq.SandboxID != "sbx-1" {
				t.Fatalf("SandboxID = %q, want sbx-1", chatReq.SandboxID)
			}
			return Reply(req, req.Params["key"]+"="+req.Params["value"])
		})
	})

	result, err := cmds.Handle(context.Background(), Request{
		Command:   "config",
		Args:      []string{"set", "alpha", "beta"},
		SandboxID: "sbx-1",
	})
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result.Text != "alpha=beta" {
		t.Fatalf("result.Text = %q, want alpha=beta", result.Text)
	}
}

func TestHandleRequiresCommand(t *testing.T) {
	cmds := New()
	_, err := cmds.Handle(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected error")
	}
}
