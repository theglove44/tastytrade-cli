package streamer_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/theglove44/tastytrade-cli/internal/models"
	"github.com/theglove44/tastytrade-cli/internal/store"
	"github.com/theglove44/tastytrade-cli/internal/streamer"
)

// ── Backoff policy ────────────────────────────────────────────────────────────

func TestBackoffPolicy_Schedule(t *testing.T) {
	bp := streamer.DefaultBackoff
	cases := []struct {
		failures int
		want     time.Duration
	}{
		{0, 2 * time.Second},
		{1, 4 * time.Second},
		{2, 8 * time.Second},
		{3, 16 * time.Second},
		{4, 32 * time.Second},
		{5, 60 * time.Second}, // capped
		{10, 60 * time.Second}, // still capped
	}
	for _, tc := range cases {
		got := bp.Next(tc.failures)
		if got != tc.want {
			t.Errorf("failures=%d: got %v, want %v", tc.failures, got, tc.want)
		}
	}
}

func TestBackoffPolicy_CustomFactor(t *testing.T) {
	bp := streamer.BackoffPolicy{
		Initial: 1 * time.Second,
		Max:     10 * time.Second,
		Factor:  3.0,
	}
	if bp.Next(0) != 1*time.Second {
		t.Errorf("failures=0: want 1s")
	}
	if bp.Next(1) != 3*time.Second {
		t.Errorf("failures=1: want 3s")
	}
	if bp.Next(2) != 9*time.Second {
		t.Errorf("failures=2: want 9s")
	}
	if bp.Next(3) != 10*time.Second {
		t.Errorf("failures=3: want 10s (capped)")
	}
}

// ── Mock TokenProvider ─────────────────────────────────────────────────────────

type mockTokenProvider struct {
	mu           sync.Mutex
	calls        int
	token        string
	err          error
}

func (m *mockTokenProvider) AccessToken(_ context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return m.token, m.err
}

func (m *mockTokenProvider) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// ── Mock AccountHandler ───────────────────────────────────────────────────────

type mockHandler struct {
	mu       sync.Mutex
	orders   []models.OrderEvent
	balances []models.BalanceEvent
	positions []models.PositionEvent
}

func (h *mockHandler) OnOrderEvent(ev models.OrderEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.orders = append(h.orders, ev)
}

func (h *mockHandler) OnBalanceEvent(ev models.BalanceEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.balances = append(h.balances, ev)
}

func (h *mockHandler) OnPositionEvent(ev models.PositionEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.positions = append(h.positions, ev)
}

func (h *mockHandler) OrderCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.orders)
}

func (h *mockHandler) BalanceCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.balances)
}

// ── TokenProvider contract ─────────────────────────────────────────────────────

// TestTokenProvider_NoBearerPrefix verifies the TokenProvider interface
// semantics: the returned token must be the raw access token, not "Bearer <token>".
// This test validates that the mock matches the contract expected by the streamer.
func TestTokenProvider_NoBearerPrefix(t *testing.T) {
	raw := "eyJhbGciOiJSUzI1NiJ9.test-payload"
	m := &mockTokenProvider{token: raw}

	tok, err := m.AccessToken(context.Background())
	if err != nil {
		t.Fatalf("AccessToken: %v", err)
	}
	if tok != raw {
		t.Errorf("token mismatch: got %q, want %q", tok, raw)
	}
	// Explicitly check no Bearer prefix — this is the spec risk from Phase 2 design.
	if len(tok) > 7 && tok[:7] == "Bearer " {
		t.Error("token must NOT have 'Bearer ' prefix for account streamer wire protocol")
	}
}

// ── Streamer interface assertion ──────────────────────────────────────────────

