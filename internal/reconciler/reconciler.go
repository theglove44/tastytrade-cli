// Package reconciler periodically fetches the canonical position list from the
// REST API and reconciles it against the in-memory MarkBook and SQLite store.
//
// # Purpose
//
// The account streamer's PositionEvent wire format does not include cost basis
// (AvgOpenPrice). Positions opened mid-session are therefore loaded into the
// MarkBook with decimal.Zero as AvgOpenPrice, which produces incorrect
// unrealised-PnL calculations until the process restarts and the startup seed
// re-runs.
//
// The reconciler fixes this by polling Exchange.Positions() on a configurable
// interval and patching any MarkBook entry whose AvgOpenPrice is stale or zero.
//
// # Design constraints
//
//  1. The reconciler may call MarkBook.LoadPosition() and MarkBook.RemovePosition()
//     directly — this is a deliberate exception to the event-bus model because
//     reconciliation is a state-correction path, not a live event path.
//  2. The reconciler must never block streamers or the event bus.
//  3. Position removal requires N consecutive REST passes where the symbol is
//     absent (default N=2, see Config.ReconcileAbsenceThreshold). A single
//     absent pass is treated as a transient API hiccup, not a confirmed close.
//  4. The reconciler must not overwrite newer data: if a streamer event updated
//     a position after the REST fetch started, that update is left intact.
//     Implemented via a fetchedAt timestamp: only positions whose
//     PositionLoadedAt < fetchedAt are eligible for update.
//  5. REST failure must leave MarkBook state entirely unchanged for that pass.
//
// # Shutdown
//
// Start() blocks until ctx is cancelled. Callers should run it in a goroutine
// and cancel the context to stop. It integrates with the root.go WaitGroup
// so st.Close() is never called while a reconciliation pass is in progress.
package reconciler

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/theglove44/tastytrade-cli/internal/client"
	"github.com/theglove44/tastytrade-cli/internal/exchange"
	"github.com/theglove44/tastytrade-cli/internal/models"
	"github.com/theglove44/tastytrade-cli/internal/store"
	"github.com/theglove44/tastytrade-cli/internal/valuation"
)

// Reconciler periodically fetches the canonical position list from REST and
// reconciles it against the MarkBook and Store.
// Start() blocks until ctx is cancelled.
type Reconciler interface {
	Start(ctx context.Context)
}

// Config holds the tunable parameters for the reconciler.
// Populated from config.Config and passed by root.go.
type Config struct {
	// Interval is the time between reconciliation passes.
	// Minimum enforced value: 10 seconds (for tests); production default: 60s.
	Interval time.Duration

	// AbsenceThreshold is the number of consecutive REST passes in which a
	// symbol must be absent before it is removed from MarkBook.
	// Default: 2. A value of 1 means "remove on first absence" — unsafe for
	// production because the TastyTrade Positions endpoint can return a stale
	// or partial list under load.
	AbsenceThreshold int
}

// DefaultConfig returns production-safe defaults.
func DefaultConfig() Config {
	return Config{
		Interval:         60 * time.Second,
		AbsenceThreshold: 2,
	}
}

// reconciler is the concrete implementation of Reconciler.
type reconciler struct {
	ex        exchange.Exchange
	st        store.Store // may be nil — skips WritePositionSnapshot
	book      *valuation.MarkBook
	accountID string
	cfg       Config
	log       *zap.Logger

	// absenceCounts tracks how many consecutive REST passes each symbol has
	// been absent. Keyed by symbol string. Cleared on next appearance.
	// Only accessed from the single goroutine running runOnce, so no mutex.
	absenceCounts map[string]int
}

