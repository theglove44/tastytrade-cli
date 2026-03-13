// Package streamer — market.go implements the TastyTrade DXLink market data streamer.
//
// Wire protocol (spec §1.6, confirmed):
//
//	Endpoint:   from QuoteToken.DxlinkURL (preferred) or config fallback
//	Auth:       QuoteToken.Token — fetched fresh before every connect/reconnect
//
//	Handshake (per connect):
//	  1. GET /api-quote-tokens → fresh QuoteToken (REST call, not WS)
//	  2. Dial WebSocket to QuoteToken.DxlinkURL (or fallback URL)
//	  3. Send SETUP
//	  4. Receive SETUP response
//	  5. Send AUTH with token
//	  6. Receive AUTH_STATE{"state":"AUTHORIZED"}
//	  7. Send CHANNEL_REQUEST for FEED service
//	  8. Receive CHANNEL_OPENED
//	  9. Send FEED_SETUP declaring compact Quote fields
//	 10. Send FEED_SUBSCRIPTION for all tracked symbols
//	 11. Loop: receive FEED_DATA, send KEEPALIVE every 30s
//
//	Critical difference from account streamer:
//	  The quote token MUST be re-fetched (REST call) before every connect.
//	  Reusing a cached token will cause AUTH failure on reconnect.
//
// Dispatch architecture:
//
//	Identical to account streamer: a buffered channel (capacity 64) decouples
//	the receive loop from QuoteHandler calls. Dropped events are logged.
package streamer

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/theglove44/tastytrade-cli/internal/client"
	"github.com/theglove44/tastytrade-cli/internal/models"
)

const (
	marketStreamerName = "market"
	dxlinkKeepalive    = 30 * time.Second
	dxlinkDataChannel  = 1 // channel ID assigned by CHANNEL_REQUEST
	dxlinkDispatchSize = 64

	// staleQuoteTimeout is the maximum time the market streamer will wait for a
	// quote before treating the connection as stale and forcing a reconnect.
	// Only active when at least one symbol is subscribed.
	staleQuoteTimeout = 90 * time.Second
)

// QuoteTokenFetcher is the interface the market streamer requires to retrieve
// a DXLink authentication token. Narrow interface — only one method required.
type QuoteTokenFetcher interface {
	QuoteToken(ctx context.Context) (models.QuoteToken, error)
}

// marketStreamer implements Streamer for the DXLink market data WebSocket.
type marketStreamer struct {
	fallbackURL string // used when QuoteToken.DxlinkURL is empty
	tokenFetch  QuoteTokenFetcher
	handler     QuoteHandler
	backoff     BackoffPolicy
	log         *zap.Logger

	// symbolsMu guards symbols — Subscribe() may be called concurrently.
	symbolsMu sync.RWMutex
	symbols   []string

	// status — guarded by statusMu
	statusMu       sync.RWMutex
	connected      bool
	connectedSince time.Time
	lastEventAt    time.Time
	lastError      string

	reconnectCount int64
	dropCount      int64
}

// NewMarketStreamer creates a DXLink market data streamer.
//
//   - fallbackURL: WS endpoint used when QuoteToken.DxlinkURL is empty
//   - symbols:     initial symbol subscription list (may be empty)
//   - tokenFetch:  QuoteTokenFetcher — typically the Exchange implementation
//   - handler:     QuoteHandler receiving decoded events
//   - log:         structured logger
func NewMarketStreamer(
	fallbackURL string,
	symbols []string,
	tokenFetch QuoteTokenFetcher,
	handler QuoteHandler,
	log *zap.Logger,
) MarketStreamer {
	syms := make([]string, len(symbols))
	copy(syms, symbols)
	return &marketStreamer{
		fallbackURL: fallbackURL,
		symbols:     syms,
		tokenFetch:  tokenFetch,
		handler:     handler,
		backoff:     DefaultBackoff,
		log:         log,
	}
}

// Name implements Streamer.
func (m *marketStreamer) Name() string { return marketStreamerName }

// Status implements Streamer. Safe to call from any goroutine.
func (m *marketStreamer) Status() StreamerStatus {
	m.statusMu.RLock()
	s := StreamerStatus{
		Name:           marketStreamerName,
		Connected:      m.connected,
		ConnectedSince: m.connectedSince,
		LastEventAt:    m.lastEventAt,
		ReconnectCount: int(atomic.LoadInt64(&m.reconnectCount)),
		LastError:      m.lastError,
	}
	m.statusMu.RUnlock()
	return s
}

