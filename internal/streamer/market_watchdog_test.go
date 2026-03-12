package streamer

// White-box tests for the stale-connection watchdog.
//
// These tests live in package streamer (not streamer_test) so they can access
// unexported fields (lastEventAt, connectedSince, symbols) and the unexported
// isStale / staleWatchdog methods directly.  This is intentional: the watchdog
// is safety-critical and we want to drive it at the unit level without running
// a real WebSocket server.
//
// Three cases required by spec:
//  1. Watchdog does NOT fire when zero symbols are subscribed.
//  2. Watchdog FIRES when symbols are subscribed and no quote received for
//     longer than the timeout.
//  3. Receiving a quote (touchLastEvent) refreshes the timer and prevents firing.

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/theglove44/tastytrade-cli/internal/models"
)

// newTestMarketStreamer builds a minimal *marketStreamer for watchdog testing.
// It wires the unexported fields directly — no real WebSocket or token fetcher
// is needed because isStale and staleWatchdog never touch the connection.
func newTestMarketStreamer(symbols []string) *marketStreamer {
	log, _ := zap.NewDevelopment()
	syms := make([]string, len(symbols))
	copy(syms, symbols)
	return &marketStreamer{
		symbols: syms,
		handler: &noopQuoteHandler{},
		backoff: DefaultBackoff,
		log:     log,
	}
}

// noopQuoteHandler satisfies QuoteHandler without doing anything.
type noopQuoteHandler struct{}

func (n *noopQuoteHandler) OnQuote(_ models.QuoteEvent) {}

// ─── Test 1: zero subscriptions → watchdog never fires ───────────────────────

// TestWatchdog_ZeroSubscriptions_NeverFires verifies that isStale returns false
// and staleWatchdog exits cleanly (via ctx cancellation, not via connCancel)
// when no symbols are subscribed — even if the last-event time is ancient.
func TestWatchdog_ZeroSubscriptions_NeverFires(t *testing.T) {
	m := newTestMarketStreamer(nil) // zero subscriptions

	// Force connectedSince to long ago so that time-based checks would fire
	// if they were incorrectly ignoring the subscription guard.
	now := time.Now()
	m.statusMu.Lock()
	m.connectedSince = now.Add(-10 * time.Minute)
	m.lastEventAt = now.Add(-10 * time.Minute)
	m.statusMu.Unlock()

	// isStale must return false with zero subscriptions regardless of timestamps.
	if m.isStale(now, 90*time.Second) {
		t.Error("isStale returned true with zero subscriptions — must return false")
	}

	// staleWatchdog must exit via ctx.Done(), NOT via connCancel.
	// We verify this by checking that connCancelCalled is false when the ctx
	// is cancelled first.
	ctx, cancel := context.WithCancel(context.Background())
	connCancelCalled := false
	connCancel := func() { connCancelCalled = true }

	done := make(chan struct{})
	go m.staleWatchdog(ctx, connCancel, 90*time.Second, 10*time.Millisecond, done)

	// Give the watchdog a few ticks (it polls every 10ms in this test).
	time.Sleep(60 * time.Millisecond)

	// Cancel the outer context — watchdog must exit.
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("staleWatchdog did not exit after context cancel within 2s")
	}

	if connCancelCalled {
		t.Error("connCancel was called with zero subscriptions — watchdog must not fire")
	}
}

// ─── Test 2: subscribed + stale → watchdog fires ─────────────────────────────

