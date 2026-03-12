// Package bus provides a minimal process-local generic event broker.
//
// Design constraints:
//   - No external dependencies — pure stdlib sync primitives.
//   - Non-blocking Publish: a full subscriber channel drops the event rather
//     than blocking the publisher goroutine. This is the correct trade-off for
//     a trading pipeline: a slow consumer (e.g. store write) must never
//     back-pressure the hot path (e.g. streamer dispatch).
//   - No persistence. Events that are dropped are gone.
//   - Process-local only. No network, no message-queue abstractions.
//   - Close is explicit and final: all subscriber channels are closed, causing
//     any range-over-channel consumers to exit cleanly.
//   - Drop observability: an optional onDrop callback is called once per
//     dropped event. Callers use this to increment metrics and emit sampled
//     log warnings without coupling the bus to Prometheus or zap.
//
// Typical usage in root.go:
//
//	orderBus := bus.New[models.OrderEvent](func() {
//	    client.Metrics.BusDroppedEvents.WithLabelValues("order").Inc()
//	})
//	ch := orderBus.Subscribe(128)
//	go func() {
//	    for ev := range ch { /* handle */ }
//	}()
//	orderBus.Publish(ev)  // called from handler — non-blocking
//	orderBus.Close()      // called on shutdown
package bus

import (
	"sync"
	"sync/atomic"
)

// Broker is a generic fan-out event broker.
// Zero value is not usable; construct with New[T]().
type Broker[T any] struct {
	mu     sync.RWMutex
	subs   []chan T
	closed bool

	// onDrop is called once per dropped event (subscriber channel full).
	// nil means no callback — drops are always silent at the bus layer.
	// Callers wire metric increments and sampled logging here.
	onDrop func()

	// dropCount is the total number of drops across all subscribers.
	// Exposed via Drops() for testing without requiring the callback.
	dropCount atomic.Int64
}

// New creates a ready-to-use Broker for events of type T.
// onDrop is called once per dropped event; pass nil for no callback.
// Typical use: pass a closure that increments a Prometheus counter and emits
// a sampled log warning (see root.go for the wiring pattern).
func New[T any](onDrop func()) *Broker[T] {
	return &Broker[T]{onDrop: onDrop}
}

// Subscribe returns a new buffered channel that will receive published events.
// capacity sets the channel buffer size; callers should size this to absorb
// bursts without dropping under normal load (e.g. 64–256 for store writes).
// The returned channel is closed when Close() is called.
//
// Subscribe may be called before or after the first Publish.
// It is safe to call from any goroutine.
func (b *Broker[T]) Subscribe(capacity int) <-chan T {
	ch := make(chan T, capacity)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		// Broker already closed — return an already-closed channel so the
		// consumer's range loop exits immediately.
		close(ch)
		return ch
	}
	b.subs = append(b.subs, ch)
	return ch
}

// Publish sends v to every subscriber channel.
// Non-blocking: if a subscriber's channel is full the event is dropped for
// that subscriber. The onDrop callback is invoked once per dropped event.
// Other subscribers are not affected.
// Safe to call from any goroutine. No-op after Close().
func (b *Broker[T]) Publish(v T) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return
	}
	for _, ch := range b.subs {
		select {
		case ch <- v:
		default:
			// Channel full — drop and notify caller.
			b.dropCount.Add(1)
			if b.onDrop != nil {
				b.onDrop()
			}
		}
	}
}

// Drops returns the total number of events dropped across all subscribers
// since the broker was created. Useful in tests without requiring the callback.
func (b *Broker[T]) Drops() int64 {
	return b.dropCount.Load()
}

// Close closes all subscriber channels, causing any range-over-channel loops
// to exit. After Close, Subscribe returns pre-closed channels and Publish is a
// no-op. Close is idempotent.
func (b *Broker[T]) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for _, ch := range b.subs {
		close(ch)
	}
}
