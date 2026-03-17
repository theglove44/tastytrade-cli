package valuation_test

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/theglove44/tastytrade-cli/internal/valuation"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

func dec(s string) decimal.Decimal {
	d, _ := decimal.NewFromString(s)
	return d
}

func now() time.Time { return time.Now().UTC() }

// ── ApplyQuote and mark derivation ────────────────────────────────────────────

func TestApplyQuote_MarkFromMidpoint(t *testing.T) {
	book := valuation.NewMarkBook()
	snap := book.ApplyQuote("SPY", dec("590.00"), dec("590.10"), dec("590.05"),
		dec("590.05"), false, now())

	if snap.MarkStale {
		t.Error("MarkStale should be false when bid and ask are present")
	}
	// mark = (590.00 + 590.10) / 2 = 590.05
	want := dec("590.05")
	if !snap.MarkPrice.Equal(want) {
		t.Errorf("MarkPrice: got %s, want %s", snap.MarkPrice, want)
	}
}

func TestApplyQuote_MarkFromLast_WhenNoBidAsk(t *testing.T) {
	book := valuation.NewMarkBook()
	snap := book.ApplyQuote(".XSP250117C580",
		decimal.Zero, decimal.Zero, dec("1.50"),
		dec("1.50"), false, now())

	if snap.MarkStale {
		t.Error("MarkStale should be false when last is non-zero")
	}
	if !snap.MarkPrice.Equal(dec("1.50")) {
		t.Errorf("MarkPrice: got %s, want 1.50", snap.MarkPrice)
	}
}

func TestApplyQuote_Stale_WhenAllZero(t *testing.T) {
	book := valuation.NewMarkBook()
	snap := book.ApplyQuote("SPY",
		decimal.Zero, decimal.Zero, decimal.Zero,
		decimal.Zero, true, now())

	if !snap.MarkStale {
		t.Error("MarkStale should be true when bid, ask, and last are all zero")
	}
	if !snap.MarkPrice.IsZero() {
		t.Errorf("MarkPrice should be zero for stale quote, got %s", snap.MarkPrice)
	}
}

func TestApplyQuote_SymbolWithNoPosition(t *testing.T) {
	// Quote arrives before any position is loaded — should still be stored.
	book := valuation.NewMarkBook()
	snap := book.ApplyQuote("NVDA", dec("900.00"), dec("900.10"), dec("900.05"),
		dec("900.05"), false, now())

	if snap.Symbol != "NVDA" {
		t.Errorf("Symbol: got %q, want %q", snap.Symbol, "NVDA")
	}
	// No position — Quantity should be empty.
	if snap.Quantity != "" {
		t.Errorf("Quantity should be empty without position, got %q", snap.Quantity)
	}
	// UnrealizedPnL should be zero without a position.
	if !snap.UnrealizedPnL.IsZero() {
		t.Errorf("UnrealizedPnL should be zero without position, got %s", snap.UnrealizedPnL)
	}
}

// ── LoadPosition ──────────────────────────────────────────────────────────────

func TestLoadPosition_NoQuoteYet(t *testing.T) {
	book := valuation.NewMarkBook()
	book.LoadPosition(".XSP250117C580", "ACCT-123", "1", "Short", dec("1.20"))

	snap := book.Snapshot(".XSP250117C580")
	if snap.MarkStale != true {
		t.Error("MarkStale should be true — no quote received yet")
	}
	if snap.Quantity != "1" {
		t.Errorf("Quantity: got %q, want %q", snap.Quantity, "1")
	}
	if snap.QuantityDirection != "Short" {
		t.Errorf("QuantityDirection: got %q, want %q", snap.QuantityDirection, "Short")
	}
	if !snap.AvgOpenPrice.Equal(dec("1.20")) {
		t.Errorf("AvgOpenPrice: got %s, want 1.20", snap.AvgOpenPrice)
	}
}

func TestLoadPosition_ReplacesExisting(t *testing.T) {
	book := valuation.NewMarkBook()
	book.LoadPosition("SPY", "ACCT-123", "10", "Long", dec("590.00"))
	book.LoadPosition("SPY", "ACCT-123", "5", "Long", dec("595.00")) // replace

	snap := book.Snapshot("SPY")
	if snap.Quantity != "5" {
		t.Errorf("Quantity after replace: got %q, want %q", snap.Quantity, "5")
	}
	if !snap.AvgOpenPrice.Equal(dec("595.00")) {
		t.Errorf("AvgOpenPrice after replace: got %s, want 595.00", snap.AvgOpenPrice)
	}
}

// ── Position-to-quote matching ────────────────────────────────────────────────

func TestPositionQuoteMatch_ExactSymbol(t *testing.T) {
	// The matching rule: exact string key.
	// Position and quote must use identical symbol strings.
	book := valuation.NewMarkBook()
	const sym = ".XSP250117C580"

	book.LoadPosition(sym, "ACCT-123", "2", "Short", dec("1.20"))
	snap := book.ApplyQuote(sym, dec("0.80"), dec("0.82"), dec("0.81"),
		dec("0.81"), false, now())

	if snap.MarkStale {
		t.Error("MarkStale should be false")
	}
	if snap.Quantity != "2" {
		t.Errorf("Quantity: got %q, want %q", snap.Quantity, "2")
	}
}