func (m *marketStreamer) setConnected(since time.Time) {
	m.statusMu.Lock()
	m.connected = true
	m.connectedSince = since
	m.lastEventAt = time.Time{} // stale detection is per-connection; reset quote clock on reconnect
	m.lastError = ""
	m.statusMu.Unlock()
	client.Metrics.StreamerUptime.WithLabelValues(marketStreamerName).Set(0)
}

func (m *marketStreamer) setDisconnected(err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	m.statusMu.Lock()
	m.connected = false
	m.lastError = msg
	m.statusMu.Unlock()
}

func (m *marketStreamer) touchLastEvent() {
	now := time.Now()
	m.statusMu.Lock()
	m.lastEventAt = now
	m.statusMu.Unlock()
	client.Metrics.LastQuoteTime.Set(float64(now.Unix()))
}

// isStale reports whether the connection should be considered stale.
// Returns true only when:
//   - there is at least one subscribed symbol, AND
//   - at least one quote has been received on this connection (lastEventAt != 0), AND
//   - no quote has been received within staleQuoteTimeout.
//
// This intentionally does NOT treat a fresh connection with zero quotes as
// stale. Closed-market conditions and illiquid symbols can legitimately produce
// no initial quotes for long periods; reconnecting in that state only causes
// churn.
func (m *marketStreamer) isStale(now time.Time, timeout time.Duration) bool {
	if len(m.currentSymbols()) == 0 {
		return false
	}
	m.statusMu.RLock()
	last := m.lastEventAt
	m.statusMu.RUnlock()
	if last.IsZero() {
		return false
	}
	return now.Sub(last) >= timeout
}

// staleWatchdog polls isStale every checkInterval and cancels connCancel when
// the connection is detected as stale, causing runOnce to return and the
// reconnect loop in Start to execute.
func (m *marketStreamer) staleWatchdog(
	ctx context.Context,
	connCancel context.CancelFunc,
	timeout time.Duration,
	checkInterval time.Duration,
	done chan<- struct{},
) {
	defer close(done)
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if m.isStale(time.Now(), timeout) {
				m.log.Warn("market streamer: no quote received — forcing reconnect",
					zap.Duration("stale_after", timeout),
					zap.Int("subscribed_symbols", len(m.currentSymbols())),
				)
				connCancel()
				return
			}
		}
	}
}

// currentSymbols returns a snapshot of the current symbol list.
func (m *marketStreamer) currentSymbols() []string {
	m.symbolsMu.RLock()
	defer m.symbolsMu.RUnlock()
	out := make([]string, len(m.symbols))
	copy(out, m.symbols)
	return out
}

// Subscribe adds symbols to the streamer's subscription set.
// Safe to call before or after Start() -- symbols added before first connect
// are sent in the initial FEED_SUBSCRIPTION; symbols added after an active
// connection are included on the next reconnect subscription.
// Duplicate symbols are deduplicated silently.
func (m *marketStreamer) Subscribe(symbols ...string) {
	m.symbolsMu.Lock()
	defer m.symbolsMu.Unlock()
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
	client.Metrics.TrackedSymbols.Set(float64(len(m.symbols)))
}

