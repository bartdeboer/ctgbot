package broker

import (
	"context"
	"fmt"
	"sync"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type ThreadTurnGate struct {
	mu    sync.Mutex
	gates map[modeluuid.UUID]*threadTurnSemaphore
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

	gate := g.retain(threadID)
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

func (g *ThreadTurnGate) retain(threadID modeluuid.UUID) *threadTurnSemaphore {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.gates == nil {
		g.gates = map[modeluuid.UUID]*threadTurnSemaphore{}
	}
	gate := g.gates[threadID]
	if gate == nil {
		gate = &threadTurnSemaphore{sem: make(chan struct{}, 1)}
		g.gates[threadID] = gate
	}
	gate.refs++
	return gate
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
