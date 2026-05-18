package workgate

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGateLimitsConcurrentWorkByKey(t *testing.T) {
	gate := New()
	firstRelease, err := gate.Acquire(context.Background(), "model", 1)
	if err != nil {
		t.Fatalf("first acquire error = %v", err)
	}
	defer firstRelease()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := gate.Acquire(ctx, "model", 1); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second acquire error = %v, want deadline", err)
	}
}

func TestGateUsesSeparateKeys(t *testing.T) {
	gate := New()
	firstRelease, err := gate.Acquire(context.Background(), "one", 1)
	if err != nil {
		t.Fatalf("first acquire error = %v", err)
	}
	defer firstRelease()

	secondRelease, err := gate.Acquire(context.Background(), "two", 1)
	if err != nil {
		t.Fatalf("second key acquire error = %v", err)
	}
	secondRelease()
}

func TestGateLimitZeroIsUnlimited(t *testing.T) {
	gate := New()
	firstRelease, err := gate.Acquire(context.Background(), "model", 0)
	if err != nil {
		t.Fatalf("first acquire error = %v", err)
	}
	defer firstRelease()
	secondRelease, err := gate.Acquire(context.Background(), "model", 0)
	if err != nil {
		t.Fatalf("second acquire error = %v", err)
	}
	secondRelease()
}
