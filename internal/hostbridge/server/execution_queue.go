package server

import (
	"context"
	"errors"
	"strings"
	"sync"
)

var ErrAliasExecutionQueueClosed = errors.New("hostbridge alias execution queue is closed")

// AliasExecutionQueue serializes process execution by operator-defined key.
// It is process-local: every runner that should coordinate must share the same
// queue. Waiting callers are admitted in arrival order.
type AliasExecutionQueue struct {
	mu     sync.Mutex
	closed bool
	keys   map[string]*aliasExecutionState
}

type aliasExecutionState struct {
	active  bool
	waiters []*aliasExecutionWaiter
}

type aliasExecutionWaiter struct {
	granted bool
	result  chan error
}

func NewAliasExecutionQueue() *AliasExecutionQueue {
	return &AliasExecutionQueue{keys: map[string]*aliasExecutionState{}}
}

// Acquire waits for the next position under key. The returned release must be
// called exactly once; it is safe to call more than once defensively.
func (q *AliasExecutionQueue) Acquire(ctx context.Context, key string) (func(), error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return func() {}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if q == nil {
		return nil, ErrAliasExecutionQueueClosed
	}

	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return nil, ErrAliasExecutionQueueClosed
	}
	if q.keys == nil {
		q.keys = map[string]*aliasExecutionState{}
	}
	state := q.keys[key]
	if state == nil {
		state = &aliasExecutionState{}
		q.keys[key] = state
	}
	if !state.active && len(state.waiters) == 0 {
		state.active = true
		q.mu.Unlock()
		return q.releaseFunc(key), nil
	}
	waiter := &aliasExecutionWaiter{result: make(chan error, 1)}
	state.waiters = append(state.waiters, waiter)
	q.mu.Unlock()

	select {
	case err := <-waiter.result:
		if err != nil {
			return nil, err
		}
		q.mu.Lock()
		if q.closed {
			q.releaseLocked(key)
			q.mu.Unlock()
			return nil, ErrAliasExecutionQueueClosed
		}
		q.mu.Unlock()
		return q.releaseFunc(key), nil
	case <-ctx.Done():
		q.mu.Lock()
		if waiter.granted {
			// The permit won the race with cancellation. Hand it to the next
			// waiter instead of leaking the active lane.
			q.releaseLocked(key)
		} else {
			q.removeWaiterLocked(key, waiter)
		}
		q.mu.Unlock()
		return nil, ctx.Err()
	}
}

// Close prevents new acquisition and wakes every queued caller. Active
// executions keep their permits until their normal request cancellation or
// completion path releases them.
func (q *AliasExecutionQueue) Close() {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	for key, state := range q.keys {
		for _, waiter := range state.waiters {
			waiter.result <- ErrAliasExecutionQueueClosed
		}
		state.waiters = nil
		if !state.active {
			delete(q.keys, key)
		}
	}
}

func (q *AliasExecutionQueue) releaseFunc(key string) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			q.mu.Lock()
			q.releaseLocked(key)
			q.mu.Unlock()
		})
	}
}

func (q *AliasExecutionQueue) releaseLocked(key string) {
	state := q.keys[key]
	if state == nil || !state.active {
		return
	}
	if q.closed {
		state.active = false
		delete(q.keys, key)
		return
	}
	if len(state.waiters) == 0 {
		state.active = false
		delete(q.keys, key)
		return
	}
	waiter := state.waiters[0]
	state.waiters = state.waiters[1:]
	waiter.granted = true
	waiter.result <- nil
}

func (q *AliasExecutionQueue) removeWaiterLocked(key string, target *aliasExecutionWaiter) {
	state := q.keys[key]
	if state == nil {
		return
	}
	for index, waiter := range state.waiters {
		if waiter != target {
			continue
		}
		state.waiters = append(state.waiters[:index], state.waiters[index+1:]...)
		break
	}
	if !state.active && len(state.waiters) == 0 {
		delete(q.keys, key)
	}
}