// Start implements Streamer. Blocks until ctx is cancelled.
func (m *marketStreamer) Start(ctx context.Context) error {
	failures := 0

	for {
		select {
		case <-ctx.Done():
			m.log.Info("market streamer stopping", zap.String("reason", ctx.Err().Error()))
			return ctx.Err()
		default:
		}

		err := m.runOnce(ctx)

		if ctx.Err() != nil {
			return ctx.Err()
		}

		m.setDisconnected(err)
		m.log.Error("market streamer disconnected",
			zap.Error(err),
			zap.Int("failures", failures),
		)

		if failures > 0 {
			atomic.AddInt64(&m.reconnectCount, 1)
			client.Metrics.StreamerReconnects.WithLabelValues(marketStreamerName).Inc()
		}

		wait := m.backoff.Next(failures)
		failures++

		m.log.Info("market streamer will reconnect",
			zap.Duration("wait", wait),
			zap.Int64("total_reconnects", atomic.LoadInt64(&m.reconnectCount)),
		)

		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// runOnce executes a single connect→handshake→receive cycle.
// CRITICAL: always fetches a fresh QuoteToken first — never reuses a cached token.
func (m *marketStreamer) runOnce(ctx context.Context) error {
	// Step 1: fetch fresh quote token — mandatory before every connect/reconnect.
	token, err := m.tokenFetch.QuoteToken(ctx)
	if err != nil {
		return fmt.Errorf("runOnce: fetch quote token: %w", err)
	}

	// Choose WS URL: prefer DxlinkURL from token; fall back to config value.
	wsURL := token.DxlinkURL
	if wsURL == "" {
		wsURL = m.fallbackURL
	}

	m.log.Info("market streamer connecting",
		zap.String("url", wsURL),
		zap.Strings("symbols", m.currentSymbols()),
	)

	// Step 2: dial WebSocket.
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return fmt.Errorf("runOnce: dial: %w", err)
	}
	defer conn.CloseNow() //nolint:errcheck

	// Step 3: send SETUP.
	if err := wsjson.Write(ctx, conn, map[string]any{
		"type":                   "SETUP",
		"channel":                0,
		"version":                "0.1",
		"keepaliveTimeout":       60,
		"acceptKeepaliveTimeout": 60,
	}); err != nil {
		return fmt.Errorf("runOnce: send SETUP: %w", err)
	}

	// Step 4: consume SETUP response.
	if err := m.expectType(ctx, conn, "SETUP"); err != nil {
		return fmt.Errorf("runOnce: await SETUP response: %w", err)
	}

	// Step 5: DXLink sends AUTH_STATE=UNAUTHORIZED after SETUP. Consume that
	// challenge before sending AUTH, per the documented protocol sequence.
	if err := m.expectAuthStateValue(ctx, conn, "UNAUTHORIZED"); err != nil {
		return fmt.Errorf("runOnce: await initial AUTH_STATE: %w", err)
	}

	// Step 6: send AUTH.
	if err := wsjson.Write(ctx, conn, map[string]any{
		"type":    "AUTH",
		"channel": 0,
		"token":   token.Token,
	}); err != nil {
		return fmt.Errorf("runOnce: send AUTH: %w", err)
	}

	// Step 7: await AUTH_STATE AUTHORIZED.
	if err := m.expectAuthStateValue(ctx, conn, "AUTHORIZED"); err != nil {
		return fmt.Errorf("runOnce: await AUTH_STATE: %w", err)
	}

	// Step 7: CHANNEL_REQUEST for FEED service.
	if err := wsjson.Write(ctx, conn, map[string]any{
		"type":    "CHANNEL_REQUEST",
		"channel": dxlinkDataChannel,
		"service": "FEED",
		"parameters": map[string]any{
			"contract": "AUTO",
		},
	}); err != nil {
		return fmt.Errorf("runOnce: send CHANNEL_REQUEST: %w", err)
	}

	// Step 8: await CHANNEL_OPENED.
	if err := m.expectType(ctx, conn, "CHANNEL_OPENED"); err != nil {
		return fmt.Errorf("runOnce: await CHANNEL_OPENED: %w", err)
	}

	// Step 9: FEED_SETUP declaring compact Quote fields.
	if err := wsjson.Write(ctx, conn, map[string]any{
		"type":                    "FEED_SETUP",
		"channel":                 dxlinkDataChannel,
		"acceptAggregationPeriod": 0.1,
		"acceptDataFormat":        "COMPACT",
		"acceptEventFields": map[string]any{
			"Quote": []string{
				"eventType", "eventSymbol",
				"bidPrice", "askPrice", "lastPrice",
				"time",
			},
		},
	}); err != nil {
		return fmt.Errorf("runOnce: send FEED_SETUP: %w", err)
	}

	// Step 10: subscribe all tracked symbols (skip if empty).
	syms := m.currentSymbols()
	if len(syms) > 0 {
		if err := m.sendSubscription(ctx, conn, syms); err != nil {
			return fmt.Errorf("runOnce: FEED_SUBSCRIPTION: %w", err)
		}
	}

	connectedAt := time.Now()
	m.setConnected(connectedAt)
	client.Metrics.TrackedSymbols.Set(float64(len(syms)))
	m.log.Info("market streamer subscribed",
		zap.Int("symbols", len(syms)),
		zap.Time("connected_at", connectedAt),
	)

	// Step 11: create a per-connection context so the stale watchdog can
	// cancel this connection without cancelling the outer Start() context.
	connCtx, connCancel := context.WithCancel(ctx)
	defer connCancel()

	// Step 12: start buffered dispatch goroutine.
	dispatchCh := make(chan models.QuoteEvent, dxlinkDispatchSize)
	dispatchDone := make(chan struct{})
	go m.dispatchLoop(connCtx, dispatchCh, dispatchDone)

	// Step 13: start keepalive goroutine.
	kaCtx, cancelKA := context.WithCancel(connCtx)
	kaDone := make(chan struct{})
	go m.keepaliveLoop(kaCtx, conn, kaDone)

	// Step 14: start stale-connection watchdog.
	// Polls every 15 seconds; fires if no quote received for staleQuoteTimeout.
	// Only active when symbols are subscribed (no-op for idle streams).
	watchdogDone := make(chan struct{})
	go m.staleWatchdog(connCtx, connCancel, staleQuoteTimeout, 15*time.Second, watchdogDone)

	// Step 15: receive loop — blocks until connection closes or connCtx cancelled.
	receiveErr := m.receiveLoop(connCtx, conn, dispatchCh)

	connCancel() // ensure keepalive and watchdog stop
	<-watchdogDone
	cancelKA()
	<-kaDone

	close(dispatchCh)
	<-dispatchDone

	return receiveErr
}

// sendSubscription writes a FEED_SUBSCRIPTION message for the given symbols.
func (m *marketStreamer) sendSubscription(ctx context.Context, conn *websocket.Conn, syms []string) error {
	type subEntry struct {
		Type   string `json:"type"`
		Symbol string `json:"symbol"`
	}
	add := make([]subEntry, len(syms))
	for i, s := range syms {
		add[i] = subEntry{Type: "Quote", Symbol: s}
	}
	return wsjson.Write(ctx, conn, map[string]any{
		"type":    "FEED_SUBSCRIPTION",
		"channel": dxlinkDataChannel,
		"add":     add,
	})
}

// expectType reads messages until one with the given type arrives.
// Other message types are discarded with a debug log.
func (m *marketStreamer) expectType(ctx context.Context, conn *websocket.Conn, wantType string) error {
	for {
		var raw json.RawMessage
		if err := wsjson.Read(ctx, conn, &raw); err != nil {
			return err
		}
		var peek models.DXLinkMsg
		if err := json.Unmarshal(raw, &peek); err != nil {
			continue
		}
		if peek.Type == wantType {
			return nil
		}
		m.log.Debug("market streamer: skipping unexpected message",
			zap.String("got", peek.Type),
			zap.String("want", wantType),
		)
	}
}

// expectAuthStateValue reads messages until AUTH_STATE arrives and then
// requires its state to match want.
func (m *marketStreamer) expectAuthStateValue(ctx context.Context, conn *websocket.Conn, want string) error {
	for {
		var raw json.RawMessage
		if err := wsjson.Read(ctx, conn, &raw); err != nil {
			return err
		}
		var as models.DXLinkAuthState
		if err := json.Unmarshal(raw, &as); err != nil {
			continue
		}
		if as.Type != "AUTH_STATE" {
			continue
		}
		if as.State == want {
			return nil
		}
		return fmt.Errorf("AUTH_STATE: server returned %q", as.State)
	}
}

// receiveLoop reads FEED_DATA frames and routes decoded quotes to dispatchCh.
func (m *marketStreamer) receiveLoop(
	ctx context.Context,
	conn *websocket.Conn,
	dispatchCh chan<- models.QuoteEvent,
) error {
	for {
		var raw json.RawMessage
		if err := wsjson.Read(ctx, conn, &raw); err != nil {
			return fmt.Errorf("receiveLoop: %w", err)
		}

		var peek models.DXLinkMsg
		if err := json.Unmarshal(raw, &peek); err != nil {
			continue
		}
		if peek.Type != "FEED_DATA" || peek.Channel != dxlinkDataChannel {
			continue
		}

		quotes, err := decodeFeedData(raw)
		if err != nil {
			m.log.Warn("market streamer: decode FEED_DATA", zap.Error(err))
			continue
		}

		for _, q := range quotes {
			select {
			case dispatchCh <- q:
			default:
				dropped := atomic.AddInt64(&m.dropCount, 1)
				m.log.Warn("market streamer: dispatch full, dropping quote",
					zap.String("symbol", q.Symbol),
					zap.Int64("total_dropped", dropped),
				)
			}
		}
	}
}

// dispatchLoop drains dispatchCh and calls QuoteHandler.OnQuote.
func (m *marketStreamer) dispatchLoop(
	_ context.Context,
	ch <-chan models.QuoteEvent,
	done chan<- struct{},
) {
	defer close(done)
	for q := range ch {
		m.touchLastEvent()
		client.Metrics.QuotesReceived.WithLabelValues(q.Symbol).Inc()
		m.handler.OnQuote(q)
	}
}

// keepaliveLoop sends KEEPALIVE on channel 0 every 30 seconds.
func (m *marketStreamer) keepaliveLoop(
	ctx context.Context,
	conn *websocket.Conn,
	done chan<- struct{},
) {
	defer close(done)
	ticker := time.NewTicker(dxlinkKeepalive)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := wsjson.Write(ctx, conn, map[string]any{
				"type":    "KEEPALIVE",
				"channel": 0,
			}); err != nil {
				m.log.Warn("market streamer: keepalive failed", zap.Error(err))
				return
			}
			m.statusMu.RLock()
			since := m.connectedSince
			m.statusMu.RUnlock()
			if !since.IsZero() {
				client.Metrics.StreamerUptime.WithLabelValues(marketStreamerName).
					Set(time.Since(since).Seconds())
			}
		}
	}
}

