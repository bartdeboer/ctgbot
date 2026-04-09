package main

import (
	"context"
	"log"
	"time"
)

type processActions struct {
	stop   context.CancelFunc
	logger *log.Logger
}

func (p *processActions) Upgrade(ctx context.Context) error {
	p.logf("running ctgbot upgrade from telegram")
	return runInstalledCtgbotCommand(ctx, "upgrade")
}

func (p *processActions) Quit(ctx context.Context) error {
	p.logf("shutting down ctgbot from telegram")
	if p.stop == nil {
		return nil
	}
	time.AfterFunc(250*time.Millisecond, p.stop)
	return nil
}

func (p *processActions) logf(format string, args ...any) {
	if p.logger != nil {
		p.logger.Printf(format, args...)
	}
}