// New creates a Reconciler.
// st may be nil — in that case WritePositionSnapshot calls are skipped and
// the reconciler still patches MarkBook correctly.
func New(
	ex exchange.Exchange,
	st store.Store,
	book *valuation.MarkBook,
	accountID string,
	cfg Config,
	log *zap.Logger,
) Reconciler {
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultConfig().Interval
	}
	if cfg.AbsenceThreshold <= 0 {
		cfg.AbsenceThreshold = DefaultConfig().AbsenceThreshold
	}
	return &reconciler{
		ex:            ex,
		st:            st,
		book:          book,
		accountID:     accountID,
		cfg:           cfg,
		log:           log,
		absenceCounts: make(map[string]int),
	}
}

// Start runs the reconciliation loop until ctx is cancelled.
// The first pass runs immediately after the first tick (i.e. after one
// Interval) so it does not race with startup position seeding.
func (r *reconciler) Start(ctx context.Context) {
	r.log.Info("reconciler: starting",
		zap.Duration("interval", r.cfg.Interval),
		zap.Int("absence_threshold", r.cfg.AbsenceThreshold),
	)

	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.log.Info("reconciler: stopping (context cancelled)")
			return
		case <-ticker.C:
			r.runOnce(ctx)
		}
	}
}

// runOnce executes a single reconciliation pass.
// It is exported via the test-only helper RunOnceForTest to allow direct
// invocation in tests without waiting for the ticker.
func (r *reconciler) runOnce(ctx context.Context) {
	client.Metrics.ReconcileRunsTotal.Inc()

	// Record the time before the REST call. Any MarkBook entry with
	// PositionLoadedAt >= fetchedAt was updated by the account streamer
	// during or after this call — we must not overwrite it.
	fetchedAt := time.Now()

	positions, err := r.ex.Positions(ctx, r.accountID)
	if err != nil {
		client.Metrics.ReconcileErrorsTotal.Inc()
		r.log.Warn("reconciler: REST positions fetch failed — MarkBook unchanged",
			zap.String("account", r.accountID),
			zap.Error(err),
		)
		return
	}

	// Build a set of symbols present in the REST response.
	restSymbols := make(map[string]models.Position, len(positions))
	for _, p := range positions {
		restSymbols[p.Symbol] = p
	}

	corrected := 0

	// ── Pass 1: add missing or update stale MarkBook entries ─────────────────
	for sym, restPos := range restSymbols {
		// Determine the effective AvgOpenPrice: prefer AverageOpenPrice from
		// REST; fall back to ClosePrice if zero (matches startup seed logic).
		avgOpen := restPos.AverageOpenPrice
		if avgOpen.IsZero() {
			avgOpen = restPos.ClosePrice
		}

		snap := r.book.Snapshot(sym)
		positionExists := snap.Quantity != "" // non-empty means a position is loaded

		if !positionExists {
			// Symbol is in REST but absent from MarkBook — add it.
			r.book.LoadPosition(
				sym,
				restPos.AccountNumber,
				restPos.Quantity.String(),
				restPos.QuantityDirection,
				avgOpen,
			)
			corrected++
			r.log.Info("reconciler: added missing position",
				zap.String("symbol", sym),
				zap.String("avg_open", avgOpen.String()),
			)
		} else {
			// Symbol exists in MarkBook. Only update if:
			//   (a) the existing AvgOpenPrice is zero (the known Phase 2 sentinel), OR
			//   (b) the REST UpdatedAt is newer than the MarkBook entry's LoadedAt
			//       AND the entry was not updated by the streamer after fetchedAt.
			//
			// Guard: if PositionLoadedAt >= fetchedAt, a streamer event arrived
			// during this REST call — leave that entry intact.
			if !snap.PositionLoadedAt.Before(fetchedAt) {
				// Streamer updated this entry after we started the REST call.
				// The streamer data is authoritative for quantity/direction;
				// but it carries decimal.Zero for AvgOpenPrice. To avoid
				// regressing a previously correct AvgOpenPrice, only patch
				// if the existing value is still zero.
				if !snap.AvgOpenPrice.IsZero() {
					continue // streamer wrote a non-zero price — trust it
				}
			}

			// Patch if AvgOpenPrice is zero or differs from REST.
			if snap.AvgOpenPrice.IsZero() || !snap.AvgOpenPrice.Equal(avgOpen) {
				r.book.LoadPosition(
					sym,
					restPos.AccountNumber,
					restPos.Quantity.String(),
					restPos.QuantityDirection,
					avgOpen,
				)
				corrected++
				r.log.Info("reconciler: corrected AvgOpenPrice",
					zap.String("symbol", sym),
					zap.String("was", snap.AvgOpenPrice.String()),
					zap.String("now", avgOpen.String()),
				)
			}
		}

		// Symbol was seen — clear any absence counter.
		delete(r.absenceCounts, sym)

		// Persist snapshot to store (best-effort; never blocks on error).
		r.writeSnapshot(ctx, restPos, fetchedAt)
	}

	// ── Pass 2: handle symbols in MarkBook absent from REST ──────────────────
	for _, snap := range r.book.AllSnapshots() {
		sym := snap.Symbol
		if snap.Quantity == "" {
			// Quote-only entry (no position) — not our concern.
			continue
		}
		if _, seen := restSymbols[sym]; seen {
			continue // already handled above
		}

		// Symbol is in MarkBook but absent from REST.
		r.absenceCounts[sym]++
		count := r.absenceCounts[sym]

		if count >= r.cfg.AbsenceThreshold {
			r.book.RemovePosition(sym)
			delete(r.absenceCounts, sym)
			r.log.Info("reconciler: removed position absent from REST",
				zap.String("symbol", sym),
				zap.Int("absence_passes", count),
			)
		} else {
			r.log.Debug("reconciler: position absent from REST — awaiting confirmation",
				zap.String("symbol", sym),
				zap.Int("absence_count", count),
				zap.Int("threshold", r.cfg.AbsenceThreshold),
			)
		}
	}

	if corrected > 0 {
		client.Metrics.ReconcilePositionsCorrected.Add(float64(corrected))
	}

	r.log.Debug("reconciler: pass complete",
		zap.Int("rest_positions", len(positions)),
		zap.Int("corrected", corrected),
	)
}

