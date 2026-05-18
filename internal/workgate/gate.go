package workgate

import (
	"context"
	"sync"
)

type Gate struct {
	mu    sync.Mutex
	lanes map[string]*lane
}

type lane struct {
	sem chan struct{}
}

func New() *Gate {
	return &Gate{lanes: map[string]*lane{}}
}

func (g *Gate) Acquire(ctx context.Context, key string, limit int) (func(), error) {
	if limit <= 0 {
		return func() {}, nil
	}
	if g == nil {
		return func() {}, nil
	}
	item := g.lane(key, limit)
	select {
	case item.sem <- struct{}{}:
		var once sync.Once
		return func() {
			once.Do(func() { <-item.sem })
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (g *Gate) lane(key string, limit int) *lane {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.lanes == nil {
		g.lanes = map[string]*lane{}
	}
	item := g.lanes[key]
	if item == nil {
		item = &lane{sem: make(chan struct{}, limit)}
		g.lanes[key] = item
	}
	return item
}
