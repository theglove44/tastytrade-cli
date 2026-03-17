// Package valuation maintains in-memory mark-to-market state for open positions.
//
// Design rationale — why in-memory only for Phase 2B:
//
//	Mark prices update every sub-second during market hours. Writing every
//	quote update to SQLite would create excessive write load with minimal
//	analytical value. The store already holds position snapshots (cost basis,
//	quantity) written every 5 minutes. The mark layer adds the live price
//	dimension on top without persisting it.
//
//	Persistence strategy for future phases:
//	  - Write a MarkSnapshot to the store at position-snapshot cadence (5 min)
//	  - Write on clean shutdown (streamer context cancel)
//	  - The store schema is already defined in MarkSnapshot below — only the
//	    migration and write method need to be added when required.
//
// Symbol matching rules:
//
//	TastyTrade and DXLink use compatible option symbols.
//	The MarkBook stores one mark per symbol string, keyed exactly as received
//	from both the position snapshot (REST) and the quote event (DXLink).
//	Callers are responsible for ensuring the symbols are in the same format
//	before inserting. No normalisation is applied here — normalisation belongs
//	at the ingestion boundary (position poller or streamer handler), not here.
//
// Concurrency:
//
//	All MarkBook methods are safe for concurrent use.
package valuation

import (
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// MarkSnapshot is the point-in-time valuation for a single position symbol.
// It is the primary output of the valuation layer and is suitable for
// persistence in a future store migration.
type MarkSnapshot struct {
	// Symbol is the instrument identifier, e.g. ".XSP250117C580" or "SPY".
	Symbol string `json:"symbol"`

	// AccountNumber links the snapshot to its account context.
	AccountNumber string `json:"account_number"`

	// MarkPrice is the derived mark: (bid+ask)/2 when both are live,
	// otherwise last price. Zero if no quote has been received yet.
	MarkPrice decimal.Decimal `json:"mark_price"`

	// BidPrice and AskPrice are the most recent values from DXLink.
	BidPrice decimal.Decimal `json:"bid_price"`
	AskPrice decimal.Decimal `json:"ask_price"`

	// LastPrice is the most recent last-trade price from DXLink.
	LastPrice decimal.Decimal `json:"last_price"`

	// MarkStale is true when mark cannot be determined from live data.
	// This happens when bid, ask, and last are all zero (pre-market, halted,
	// or no quote received yet).
	MarkStale bool `json:"mark_stale"`

	// Quantity is the position size as a decimal string (from position snapshot).
	// Positive for Long, negative for Short. Empty if no position snapshot loaded.
	Quantity string `json:"quantity"`

	// QuantityDirection is "Long" or "Short" from the position snapshot.
	QuantityDirection string `json:"quantity_direction"`

	// AvgOpenPrice is the cost basis from the position snapshot.
	AvgOpenPrice decimal.Decimal `json:"avg_open_price"`

	// UnrealizedPnL = (MarkPrice - AvgOpenPrice) * quantity * multiplier.
	// Zero if MarkStale is true or no position snapshot exists.
	// Multiplier is assumed to be 100 for equity options; 1 for equities.
	// A future phase should read multiplier from the position model.
	UnrealizedPnL decimal.Decimal `json:"unrealized_pnl"`

	// QuoteUpdatedAt is the time the most recent quote was applied.
	// Zero if no quote received for this symbol yet.
	QuoteUpdatedAt time.Time `json:"quote_updated_at,omitempty"`

	// PositionLoadedAt is the time the position snapshot was loaded into the book.
	// Zero if no position snapshot exists for this symbol.
	PositionLoadedAt time.Time `json:"position_loaded_at,omitempty"`
}

// positionInfo holds the position-side data for one symbol.
type positionInfo struct {
	accountNumber     string
	quantity          string
	quantityDirection string
	avgOpenPrice      decimal.Decimal
	loadedAt          time.Time
}

// quoteInfo holds the most recent quote data for one symbol.
type quoteInfo struct {
	bid       decimal.Decimal
	ask       decimal.Decimal
	last      decimal.Decimal
	mark      decimal.Decimal
	markStale bool
	updatedAt time.Time
}

// MarkBook is the in-memory mark-to-market store.
// It maps symbol → (position info, latest quote) and computes MarkSnapshot
// on demand.
type MarkBook struct {
	mu        sync.RWMutex
	positions map[string]positionInfo // symbol → position
	quotes    map[string]quoteInfo    // symbol → latest quote
}

// NewMarkBook creates an empty MarkBook.
func NewMarkBook() *MarkBook {
	return &MarkBook{
		positions: make(map[string]positionInfo),
		quotes:    make(map[string]quoteInfo),
	}
}

// LoadPosition stores or replaces position-side data for a symbol.
// Called by the REST position poller or the account streamer position event handler.
func (b *MarkBook) LoadPosition(
	symbol string,
	accountNumber string,
	quantity string,
	quantityDirection string,
	avgOpenPrice decimal.Decimal,
) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.positions[symbol] = positionInfo{
		accountNumber:     accountNumber,
		quantity:          quantity,
		quantityDirection: quantityDirection,
		avgOpenPrice:      avgOpenPrice,
		loadedAt:          time.Now(),
	}
}

