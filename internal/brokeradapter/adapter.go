package brokeradapter

import (
	appsvc "github.com/bartdeboer/ctgbot/internal/app"
	"github.com/bartdeboer/ctgbot/internal/broker"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

func NewWithDeps(storage repository.Storage, resolver appsvc.ComponentResolver, logf func(format string, args ...any)) *broker.Broker {
	return broker.New(appsvc.NewServiceWithLogger(storage, resolver, logf), logf)
}