// TestWatchdog_StaleConnection_ForcesReconnect verifies that staleWatchdog
// calls connCancel when symbols are subscribed and no quote has arrived within
// the timeout window.
func TestWatchdog_StaleConnection_ForcesReconnect(t *testing.T) {
	m := newTestMarketStreamer([]string{"SPY", ".XSP250117C580"})

	// Set connectedSince to long ago and leave lastEventAt zero.
	// isStale falls back to connectedSince when lastEventAt is zero.
	now := time.Now()
	m.statusMu.Lock()
	m.connectedSince = now.Add(-5 * time.Minute) // well past any timeout
	// lastEventAt intentionally left zero — no quote ever received
	m.statusMu.Unlock()

	// Sanity-check isStale before running the goroutine.
	if !m.isStale(now, 90*time.Second) {
		t.Fatal("isStale returned false — test precondition not met")
	}

	connCancelCalled := make(chan struct{}, 1)
	connCancel := func() {
		select {
		case connCancelCalled <- struct{}{}:
		default:
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	// Use a very short timeout (1ms) and check interval (5ms) so the test
	// completes in milliseconds rather than 90 seconds.
	go m.staleWatchdog(ctx, connCancel, 1*time.Millisecond, 5*time.Millisecond, done)

	select {
	case <-connCancelCalled:
		// Watchdog fired — correct.
	case <-time.After(2 * time.Second):
		t.Fatal("staleWatchdog did not call connCancel within 2s for a stale connection")
	}

	// Watchdog should also exit (done closed) after calling connCancel.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("staleWatchdog goroutine did not exit after firing")
	}
}

// ─── Test 3: quote received → timer refreshed → watchdog does not fire ───────

// TestWatchdog_QuoteRefreshesTimer verifies that calling touchLastEvent resets
// the stale clock, preventing the watchdog from firing even when the connection
// has been open for longer than the timeout.
func TestWatchdog_QuoteRefreshesTimer(t *testing.T) {
	m := newTestMarketStreamer([]string{"SPY"})

	// Set connectedSince to long ago — without quotes, this would be stale.
	m.statusMu.Lock()
	m.connectedSince = time.Now().Add(-5 * time.Minute)
	m.statusMu.Unlock()

	// Simulate receiving a quote just now — touchLastEvent writes lastEventAt = now.
	m.touchLastEvent()

	// isStale must return false: lastEventAt is fresh, timeout is 90s.
	if m.isStale(time.Now(), 90*time.Second) {
		t.Error("isStale returned true immediately after touchLastEvent — timer not refreshed")
	}

	// Run the watchdog with a generous timeout (500ms) and fast check (10ms).
	// Over a 100ms window it should never fire because lastEventAt is fresh.
	connCancelCalled := false
	connCancel := func() { connCancelCalled = true }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go m.staleWatchdog(ctx, connCancel, 500*time.Millisecond, 10*time.Millisecond, done)

	// Wait for several check cycles.
	time.Sleep(80 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("staleWatchdog did not exit after context cancel")
	}

	if connCancelCalled {
		t.Error("connCancel was called after touchLastEvent — quote should have refreshed the timer")
	}
}

// ─── Additional: isStale edge cases ──────────────────────────────────────────

// TestIsStale_ZeroConnectedSince verifies that isStale returns false when both
// lastEventAt and connectedSince are zero (streamer not yet connected).
// This prevents spurious fires during the handshake window.
func TestIsStale_ZeroConnectedSince(t *testing.T) {
	m := newTestMarketStreamer([]string{"SPY"})
	// Both times are zero (zero value of time.Time) — streamer not yet connected.
	if m.isStale(time.Now(), 90*time.Second) {
		t.Error("isStale must return false when connectedSince is zero (not yet connected)")
	}
}

// TestIsStale_JustBelowTimeout verifies the boundary: elapsed time one
// nanosecond below the timeout must NOT be stale.
// We use a fixed `now` for both the ref calculation and the isStale call so
// there is no wall-clock drift between the two statements.
func TestIsStale_JustBelowTimeout(t *testing.T) {
	m := newTestMarketStreamer([]string{"SPY"})
	timeout := 90 * time.Second

	now := time.Now()
	ref := now.Add(-timeout + time.Nanosecond) // 1ns before timeout
	m.statusMu.Lock()
	m.lastEventAt = ref
	m.statusMu.Unlock()

	// Pass the same `now` so elapsed = timeout-1ns, which is < timeout.
	if m.isStale(now, timeout) {
		t.Error("isStale returned true 1ns before timeout — boundary off-by-one")
	}
}

// TestIsStale_AtTimeout verifies that elapsed time exactly at the timeout IS stale.
func TestIsStale_AtTimeout(t *testing.T) {
	m := newTestMarketStreamer([]string{"SPY"})
	timeout := 90 * time.Second

	now := time.Now()
	ref := now.Add(-timeout) // exactly at timeout
	m.statusMu.Lock()
	m.lastEventAt = ref
	m.statusMu.Unlock()

	// Pass the same `now` so elapsed = exactly timeout.
	if !m.isStale(now, timeout) {
		t.Error("isStale returned false at exactly the timeout — should be stale")
	}
}
