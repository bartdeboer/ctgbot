package messaging

import (
	brokerpkg "github.com/bartdeboer/ctgbot/internal/broker"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

// Service is the core thread-oriented messaging domain service.
//
// Different adapters should call this same service:
//
// - the built-in messaging command surface
// - hostbridge commands
// - remote HTTP clients
// - future web clients
type Service struct {
	Storage repository.Storage
	Broker  *brokerpkg.Broker
}

func New(storage repository.Storage, broker *brokerpkg.Broker) *Service {
	return &Service{
		Storage: storage,
		Broker:  broker,
	}
}
