package reconciler_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/theglove44/tastytrade-cli/internal/client"
	"github.com/theglove44/tastytrade-cli/internal/models"
	"github.com/theglove44/tastytrade-cli/internal/reconciler"
	"github.com/theglove44/tastytrade-cli/internal/store"
	"github.com/theglove44/tastytrade-cli/internal/valuation"
)

func dec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic("bad decimal in test: " + s)
	}
	return d
}

func newPos(symbol, qty, dir, avgOpen string) models.Position {
	return models.Position{
		AccountNumber:     "TEST123",
		Symbol:            symbol,
		InstrumentType:    "Equity Option",
		Quantity:          dec(qty),
		QuantityDirection: dir,
		AverageOpenPrice:  dec(avgOpen),
		UpdatedAt:         time.Now(),
	}
}

func newBook() *valuation.MarkBook { return valuation.NewMarkBook() }

func silentLogger() *zap.Logger {
	l, _ := zap.NewDevelopment()
	return l
}

func observedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	return zap.New(core), logs
}

func newRec(ex *mockExchange, st store.Store, book *valuation.MarkBook, cfg reconciler.Config) reconciler.Reconciler {
	return reconciler.New(ex, st, book, "TEST123", cfg, silentLogger())
}

func tightConfig() reconciler.Config {
	return reconciler.Config{Interval: 10 * time.Millisecond, AbsenceThreshold: 2}
}

type mockExchange struct {
	mu        sync.Mutex
	positions []models.Position
	err       error
	callCount int
}

func (m *mockExchange) setPositions(ps []models.Position) {
	m.mu.Lock()
	m.positions = ps
	m.mu.Unlock()
}

func (m *mockExchange) setError(err error) {
	m.mu.Lock()
	m.err = err
	m.mu.Unlock()
}

func (m *mockExchange) Positions(_ context.Context, _ string) ([]models.Position, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	if m.err != nil {
		return nil, m.err
	}
	out := make([]models.Position, len(m.positions))
	copy(out, m.positions)
	return out, nil
}

func (m *mockExchange) Accounts(_ context.Context) ([]models.Account, error)       { return nil, nil }
func (m *mockExchange) Orders(_ context.Context, _ string) ([]models.Order, error) { return nil, nil }
func (m *mockExchange) DryRun(_ context.Context, _ string, _ models.NewOrder, _ string) (models.DryRunResult, error) {
	return models.DryRunResult{}, nil
}
func (m *mockExchange) QuoteToken(_ context.Context) (models.QuoteToken, error) {
	return models.QuoteToken{}, nil
}

type mockStore struct {
	mu        sync.Mutex
	snapshots []store.PositionSnapshot
	writeErr  error
}

func (s *mockStore) WritePositionSnapshot(_ context.Context, snap store.PositionSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writeErr != nil {
		return s.writeErr
	}
	s.snapshots = append(s.snapshots, snap)
	return nil
}

func (s *mockStore) snapshotCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.snapshots)
}

func (s *mockStore) latestForSymbol(sym string) (store.PositionSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.snapshots) - 1; i >= 0; i-- {
		if s.snapshots[i].Symbol == sym {
			return s.snapshots[i], true
		}
	}
	return store.PositionSnapshot{}, false
}