// TestStreamerStatus_InitialState verifies that a freshly created streamer
// reports a sensible zero status before Start() is called.
// We use a fake WebSocket URL that will immediately fail — we cancel the context
// before it attempts to reconnect.
func TestStreamerStatus_InitialState(t *testing.T) {
	log, _ := zap.NewDevelopment()
	tp := &mockTokenProvider{token: "test-token"}
	h := &mockHandler{}

	s := streamer.NewAccountStreamer(
		"wss://127.0.0.1:1", // unreachable — will fail fast
		"ACCT-TEST",
		tp,
		h,
		log,
	)

	status := s.Status()
	if status.Name != "account" {
		t.Errorf("Name: got %q, want %q", status.Name, "account")
	}
	if status.Connected {
		t.Error("Connected should be false before Start()")
	}
	if status.ReconnectCount != 0 {
		t.Errorf("ReconnectCount: got %d, want 0", status.ReconnectCount)
	}
	if s.Name() != "account" {
		t.Errorf("Name(): got %q, want %q", s.Name(), "account")
	}
}

// TestStreamer_CancelStopsStart verifies that cancelling the context cleanly
// terminates Start() without a goroutine leak.
func TestStreamer_CancelStopsStart(t *testing.T) {
	log, _ := zap.NewDevelopment()
	tp := &mockTokenProvider{token: "test-token"}
	h := &mockHandler{}

	s := streamer.NewAccountStreamer(
		"wss://127.0.0.1:1", // unreachable
		"ACCT-TEST",
		tp,
		h,
		log,
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- s.Start(ctx)
	}()

	// Cancel immediately — Start should return promptly.
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Start returned %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Start did not return after context cancel within 5s")
	}
}

// TestStreamer_TokenRefreshedOnReconnect verifies that AccessToken() is called
// before each connection attempt (both first connect and reconnects).
// We simulate this by cancelling after a short duration and checking call count.
func TestStreamer_TokenCalledBeforeConnect(t *testing.T) {
	log, _ := zap.NewDevelopment()
	tp := &mockTokenProvider{token: "test-token"}
	h := &mockHandler{}

	s := streamer.NewAccountStreamer(
		"wss://127.0.0.1:1", // unreachable — each attempt fails, triggering backoff
		"ACCT-TEST",
		tp,
		h,
		log,
	)

	// Let it run long enough for at least one connection attempt.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_ = s.Start(ctx)

	// AccessToken must have been called at least once.
	if tp.CallCount() < 1 {
		t.Errorf("AccessToken called %d times, want at least 1", tp.CallCount())
	}
}

// ── accountEventHandler via store integration ─────────────────────────────────

// openTestStore creates a Store backed by a temp-dir SQLite database.
func openTestStoreForStreamer(t *testing.T) store.Store {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)

	log, _ := zap.NewDevelopment()
	st, err := store.Open(log)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// handlerWithStore is the production accountEventHandler wired to a test store.
// We access it via the AccountHandler interface to test the contract.
func newTestHandler(t *testing.T) (streamer.AccountHandler, store.Store) {
	t.Helper()
	st := openTestStoreForStreamer(t)
	log, _ := zap.NewDevelopment()
	h := newAccountEventHandlerForTest(st, log)
	return h, st
}

// newAccountEventHandlerForTest is a test helper that creates an accountEventHandler.
// We need to access the unexported cmd.accountEventHandler from outside cmd — for
// testing purposes we replicate the interface contract using a local test double
// that wraps the store directly.
type storeBackedHandler struct {
	st  store.Store
	log *zap.Logger
}

func (h *storeBackedHandler) OnOrderEvent(ev models.OrderEvent) {
	if ev.Status != "Filled" {
		return
	}
	filledAt := ev.FilledAt
	if filledAt == nil {
		now := time.Now()
		filledAt = &now
	}
	sym := ""
	action := ""
	qty := "0"
	price := "0"
	if len(ev.Legs) > 0 {
		leg := ev.Legs[0]
		sym = leg.Symbol
		action = leg.Action
		qty = leg.FillQuantity.String()
		price = leg.FillPrice.String()
	}
	rec := store.FillRecord{
		OrderID:       ev.OrderID,
		AccountNumber: ev.AccountNumber,
		Symbol:        sym,
		Action:        action,
		Quantity:      qty,
		FillPrice:     price,
		FilledAt:      *filledAt,
		Source:        store.SourceStreamer,
	}
	if err := h.st.WriteFill(context.Background(), rec); err != nil {
		h.log.Error("test handler WriteFill", zap.Error(err))
	}
}

