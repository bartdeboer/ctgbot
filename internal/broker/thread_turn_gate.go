package broker

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

var ErrTurnGateClosed = errors.New("turn gate is shutting down")

type ThreadTurnGate struct {
	mu      sync.Mutex
	gates   map[modeluuid.UUID]*threadTurnSemaphore
	closing bool
}

type threadTurnSemaphore struct {
	sem  chan struct{}
	refs int
}

func NewThreadTurnGate() *ThreadTurnGate {
	return &ThreadTurnGate{gates: map[modeluuid.UUID]*threadTurnSemaphore{}}
}

func (g *ThreadTurnGate) Run(ctx context.Context, threadID modeluuid.UUID, fn func() error) error {
	if fn == nil {
		return nil
	}
	if g == nil {
		return fn()
	}
	release, err := g.Acquire(ctx, threadID)
	if err != nil {
		return err
	}
	defer release()
	return fn()
}

func (g *ThreadTurnGate) Acquire(ctx context.Context, threadID modeluuid.UUID) (func(), error) {
	if g == nil {
		return func() {}, nil
	}
	if threadID.IsNull() {
		return nil, fmt.Errorf("missing thread id")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	gate, err := g.retain(threadID)
	if err != nil {
		return nil, err
	}
	select {
	case gate.sem <- struct{}{}:
		return func() {
			<-gate.sem
			g.release(threadID, gate)
		}, nil
	case <-ctx.Done():
		g.release(threadID, gate)
		return nil, ctx.Err()
	}
}

// BeginShutdown atomically closes turn admission when the gate is idle.
// Force closes admission regardless of work already holding or waiting for a
// scope; cancellation remains the runtime owner's responsibility.
func (g *ThreadTurnGate) BeginShutdown(force bool) (outstanding int, accepted bool) {
	if g == nil {
		return 0, true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, gate := range g.gates {
		if gate != nil {
			outstanding += gate.refs
		}
	}
	if outstanding > 0 && !force {
		return outstanding, false
	}
	if g.closing {
		return outstanding, true
	}
	g.closing = true
	return outstanding, true
}

func (g *ThreadTurnGate) Busy(threadID modeluuid.UUID) bool {
	if g == nil || threadID.IsNull() {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	gate := g.gates[threadID]
	return gate != nil && len(gate.sem) > 0
}

func (g *ThreadTurnGate) retain(threadID modeluuid.UUID) (*threadTurnSemaphore, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closing {
		return nil, ErrTurnGateClosed
	}
	if g.gates == nil {
		g.gates = map[modeluuid.UUID]*threadTurnSemaphore{}
	}
	gate := g.gates[threadID]
	if gate == nil {
		gate = &threadTurnSemaphore{sem: make(chan struct{}, 1)}
		g.gates[threadID] = gate
	}
	gate.refs++
	return gate, nil
}

func (g *ThreadTurnGate) release(threadID modeluuid.UUID, gate *threadTurnSemaphore) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.gates == nil || gate == nil {
		return
	}
	gate.refs--
	if gate.refs <= 0 && len(gate.sem) == 0 {
		delete(g.gates, threadID)
	}
}
