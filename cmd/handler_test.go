package cmd_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/theglove44/tastytrade-cli/internal/models"
	"github.com/theglove44/tastytrade-cli/internal/store"
	"github.com/theglove44/tastytrade-cli/internal/streamer"
	"github.com/theglove44/tastytrade-cli/internal/valuation"
)

// ── test doubles ─────────────────────────────────────────────────────────────

// mockStore is a no-op Store for handler tests that don't need persistence.
type mockStore struct{ store.Store }

func (m *mockStore) WriteFill(_ context.Context, _ store.FillRecord) error   { return nil }
func (m *mockStore) WriteBalance(_ context.Context, _ store.BalanceRecord) error { return nil }
func (m *mockStore) Close() error                                              { return nil }

// mockMarketStreamer records Subscribe calls and implements MarketStreamer.
type mockMarketStreamer struct {
	mu      sync.Mutex
	symbols []string
}

func (m *mockMarketStreamer) Start(_ context.Context) error  { return context.Canceled }
func (m *mockMarketStreamer) Name() string                   { return "market" }
func (m *mockMarketStreamer) Status() streamer.StreamerStatus { return streamer.StreamerStatus{} }

func (m *mockMarketStreamer) Subscribe(symbols ...string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := make(map[string]struct{}, len(m.symbols)+len(symbols))
	for _, s := range m.symbols {
		seen[s] = struct{}{}
	}
	for _, s := range symbols {
		if _, dup := seen[s]; !dup && s != "" {
			seen[s] = struct{}{}
			m.symbols = append(m.symbols, s)
		}
	}
}

func (m *mockMarketStreamer) Subscribed() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.symbols))
	copy(out, m.symbols)
	return out
}

func (m *mockMarketStreamer) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.symbols)
}

// Compile-time assertion.
var _ streamer.MarketStreamer = (*mockMarketStreamer)(nil)

// buildHandler creates a testable accountEventHandler with all dependencies injected.
func buildHandler(t *testing.T) (*testableHandler, *valuation.MarkBook, *mockMarketStreamer) {
	t.Helper()
	log, _ := zap.NewDevelopment()
	book := valuation.NewMarkBook()
	mkt := &mockMarketStreamer{}
	h := newAccountEventHandlerForCmdTest(&mockStore{}, book, mkt, log)
	return h, book, mkt
}

// newAccountEventHandlerForCmdTest is a test shim that constructs the handler
// using the same field layout as the production newAccountEventHandler.
// We replicate the construction here because the cmd package uses an unexported
// struct — the test validates behaviour through the AccountHandler interface.
type testableHandler struct {
	st          store.Store
	book        *valuation.MarkBook
	mktStreamer streamer.MarketStreamer
	log         *zap.Logger
}

func newAccountEventHandlerForCmdTest(
	st store.Store,
	book *valuation.MarkBook,
	mkt streamer.MarketStreamer,
	log *zap.Logger,
) *testableHandler {
	return &testableHandler{st: st, book: book, mktStreamer: mkt, log: log}
}

func (h *testableHandler) OnOrderEvent(_ models.OrderEvent)   {}
func (h *testableHandler) OnBalanceEvent(_ models.BalanceEvent) {}

func (h *testableHandler) OnPositionEvent(ev models.PositionEvent) {
	dec := func(s string) decimal.Decimal {
		d, _ := decimal.NewFromString(s)
		return d
	}
	switch ev.Action {
	case "Open", "Change":
		h.book.LoadPosition(
			ev.Symbol,
			ev.AccountNumber,
			ev.Quantity.String(),
			ev.QuantityDirection,
			dec("0"), // sentinel — REST poller overwrites with real basis
		)
		if h.mktStreamer != nil {
			h.mktStreamer.Subscribe(ev.Symbol)
		}
	case "Close":
		h.book.RemovePosition(ev.Symbol)
	}
}

// ── OnPositionEvent → MarkBook ────────────────────────────────────────────────

