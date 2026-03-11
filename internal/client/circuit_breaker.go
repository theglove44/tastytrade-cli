package client

import (
	"fmt"
	"sync"
	"time"
)

// CircuitBreaker enforces a maximum order count over a rolling time window.
// Once tripped it requires manual reset — it will not auto-recover.
// Call Allow() before every order submission (live and dry-run).
type CircuitBreaker struct {
	mu          sync.Mutex
	windowStart time.Time
	count       int
	maxOrders   int
	window      time.Duration
	tripped     bool
}

// NewCircuitBreaker creates a breaker with the given per-window limit.
// Recommended initial values: maxOrders=10, window=time.Hour.
func NewCircuitBreaker(maxOrders int, window time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxOrders:   maxOrders,
		window:      window,
		windowStart: time.Now(),
	}
}

// Allow returns (true, "") if the order can proceed.
// Returns (false, reason) if the circuit is tripped or the window limit is hit.
// Updates the Prometheus circuit_breaker_state gauge.
func (cb *CircuitBreaker) Allow() (bool, string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.tripped {
		Metrics.CircuitBreakerState.Set(1)
		return false, fmt.Sprintf("circuit breaker tripped — manual Reset() required (limit was %d/%s)",
			cb.maxOrders, cb.window)
	}

	// Roll the window if it has expired.
	if time.Since(cb.windowStart) > cb.window {
		cb.count = 0
		cb.windowStart = time.Now()
	}

	cb.count++
	if cb.count > cb.maxOrders {
		cb.tripped = true
		Metrics.CircuitBreakerState.Set(1)
		return false, fmt.Sprintf("order limit %d/%s exceeded — circuit tripped",
			cb.maxOrders, cb.window)
	}

	Metrics.CircuitBreakerState.Set(0)
	return true, ""
}

// Reset manually clears the tripped state and resets the window counter.
// Only call after a human has reviewed the alert and confirmed it is safe to resume.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.tripped = false
	cb.count = 0
	cb.windowStart = time.Now()
	Metrics.CircuitBreakerState.Set(0)
}

// State returns a human-readable summary of current breaker state.
func (cb *CircuitBreaker) State() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.tripped {
		return fmt.Sprintf("TRIPPED (limit: %d/%s, count: %d)", cb.maxOrders, cb.window, cb.count)
	}
	age := time.Since(cb.windowStart).Round(time.Second)
	return fmt.Sprintf("OK (%d/%d orders in current window, window age: %s)",
		cb.count, cb.maxOrders, age)
}
