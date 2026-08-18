package broker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func TestThreadTurnGateSerializesSameThread(t *testing.T) {
	gate := NewThreadTurnGate()
	threadID := modeluuid.New()
	enteredFirst := make(chan struct{})
	releaseFirst := make(chan struct{})
	finishedFirst := make(chan struct{})
	enteredSecond := make(chan struct{}, 1)

	go func() {
		defer close(finishedFirst)
		err := gate.Run(context.Background(), threadID, func() error {
			close(enteredFirst)
			<-releaseFirst
			return nil
		})
		if err != nil {
			t.Errorf("first Run() error = %v", err)
		}
	}()
	<-enteredFirst

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- gate.Run(context.Background(), threadID, func() error {
			enteredSecond <- struct{}{}
			return nil
		})
	}()

	select {
	case <-enteredSecond:
		t.Fatal("second turn entered before first released")
	case <-time.After(20 * time.Millisecond):
	}

	close(releaseFirst)
	<-finishedFirst
	select {
	case <-enteredSecond:
	case <-time.After(time.Second):
		t.Fatal("second turn did not enter after first released")
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
}

func TestThreadTurnGateWaitCanBeCanceled(t *testing.T) {
	gate := NewThreadTurnGate()
	threadID := modeluuid.New()
	releaseFirst, err := gate.Acquire(context.Background(), threadID)
	if err != nil {
		t.Fatalf("Acquire(first) error = %v", err)
	}
	defer releaseFirst()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = gate.Acquire(ctx, threadID)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire(canceled) error = %v, want context.Canceled", err)
	}
}

func TestThreadTurnGateNormalShutdownRequiresIdleGate(t *testing.T) {
	gate := NewThreadTurnGate()
	activeThread := modeluuid.New()
	releaseActive, err := gate.Acquire(context.Background(), activeThread)
	if err != nil {
		t.Fatalf("Acquire(active) error = %v", err)
	}

	if outstanding, accepted := gate.BeginShutdown(false); accepted || outstanding != 1 {
		t.Fatalf("BeginShutdown(false) = (%d, %v), want (1, false)", outstanding, accepted)
	}

	// A refused shutdown must leave admission open.
	releaseOther, err := gate.Acquire(context.Background(), modeluuid.New())
	if err != nil {
		t.Fatalf("Acquire(after refused shutdown) error = %v", err)
	}
	releaseOther()
	releaseActive()

	if outstanding, accepted := gate.BeginShutdown(false); !accepted || outstanding != 0 {
		t.Fatalf("BeginShutdown(false) after idle = (%d, %v), want (0, true)", outstanding, accepted)
	}
	if _, err := gate.Acquire(context.Background(), modeluuid.New()); !errors.Is(err, ErrTurnGateClosed) {
		t.Fatalf("Acquire(after shutdown) error = %v, want ErrTurnGateClosed", err)
	}
}

func TestThreadTurnGateForceShutdownClosesAdmissionWithActiveTurn(t *testing.T) {
	gate := NewThreadTurnGate()
	release, err := gate.Acquire(context.Background(), modeluuid.New())
	if err != nil {
		t.Fatalf("Acquire(active) error = %v", err)
	}

	if outstanding, accepted := gate.BeginShutdown(true); !accepted || outstanding != 1 {
		t.Fatalf("BeginShutdown(true) = (%d, %v), want (1, true)", outstanding, accepted)
	}
	if _, err := gate.Acquire(context.Background(), modeluuid.New()); !errors.Is(err, ErrTurnGateClosed) {
		t.Fatalf("Acquire(after forced shutdown) error = %v, want ErrTurnGateClosed", err)
	}
	if outstanding, accepted := gate.BeginShutdown(false); accepted || outstanding != 1 {
		t.Fatalf("BeginShutdown(false) after forced shutdown = (%d, %v), want (1, false)", outstanding, accepted)
	}

	// Existing holders remain releasable while the runtime owner cancels them.
	release()
}