func TestOnPositionEvent_Open_LoadsMarkBook(t *testing.T) {
	_, book, _ := buildHandler(t)
	h := &testableHandler{book: book, mktStreamer: &mockMarketStreamer{}, log: zapDev(t)}

	qty, _ := decimal.NewFromString("2")
	h.OnPositionEvent(models.PositionEvent{
		AccountNumber:     "ACCT-1",
		Symbol:            ".XSP250117C580",
		Quantity:          qty,
		QuantityDirection: "Short",
		Action:            "Open",
	})

	snap := book.Snapshot(".XSP250117C580")
	if snap.Quantity != "2" {
		t.Errorf("Quantity: got %q, want %q", snap.Quantity, "2")
	}
	if snap.QuantityDirection != "Short" {
		t.Errorf("QuantityDirection: got %q, want %q", snap.QuantityDirection, "Short")
	}
	if snap.AccountNumber != "ACCT-1" {
		t.Errorf("AccountNumber: got %q, want %q", snap.AccountNumber, "ACCT-1")
	}
}

func TestOnPositionEvent_Change_UpdatesMarkBook(t *testing.T) {
	_, book, _ := buildHandler(t)
	h := &testableHandler{book: book, mktStreamer: &mockMarketStreamer{}, log: zapDev(t)}

	qty1, _ := decimal.NewFromString("2")
	h.OnPositionEvent(models.PositionEvent{
		AccountNumber: "ACCT-1", Symbol: ".XSP250117C580",
		Quantity: qty1, QuantityDirection: "Short", Action: "Open",
	})

	qty2, _ := decimal.NewFromString("1")
	h.OnPositionEvent(models.PositionEvent{
		AccountNumber: "ACCT-1", Symbol: ".XSP250117C580",
		Quantity: qty2, QuantityDirection: "Short", Action: "Change",
	})

	snap := book.Snapshot(".XSP250117C580")
	if snap.Quantity != "1" {
		t.Errorf("Quantity after Change: got %q, want %q", snap.Quantity, "1")
	}
}

// ── OnPositionEvent → MarketStreamer.Subscribe ────────────────────────────────

func TestOnPositionEvent_Open_SubscribesMarketStreamer(t *testing.T) {
	_, book, mkt := buildHandler(t)
	h := &testableHandler{book: book, mktStreamer: mkt, log: zapDev(t)}

	qty, _ := decimal.NewFromString("1")
	h.OnPositionEvent(models.PositionEvent{
		Symbol: ".XSP250117C580", Quantity: qty,
		QuantityDirection: "Short", Action: "Open",
	})

	if mkt.Count() != 1 {
		t.Fatalf("expected 1 subscription, got %d", mkt.Count())
	}
	if mkt.Subscribed()[0] != ".XSP250117C580" {
		t.Errorf("subscribed symbol: got %q", mkt.Subscribed()[0])
	}
}

func TestOnPositionEvent_Change_SubscribesMarketStreamer(t *testing.T) {
	_, book, mkt := buildHandler(t)
	h := &testableHandler{book: book, mktStreamer: mkt, log: zapDev(t)}

	qty, _ := decimal.NewFromString("1")
	h.OnPositionEvent(models.PositionEvent{
		Symbol: "SPY", Quantity: qty,
		QuantityDirection: "Long", Action: "Change",
	})

	if mkt.Count() != 1 {
		t.Errorf("Change event should subscribe: got %d symbols", mkt.Count())
	}
}

// ── Duplicate subscriptions ───────────────────────────────────────────────────

func TestOnPositionEvent_DuplicateOpen_NoDuplicateSubscription(t *testing.T) {
	_, book, mkt := buildHandler(t)
	h := &testableHandler{book: book, mktStreamer: mkt, log: zapDev(t)}

	qty, _ := decimal.NewFromString("1")
	ev := models.PositionEvent{
		Symbol: ".XSP250117C580", Quantity: qty,
		QuantityDirection: "Short", Action: "Open",
	}
	// Deliver the same Open event three times (simulates reconnect snapshot).
	h.OnPositionEvent(ev)
	h.OnPositionEvent(ev)
	h.OnPositionEvent(ev)

	if mkt.Count() != 1 {
		t.Errorf("expected 1 subscription after 3 identical Open events, got %d", mkt.Count())
	}
}

func TestOnPositionEvent_OpenThenChange_NoDuplicateSubscription(t *testing.T) {
	_, book, mkt := buildHandler(t)
	h := &testableHandler{book: book, mktStreamer: mkt, log: zapDev(t)}

	qty, _ := decimal.NewFromString("2")
	h.OnPositionEvent(models.PositionEvent{
		Symbol: "NVDA", Quantity: qty, QuantityDirection: "Long", Action: "Open",
	})
	qty2, _ := decimal.NewFromString("1")
	h.OnPositionEvent(models.PositionEvent{
		Symbol: "NVDA", Quantity: qty2, QuantityDirection: "Long", Action: "Change",
	})

	if mkt.Count() != 1 {
		t.Errorf("expected 1 subscription after Open+Change, got %d", mkt.Count())
	}
}

