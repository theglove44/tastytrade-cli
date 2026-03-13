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
	"sync"
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
	LatestResult() (Result, bool)
}

// Status is the structured outcome of a reconciliation pass.
type Status string

const (
	StatusOK            Status = "ok"
	StatusDriftDetected Status = "drift_detected"
	StatusPartial       Status = "partial"
	StatusError         Status = "error"
)

// Mismatch captures one detected drift item from a reconciliation pass.
type Mismatch struct {
	Symbol   string `json:"symbol"`
	Category string `json:"category"`
	Action   string `json:"action,omitempty"`
}

// Result is the structured outcome of a single reconciliation pass.
type Result struct {
	RunAt              time.Time      `json:"run_at"`
	Duration           time.Duration  `json:"duration"`
	Status             Status         `json:"status"`
	PositionsChecked   int            `json:"positions_checked"`
	SymbolsChecked     int            `json:"symbols_checked"`
	MismatchCount      int            `json:"mismatch_count"`
	MismatchCategories map[string]int `json:"mismatch_categories,omitempty"`
	RecoveryTriggered  bool           `json:"recovery_triggered"`
	Action             string         `json:"action,omitempty"`
	ErrorText          string         `json:"error_text,omitempty"`
	Mismatches         []Mismatch     `json:"mismatches,omitempty"`
}

