package main

import (
	"context"
	"testing"

	"github.com/bartdeboer/go-clir"
)

func TestCLIRPrefersLongerRouteOverPrefixRoute(t *testing.T) {
	router := clir.New()
	hit := ""
	router.Routes(func(b *clir.Builder) {
		b.Handle("config", "root", func(req *clir.Request) error {
			hit = "root"
			return nil
		})
		b.Handle("config set <key> <value>", "set", func(req *clir.Request) error {
			hit = "set:" + req.Params["key"] + "=" + req.Params["value"]
			return nil
		})
	})

	if err := router.Run(context.Background(), []string{"config", "set", "docker.image", "ctgbot:test"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if hit != "set:docker.image=ctgbot:test" {
		t.Fatalf("hit = %q, want longer set route", hit)
	}
}

func TestCLIRPrefersLiteralSegmentOverParameterSegment(t *testing.T) {
	router := clir.New()
	hit := ""
	router.Routes(func(b *clir.Builder) {
		b.Handle("config <scope> <id>", "generic scope", func(req *clir.Request) error {
			hit = "generic:" + req.Params["scope"]
			return nil
		})
		b.Handle("config chat <chatID>", "chat scope", func(req *clir.Request) error {
			hit = "chat:" + req.Params["chatID"]
			return nil
		})
	})

	if err := router.Run(context.Background(), []string{"config", "chat", "abc"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if hit != "chat:abc" {
		t.Fatalf("hit = %q, want literal chat route", hit)
	}
}

func TestCLIRCanDistinguishFutureThreadScopedConfigRoute(t *testing.T) {
	router := clir.New()
	hit := ""
	router.Routes(func(b *clir.Builder) {
		b.Handle("config chat <chatID> set <key> <value>", "chat set", func(req *clir.Request) error {
			hit = "chat:" + req.Params["chatID"]
			return nil
		})
		b.Handle("config chat <chatID> thread <threadID> set <key> <value>", "thread set", func(req *clir.Request) error {
			hit = "thread:" + req.Params["chatID"] + "/" + req.Params["threadID"]
			return nil
		})
	})

	if err := router.Run(context.Background(), []string{"config", "chat", "chat-1", "thread", "thread-9", "set", "thread.enabled", "true"}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if hit != "thread:chat-1/thread-9" {
		t.Fatalf("hit = %q, want thread scoped route", hit)
	}
}
