package service

import (
	"sync"
	"testing"
	"time"
)

func TestCircuitBreaker_StartsAllowing(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond)
	if !cb.Allow() {
		t.Fatal("expected Allow() = true when closed")
	}
}

func TestCircuitBreaker_OpensAfterMaxFailures(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond)

	cb.RecordFailure()
	cb.RecordFailure()
	if !cb.Allow() {
		t.Fatal("should still allow before max failures")
	}

	cb.RecordFailure()
	if cb.Allow() {
		t.Fatal("should deny after max failures")
	}
}

func TestCircuitBreaker_TransitionsToHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond)
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.Allow() {
		t.Fatal("should be open immediately")
	}

	time.Sleep(150 * time.Millisecond)

	if !cb.Allow() {
		t.Fatal("should allow in half-open after timeout")
	}
	if cb.State() != stateHalfOpen {
		t.Fatalf("expected half-open, got %d", cb.State())
	}
}

func TestCircuitBreaker_ClosesOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond)
	cb.RecordFailure()
	cb.RecordFailure()

	time.Sleep(150 * time.Millisecond)
	cb.Allow()
	cb.RecordSuccess()

	if cb.State() != stateClosed {
		t.Fatalf("expected closed after success, got %d", cb.State())
	}
	if !cb.Allow() {
		t.Fatal("should allow when closed")
	}
}

func TestCircuitBreaker_ReopensOnFailureInHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond)
	cb.RecordFailure()
	cb.RecordFailure()

	time.Sleep(150 * time.Millisecond)
	cb.Allow()
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != stateOpen {
		t.Fatalf("expected open after half-open failure, got %d", cb.State())
	}
}

func TestCircuitBreaker_ConcurrentSafety(t *testing.T) {
	cb := NewCircuitBreaker(100, 100*time.Millisecond)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cb.Allow()
				cb.RecordFailure()
				cb.RecordSuccess()
			}
		}()
	}

	wg.Wait()
}