func (r *Result) addMismatch(symbol, category, action string) {
	r.MismatchCount++
	r.MismatchCategories[category]++
	r.Mismatches = append(r.Mismatches, Mismatch{Symbol: symbol, Category: category, Action: action})
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

	latestMu     sync.RWMutex
	latestResult *Result
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

// LatestResult returns the latest completed reconciliation result.
func (r *reconciler) LatestResult() (Result, bool) {
	r.latestMu.RLock()
	defer r.latestMu.RUnlock()
	if r.latestResult == nil {
		return Result{}, false
	}
	return cloneResult(*r.latestResult), true
}

// runOnce executes a single reconciliation pass.
// It is exported via the test-only helper RunOnceForTest to allow direct
// invocation in tests without waiting for the ticker.
func (r *reconciler) runOnce(ctx context.Context) Result {
	startedAt := time.Now()
	result := Result{
		RunAt:              startedAt,
		Status:             StatusOK,
		MismatchCategories: make(map[string]int),
	}
	client.Metrics.ReconcileRunsTotal.Inc()

	// Record the time before the REST call. Any MarkBook entry with
	// PositionLoadedAt >= fetchedAt was updated by the account streamer
	// during or after this call — we must not overwrite it.
	fetchedAt := startedAt

	positions, err := r.ex.Positions(ctx, r.accountID)
	if err != nil {
		result.Status = StatusError
		result.ErrorText = err.Error()
		result.Duration = time.Since(startedAt)
		client.Metrics.ReconcileErrorsTotal.Inc()
		r.setLatestResult(result)
		r.recordMetrics(result)
		r.log.Warn("reconciler: pass complete",
			zap.String("status", string(result.Status)),
			zap.String("account", r.accountID),
			zap.Int("positions_checked", result.PositionsChecked),
			zap.Int("symbols_checked", result.SymbolsChecked),
			zap.Int("mismatches", result.MismatchCount),
			zap.Bool("recovery_triggered", result.RecoveryTriggered),
			zap.String("action", result.Action),
			zap.Duration("duration", result.Duration),
			zap.String("error", result.ErrorText),
		)
		return result
	}

	result.PositionsChecked = len(positions)

	restSymbols := make(map[string]models.Position, len(positions))
	for _, p := range positions {
		restSymbols[p.Symbol] = p
	}

	storeWriteErrors := 0

	for sym, restPos := range restSymbols {
		avgOpen := restPos.AverageOpenPrice
		if avgOpen.IsZero() {
			avgOpen = restPos.ClosePrice
		}

		snap := r.book.Snapshot(sym)
		positionExists := snap.Quantity != ""

		if !positionExists {
			r.book.LoadPosition(
				sym,
				restPos.AccountNumber,
				restPos.Quantity.String(),
				restPos.QuantityDirection,
				avgOpen,
			)
			result.addMismatch(sym, "missing_in_markbook", "load_position")
		} else {
			if !snap.PositionLoadedAt.Before(fetchedAt) && !snap.AvgOpenPrice.IsZero() {
				delete(r.absenceCounts, sym)
				if err := r.writeSnapshot(ctx, restPos, fetchedAt); err != nil {
					storeWriteErrors++
				}
				continue
			}

			if snap.AvgOpenPrice.IsZero() || !snap.AvgOpenPrice.Equal(avgOpen) {
				r.book.LoadPosition(
					sym,
					restPos.AccountNumber,
					restPos.Quantity.String(),
					restPos.QuantityDirection,
					avgOpen,
				)
				result.addMismatch(sym, "avg_open_drift", "load_position")
			}
		}

		delete(r.absenceCounts, sym)
		if err := r.writeSnapshot(ctx, restPos, fetchedAt); err != nil {
			storeWriteErrors++
		}
	}

	bookPositions := 0
	for _, snap := range r.book.AllSnapshots() {
		sym := snap.Symbol
		if snap.Quantity == "" {
			continue
		}
		bookPositions++
		if _, seen := restSymbols[sym]; seen {
			continue
		}

		r.absenceCounts[sym]++
		count := r.absenceCounts[sym]

		if count >= r.cfg.AbsenceThreshold {
			r.book.RemovePosition(sym)
			delete(r.absenceCounts, sym)
			result.addMismatch(sym, "absent_from_rest", "remove_position")
		} else {
			result.addMismatch(sym, "absence_pending", "await_confirmation")
		}
	}
	result.SymbolsChecked = maxInt(result.PositionsChecked, bookPositions)

	result.Duration = time.Since(startedAt)
	if storeWriteErrors > 0 {
		result.Status = StatusPartial
		result.ErrorText = "snapshot_write_failed"
	}
	if result.MismatchCount > 0 {
		if result.Status == StatusOK {
			result.Status = StatusDriftDetected
		}
		result.RecoveryTriggered = true
		result.Action = "markbook_reconciled"
		client.Metrics.ReconcilePositionsCorrected.Add(float64(result.MismatchCount))
		for _, mm := range result.Mismatches {
			r.log.Debug("reconciler: mismatch detected",
				zap.String("symbol", mm.Symbol),
				zap.String("category", mm.Category),
				zap.String("action", mm.Action),
			)
		}
	}
	if result.Status == StatusPartial && result.Action == "" {
		result.Action = "snapshot_write_degraded"
	}

	r.setLatestResult(result)
	r.recordMetrics(result)
	r.log.Info("reconciler: pass complete",
		zap.String("status", string(result.Status)),
		zap.String("account", r.accountID),
		zap.Int("positions_checked", result.PositionsChecked),
		zap.Int("symbols_checked", result.SymbolsChecked),
		zap.Int("mismatches", result.MismatchCount),
		zap.Bool("recovery_triggered", result.RecoveryTriggered),
		zap.String("action", result.Action),
		zap.Duration("duration", result.Duration),
	)
	return result
}

func (r *reconciler) setLatestResult(result Result) {
	copyResult := cloneResult(result)
	r.latestMu.Lock()
	defer r.latestMu.Unlock()
	r.latestResult = &copyResult
}

func (r *reconciler) recordMetrics(result Result) {
	client.Metrics.ReconcileRunsByStatus.WithLabelValues(string(result.Status)).Inc()
	client.Metrics.ReconcileLastStatus.WithLabelValues(string(result.Status)).Set(1)
	for _, status := range []Status{StatusOK, StatusDriftDetected, StatusPartial, StatusError} {
		if status == result.Status {
			continue
		}
		client.Metrics.ReconcileLastStatus.WithLabelValues(string(status)).Set(0)
	}
	client.Metrics.ReconcileLastDurationSeconds.Set(result.Duration.Seconds())
	client.Metrics.ReconcileLastMismatchCount.Set(float64(result.MismatchCount))
	if result.ErrorText != "" {
		client.Metrics.ReconcileErrorsByType.WithLabelValues(result.ErrorText).Inc()
	}
}

// writeSnapshot persists a position snapshot from the REST response.
// Errors are logged but never propagated — a write failure must not abort the
// reconciliation pass or modify MarkBook state.
func (r *reconciler) writeSnapshot(ctx context.Context, p models.Position, snapshottedAt time.Time) error {
	if r.st == nil {
		return nil
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
		return err
	}
	return nil
}

// effectiveAvgOpen returns AverageOpenPrice if non-zero, else ClosePrice.
// Mirrors the startup seed logic in seedMarkBookFromREST.
func effectiveAvgOpen(p models.Position) decimal.Decimal {
	if !p.AverageOpenPrice.IsZero() {
		return p.AverageOpenPrice
	}
	return p.ClosePrice
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func cloneResult(in Result) Result {
	out := in
	if in.MismatchCategories != nil {
		out.MismatchCategories = make(map[string]int, len(in.MismatchCategories))
		for k, v := range in.MismatchCategories {
			out.MismatchCategories[k] = v
		}
	}
	if in.Mismatches != nil {
		out.Mismatches = append([]Mismatch(nil), in.Mismatches...)
	}
	return out
}

// RunOnceForTest exposes runOnce for direct invocation in tests.
// Not part of the Reconciler interface — test files import this package
// directly and type-assert to *reconciler.
func RunOnceForTest(r Reconciler, ctx context.Context) Result {
	return r.(*reconciler).runOnce(ctx)
}