func (s *mockStore) WriteFill(_ context.Context, _ store.FillRecord) error       { return nil }
func (s *mockStore) WriteBalance(_ context.Context, _ store.BalanceRecord) error { return nil }
func (s *mockStore) LatestBalance(_ context.Context, _ string) (store.BalanceRecord, error) {
	return store.BalanceRecord{}, nil
}
func (s *mockStore) RecentFills(_ context.Context, _ string, _ time.Time) ([]store.FillRecord, error) {
	return nil, nil
}
func (s *mockStore) ActivePositionSymbols(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (s *mockStore) Close() error { return nil }

func TestReconciler_LatestResult_NoRunYet(t *testing.T) {
	book := newBook()
	ex := &mockExchange{}
	r := reconciler.New(ex, nil, book, "TEST123", tightConfig(), silentLogger())

	if _, ok := r.LatestResult(); ok {
		t.Fatal("LatestResult reported available before any run")
	}
}

func TestReconciler_RunOnce_HealthyResult(t *testing.T) {
	book := newBook()
	book.LoadPosition("SPY", "TEST123", "10", "Long", dec("450.00"))
	ex := &mockExchange{}
	ex.setPositions([]models.Position{newPos("SPY", "10", "Long", "450.00")})
	beforeOK := testutil.ToFloat64(client.Metrics.ReconcileRunsByStatus.WithLabelValues(string(reconciler.StatusOK)))
	logger, logs := observedLogger()
	r := reconciler.New(ex, nil, book, "TEST123", tightConfig(), logger)

	result := reconciler.RunOnceForTest(r, context.Background())

	if result.Status != reconciler.StatusOK {
		t.Fatalf("Status = %q, want %q", result.Status, reconciler.StatusOK)
	}
	if result.MismatchCount != 0 {
		t.Fatalf("MismatchCount = %d, want 0", result.MismatchCount)
	}
	if result.PositionsChecked != 1 {
		t.Fatalf("PositionsChecked = %d, want 1", result.PositionsChecked)
	}
	if result.RecoveryTriggered {
		t.Fatal("RecoveryTriggered = true, want false")
	}
	if got := testutil.ToFloat64(client.Metrics.ReconcileRunsByStatus.WithLabelValues(string(reconciler.StatusOK))); got != beforeOK+1 {
		t.Fatalf("ok status counter delta = %v, want +1", got-beforeOK)
	}
	if got := testutil.ToFloat64(client.Metrics.ReconcileLastStatus.WithLabelValues(string(reconciler.StatusOK))); got != 1 {
		t.Fatalf("last status ok gauge = %v, want 1", got)
	}
	entries := logs.FilterMessage("reconciler: pass complete").All()
	if len(entries) != 1 {
		t.Fatalf("summary log count = %d, want 1", len(entries))
	}
	latest, ok := r.LatestResult()
	if !ok {
		t.Fatal("LatestResult unavailable after healthy run")
	}
	if latest.Status != reconciler.StatusOK || latest.MismatchCount != 0 || latest.PositionsChecked != 1 {
		t.Fatalf("LatestResult = %+v, want healthy snapshot", latest)
	}
}

func TestReconciler_RunOnce_DriftDetectedResult(t *testing.T) {
	book := newBook()
	book.LoadPosition(".XSP250117C580", "TEST123", "1", "Short", decimal.Zero)
	ex := &mockExchange{}
	ex.setPositions([]models.Position{newPos(".XSP250117C580", "1", "Short", "2.30")})
	beforeDrift := testutil.ToFloat64(client.Metrics.ReconcileRunsByStatus.WithLabelValues(string(reconciler.StatusDriftDetected)))
	beforeCorrected := testutil.ToFloat64(client.Metrics.ReconcilePositionsCorrected)
	logger, logs := observedLogger()
	r := reconciler.New(ex, nil, book, "TEST123", tightConfig(), logger)

	result := reconciler.RunOnceForTest(r, context.Background())

	if result.Status != reconciler.StatusDriftDetected {
		t.Fatalf("Status = %q, want %q", result.Status, reconciler.StatusDriftDetected)
	}
	if result.MismatchCount != 1 {
		t.Fatalf("MismatchCount = %d, want 1", result.MismatchCount)
	}
	if result.MismatchCategories["avg_open_drift"] != 1 {
		t.Fatalf("avg_open_drift count = %d, want 1", result.MismatchCategories["avg_open_drift"])
	}
	if !result.RecoveryTriggered {
		t.Fatal("RecoveryTriggered = false, want true")
	}
	if !book.Snapshot(".XSP250117C580").AvgOpenPrice.Equal(dec("2.30")) {
		t.Fatal("MarkBook was not corrected")
	}
	if got := testutil.ToFloat64(client.Metrics.ReconcileRunsByStatus.WithLabelValues(string(reconciler.StatusDriftDetected))); got != beforeDrift+1 {
		t.Fatalf("drift status counter delta = %v, want +1", got-beforeDrift)
	}
	if got := testutil.ToFloat64(client.Metrics.ReconcilePositionsCorrected); got != beforeCorrected+1 {
		t.Fatalf("positions corrected counter delta = %v, want +1", got-beforeCorrected)
	}
	if got := testutil.ToFloat64(client.Metrics.ReconcileLastMismatchCount); got != 1 {
		t.Fatalf("last mismatch gauge = %v, want 1", got)
	}
	if len(logs.FilterMessage("reconciler: mismatch detected").All()) == 0 {
		t.Fatal("expected mismatch detail debug log")
	}
	latest, ok := r.LatestResult()
	if !ok {
		t.Fatal("LatestResult unavailable after drift run")
	}
	if latest.Status != reconciler.StatusDriftDetected || latest.MismatchCategories["avg_open_drift"] != 1 {
		t.Fatalf("LatestResult = %+v, want drift snapshot", latest)
	}
}

func TestReconciler_RunOnce_ErrorResult(t *testing.T) {
	book := newBook()
	book.LoadPosition(".SPX240119P4500", "TEST123", "2", "Short", dec("3.75"))
	ex := &mockExchange{}
	ex.setError(errors.New("simulated REST timeout"))
	beforeErr := testutil.ToFloat64(client.Metrics.ReconcileErrorsTotal)
	beforeErrType := testutil.ToFloat64(client.Metrics.ReconcileErrorsByType.WithLabelValues("simulated REST timeout"))
	logger, logs := observedLogger()
	r := reconciler.New(ex, nil, book, "TEST123", tightConfig(), logger)

	result := reconciler.RunOnceForTest(r, context.Background())

	if result.Status != reconciler.StatusError {
		t.Fatalf("Status = %q, want %q", result.Status, reconciler.StatusError)
	}
	if result.ErrorText != "simulated REST timeout" {
		t.Fatalf("ErrorText = %q, want simulated REST timeout", result.ErrorText)
	}
	if !book.Snapshot(".SPX240119P4500").AvgOpenPrice.Equal(dec("3.75")) {
		t.Fatal("MarkBook changed on error path")
	}
	if got := testutil.ToFloat64(client.Metrics.ReconcileErrorsTotal); got != beforeErr+1 {
		t.Fatalf("reconcile errors counter delta = %v, want +1", got-beforeErr)
	}
	if got := testutil.ToFloat64(client.Metrics.ReconcileErrorsByType.WithLabelValues("simulated REST timeout")); got != beforeErrType+1 {
		t.Fatalf("error-by-type counter delta = %v, want +1", got-beforeErrType)
	}
	entries := logs.FilterMessage("reconciler: pass complete").All()
	if len(entries) != 1 || entries[0].Level != zapcore.WarnLevel {
		t.Fatal("expected single warn summary log for error path")
	}
	latest, ok := r.LatestResult()
	if !ok {
		t.Fatal("LatestResult unavailable after error run")
	}
	if latest.Status != reconciler.StatusError || latest.ErrorText != "simulated REST timeout" {
		t.Fatalf("LatestResult = %+v, want error snapshot", latest)
	}
}

func TestReconciler_RunOnce_PartialResultOnStoreWriteError(t *testing.T) {
	book := newBook()
	st := &mockStore{writeErr: errors.New("disk full")}
	ex := &mockExchange{}
	ex.setPositions([]models.Position{newPos("AMZN", "5", "Long", "185.00")})
	beforePartial := testutil.ToFloat64(client.Metrics.ReconcileRunsByStatus.WithLabelValues(string(reconciler.StatusPartial)))
	r := reconciler.New(ex, st, book, "TEST123", tightConfig(), silentLogger())

	result := reconciler.RunOnceForTest(r, context.Background())

	if result.Status != reconciler.StatusPartial {
		t.Fatalf("Status = %q, want %q", result.Status, reconciler.StatusPartial)
	}
	if result.ErrorText != "snapshot_write_failed" {
		t.Fatalf("ErrorText = %q, want snapshot_write_failed", result.ErrorText)
	}
	if book.Snapshot("AMZN").Quantity == "" {
		t.Fatal("MarkBook position missing despite partial success")
	}
	if got := testutil.ToFloat64(client.Metrics.ReconcileRunsByStatus.WithLabelValues(string(reconciler.StatusPartial))); got != beforePartial+1 {
		t.Fatalf("partial status counter delta = %v, want +1", got-beforePartial)
	}
}

func TestReconciler_AbsenceCounterPreventsRemoval(t *testing.T) {
	book := newBook()
	book.LoadPosition("AAPL", "TEST123", "5", "Long", dec("190.00"))
	ex := &mockExchange{}
	ex.setPositions([]models.Position{})
	r := newRec(ex, nil, book, reconciler.Config{Interval: 10 * time.Millisecond, AbsenceThreshold: 2})

	result := reconciler.RunOnceForTest(r, context.Background())
	if result.Status != reconciler.StatusDriftDetected {
		t.Fatalf("Status = %q, want drift_detected", result.Status)
	}
	if book.Snapshot("AAPL").Quantity == "" {
		t.Fatal("position removed on first absence")
	}
}

func TestReconciler_RemovesPositionAfterNPasses(t *testing.T) {
	book := newBook()
	book.LoadPosition("TSLA", "TEST123", "3", "Long", dec("210.00"))
	ex := &mockExchange{}
	ex.setPositions([]models.Position{})
	r := newRec(ex, nil, book, reconciler.Config{Interval: 10 * time.Millisecond, AbsenceThreshold: 2})

	reconciler.RunOnceForTest(r, context.Background())
	reconciler.RunOnceForTest(r, context.Background())
	if book.Snapshot("TSLA").Quantity != "" {
		t.Fatal("position still present after threshold")
	}
}

func TestReconciler_AbsenceCounterResetOnReappearance(t *testing.T) {
	book := newBook()
	book.LoadPosition("NVDA", "TEST123", "1", "Long", dec("500.00"))
	ex := &mockExchange{}
	r := newRec(ex, nil, book, reconciler.Config{Interval: 10 * time.Millisecond, AbsenceThreshold: 3})

	ex.setPositions([]models.Position{})
	reconciler.RunOnceForTest(r, context.Background())
	ex.setPositions([]models.Position{newPos("NVDA", "1", "Long", "500.00")})
	reconciler.RunOnceForTest(r, context.Background())
	ex.setPositions([]models.Position{})
	reconciler.RunOnceForTest(r, context.Background())
	if book.Snapshot("NVDA").Quantity == "" {
		t.Fatal("position removed after only 1 absence post-reset")
	}
}

func TestReconciler_WritesPositionSnapshot(t *testing.T) {
	book := newBook()
	st := &mockStore{}
	ex := &mockExchange{}
	ex.setPositions([]models.Position{
		newPos(".QQQ230915C350", "1", "Long", "4.20"),
		newPos("QQQ", "100", "Long", "360.00"),
	})
	r := reconciler.New(ex, st, book, "TEST123", tightConfig(), silentLogger())

	reconciler.RunOnceForTest(r, context.Background())
	if got := st.snapshotCount(); got != 2 {
		t.Fatalf("snapshot count = %d, want 2", got)
	}
	snap, ok := st.latestForSymbol(".QQQ230915C350")
	if !ok {
		t.Fatal("missing snapshot")
	}
	if snap.Source != store.SourceReconciliation {
		t.Fatalf("snapshot source = %q, want %q", snap.Source, store.SourceReconciliation)
	}
}

func TestReconciler_Start_RunsUntilContextCancel(t *testing.T) {
	book := newBook()
	ex := &mockExchange{}
	ex.setPositions([]models.Position{})
	r := reconciler.New(ex, nil, book, "TEST123", tightConfig(), silentLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		r.Start(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not return within 2s after context cancel")
	}
}

func TestReconciler_FallbackToClosePrice(t *testing.T) {
	book := newBook()
	ex := &mockExchange{}
	pos := newPos(".SPY230120C400", "1", "Short", "0")
	pos.ClosePrice = dec("1.25")
	ex.setPositions([]models.Position{pos})
	r := newRec(ex, nil, book, tightConfig())

	reconciler.RunOnceForTest(r, context.Background())
	if !book.Snapshot(".SPY230120C400").AvgOpenPrice.Equal(dec("1.25")) {
		t.Fatal("ClosePrice fallback not applied")
	}
}

func TestReconciler_MultiplePositions(t *testing.T) {
	book := newBook()
	book.LoadPosition("GOOG", "TEST123", "1", "Long", dec("140.00"))
	book.LoadPosition(".GOOG240119C150", "TEST123", "2", "Long", decimal.Zero)
	ex := &mockExchange{}
	ex.setPositions([]models.Position{
		newPos("GOOG", "1", "Long", "140.00"),
		newPos(".GOOG240119C150", "2", "Long", "3.50"),
		newPos(".GOOG240119P130", "2", "Short", "2.10"),
	})
	r := newRec(ex, nil, book, tightConfig())

	result := reconciler.RunOnceForTest(r, context.Background())
	if result.MismatchCount != 2 {
		t.Fatalf("MismatchCount = %d, want 2", result.MismatchCount)
	}
	if !book.Snapshot(".GOOG240119C150").AvgOpenPrice.Equal(dec("3.50")) {
		t.Fatal("existing option was not corrected")
	}
	if book.Snapshot(".GOOG240119P130").Quantity == "" {
		t.Fatal("new position not added")
	}
}
