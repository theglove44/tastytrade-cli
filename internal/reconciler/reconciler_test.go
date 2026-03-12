package reconciler_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/theglove44/tastytrade-cli/internal/models"
	"github.com/theglove44/tastytrade-cli/internal/reconciler"
	"github.com/theglove44/tastytrade-cli/internal/store"
	"github.com/theglove44/tastytrade-cli/internal/valuation"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

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

func newBook() *valuation.MarkBook {
	return valuation.NewMarkBook()
}

func silentLogger() *zap.Logger {
	l, _ := zap.NewDevelopment()
	return l
}

func newRec(ex *mockExchange, st store.Store, book *valuation.MarkBook, cfg reconciler.Config) reconciler.Reconciler {
	return reconciler.New(ex, st, book, "TEST123", cfg, silentLogger())
}

func tightConfig() reconciler.Config {
	return reconciler.Config{
		Interval:         10 * time.Millisecond, // fast for Start() test
		AbsenceThreshold: 2,
	}
}

// ── mockExchange ──────────────────────────────────────────────────────────────

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

// Satisfy exchange.Exchange interface — stubs for unused methods.
func (m *mockExchange) Accounts(_ context.Context) ([]models.Account, error)       { return nil, nil }
func (m *mockExchange) Orders(_ context.Context, _ string) ([]models.Order, error) { return nil, nil }
func (m *mockExchange) DryRun(_ context.Context, _ string, _ models.NewOrder, _ string) (models.DryRunResult, error) {
	return models.DryRunResult{}, nil
}
func (m *mockExchange) QuoteToken(_ context.Context) (models.QuoteToken, error) {
	return models.QuoteToken{}, nil
}

// ── mockStore ─────────────────────────────────────────────────────────────────

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

// Stub remaining Store methods.
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

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestReconciler_AddsNewPosition(t *testing.T) {
	book := newBook()
	ex := &mockExchange{}
	ex.setPositions([]models.Position{
		newPos(".SPY230120C400", "2", "Short", "1.50"),
	})

	r := newRec(ex, nil, book, tightConfig())
	reconciler.RunOnceForTest(r, context.Background())

	snap := book.Snapshot(".SPY230120C400")
	if snap.Quantity == "" {
		t.Fatal("position was not added to MarkBook")
	}
	if snap.Quantity != "2" {
		t.Errorf("Quantity = %q, want %q", snap.Quantity, "2")
	}
	if snap.QuantityDirection != "Short" {
		t.Errorf("QuantityDirection = %q, want %q", snap.QuantityDirection, "Short")
	}
	if !snap.AvgOpenPrice.Equal(dec("1.50")) {
		t.Errorf("AvgOpenPrice = %s, want 1.50", snap.AvgOpenPrice)
	}
}

func TestReconciler_CorrectsZeroAvgOpenPrice(t *testing.T) {
	book := newBook()
	book.LoadPosition(".XSP250117C580", "TEST123", "1", "Short", decimal.Zero)

	ex := &mockExchange{}
	ex.setPositions([]models.Position{
		newPos(".XSP250117C580", "1", "Short", "2.30"),
	})

	r := newRec(ex, nil, book, tightConfig())
	reconciler.RunOnceForTest(r, context.Background())

	snap := book.Snapshot(".XSP250117C580")
	if !snap.AvgOpenPrice.Equal(dec("2.30")) {
		t.Errorf("AvgOpenPrice = %s, want 2.30 (reconciler should have patched zero)", snap.AvgOpenPrice)
	}
}