// ApplyQuote updates the mark price for a symbol from a live quote.
// If no position exists for the symbol, the quote is still stored — the
// position may arrive later via a snapshot update.
// Returns the updated MarkSnapshot for the symbol.
func (b *MarkBook) ApplyQuote(
	symbol string,
	bid, ask, last, mark decimal.Decimal,
	markStale bool,
	updatedAt time.Time,
) MarkSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.quotes[symbol] = quoteInfo{
		bid:       bid,
		ask:       ask,
		last:      last,
		mark:      mark,
		markStale: markStale,
		updatedAt: updatedAt,
	}

	return b.snapshotLocked(symbol)
}

// Snapshot returns the current MarkSnapshot for a symbol.
// Returns a zero-value snapshot with MarkStale=true if no quote has arrived.
func (b *MarkBook) Snapshot(symbol string) MarkSnapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.snapshotLocked(symbol)
}

// AllSnapshots returns a snapshot of every symbol currently in the book.
// Only symbols with at least a position or a quote are included.
func (b *MarkBook) AllSnapshots() []MarkSnapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()

	seen := make(map[string]struct{})
	for sym := range b.positions {
		seen[sym] = struct{}{}
	}
	for sym := range b.quotes {
		seen[sym] = struct{}{}
	}

	out := make([]MarkSnapshot, 0, len(seen))
	for sym := range seen {
		out = append(out, b.snapshotLocked(sym))
	}
	return out
}

// PositionSymbols returns all symbols that have a loaded position.
// Used by the market streamer to build its subscription list.
func (b *MarkBook) PositionSymbols() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	syms := make([]string, 0, len(b.positions))
	for sym := range b.positions {
		syms = append(syms, sym)
	}
	return syms
}

// snapshotLocked computes a MarkSnapshot for symbol under the read lock.
// Caller must hold b.mu (read or write) before calling.
func (b *MarkBook) snapshotLocked(symbol string) MarkSnapshot {
	snap := MarkSnapshot{
		Symbol:    symbol,
		MarkStale: true, // default until a quote arrives
	}

	if pos, ok := b.positions[symbol]; ok {
		snap.AccountNumber = pos.accountNumber
		snap.Quantity = pos.quantity
		snap.QuantityDirection = pos.quantityDirection
		snap.AvgOpenPrice = pos.avgOpenPrice
		snap.PositionLoadedAt = pos.loadedAt
	}

	q, hasQuote := b.quotes[symbol]
	if !hasQuote {
		return snap
	}

	snap.BidPrice = q.bid
	snap.AskPrice = q.ask
	snap.LastPrice = q.last
	snap.MarkPrice = q.mark
	snap.MarkStale = q.markStale
	snap.QuoteUpdatedAt = q.updatedAt

	// Compute unrealized P&L only when all required data is present.
	if !q.markStale && snap.Quantity != "" && snap.Quantity != "0" {
		qty, err := decimal.NewFromString(snap.Quantity)
		if err == nil && !qty.IsZero() {
			multiplier := optionMultiplier(symbol)
			// Short positions have negative P&L direction relative to mark increase.
			// Convention: qty is always positive; direction flips for short.
			pnl := q.mark.Sub(snap.AvgOpenPrice).Mul(qty).Mul(multiplier)
			if snap.QuantityDirection == "Short" {
				pnl = pnl.Neg()
			}
			snap.UnrealizedPnL = pnl
		}
	}

	return snap
}

// optionMultiplier returns the contract multiplier for a symbol.
// Options (symbols starting with ".") use 100; equities use 1.
// A future phase should derive this from the instrument metadata.
func optionMultiplier(symbol string) decimal.Decimal {
	if len(symbol) > 0 && symbol[0] == '.' {
		return decimal.NewFromInt(100)
	}
	return decimal.NewFromInt(1)
}

// RemovePosition removes position-side data for a symbol from the MarkBook.
// Called when the account streamer delivers a Close PositionEvent.
//
// The quote entry for the symbol is also removed: with no open position
// the quote has no valuation context, and retaining it would cause
// AllSnapshots() to include a ghost entry in P&L roll-ups.
//
// The market streamer subscription for the symbol is NOT removed here —
// the DXLink wire protocol has no per-symbol unsubscribe message, so the
// subscription persists until the next reconnect. A quote arriving for a
// removed symbol is silently stored as a quote-only entry and will show
// MarkStale=false but Quantity="" — safe and clearly distinguishable.
func (b *MarkBook) RemovePosition(symbol string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.positions, symbol)
	delete(b.quotes, symbol)
}
