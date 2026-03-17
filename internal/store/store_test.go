package store_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/theglove44/tastytrade-cli/internal/store"
)

// openTestStore creates a Store backed by a temp-dir SQLite database.
// The caller is responsible for closing it.
func openTestStore(t *testing.T) store.Store {
	t.Helper()
	dir := t.TempDir()
	// Point the store at our temp dir by setting XDG_CONFIG_HOME.
	t.Setenv("XDG_CONFIG_HOME", dir)
	// On macOS UserConfigDir() uses $HOME/Library/Application Support — override.
	t.Setenv("HOME", dir)
	t.Setenv("AppData", dir) // Windows guard

	// Create the expected subdirectory so Open() can write the DB there.
	if err := os.MkdirAll(filepath.Join(dir, "tastytrade-cli"), 0700); err != nil {
		t.Fatal(err)
	}

	log, _ := zap.NewDevelopment()
	st, err := store.Open(log)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// ── Fill persistence ─────────────────────────────────────────────────────────

func TestWriteFill_Persist(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	fill := store.FillRecord{
		OrderID:       "ORD-001",
		AccountNumber: "ACCT-123",
		Symbol:        ".XSP250117C580",
		Action:        "Sell to Open",
		Quantity:      "1",
		FillPrice:     "1.20",
		FilledAt:      time.Now().UTC().Truncate(time.Second),
		Strategy:      "iron_condor",
		Source:        store.SourceStreamer,
	}

	if err := st.WriteFill(ctx, fill); err != nil {
		t.Fatalf("WriteFill: %v", err)
	}

	fills, err := st.RecentFills(ctx, "ACCT-123", time.Now().Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("RecentFills: %v", err)
	}
	if len(fills) != 1 {
		t.Fatalf("expected 1 fill, got %d", len(fills))
	}
	got := fills[0]
	if got.OrderID != fill.OrderID {
		t.Errorf("OrderID: got %q, want %q", got.OrderID, fill.OrderID)
	}
	if got.Symbol != fill.Symbol {
		t.Errorf("Symbol: got %q, want %q", got.Symbol, fill.Symbol)
	}
	if got.Source != store.SourceStreamer {
		t.Errorf("Source: got %q, want %q", got.Source, store.SourceStreamer)
	}
}

func TestWriteFill_Idempotent(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	fill := store.FillRecord{
		OrderID:       "ORD-DUPE",
		AccountNumber: "ACCT-123",
		Symbol:        ".XSP250117C580",
		Action:        "Sell to Open",
		Quantity:      "1",
		FillPrice:     "1.20",
		FilledAt:      time.Now().UTC(),
		Source:        store.SourceStreamer,
	}

	// Write twice — simulates reconnect snapshot delivering same fill.
	if err := st.WriteFill(ctx, fill); err != nil {
		t.Fatalf("first WriteFill: %v", err)
	}
	if err := st.WriteFill(ctx, fill); err != nil {
		t.Fatalf("second WriteFill (idempotency): %v", err)
	}

	fills, err := st.RecentFills(ctx, "ACCT-123", time.Now().Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("RecentFills: %v", err)
	}
	if len(fills) != 1 {
		t.Errorf("idempotency failed: expected 1 row, got %d", len(fills))
	}
}

func TestWriteFill_SourceField(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	for _, src := range []store.Source{
		store.SourceStreamer,
		store.SourceRESTSync,
		store.SourceReconciliation,
	} {
		fill := store.FillRecord{
			OrderID:       "ORD-" + string(src),
			AccountNumber: "ACCT-SRC",
			Symbol:        "SPY",
			Action:        "Buy to Open",
			Quantity:      "1",
			FillPrice:     "500.00",
			FilledAt:      time.Now().UTC(),
			Source:        src,
		}
		if err := st.WriteFill(ctx, fill); err != nil {
			t.Errorf("WriteFill source=%q: %v", src, err)
		}
	}

	fills, err := st.RecentFills(ctx, "ACCT-SRC", time.Now().Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("RecentFills: %v", err)
	}
	if len(fills) != 3 {
		t.Fatalf("expected 3 fills, got %d", len(fills))
	}
}

// ── Balance persistence ───────────────────────────────────────────────────────

func TestWriteBalance_Persist(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	bal := store.BalanceRecord{
		AccountNumber:       "ACCT-123",
		NetLiquidatingValue: "25000.00",
		BuyingPower:         "12000.00",
		UpdatedAt:           time.Now().UTC().Truncate(time.Second),
		Source:              store.SourceStreamer,
	}

	if err := st.WriteBalance(ctx, bal); err != nil {
		t.Fatalf("WriteBalance: %v", err)
	}

	got, err := st.LatestBalance(ctx, "ACCT-123")
	if err != nil {
		t.Fatalf("LatestBalance: %v", err)
	}
	if got.AccountNumber == "" {
		t.Fatal("LatestBalance returned zero-value — record not persisted")
	}
	if got.NetLiquidatingValue != bal.NetLiquidatingValue {
		t.Errorf("NLQ: got %q, want %q", got.NetLiquidatingValue, bal.NetLiquidatingValue)
	}
	if got.Source != store.SourceStreamer {
		t.Errorf("Source: got %q, want %q", got.Source, store.SourceStreamer)
	}
}

func TestWriteBalance_LatestWins(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	first := store.BalanceRecord{
		AccountNumber:       "ACCT-123",
		NetLiquidatingValue: "20000.00",
		BuyingPower:         "10000.00",
		UpdatedAt:           time.Now().Add(-5 * time.Minute).UTC(),
		Source:              store.SourceStreamer,
	}
	second := store.BalanceRecord{
		AccountNumber:       "ACCT-123",
		NetLiquidatingValue: "21000.00",
		BuyingPower:         "10500.00",
		UpdatedAt:           time.Now().UTC(),
		Source:              store.SourceStreamer,
	}

	if err := st.WriteBalance(ctx, first); err != nil {
		t.Fatalf("WriteBalance first: %v", err)
	}
	if err := st.WriteBalance(ctx, second); err != nil {
		t.Fatalf("WriteBalance second: %v", err)
	}

	got, err := st.LatestBalance(ctx, "ACCT-123")
	if err != nil {
		t.Fatalf("LatestBalance: %v", err)
	}
	if got.NetLiquidatingValue != second.NetLiquidatingValue {
		t.Errorf("LatestBalance should return most recent: got %q, want %q",
			got.NetLiquidatingValue, second.NetLiquidatingValue)
	}
}

func TestLatestBalance_NoRecord(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	got, err := st.LatestBalance(ctx, "NO-SUCH-ACCOUNT")
	if err != nil {
		t.Fatalf("LatestBalance: unexpected error: %v", err)
	}
	if got.AccountNumber != "" {
		t.Errorf("expected zero-value, got AccountNumber=%q", got.AccountNumber)
	}
}

// ── Position snapshot persistence ─────────────────────────────────────────────

func TestWritePositionSnapshot_Persist(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	exp := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Second)
	snap := store.PositionSnapshot{
		AccountNumber:     "ACCT-123",
		Symbol:            ".XSP250117C580",
		InstrumentType:    "Equity Option",
		Quantity:          "1",
		QuantityDirection: "Short",
		AvgOpenPrice:      "1.20",
		ClosePrice:        "0.80",
		ExpiresAt:         &exp,
		SnapshottedAt:     time.Now().UTC().Truncate(time.Second),
		Source:            store.SourceRESTSync,
	}

	if err := st.WritePositionSnapshot(ctx, snap); err != nil {
		t.Fatalf("WritePositionSnapshot: %v", err)
	}
	// No read method in Phase 2A — verify with a second write (should succeed cleanly).
	if err := st.WritePositionSnapshot(ctx, snap); err != nil {
		t.Fatalf("second WritePositionSnapshot: %v", err)
	}
}

