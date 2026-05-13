package inbound

import (
	"context"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type testFilter struct {
	precedence int
	fn         func(ChannelEvent) FilterResult
}

func (f testFilter) InboundFilterPrecedence() int { return f.precedence }
func (f testFilter) FilterInbound(ctx context.Context, req ChannelEvent) (FilterResult, error) {
	_ = ctx
	if f.fn == nil {
		return Pass(req), nil
	}
	return f.fn(req), nil
}

func TestFilterChainRunsByPrecedenceAndTransforms(t *testing.T) {
	first := testFilter{precedence: 20, fn: func(req ChannelEvent) FilterResult {
		req.Event.Payload.Text.Text += " second"
		return Pass(req)
	}}
	second := testFilter{precedence: 10, fn: func(req ChannelEvent) FilterResult {
		req.Event.Payload.Text.Text += " first"
		return Pass(req)
	}}
	chain, failure, err := NewFilterChain(context.Background(), []Filterer{first, second})
	if err != nil {
		t.Fatalf("NewFilterChain() error = %v", err)
	}
	if failure != nil {
		t.Fatalf("NewFilterChain() failure = %#v", failure)
	}
	result, err := chain.Run(context.Background(), ChannelEvent{Event: component.InboundEvent{ComponentID: modeluuid.New()}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Action != FilterActionPass || result.Event.Payload.Text.Text != " first second" {
		t.Fatalf("result = %#v", result)
	}
}

func TestFilterChainStopsOnDrop(t *testing.T) {
	ranSecond := false
	chain, failure, err := NewFilterChain(context.Background(), []Filterer{
		testFilter{precedence: 10, fn: func(req ChannelEvent) FilterResult { return Drop(req, "blocked") }},
		testFilter{precedence: 20, fn: func(req ChannelEvent) FilterResult { ranSecond = true; return Pass(req) }},
	})
	if err != nil || failure != nil {
		t.Fatalf("NewFilterChain() err=%v failure=%#v", err, failure)
	}
	result, err := chain.Run(context.Background(), ChannelEvent{Event: component.InboundEvent{}})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Action != FilterActionDrop || result.Reason != "blocked" {
		t.Fatalf("result = %#v", result)
	}
	if ranSecond {
		t.Fatal("second filter ran after drop")
	}
}