func TestReconciler_DoesNotUpdateIfNotZeroAndUnchanged(t *testing.T) {
	book := newBook()
	book.LoadPosition("SPY", "TEST123", "10", "Long", dec("450.00"))

	loadedAt := book.Snapshot("SPY").PositionLoadedAt

	ex := &mockExchange{}
	ex.setPositions([]models.Position{
		newPos("SPY", "10", "Long", "450.00"),
	})

	r := newRec(ex, nil, book, tightConfig())
	reconciler.RunOnceForTest(r, context.Background())

	snap := book.Snapshot("SPY")
	if !snap.AvgOpenPrice.Equal(dec("450.00")) {
		t.Errorf("AvgOpenPrice changed unexpectedly: got %s", snap.AvgOpenPrice)
	}
	if snap.PositionLoadedAt.After(loadedAt) {
		t.Error("PositionLoadedAt advanced — reconciler unnecessarily re-loaded position")
	}
}

func TestReconciler_AbsenceCounterPreventsRemoval(t *testing.T) {
	book := newBook()
	book.LoadPosition("AAPL", "TEST123", "5", "Long", dec("190.00"))

	ex := &mockExchange{}
	ex.setPositions([]models.Position{})

	cfg := reconciler.Config{Interval: 10 * time.Millisecond, AbsenceThreshold: 2}
	r := newRec(ex, nil, book, cfg)

	reconciler.RunOnceForTest(r, context.Background())
	snap := book.Snapshot("AAPL")
	if snap.Quantity == "" {
		t.Fatal("position removed on first absence — expected preservation until threshold")
	}
}

func TestReconciler_RemovesPositionAfterNPasses(t *testing.T) {
	book := newBook()
	book.LoadPosition("TSLA", "TEST123", "3", "Long", dec("210.00"))

	ex := &mockExchange{}
	ex.setPositions([]models.Position{})

	cfg := reconciler.Config{Interval: 10 * time.Millisecond, AbsenceThreshold: 2}
	r := newRec(ex, nil, book, cfg)

	reconciler.RunOnceForTest(r, context.Background())
	if book.Snapshot("TSLA").Quantity == "" {
		t.Fatal("removed too early after pass 1")
	}

	reconciler.RunOnceForTest(r, context.Background())
	snap := book.Snapshot("TSLA")
	if snap.Quantity != "" {
		t.Errorf("position still present after %d absence passes — expected removal", 2)
	}
}

func TestReconciler_AbsenceCounterResetOnReappearance(t *testing.T) {
	book := newBook()
	book.LoadPosition("NVDA", "TEST123", "1", "Long", dec("500.00"))

	ex := &mockExchange{}

	cfg := reconciler.Config{Interval: 10 * time.Millisecond, AbsenceThreshold: 3}
	r := newRec(ex, nil, book, cfg)

	ex.setPositions([]models.Position{})
	reconciler.RunOnceForTest(r, context.Background())

	ex.setPositions([]models.Position{newPos("NVDA", "1", "Long", "500.00")})
	reconciler.RunOnceForTest(r, context.Background())

	ex.setPositions([]models.Position{})
	reconciler.RunOnceForTest(r, context.Background())
	if book.Snapshot("NVDA").Quantity == "" {
		t.Fatal("position removed after only 1 absence post-reset — counter was not cleared")
	}
}

func TestReconciler_RESTFailureDoesNotModifyMarkBook(t *testing.T) {
	book := newBook()
	book.LoadPosition(".SPX240119P4500", "TEST123", "2", "Short", dec("3.75"))

	before := book.AllSnapshots()

	ex := &mockExchange{}
	ex.setError(errors.New("simulated REST timeout"))

	r := newRec(ex, nil, book, tightConfig())
	reconciler.RunOnceForTest(r, context.Background())

	after := book.AllSnapshots()

	if len(before) != len(after) {
		t.Errorf("MarkBook entry count changed: before=%d after=%d", len(before), len(after))
	}
	snap := book.Snapshot(".SPX240119P4500")
	if !snap.AvgOpenPrice.Equal(dec("3.75")) {
		t.Errorf("AvgOpenPrice changed after REST failure: got %s", snap.AvgOpenPrice)
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
		t.Errorf("WritePositionSnapshot called %d times, want 2", got)
	}

	snap, ok := st.latestForSymbol(".QQQ230915C350")
	if !ok {
		t.Fatal("no snapshot written for .QQQ230915C350")
	}
	snapPrice, err := decimal.NewFromString(snap.AvgOpenPrice)
	if err != nil {
		t.Fatalf("cannot parse snapshot AvgOpenPrice %q: %v", snap.AvgOpenPrice, err)
	}
	if !snapPrice.Equal(dec("4.20")) {
		t.Errorf("snapshot AvgOpenPrice = %q, want 4.20", snap.AvgOpenPrice)
	}
	if snap.Source != store.SourceReconciliation {
		t.Errorf("snapshot Source = %q, want %q", snap.Source, store.SourceReconciliation)
	}
}

