package service

import (
	"sync/atomic"
	"time"
)

const (
	stateClosed   int32 = 0
	stateOpen     int32 = 1
	stateHalfOpen int32 = 2
)

type CircuitBreaker struct {
	maxFailures  int32
	resetTimeout time.Duration

	state    atomic.Int32
	failures atomic.Int32
	openedAt atomic.Int64
}

func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	cb := &CircuitBreaker{
		maxFailures:  int32(maxFailures),
		resetTimeout: resetTimeout,
	}
	return cb
}

func (cb *CircuitBreaker) Allow() bool {
	switch cb.state.Load() {
	case stateClosed:
		return true
	case stateOpen:
		if time.Now().UnixNano()-cb.openedAt.Load() >= cb.resetTimeout.Nanoseconds() {
			cb.state.CompareAndSwap(stateOpen, stateHalfOpen)
			return true
		}
		return false
	case stateHalfOpen:
		return true
	default:
		return false
	}
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.failures.Store(0)
	cb.state.Store(stateClosed)
}

func (cb *CircuitBreaker) RecordFailure() {
	f := cb.failures.Add(1)
	if f >= cb.maxFailures {
		cb.state.Store(stateOpen)
		cb.openedAt.Store(time.Now().UnixNano())
	}
}

func (cb *CircuitBreaker) State() int32 {
	return cb.state.Load()
}

func (cb *CircuitBreaker) SetOpen() {
	cb.state.Store(stateOpen)
	cb.openedAt.Store(time.Now().UnixNano())
}