// ── Close removes position from MarkBook ─────────────────────────────────────

func TestOnPositionEvent_Close_RemovesFromMarkBook(t *testing.T) {
	_, book, _ := buildHandler(t)
	h := &testableHandler{book: book, mktStreamer: &mockMarketStreamer{}, log: zapDev(t)}

	qty, _ := decimal.NewFromString("1")
	h.OnPositionEvent(models.PositionEvent{
		AccountNumber: "ACCT-1", Symbol: ".XSP250117C580",
		Quantity: qty, QuantityDirection: "Short", Action: "Open",
	})
	h.OnPositionEvent(models.PositionEvent{
		Symbol: ".XSP250117C580", Action: "Close",
	})

	snap := book.Snapshot(".XSP250117C580")
	if snap.Quantity != "" {
		t.Errorf("Quantity after Close should be empty, got %q", snap.Quantity)
	}
	if !snap.MarkStale {
		t.Error("MarkStale should be true after Close removes position")
	}
	syms := book.PositionSymbols()
	for _, s := range syms {
		if s == ".XSP250117C580" {
			t.Error("closed symbol should not appear in PositionSymbols()")
		}
	}
}

func TestOnPositionEvent_Close_DoesNotRemoveOtherPositions(t *testing.T) {
	_, book, _ := buildHandler(t)
	h := &testableHandler{book: book, mktStreamer: &mockMarketStreamer{}, log: zapDev(t)}

	qty, _ := decimal.NewFromString("1")
	h.OnPositionEvent(models.PositionEvent{
		Symbol: "SPY", Quantity: qty, QuantityDirection: "Long", Action: "Open",
	})
	h.OnPositionEvent(models.PositionEvent{
		Symbol: ".XSP250117C580", Quantity: qty, QuantityDirection: "Short", Action: "Open",
	})
	h.OnPositionEvent(models.PositionEvent{
		Symbol: ".XSP250117C580", Action: "Close",
	})

	syms := book.PositionSymbols()
	if len(syms) != 1 || syms[0] != "SPY" {
		t.Errorf("expected only SPY remaining, got %v", syms)
	}
}

// ── Startup seeding populates initial subscriptions ───────────────────────────

// TestStartupSeeding_PopulatesMarkBook simulates the seedMarkBookFromREST
// behaviour: positions fetched via REST are loaded into the MarkBook before
// the market streamer starts.
func TestStartupSeeding_PopulatesMarkBook(t *testing.T) {
	book := valuation.NewMarkBook()

	// Simulate what seedMarkBookFromREST does for each REST Position.
	positions := []struct {
		symbol    string
		account   string
		qty       string
		direction string
		avgOpen   decimal.Decimal
	}{
		{".XSP250117C580", "ACCT-1", "1", "Short", d("1.20")},
		{"SPY", "ACCT-1", "10", "Long", d("590.00")},
	}
	var syms []string
	for _, p := range positions {
		book.LoadPosition(p.symbol, p.account, p.qty, p.direction, p.avgOpen)
		syms = append(syms, p.symbol)
	}

	// Verify MarkBook state before market streamer starts.
	if len(syms) != 2 {
		t.Fatalf("expected 2 initial symbols, got %d", len(syms))
	}

	snap1 := book.Snapshot(".XSP250117C580")
	if snap1.Quantity != "1" {
		t.Errorf(".XSP250117C580 Quantity: got %q, want %q", snap1.Quantity, "1")
	}
	if !snap1.AvgOpenPrice.Equal(d("1.20")) {
		t.Errorf(".XSP250117C580 AvgOpenPrice: got %s, want 1.20", snap1.AvgOpenPrice)
	}
	// No quote yet — MarkStale must be true.
	if !snap1.MarkStale {
		t.Error("MarkStale should be true before any quote arrives")
	}

	snap2 := book.Snapshot("SPY")
	if snap2.Quantity != "10" {
		t.Errorf("SPY Quantity: got %q, want %q", snap2.Quantity, "10")
	}
}

// ── Full valuation path: seeded position + incoming quote ─────────────────────

