package clisetter

import (
	"context"
	"testing"

	"github.com/bartdeboer/go-clir"
)

type testSetters struct {
	dockerImage string
	chatID      string
	enabled     bool
	alias       string
	dir         string
}

func (t *testSetters) SetDockerImage(in struct {
	Image string `flag:"set-docker-image"`
}) error {
	t.dockerImage = in.Image
	return nil
}

type testChatEnabledInput struct {
	ChatID     string `arg:"chatID" segment:"chat"`
	SetEnabled bool   `flag:"set-enabled"`
}

func (t *testSetters) SetChatEnabled(in testChatEnabledInput) error {
	t.chatID = in.ChatID
	t.enabled = in.SetEnabled
	return nil
}

type testChatAliasDirInput struct {
	ChatID string `arg:"chatID" segment:"chat"`
	Alias  string `arg:"alias" segment:"hostbridge"`
	SetDir string `flag:"set-dir"`
}

func (t *testSetters) SetChatHostbridgeAliasDir(in testChatAliasDirInput) error {
	t.chatID = in.ChatID
	t.alias = in.Alias
	t.dir = in.SetDir
	return nil
}

func TestRegisterRoutes_RootSetter(t *testing.T) {
	r := clir.New()
	target := &testSetters{}

	r.Routes(func(b *clir.Builder) {
		b.Route("config", func(b *clir.Builder) {
			if err := New(target).RegisterRoutes(b); err != nil {
				t.Fatalf("RegisterRoutes error: %v", err)
			}
		})
	})

	if err := r.Run(context.Background(), []string{"config", "--set-docker-image", "ctgbot:latest"}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if target.dockerImage != "ctgbot:latest" {
		t.Fatalf("expected docker image to be set, got %q", target.dockerImage)
	}
}

func TestRegisterRoutes_RouteStructSetter(t *testing.T) {
	r := clir.New()
	target := &testSetters{}

	r.Routes(func(b *clir.Builder) {
		b.Route("config", func(b *clir.Builder) {
			if err := New(target).RegisterRoutes(b); err != nil {
				t.Fatalf("RegisterRoutes error: %v", err)
			}
		})
	})

	if err := r.Run(context.Background(), []string{"config", "chat", "abc", "--set-enabled", "true"}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if target.chatID != "abc" || !target.enabled {
		t.Fatalf("expected chat enabled call, got chatID=%q enabled=%t", target.chatID, target.enabled)
	}
}

func TestRegisterRoutes_MultiSegmentRouteStructSetter(t *testing.T) {
	r := clir.New()
	target := &testSetters{}

	r.Routes(func(b *clir.Builder) {
		b.Route("config", func(b *clir.Builder) {
			if err := New(target).RegisterRoutes(b); err != nil {
				t.Fatalf("RegisterRoutes error: %v", err)
			}
		})
	})

	if err := r.Run(context.Background(), []string{"config", "chat", "abc", "hostbridge", "origin", "--set-dir", "/repo"}); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if target.chatID != "abc" || target.alias != "origin" || target.dir != "/repo" {
		t.Fatalf("unexpected target state chatID=%q alias=%q dir=%q", target.chatID, target.alias, target.dir)
	}
}
