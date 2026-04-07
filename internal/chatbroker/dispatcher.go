package chatbroker

import (
	"context"
	"sync"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type Dispatcher struct {
	mu    sync.Mutex
	locks map[dispatchKey]*chatLock
}

type dispatchKey struct {
	ChatID   modeluuid.UUID
	ThreadID modeluuid.UUID
}

type chatLock struct {
	mu       sync.Mutex
	refCount int
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		locks: make(map[dispatchKey]*chatLock),
	}
}

func (d *Dispatcher) Run(ctx context.Context, key dispatchKey, fn func(context.Context) error) error {
	lock := d.acquire(key)
	defer d.release(key, lock)

	lock.mu.Lock()
	defer lock.mu.Unlock()

	if fn == nil {
		return nil
	}
	return fn(ctx)
}

func (d *Dispatcher) acquire(key dispatchKey) *chatLock {
	d.mu.Lock()
	defer d.mu.Unlock()

	lock := d.locks[key]
	if lock == nil {
		lock = &chatLock{}
		d.locks[key] = lock
	}
	lock.refCount++
	return lock
}

func (d *Dispatcher) release(key dispatchKey, lock *chatLock) {
	if lock == nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	current := d.locks[key]
	if current != lock {
		return
	}

	current.refCount--
	if current.refCount <= 0 {
		delete(d.locks, key)
	}
}