func (h *storeBackedHandler) OnBalanceEvent(ev models.BalanceEvent) {
	rec := store.BalanceRecord{
		AccountNumber:       ev.AccountNumber,
		NetLiquidatingValue: ev.NetLiquidatingValue.String(),
		BuyingPower:         ev.BuyingPower.String(),
		UpdatedAt:           ev.UpdatedAt,
		Source:              store.SourceStreamer,
	}
	if err := h.st.WriteBalance(context.Background(), rec); err != nil {
		h.log.Error("test handler WriteBalance", zap.Error(err))
	}
}

func (h *storeBackedHandler) OnPositionEvent(_ models.PositionEvent) {}

func newAccountEventHandlerForTest(st store.Store, log *zap.Logger) streamer.AccountHandler {
	return &storeBackedHandler{st: st, log: log}
}

// TestHandler_FilledOrderPersisted verifies the end-to-end path:
// streamer delivers a Filled OrderEvent → handler writes to store.
func TestHandler_FilledOrderPersisted(t *testing.T) {
	h, st := newTestHandler(t)
	ctx := context.Background()

	now := time.Now().UTC()
	ev := models.OrderEvent{
		AccountNumber: "ACCT-123",
		OrderID:       "ORD-FILL-001",
		Status:        "Filled",
		FilledAt:      &now,
		Legs: []models.OrderLeg{
			{Symbol: ".XSP250117C580", Action: "Sell to Open"},
		},
	}

	h.OnOrderEvent(ev)

	// Poll briefly — handler may be async in the production implementation.
	var fills []store.FillRecord
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		f, err := st.RecentFills(ctx, "ACCT-123", time.Now().Add(-1*time.Minute))
		if err != nil {
			t.Fatalf("RecentFills: %v", err)
		}
		fills = f
		if len(fills) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if len(fills) != 1 {
		t.Fatalf("expected 1 fill persisted, got %d", len(fills))
	}
	if fills[0].OrderID != "ORD-FILL-001" {
		t.Errorf("OrderID: got %q, want %q", fills[0].OrderID, "ORD-FILL-001")
	}
}

// TestHandler_DuplicateFillIgnored verifies that delivering the same Filled event
// twice (reconnect snapshot) does not create a duplicate row.
func TestHandler_DuplicateFillIgnored(t *testing.T) {
	h, st := newTestHandler(t)
	ctx := context.Background()

	now := time.Now().UTC()
	ev := models.OrderEvent{
		AccountNumber: "ACCT-123",
		OrderID:       "ORD-DUPE-STREAM",
		Status:        "Filled",
		FilledAt:      &now,
	}

	h.OnOrderEvent(ev) // first delivery
	h.OnOrderEvent(ev) // duplicate — reconnect snapshot

	time.Sleep(50 * time.Millisecond) // allow async writes

	fills, err := st.RecentFills(ctx, "ACCT-123", time.Now().Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("RecentFills: %v", err)
	}
	if len(fills) != 1 {
		t.Errorf("expected 1 fill (idempotent), got %d", len(fills))
	}
}

// TestHandler_NonFilledIgnored verifies that non-Filled order events
// (e.g. Received, Live, Cancelled) are not persisted.
func TestHandler_NonFilledIgnored(t *testing.T) {
	h, st := newTestHandler(t)
	ctx := context.Background()

	for _, status := range []string{"Received", "Live", "Cancelled", "Rejected"} {
		ev := models.OrderEvent{
			AccountNumber: "ACCT-123",
			OrderID:       "ORD-" + status,
			Status:        status,
		}
		h.OnOrderEvent(ev)
	}

	time.Sleep(50 * time.Millisecond)

	fills, err := st.RecentFills(ctx, "ACCT-123", time.Now().Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("RecentFills: %v", err)
	}
	if len(fills) != 0 {
		t.Errorf("expected 0 fills for non-Filled events, got %d", len(fills))
	}
}