func TestPositionQuoteMatch_SymbolMismatch_NoMatch(t *testing.T) {
	// Symbols with different capitalisation or format should NOT match.
	// This validates the no-normalisation contract.
	book := valuation.NewMarkBook()

	book.LoadPosition(".XSP250117C580", "ACCT-123", "1", "Short", dec("1.20"))
	// Quote for a different symbol — should NOT update the position's mark.
	book.ApplyQuote(".XSP250117C590", dec("0.50"), dec("0.52"), dec("0.51"),
		dec("0.51"), false, now())

	snap := book.Snapshot(".XSP250117C580")
	if !snap.MarkStale {
		t.Error("Snapshot for .XSP250117C580 should still be stale — no matching quote")
	}
}

// ── UnrealizedPnL calculation ────────────────────────────────────────────────

func TestUnrealizedPnL_LongEquity(t *testing.T) {
	// Long 10 SPY @ 590.00, mark = 600.00
	// Expected PnL = (600 - 590) * 10 * 1 = 100
	book := valuation.NewMarkBook()
	book.LoadPosition("SPY", "ACCT-123", "10", "Long", dec("590.00"))
	snap := book.ApplyQuote("SPY", dec("599.95"), dec("600.05"), dec("600.00"),
		dec("600.00"), false, now())

	expected := dec("100") // (600 - 590) * 10 * 1
	if !snap.UnrealizedPnL.Equal(expected) {
		t.Errorf("UnrealizedPnL: got %s, want %s", snap.UnrealizedPnL, expected)
	}
}

func TestUnrealizedPnL_ShortOption(t *testing.T) {
	// Short 1 .XSP250117C580 @ 1.20, mark = 0.80
	// Expected PnL = -(0.80 - 1.20) * 1 * 100 = 40 (profit for short)
	book := valuation.NewMarkBook()
	book.LoadPosition(".XSP250117C580", "ACCT-123", "1", "Short", dec("1.20"))
	snap := book.ApplyQuote(".XSP250117C580", dec("0.79"), dec("0.81"), dec("0.80"),
		dec("0.80"), false, now())

	expected := dec("40")
	if !snap.UnrealizedPnL.Equal(expected) {
		t.Errorf("UnrealizedPnL: got %s, want %s", snap.UnrealizedPnL, expected)
	}
}

func TestUnrealizedPnL_StaleQuote_ZeroPnL(t *testing.T) {
	// No PnL when mark is stale.
	book := valuation.NewMarkBook()
	book.LoadPosition("SPY", "ACCT-123", "10", "Long", dec("590.00"))
	snap := book.ApplyQuote("SPY",
		decimal.Zero, decimal.Zero, decimal.Zero,
		decimal.Zero, true, now())

	if !snap.UnrealizedPnL.IsZero() {
		t.Errorf("UnrealizedPnL should be zero for stale mark, got %s", snap.UnrealizedPnL)
	}
}

func TestUnrealizedPnL_NoPosition_ZeroPnL(t *testing.T) {
	// No PnL when position not loaded.
	book := valuation.NewMarkBook()
	snap := book.ApplyQuote("SPY", dec("600.00"), dec("600.10"), dec("600.05"),
		dec("600.05"), false, now())

	if !snap.UnrealizedPnL.IsZero() {
		t.Errorf("UnrealizedPnL should be zero without position, got %s", snap.UnrealizedPnL)
	}
}

// ── AllSnapshots and PositionSymbols ─────────────────────────────────────────

func TestAllSnapshots_UnionOfPositionsAndQuotes(t *testing.T) {
	book := valuation.NewMarkBook()

	// Position with no quote.
	book.LoadPosition("SPY", "ACCT-123", "10", "Long", dec("590.00"))
	// Quote with no position.
	book.ApplyQuote("NVDA", dec("900.00"), dec("900.10"), dec("900.05"),
		dec("900.05"), false, now())
	// Both.
	book.LoadPosition(".XSP250117C580", "ACCT-123", "1", "Short", dec("1.20"))
	book.ApplyQuote(".XSP250117C580", dec("0.80"), dec("0.82"), dec("0.81"),
		dec("0.81"), false, now())

	snaps := book.AllSnapshots()
	if len(snaps) != 3 {
		t.Errorf("AllSnapshots: got %d snapshots, want 3", len(snaps))
	}
}

func TestPositionSymbols_OnlyPositions(t *testing.T) {
	book := valuation.NewMarkBook()
	book.LoadPosition("SPY", "ACCT-123", "10", "Long", dec("590.00"))
	book.ApplyQuote("NVDA", dec("900.00"), dec("900.10"), dec("900.05"),
		dec("900.05"), false, now())

	syms := book.PositionSymbols()
	if len(syms) != 1 || syms[0] != "SPY" {
		t.Errorf("PositionSymbols: got %v, want [SPY]", syms)
	}
}

// ── Snapshot missing symbol ────────────────────────────────────────────────────

func TestSnapshot_MissingSymbol_StaleDefault(t *testing.T) {
	book := valuation.NewMarkBook()
	snap := book.Snapshot("UNKNOWN")

	if !snap.MarkStale {
		t.Error("Snapshot for unknown symbol should be stale")
	}
	if snap.Symbol != "UNKNOWN" {
		t.Errorf("Symbol: got %q, want %q", snap.Symbol, "UNKNOWN")
	}
}

// ── Concurrent safety ──────────────────────────────────────────────────────────

func TestMarkBook_Concurrent(t *testing.T) {
	book := valuation.NewMarkBook()
	done := make(chan struct{})

	// Writer goroutine: apply quotes rapidly.
	go func() {
		for i := 0; i < 1000; i++ {
			book.ApplyQuote("SPY", dec("600.00"), dec("600.10"), dec("600.05"),
				dec("600.05"), false, now())
		}
		close(done)
	}()

	// Reader goroutine: snapshot concurrently.
	for i := 0; i < 500; i++ {
		_ = book.Snapshot("SPY")
	}
	<-done
}
