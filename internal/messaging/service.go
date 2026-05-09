package messaging

import (
	"github.com/bartdeboer/ctgbot/internal/repository"
)

// Service is the core thread-oriented messaging read/query domain service.
//
// Different adapters should call this same service:
//
// - the built-in messaging command surface
// - hostbridge commands
// - remote HTTP clients
// - future web clients
type Service struct {
	Storage repository.Storage
}

func New(storage repository.Storage) *Service {
	return &Service{
		Storage: storage,
	}
}
