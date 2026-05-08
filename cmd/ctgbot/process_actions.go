package main

import (
	"context"
	"log"

	"github.com/bartdeboer/go-clistate"
)

type runtimeProcessActions struct {
	projectProcessActions
}

func newRuntimeProcessActions(globalStore *clistate.Store, stop context.CancelFunc, logger *log.Logger) *runtimeProcessActions {
	return &runtimeProcessActions{
		projectProcessActions: projectProcessActions{
			globalStore: globalStore,
			stop:        stop,
			logger:      logger,
		},
	}
}