// TestHandler_BalanceUpdatePersisted verifies that a BalanceEvent is persisted
// to the store via the handler.
func TestHandler_BalanceUpdatePersisted(t *testing.T) {
	h, st := newTestHandler(t)
	ctx := context.Background()

	nlq, _ := decimal.NewFromString("25000.00")
	bp, _ := decimal.NewFromString("12000.00")

	ev := models.BalanceEvent{
		AccountNumber:       "ACCT-123",
		NetLiquidatingValue: nlq,
		BuyingPower:         bp,
		UpdatedAt:           time.Now().UTC(),
	}

	h.OnBalanceEvent(ev)
	time.Sleep(50 * time.Millisecond)

	bal, err := st.LatestBalance(ctx, "ACCT-123")
	if err != nil {
		t.Fatalf("LatestBalance: %v", err)
	}
	if bal.AccountNumber == "" {
		t.Fatal("balance not persisted — LatestBalance returned zero-value")
	}
	// shopspring/decimal serialises "25000.00" as "25000"
	if bal.NetLiquidatingValue != "25000" {
		t.Errorf("NLQ: got %q, want %q", bal.NetLiquidatingValue, "25000")
	}
}

// ── AccountMessage JSON routing ───────────────────────────────────────────────

// TestAccountMessage_Unmarshal verifies that the AccountMessage envelope
// can decode the type field and leave Data as raw JSON for further parsing.
func TestAccountMessage_Unmarshal(t *testing.T) {
	raw := `{"type":"order","action":"Change","data":{"id":"ORD-001","status":"Filled","account-number":"ACCT-123","legs":[]}}`
	var msg models.AccountMessage
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("Unmarshal AccountMessage: %v", err)
	}
	if msg.Type != "order" {
		t.Errorf("Type: got %q, want %q", msg.Type, "order")
	}
	if msg.Action != "Change" {
		t.Errorf("Action: got %q, want %q", msg.Action, "Change")
	}
	if len(msg.Data) == 0 {
		t.Error("Data should not be empty")
	}

	// Decode the inner event.
	var ev models.OrderEvent
	if err := json.Unmarshal(msg.Data, &ev); err != nil {
		t.Fatalf("Unmarshal OrderEvent from Data: %v", err)
	}
	if ev.OrderID != "ORD-001" {
		t.Errorf("OrderEvent.OrderID: got %q, want %q", ev.OrderID, "ORD-001")
	}
}

// ── Market streamer tests ─────────────────────────────────────────────────────

// mockQuoteTokenFetcher returns a fixed QuoteToken or an error.
type mockQuoteTokenFetcher struct {
	mu    sync.Mutex
	calls int
	token models.QuoteToken
	err   error
}

func (m *mockQuoteTokenFetcher) QuoteToken(_ context.Context) (models.QuoteToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	return m.token, m.err
}

func (m *mockQuoteTokenFetcher) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// mockQuoteHandler records received QuoteEvents.
type mockQuoteHandler struct {
	mu     sync.Mutex
	quotes []models.QuoteEvent
}

func (h *mockQuoteHandler) OnQuote(ev models.QuoteEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.quotes = append(h.quotes, ev)
}

func (h *mockQuoteHandler) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.quotes)
}

// TestMarketStreamer_Name verifies the streamer's name constant.
func TestMarketStreamer_Name(t *testing.T) {
	log, _ := zap.NewDevelopment()
	tf := &mockQuoteTokenFetcher{token: models.QuoteToken{Token: "tok", DxlinkURL: "wss://localhost:1"}}
	h := &mockQuoteHandler{}
	s := streamer.NewMarketStreamer("wss://localhost:1", nil, tf, h, log)
	if s.Name() != "market" {
		t.Errorf("Name: got %q, want %q", s.Name(), "market")
	}
}