// writeSnapshot persists a position snapshot from the REST response.
// Errors are logged but never propagated — a write failure must not abort the
// reconciliation pass or modify MarkBook state.
func (r *reconciler) writeSnapshot(ctx context.Context, p models.Position, snapshottedAt time.Time) {
	if r.st == nil {
		return
	}
	snap := store.PositionSnapshot{
		AccountNumber:     p.AccountNumber,
		Symbol:            p.Symbol,
		InstrumentType:    p.InstrumentType,
		Quantity:          p.Quantity.String(),
		QuantityDirection: p.QuantityDirection,
		AvgOpenPrice:      effectiveAvgOpen(p).String(),
		ClosePrice:        p.ClosePrice.String(),
		ExpiresAt:         p.ExpiresAt,
		SnapshottedAt:     snapshottedAt,
		Source:            store.SourceReconciliation,
	}
	if err := r.st.WritePositionSnapshot(ctx, snap); err != nil {
		r.log.Warn("reconciler: WritePositionSnapshot failed",
			zap.String("symbol", p.Symbol),
			zap.Error(err),
		)
	}
}

// effectiveAvgOpen returns AverageOpenPrice if non-zero, else ClosePrice.
// Mirrors the startup seed logic in seedMarkBookFromREST.
func effectiveAvgOpen(p models.Position) decimal.Decimal {
	if !p.AverageOpenPrice.IsZero() {
		return p.AverageOpenPrice
	}
	return p.ClosePrice
}

// RunOnceForTest exposes runOnce for direct invocation in tests.
// Not part of the Reconciler interface — test files import this package
// directly and type-assert to *reconciler.
func RunOnceForTest(r Reconciler, ctx context.Context) {
	r.(*reconciler).runOnce(ctx)
}
