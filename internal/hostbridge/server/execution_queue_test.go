package server

import (
	"context"
	"errors"
	"testing"
	"time"
)

type acquiredPermit struct {
	position int
	release  func()
}

func TestAliasExecutionQueueAdmitsWaitersFIFO(t *testing.T) {
	queue := NewAliasExecutionQueue()
	releaseActive, err := queue.Acquire(context.Background(), "registry")
	if err != nil {
		t.Fatalf("Acquire(active) error = %v", err)
	}

	acquired := make(chan acquiredPermit, 3)
	for position := 1; position <= 3; position++ {
		position := position
		go func() {
			release, acquireErr := queue.Acquire(context.Background(), "registry")
			if acquireErr != nil {
				acquired <- acquiredPermit{position: -position}
				return
			}
			acquired <- acquiredPermit{position: position, release: release}
		}()
		waitForQueueWaiters(t, queue, "registry", position)
	}

	releaseActive()
	for want := 1; want <= 3; want++ {
		select {
		case permit := <-acquired:
			if permit.position != want {
				t.Fatalf("acquired position = %d, want %d", permit.position, want)
			}
			permit.release()
		case <-time.After(time.Second):
			t.Fatalf("waiter %d was not admitted", want)
		}
	}
}

func TestAliasExecutionQueueCancellationRemovesWaiter(t *testing.T) {
	queue := NewAliasExecutionQueue()
	releaseActive, err := queue.Acquire(context.Background(), "registry")
	if err != nil {
		t.Fatalf("Acquire(active) error = %v", err)
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	canceled := make(chan error, 1)
	go func() {
		_, acquireErr := queue.Acquire(canceledCtx, "registry")
		canceled <- acquireErr
	}()
	waitForQueueWaiters(t, queue, "registry", 1)

	next := make(chan acquiredPermit, 1)
	go func() {
		release, acquireErr := queue.Acquire(context.Background(), "registry")
		if acquireErr != nil {
			next <- acquiredPermit{position: -1}
			return
		}
		next <- acquiredPermit{position: 1, release: release}
	}()
	waitForQueueWaiters(t, queue, "registry", 2)

	cancel()
	if err := <-canceled; !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire(canceled) error = %v, want context.Canceled", err)
	}
	waitForQueueWaiters(t, queue, "registry", 1)

	releaseActive()
	select {
	case permit := <-next:
		if permit.position != 1 {
			t.Fatalf("next waiter failed to acquire: %+v", permit)
		}
		permit.release()
	case <-time.After(time.Second):
		t.Fatal("next waiter did not acquire after cancellation")
	}
}

func TestAliasExecutionQueueUsesIndependentKeys(t *testing.T) {
	queue := NewAliasExecutionQueue()
	releaseA, err := queue.Acquire(context.Background(), "a")
	if err != nil {
		t.Fatalf("Acquire(a) error = %v", err)
	}
	defer releaseA()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	releaseB, err := queue.Acquire(ctx, "b")
	if err != nil {
		t.Fatalf("Acquire(b) error = %v", err)
	}
	releaseB()
}

func TestAliasExecutionQueueCloseWakesWaiters(t *testing.T) {
	queue := NewAliasExecutionQueue()
	releaseActive, err := queue.Acquire(context.Background(), "registry")
	if err != nil {
		t.Fatalf("Acquire(active) error = %v", err)
	}

	waiting := make(chan error, 1)
	go func() {
		_, acquireErr := queue.Acquire(context.Background(), "registry")
		waiting <- acquireErr
	}()
	waitForQueueWaiters(t, queue, "registry", 1)

	queue.Close()
	if err := <-waiting; !errors.Is(err, ErrAliasExecutionQueueClosed) {
		t.Fatalf("Acquire(waiting) error = %v, want queue closed", err)
	}
	if _, err := queue.Acquire(context.Background(), "registry"); !errors.Is(err, ErrAliasExecutionQueueClosed) {
		t.Fatalf("Acquire(after close) error = %v, want queue closed", err)
	}
	releaseActive()
	queue.Close()
}

func waitForQueueWaiters(t *testing.T, queue *AliasExecutionQueue, key string, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		queue.mu.Lock()
		state := queue.keys[key]
		got := 0
		if state != nil {
			got = len(state.waiters)
		}
		queue.mu.Unlock()
		if got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("queue %q did not reach %d waiters", key, want)
}
