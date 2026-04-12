package main

import (
	"context"
	"log"
	"time"
)

type runtimeProcessActions struct {
	stop    context.CancelFunc
	upgrade func(context.Context) error
	logger  *log.Logger
}

func (p *runtimeProcessActions) Upgrade(ctx context.Context) error {
	p.logf("running ctgbot upgrade from telegram")
	if p == nil || p.upgrade == nil {
		return nil
	}
	return p.upgrade(ctx)
}

func (p *runtimeProcessActions) Quit(ctx context.Context) error {
	_ = ctx
	p.logf("shutting down ctgbot from telegram")
	if p == nil || p.stop == nil {
		return nil
	}
	time.AfterFunc(250*time.Millisecond, p.stop)
	return nil
}

func (p *runtimeProcessActions) logf(format string, args ...any) {
	if p != nil && p.logger != nil {
		p.logger.Printf(format, args...)
	}
}
