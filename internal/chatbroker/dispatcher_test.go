package chatbroker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestDispatcherSerializesSameChat(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()
	key := dispatchKey{ChatID: 1, ThreadID: 0}

	started := make(chan struct{}, 2)
	releaseFirst := make(chan struct{})
	secondEntered := atomic.Bool{}
	done := make(chan struct{})

	go func() {
		_ = d.Run(context.Background(), key, func(context.Context) error {
			started <- struct{}{}
			<-releaseFirst
			return nil
		})
	}()

	<-started

	go func() {
		_ = d.Run(context.Background(), key, func(context.Context) error {
			secondEntered.Store(true)
			close(done)
			return nil
		})
	}()

	time.Sleep(50 * time.Millisecond)
	if secondEntered.Load() {
		t.Fatalf("second handler entered before first finished")
	}

	close(releaseFirst)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for second handler")
	}
}

func TestDispatcherAllowsDifferentChats(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()

	firstKey := dispatchKey{ChatID: 1, ThreadID: 0}
	secondKey := dispatchKey{ChatID: 2, ThreadID: 0}

	firstStarted := make(chan struct{})
	secondDone := make(chan struct{})
	releaseFirst := make(chan struct{})

	go func() {
		_ = d.Run(context.Background(), firstKey, func(context.Context) error {
			close(firstStarted)
			<-releaseFirst
			return nil
		})
	}()

	<-firstStarted

	go func() {
		_ = d.Run(context.Background(), secondKey, func(context.Context) error {
			close(secondDone)
			return nil
		})
	}()

	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatalf("different chat was blocked by first chat")
	}

	close(releaseFirst)
}