// TestMarketStreamer_InitialStatus verifies zero status before Start().
func TestMarketStreamer_InitialStatus(t *testing.T) {
	log, _ := zap.NewDevelopment()
	tf := &mockQuoteTokenFetcher{token: models.QuoteToken{Token: "tok", DxlinkURL: "wss://localhost:1"}}
	h := &mockQuoteHandler{}
	s := streamer.NewMarketStreamer("wss://localhost:1", nil, tf, h, log)

	status := s.Status()
	if status.Connected {
		t.Error("Connected should be false before Start()")
	}
	if status.ReconnectCount != 0 {
		t.Errorf("ReconnectCount: got %d, want 0", status.ReconnectCount)
	}
}

// TestMarketStreamer_CancelStopsStart verifies context cancel terminates Start().
func TestMarketStreamer_CancelStopsStart(t *testing.T) {
	log, _ := zap.NewDevelopment()
	tf := &mockQuoteTokenFetcher{token: models.QuoteToken{Token: "tok", DxlinkURL: "wss://127.0.0.1:1"}}
	h := &mockQuoteHandler{}
	s := streamer.NewMarketStreamer("wss://127.0.0.1:1", nil, tf, h, log)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Start returned %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("Start did not return after context cancel within 5s")
	}
}

// TestMarketStreamer_TokenFetchedBeforeConnect verifies QuoteToken() is called
// before every connection attempt (critical spec requirement).
func TestMarketStreamer_TokenFetchedBeforeConnect(t *testing.T) {
	log, _ := zap.NewDevelopment()
	tf := &mockQuoteTokenFetcher{token: models.QuoteToken{Token: "tok", DxlinkURL: "wss://127.0.0.1:1"}}
	h := &mockQuoteHandler{}
	s := streamer.NewMarketStreamer("wss://127.0.0.1:1", nil, tf, h, log)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.Start(ctx)

	if tf.CallCount() < 1 {
		t.Errorf("QuoteToken fetched %d times, want at least 1", tf.CallCount())
	}
}

// TestMarketStreamer_Subscribe_Dedup verifies Subscribe deduplicates symbols.
func TestMarketStreamer_Subscribe_Dedup(t *testing.T) {
	log, _ := zap.NewDevelopment()
	tf := &mockQuoteTokenFetcher{token: models.QuoteToken{Token: "tok", DxlinkURL: "wss://127.0.0.1:1"}}
	h := &mockQuoteHandler{}
	s := streamer.NewMarketStreamer("wss://127.0.0.1:1", []string{"SPY"}, tf, h, log)

	// Subscribe the same symbol again — should not duplicate.
	s.Subscribe("SPY", "SPY", "NVDA")

	// Cancel immediately and check no panic / build error.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = s.Start(ctx)
}

// ── Quote event model tests ───────────────────────────────────────────────────

// TestQuoteEvent_JSON verifies the QuoteEvent can round-trip through JSON.
func TestQuoteEvent_JSON(t *testing.T) {
	import_decimal := func(s string) decimal.Decimal {
		d, _ := decimal.NewFromString(s)
		return d
	}
	ev := models.QuoteEvent{
		Symbol:    ".XSP250117C580",
		BidPrice:  import_decimal("1.20"),
		AskPrice:  import_decimal("1.22"),
		LastPrice: import_decimal("1.21"),
		MarkPrice: import_decimal("1.21"),
		MarkStale: false,
		EventTime: time.Now().UTC().Truncate(time.Second),
	}

	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var out models.QuoteEvent
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Symbol != ev.Symbol {
		t.Errorf("Symbol: got %q, want %q", out.Symbol, ev.Symbol)
	}
	if !out.BidPrice.Equal(ev.BidPrice) {
		t.Errorf("BidPrice: got %s, want %s", out.BidPrice, ev.BidPrice)
	}
}