// ── FEED_DATA decoding ────────────────────────────────────────────────────────

// decodeFeedData parses a FEED_DATA frame into QuoteEvents.
// DXLink compact format: data is an array of compact event arrays.
// Each inner array: ["Quote", symbol, bid, ask, last, timeMs]
func decodeFeedData(raw json.RawMessage) ([]models.QuoteEvent, error) {
	var fd models.DXLinkFeedData
	if err := json.Unmarshal(raw, &fd); err != nil {
		return nil, err
	}

	var events []models.QuoteEvent
	for _, item := range fd.Data {
		// item may be a single row or an array of rows depending on batching.
		// Try as array-of-rows first; if the first element is a string it's
		// a single row.
		var rows []json.RawMessage
		if err := json.Unmarshal(item, &rows); err != nil || len(rows) == 0 {
			continue
		}

		// Peek at rows[0]: if it's a string, this item IS a single row.
		var firstStr string
		if err := json.Unmarshal(rows[0], &firstStr); err == nil {
			// Single row — parse directly.
			if q, ok := parseCompactQuote(rows); ok {
				events = append(events, q)
			}
		} else {
			// Array of rows — each rows[i] is itself a compact event array.
			for _, rowRaw := range rows {
				var row []json.RawMessage
				if err := json.Unmarshal(rowRaw, &row); err != nil {
					continue
				}
				if q, ok := parseCompactQuote(row); ok {
					events = append(events, q)
				}
			}
		}
	}
	return events, nil
}