func TestReconciler_StoreWriteErrorDoesNotAbortPass(t *testing.T) {
	book := newBook()
	st := &mockStore{writeErr: errors.New("disk full")}
	ex := &mockExchange{}
	ex.setPositions([]models.Position{
		newPos("AMZN", "5", "Long", "185.00"),
	})

	r := reconciler.New(ex, st, book, "TEST123", tightConfig(), silentLogger())
	reconciler.RunOnceForTest(r, context.Background())

	snap := book.Snapshot("AMZN")
	if snap.Quantity == "" {
		t.Fatal("position not added to MarkBook despite store write failure")
	}
	if !snap.AvgOpenPrice.Equal(dec("185.00")) {
		t.Errorf("AvgOpenPrice = %s, want 185.00", snap.AvgOpenPrice)
	}
}

func TestReconciler_Start_RunsUntilContextCancel(t *testing.T) {
	book := newBook()
	ex := &mockExchange{}
	ex.setPositions([]models.Position{})

	cfg := reconciler.Config{Interval: 10 * time.Millisecond, AbsenceThreshold: 2}
	r := reconciler.New(ex, nil, book, "TEST123", cfg, silentLogger())

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

	snap := book.Snapshot(".SPY230120C400")
	if !snap.AvgOpenPrice.Equal(dec("1.25")) {
		t.Errorf("AvgOpenPrice = %s, want 1.25 (ClosePrice fallback)", snap.AvgOpenPrice)
	}
}

func TestReconciler_NoUnnecessaryReloadWhenPriceCorrect(t *testing.T) {
	book := newBook()
	book.LoadPosition("MSFT", "TEST123", "10", "Long", dec("380.00"))
	loadedAt := book.Snapshot("MSFT").PositionLoadedAt

	ex := &mockExchange{}
	ex.setPositions([]models.Position{
		newPos("MSFT", "10", "Long", "380.00"),
	})

	r := newRec(ex, nil, book, tightConfig())
	reconciler.RunOnceForTest(r, context.Background())

	snap := book.Snapshot("MSFT")
	if !snap.AvgOpenPrice.Equal(dec("380.00")) {
		t.Errorf("AvgOpenPrice = %s, want 380.00", snap.AvgOpenPrice)
	}
	if snap.PositionLoadedAt.After(loadedAt) {
		t.Errorf("PositionLoadedAt advanced — reconciler re-loaded an already-correct position")
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
	reconciler.RunOnceForTest(r, context.Background())

	if !book.Snapshot("GOOG").AvgOpenPrice.Equal(dec("140.00")) {
		t.Errorf("GOOG AvgOpenPrice changed unexpectedly")
	}
	if !book.Snapshot(".GOOG240119C150").AvgOpenPrice.Equal(dec("3.50")) {
		t.Errorf(".GOOG240119C150 AvgOpenPrice = %s, want 3.50", book.Snapshot(".GOOG240119C150").AvgOpenPrice)
	}
	snap := book.Snapshot(".GOOG240119P130")
	if snap.Quantity == "" {
		t.Fatal(".GOOG240119P130 not added to MarkBook")
	}
	if !snap.AvgOpenPrice.Equal(dec("2.10")) {
		t.Errorf(".GOOG240119P130 AvgOpenPrice = %s, want 2.10", snap.AvgOpenPrice)
	}
}
