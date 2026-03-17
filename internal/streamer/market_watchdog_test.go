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
//  2. Watchdog does NOT fire when symbols are subscribed but zero quotes have
//     ever been received on the connection.
//  3. After quote flow has started, watchdog FIRES when quotes go stale.

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

// ─── Test 2: subscribed + zero quotes ever received → watchdog does not fire ─

// TestWatchdog_NoInitialQuotes_DoesNotForceReconnect verifies that staleWatchdog
// does not reconnect churn a connection that has subscribed symbols but has not
// yet received any quote on this connection.
func TestWatchdog_NoInitialQuotes_DoesNotForceReconnect(t *testing.T) {
	m := newTestMarketStreamer([]string{"SPY", ".XSP250117C580"})

	// Even if the connection has been open for a long time, zero quotes received
	// must not be considered stale.
	now := time.Now()
	m.statusMu.Lock()
	m.connectedSince = now.Add(-5 * time.Minute)
	m.lastEventAt = time.Time{}
	m.statusMu.Unlock()

	if m.isStale(now, 90*time.Second) {
		t.Fatal("isStale returned true with zero quotes received — closed-market connections must not churn")
	}

	ctx, cancel := context.WithCancel(context.Background())
	connCancelCalled := false
	connCancel := func() { connCancelCalled = true }
	done := make(chan struct{})
	go m.staleWatchdog(ctx, connCancel, 1*time.Millisecond, 5*time.Millisecond, done)

	time.Sleep(60 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("staleWatchdog did not exit after context cancel")
	}
	if connCancelCalled {
		t.Fatal("connCancel was called despite zero quotes ever received")
	}
}

// ─── Test 3: quote flow started then went stale → watchdog fires ─────────────

// TestWatchdog_QuoteRefreshesTimer verifies that calling touchLastEvent resets
// the stale clock, preventing the watchdog from firing while the quote remains
// fresh.
func TestWatchdog_QuoteRefreshesTimer(t *testing.T) {
	m := newTestMarketStreamer([]string{"SPY"})
	m.touchLastEvent()

	if m.isStale(time.Now(), 90*time.Second) {
		t.Error("isStale returned true immediately after touchLastEvent — timer not refreshed")
	}

	connCancelCalled := false
	connCancel := func() { connCancelCalled = true }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go m.staleWatchdog(ctx, connCancel, 500*time.Millisecond, 10*time.Millisecond, done)

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

// TestWatchdog_QuoteFlowStartedThenWentStale verifies that after at least one
// quote has been received, the watchdog still reconnects a genuinely stale feed.
func TestWatchdog_QuoteFlowStartedThenWentStale(t *testing.T) {
	m := newTestMarketStreamer([]string{"SPY"})
	now := time.Now()
	m.statusMu.Lock()
	m.lastEventAt = now.Add(-5 * time.Minute)
	m.statusMu.Unlock()

	if !m.isStale(now, 90*time.Second) {
		t.Fatal("isStale returned false after quote flow had gone stale")
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
	go m.staleWatchdog(ctx, connCancel, 1*time.Millisecond, 5*time.Millisecond, done)

	select {
	case <-connCancelCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("staleWatchdog did not call connCancel within 2s for a stale post-quote connection")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("staleWatchdog goroutine did not exit after firing")
	}
}

// TestSetConnected_ResetsQuoteClock verifies that stale detection is scoped to
// the current websocket connection. A previous connection may have received
// quotes, but a fresh reconnect with zero quotes so far must not be considered
// stale just because the old lastEventAt timestamp was ancient.
func TestSetConnected_ResetsQuoteClock(t *testing.T) {
	m := newTestMarketStreamer([]string{"SPY"})
	now := time.Now()
	m.statusMu.Lock()
	m.lastEventAt = now.Add(-10 * time.Minute)
	m.connectedSince = now.Add(-10 * time.Minute)
	m.statusMu.Unlock()

	if !m.isStale(now, 90*time.Second) {
		t.Fatal("precondition failed: old connection should be stale")
	}

	m.setConnected(now)

	m.statusMu.RLock()
	last := m.lastEventAt
	since := m.connectedSince
	m.statusMu.RUnlock()
	if !last.IsZero() {
		t.Fatal("setConnected did not reset lastEventAt for the new connection")
	}
	if since.IsZero() {
		t.Fatal("setConnected did not record connectedSince")
	}
	if m.isStale(now.Add(10*time.Minute), 90*time.Second) {
		t.Fatal("new connection with zero quotes was considered stale after reconnect")
	}
}

// ─── Additional: isStale edge cases ──────────────────────────────────────────

// TestIsStale_ZeroConnectedSince verifies that isStale returns false when no
// quote has ever been received.
func TestIsStale_ZeroConnectedSince(t *testing.T) {
	m := newTestMarketStreamer([]string{"SPY"})
	if m.isStale(time.Now(), 90*time.Second) {
		t.Error("isStale must return false when no quote has ever been received")
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