// parseCompactQuote decodes a single compact Quote row.
// Fields: ["Quote", symbol, bidPrice, askPrice, lastPrice, timeMs]
func parseCompactQuote(fields []json.RawMessage) (models.QuoteEvent, bool) {
	if len(fields) < 6 {
		return models.QuoteEvent{}, false
	}

	var eventType string
	if err := json.Unmarshal(fields[0], &eventType); err != nil || eventType != "Quote" {
		return models.QuoteEvent{}, false
	}

	var symbol string
	if err := json.Unmarshal(fields[1], &symbol); err != nil || symbol == "" {
		return models.QuoteEvent{}, false
	}

	bid := decimalFromJSON(fields[2])
	ask := decimalFromJSON(fields[3])
	last := decimalFromJSON(fields[4])

	var timeMs int64
	_ = json.Unmarshal(fields[5], &timeMs)
	eventTime := time.UnixMilli(timeMs).UTC()

	mark, stale := deriveMarkPrice(bid, ask, last)

	return models.QuoteEvent{
		Symbol:    symbol,
		BidPrice:  bid,
		AskPrice:  ask,
		LastPrice: last,
		MarkPrice: mark,
		MarkStale: stale,
		EventTime: eventTime,
	}, true
}

// deriveMarkPrice computes the mark price from bid/ask/last.
// Rule:
//   - If both bid and ask are non-zero: mark = (bid + ask) / 2
//   - Else if last is non-zero: mark = last
//   - Else: mark = 0, stale = true
func deriveMarkPrice(bid, ask, last decimal.Decimal) (mark decimal.Decimal, stale bool) {
	two := decimal.NewFromInt(2)
	if bid.IsPositive() && ask.IsPositive() {
		return bid.Add(ask).Div(two), false
	}
	if last.IsPositive() {
		return last, false
	}
	return decimal.Zero, true
}

// decimalFromJSON parses a JSON number or numeric string into decimal.Decimal.
// Returns zero on any parse error.
func decimalFromJSON(raw json.RawMessage) decimal.Decimal {
	// Try as a JSON number (float) first.
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return decimal.NewFromFloat(f)
	}
	// Try as a quoted string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		d, err := decimal.NewFromString(s)
		if err == nil {
			return d
		}
	}
	return decimal.Zero
}

// Compile-time assertion: *marketStreamer implements Streamer.
var _ Streamer = (*marketStreamer)(nil)

// Compile-time assertion: *marketStreamer implements MarketStreamer.
var _ MarketStreamer = (*marketStreamer)(nil)
