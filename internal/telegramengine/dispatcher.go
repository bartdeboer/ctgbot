package telegramengine

import (
	"context"
	"sync"

	"github.com/bartdeboer/go-codextgbot/internal/chatmodel"
)

type Dispatcher struct {
	mu    sync.Mutex
	locks map[chatmodel.ChatKey]*chatLock
}

type chatLock struct {
	mu       sync.Mutex
	refCount int
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		locks: make(map[chatmodel.ChatKey]*chatLock),
	}
}

func (d *Dispatcher) Run(ctx context.Context, key chatmodel.ChatKey, fn func(context.Context) error) error {
	lock := d.acquire(key)
	defer d.release(key, lock)

	lock.mu.Lock()
	defer lock.mu.Unlock()

	if fn == nil {
		return nil
	}
	return fn(ctx)
}

func (d *Dispatcher) acquire(key chatmodel.ChatKey) *chatLock {
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

func (d *Dispatcher) release(key chatmodel.ChatKey, lock *chatLock) {
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