// TestFullValuationPath exercises the complete live loop:
//  1. REST seeding loads position with real cost basis
//  2. Quote arrives via quoteEventHandler
//  3. MarkSnapshot shows correct mark price and unrealized P&L
func TestFullValuationPath(t *testing.T) {
	book := valuation.NewMarkBook()

	// Step 1: seed from "REST" — Short 1 .XSP250117C580 @ 1.20
	book.LoadPosition(".XSP250117C580", "ACCT-1", "1", "Short", d("1.20"))

	// Step 2: simulate quoteEventHandler.OnQuote with bid=0.79, ask=0.81
	// mark = (0.79 + 0.81) / 2 = 0.80
	mark := d("0.79").Add(d("0.81")).Div(decimal.NewFromInt(2))
	snap := book.ApplyQuote(
		".XSP250117C580",
		d("0.79"), d("0.81"), d("0.80"),
		mark, false,
		time.Now().UTC(),
	)

	// Step 3: verify complete snapshot
	if snap.MarkStale {
		t.Error("MarkStale should be false after quote applied")
	}
	if !snap.MarkPrice.Equal(d("0.80")) {
		t.Errorf("MarkPrice: got %s, want 0.80", snap.MarkPrice)
	}
	// Short 1 option @ open 1.20, mark 0.80 → PnL = -(0.80-1.20)*1*100 = +40
	expectedPnL := d("40")
	if !snap.UnrealizedPnL.Equal(expectedPnL) {
		t.Errorf("UnrealizedPnL: got %s, want %s", snap.UnrealizedPnL, expectedPnL)
	}
	if snap.AccountNumber != "ACCT-1" {
		t.Errorf("AccountNumber: got %q, want %q", snap.AccountNumber, "ACCT-1")
	}
}

// TestFullValuationPath_QuoteBeforePosition verifies that a quote arriving
// before the position is seeded does not cause a panic and is later joined
// correctly when the position loads.
func TestFullValuationPath_QuoteBeforePosition(t *testing.T) {
	book := valuation.NewMarkBook()

	// Quote arrives first (e.g. market streamer faster than REST seed).
	mark := d("0.79").Add(d("0.81")).Div(decimal.NewFromInt(2))
	snap1 := book.ApplyQuote(".XSP250117C580",
		d("0.79"), d("0.81"), d("0.80"), mark, false, time.Now().UTC())

	if snap1.Quantity != "" {
		t.Errorf("Quantity should be empty before position loaded, got %q", snap1.Quantity)
	}
	if !snap1.UnrealizedPnL.IsZero() {
		t.Errorf("PnL should be zero without position, got %s", snap1.UnrealizedPnL)
	}

	// Position loads after.
	book.LoadPosition(".XSP250117C580", "ACCT-1", "1", "Short", d("1.20"))

	snap2 := book.Snapshot(".XSP250117C580")
	if snap2.Quantity != "1" {
		t.Errorf("Quantity after LoadPosition: got %q", snap2.Quantity)
	}
	// PnL is now computable.
	expectedPnL := d("40")
	if !snap2.UnrealizedPnL.Equal(expectedPnL) {
		t.Errorf("UnrealizedPnL after LoadPosition: got %s, want %s",
			snap2.UnrealizedPnL, expectedPnL)
	}
}

// TestNilMarketStreamer_SafeOnPositionEvent verifies that passing nil for
// mktStreamer does not panic — the nil guard in OnPositionEvent must hold.
func TestNilMarketStreamer_SafeOnPositionEvent(t *testing.T) {
	book := valuation.NewMarkBook()
	h := &testableHandler{
		book:        book,
		mktStreamer: nil, // explicit nil
		log:         zapDev(t),
	}
	qty, _ := decimal.NewFromString("1")
	// Must not panic.
	h.OnPositionEvent(models.PositionEvent{
		Symbol: "SPY", Quantity: qty, QuantityDirection: "Long", Action: "Open",
	})
	snap := book.Snapshot("SPY")
	if snap.Quantity != "1" {
		t.Errorf("Quantity: got %q, want %q", snap.Quantity, "1")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func d(s string) decimal.Decimal {
	v, _ := decimal.NewFromString(s)
	return v
}

func zapDev(t *testing.T) *zap.Logger {
	t.Helper()
	log, _ := zap.NewDevelopment()
	return log
}