// ── Concurrent write safety ───────────────────────────────────────────────────

func TestConcurrentWrites(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()

	const goroutines = 8
	var wg sync.WaitGroup
	errs := make(chan error, goroutines*2)

	for i := 0; i < goroutines; i++ {
		wg.Add(2)
		idx := i

		// Goroutine 1: write fills
		go func() {
			defer wg.Done()
			fill := store.FillRecord{
				OrderID:       fmt.Sprintf("CONCURRENT-FILL-%d", idx),
				AccountNumber: "ACCT-CONC",
				Symbol:        "XSP",
				Action:        "Sell to Open",
				Quantity:      "1",
				FillPrice:     "1.00",
				FilledAt:      time.Now().UTC(),
				Source:        store.SourceStreamer,
			}
			if err := st.WriteFill(ctx, fill); err != nil {
				errs <- err
			}
		}()

		// Goroutine 2: write position snapshots
		go func() {
			defer wg.Done()
			snap := store.PositionSnapshot{
				AccountNumber:     "ACCT-CONC",
				Symbol:            fmt.Sprintf("SYM-%d", idx),
				InstrumentType:    "Equity Option",
				Quantity:          "1",
				QuantityDirection: "Long",
				AvgOpenPrice:      "100.00",
				ClosePrice:        "100.00",
				SnapshottedAt:     time.Now().UTC(),
				Source:            store.SourceStreamer,
			}
			if err := st.WritePositionSnapshot(ctx, snap); err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent write error: %v", err)
	}
}

// ── Schema migration idempotency ──────────────────────────────────────────────

func TestMigration_Idempotent(t *testing.T) {
	// Open the same database twice — migrations must not error on second open.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)

	if err := os.MkdirAll(filepath.Join(dir, "tastytrade-cli"), 0700); err != nil {
		t.Fatal(err)
	}

	log, _ := zap.NewDevelopment()

	st1, err := store.Open(log)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	_ = st1.Close()

	st2, err := store.Open(log)
	if err != nil {
		t.Fatalf("second Open (idempotent migration): %v", err)
	}
	_ = st2.Close()
}
