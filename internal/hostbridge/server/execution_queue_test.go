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
	err      error
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

func TestAliasExecutionQueueCancelRacingWithGrantKeepsLaneReusable(t *testing.T) {
	for round := 0; round < 100; round++ {
		queue := NewAliasExecutionQueue()
		_, err := queue.Acquire(context.Background(), "registry")
		if err != nil {
			t.Fatalf("round %d: Acquire(active) error = %v", round, err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		first := make(chan acquiredPermit, 1)
		go func() {
			release, acquireErr := queue.Acquire(ctx, "registry")
			first <- acquiredPermit{release: release, err: acquireErr}
		}()
		waitForQueueWaiters(t, queue, "registry", 1)

		next := make(chan acquiredPermit, 1)
		go func() {
			release, acquireErr := queue.Acquire(context.Background(), "registry")
			next <- acquiredPermit{release: release, err: acquireErr}
		}()
		waitForQueueWaiters(t, queue, "registry", 2)

		// Cancellation wakes the waiter while q.mu is held. Grant it before
		// unlocking so the selected cancellation branch observes granted=true
		// and must hand the lane to the next waiter.
		queue.mu.Lock()
		cancel()
		queue.releaseLocked("registry")
		queue.mu.Unlock()

		firstResult := <-first
		if !errors.Is(firstResult.err, context.Canceled) {
			t.Fatalf("round %d: first Acquire() error = %v, want context.Canceled", round, firstResult.err)
		}
		nextResult := <-next
		if nextResult.err != nil {
			t.Fatalf("round %d: next Acquire() error = %v", round, nextResult.err)
		}
		nextResult.release()
		assertQueueKeyDrained(t, queue, "registry")
	}
}

func TestAliasExecutionQueueCloseBetweenGrantAndReceiveRefusesWaiter(t *testing.T) {
	queue := NewAliasExecutionQueue()
	_, err := queue.Acquire(context.Background(), "registry")
	if err != nil {
		t.Fatalf("Acquire(active) error = %v", err)
	}
	waiting := make(chan error, 1)
	go func() {
		_, acquireErr := queue.Acquire(context.Background(), "registry")
		waiting <- acquireErr
	}()
	waitForQueueWaiters(t, queue, "registry", 1)

	queue.mu.Lock()
	queue.releaseLocked("registry")
	queue.closeLocked()
	queue.mu.Unlock()

	if err := <-waiting; !errors.Is(err, ErrAliasExecutionQueueClosed) {
		t.Fatalf("Acquire() error = %v, want queue closed", err)
	}
	assertQueueKeyDrained(t, queue, "registry")
}

func TestAliasExecutionQueueDuplicateReleaseDoesNotAdmitTwoWaiters(t *testing.T) {
	queue := NewAliasExecutionQueue()
	releaseActive, err := queue.Acquire(context.Background(), "registry")
	if err != nil {
		t.Fatalf("Acquire(active) error = %v", err)
	}

	acquired := make(chan acquiredPermit, 2)
	for position := 1; position <= 2; position++ {
		position := position
		go func() {
			release, acquireErr := queue.Acquire(context.Background(), "registry")
			acquired <- acquiredPermit{position: position, release: release, err: acquireErr}
		}()
		waitForQueueWaiters(t, queue, "registry", position)
	}

	releaseActive()
	first := <-acquired
	if first.position != 1 || first.err != nil {
		t.Fatalf("first permit = %+v, want waiter 1", first)
	}
	releaseActive()
	select {
	case permit := <-acquired:
		t.Fatalf("duplicate release admitted waiter early: %+v", permit)
	case <-time.After(20 * time.Millisecond):
	}
	first.release()
	second := <-acquired
	if second.position != 2 || second.err != nil {
		t.Fatalf("second permit = %+v, want waiter 2", second)
	}
	second.release()
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

func assertQueueKeyDrained(t *testing.T, queue *AliasExecutionQueue, key string) {
	t.Helper()
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if state := queue.keys[key]; state != nil {
		t.Fatalf("queue %q retained state: %+v", key, state)
	}
}
