// Package streamer — account.go implements the TastyTrade account streamer.
//
// Wire protocol (spec §1.5, confirmed):
//
//	Endpoint:   wss://streamer.tastytrade.com
//	auth-token: raw access_token — NO "Bearer" prefix
//
//	Handshake sequence:
//	  1. Dial WebSocket
//	  2. Send connect:          {"action":"connect","value":["<ACCT>"],"request-id":1,"auth-token":"<RAW>"}
//	  3. Send account-subscribe:{"action":"account-subscribe","value":["<ACCT>"],"request-id":2,"auth-token":"<RAW>"}
//	  4. Loop: receive events, send heartbeat every 30s
//
//	On reconnect:
//	  1. Wait backoff
//	  2. EnsureToken() — always call before using the token
//	  3. Dial → connect → account-subscribe with fresh token
//
// Dispatch architecture:
//
//	The WebSocket receive loop writes decoded events to a buffered channel
//	(dispatchCh, capacity 64). A separate goroutine drains the channel and
//	calls AccountHandler methods. This ensures the receive loop never blocks
//	on slow handler implementations.
//
//	If the dispatch channel fills, events are dropped and a warning is logged.
//	A fill drop count is tracked in StreamerStatus.
package streamer

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/theglove44/tastytrade-cli/internal/client"
	"github.com/theglove44/tastytrade-cli/internal/models"
)

const (
	accountStreamerName = "account"
	heartbeatInterval  = 30 * time.Second
	dispatchBufSize    = 64
)

// TokenProvider is the interface the account streamer requires from the
// client layer. Keeping it narrow means a future paper/sim backend only
// needs to implement two methods, not the entire *client.Client.
type TokenProvider interface {
	// AccessToken returns the current raw access token, refreshing if needed.
	// Returns the bare token string — NOT "Bearer <token>".
	AccessToken(ctx context.Context) (string, error)
}

// Compile-time check: *client.Client satisfies TokenProvider.
var _ TokenProvider = (*client.Client)(nil)

// dispatchMsg is the internal event type passed through the dispatch channel.
type dispatchMsg struct {
	msgType string
	raw     json.RawMessage
}

// accountStreamer implements Streamer for the TastyTrade account WebSocket.
type accountStreamer struct {
	wsURL      string
	accountID  string
	token      TokenProvider
	handler    AccountHandler
	backoff     BackoffPolicy
	log        *zap.Logger

	// status fields — guarded by statusMu
	statusMu       sync.RWMutex
	connected      bool
	connectedSince time.Time
	lastEventAt    time.Time
	lastError      string

	// reconnectCount is updated atomically so Status() never needs the lock.
	reconnectCount int64

	// dropCount tracks events dropped due to full dispatch channel (diagnostic).
	dropCount int64
}

// NewAccountStreamer creates an account streamer.
//
//   - wsURL:     WebSocket endpoint, e.g. "wss://streamer.tastytrade.com"
//   - accountID: TastyTrade account number to subscribe to
//   - token:     TokenProvider — typically *client.Client
//   - handler:   AccountHandler that receives decoded events
//   - log:       structured logger
func NewAccountStreamer(
	wsURL string,
	accountID string,
	token TokenProvider,
	handler AccountHandler,
	log *zap.Logger,
) Streamer {
	return &accountStreamer{
		wsURL:     wsURL,
		accountID: accountID,
		token:     token,
		handler:   handler,
		backoff:   DefaultBackoff,
		log:       log,
	}
}

// Name implements Streamer.
func (a *accountStreamer) Name() string { return accountStreamerName }

// Status implements Streamer. Safe to call from any goroutine.
func (a *accountStreamer) Status() StreamerStatus {
	a.statusMu.RLock()
	s := StreamerStatus{
		Name:           accountStreamerName,
		Connected:      a.connected,
		ConnectedSince: a.connectedSince,
		LastEventAt:    a.lastEventAt,
		ReconnectCount: int(atomic.LoadInt64(&a.reconnectCount)),
		LastError:      a.lastError,
	}
	a.statusMu.RUnlock()
	return s
}

func (a *accountStreamer) setConnected(since time.Time) {
	a.statusMu.Lock()
	a.connected = true
	a.connectedSince = since
	a.lastError = ""
	a.statusMu.Unlock()

	client.Metrics.StreamerUptime.WithLabelValues(accountStreamerName).Set(0)
}

func (a *accountStreamer) setDisconnected(err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	a.statusMu.Lock()
	a.connected = false
	a.lastError = msg
	a.statusMu.Unlock()
}

func (a *accountStreamer) touchLastEvent() {
	a.statusMu.Lock()
	a.lastEventAt = time.Now()
	a.statusMu.Unlock()
}

// Start implements Streamer. Blocks until ctx is cancelled.
func (a *accountStreamer) Start(ctx context.Context) error {
	failures := 0

	for {
		// Check for cancellation before attempting (re)connect.
		select {
		case <-ctx.Done():
			a.log.Info("account streamer stopping", zap.String("reason", ctx.Err().Error()))
			return ctx.Err()
		default:
		}

		// Attempt one connection cycle.
		err := a.runOnce(ctx)

		if ctx.Err() != nil {
			// Context was cancelled during runOnce — clean exit.
			return ctx.Err()
		}

		// Connection ended with an error.
		a.setDisconnected(err)
		a.log.Error("account streamer disconnected",
			zap.Error(err),
			zap.Int("failures", failures),
		)

		// Not the first failure — increment reconnect counter.
		if failures > 0 {
			atomic.AddInt64(&a.reconnectCount, 1)
			client.Metrics.StreamerReconnects.WithLabelValues(accountStreamerName).Inc()
		}

		wait := a.backoff.Next(failures)
		failures++

		a.log.Info("account streamer will reconnect",
			zap.Duration("wait", wait),
			zap.Int64("total_reconnects", atomic.LoadInt64(&a.reconnectCount)),
		)

		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// runOnce executes a single connect→subscribe→receive cycle.
// Returns when the connection closes (cleanly or with error).
func (a *accountStreamer) runOnce(ctx context.Context) error {
	// Step 1: always ensure a fresh token before connecting.
	// The account streamer uses the raw token — NOT the "Bearer" form.
	rawToken, err := a.token.AccessToken(ctx)
	if err != nil {
		return fmt.Errorf("runOnce: AccessToken: %w", err)
	}

	// Step 2: dial WebSocket.
	a.log.Info("account streamer connecting", zap.String("url", a.wsURL))
	conn, _, err := websocket.Dial(ctx, a.wsURL, nil)
	if err != nil {
		return fmt.Errorf("runOnce: dial: %w", err)
	}
	defer conn.CloseNow() //nolint:errcheck

	// Step 3: send connect message.
	connectMsg := map[string]any{
		"action":     "connect",
		"value":      []string{a.accountID},
		"request-id": 1,
		"auth-token": rawToken,
	}
	if err := wsjson.Write(ctx, conn, connectMsg); err != nil {
		return fmt.Errorf("runOnce: send connect: %w", err)
	}

	// Step 4: send account-subscribe.
	subscribeMsg := map[string]any{
		"action":     "account-subscribe",
		"value":      []string{a.accountID},
		"request-id": 2,
		"auth-token": rawToken,
	}
	if err := wsjson.Write(ctx, conn, subscribeMsg); err != nil {
		return fmt.Errorf("runOnce: send account-subscribe: %w", err)
	}

	connectedAt := time.Now()
	a.setConnected(connectedAt)
	a.log.Info("account streamer subscribed",
		zap.String("account", a.accountID),
		zap.Time("connected_at", connectedAt),
	)

	// Step 5: start buffered dispatch goroutine.
	dispatchCh := make(chan dispatchMsg, dispatchBufSize)
	dispatchDone := make(chan struct{})
	go a.dispatchLoop(ctx, dispatchCh, dispatchDone)

	// Step 6: run heartbeat goroutine.
	heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
	heartbeatDone := make(chan struct{})
	go a.heartbeatLoop(heartbeatCtx, conn, rawToken, heartbeatDone)

	// Step 7: receive loop (blocks until connection closes or ctx cancelled).
	receiveErr := a.receiveLoop(ctx, conn, dispatchCh)

	// Tear down.
	cancelHeartbeat()
	<-heartbeatDone

	close(dispatchCh)
	<-dispatchDone

	return receiveErr
}

// receiveLoop reads messages from the WebSocket and routes them to dispatchCh.
// Never blocks on dispatch — drops events and logs a warning if channel is full.
func (a *accountStreamer) receiveLoop(
	ctx context.Context,
	conn *websocket.Conn,
	dispatchCh chan<- dispatchMsg,
) error {
	for {
		var raw json.RawMessage
		if err := wsjson.Read(ctx, conn, &raw); err != nil {
			return fmt.Errorf("receiveLoop: read: %w", err)
		}

		// Peek at the type field without full decode.
		var peek struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &peek); err != nil {
			a.log.Warn("account streamer: unreadable message", zap.Error(err))
			continue
		}

		// Skip heartbeat-ack and untyped ack messages — not dispatched.
		if peek.Type == "heartbeat-ack" || peek.Type == "" {
			continue
		}

		msg := dispatchMsg{msgType: peek.Type, raw: raw}

		select {
		case dispatchCh <- msg:
		default:
			// Channel full — log drop and continue receiving.
			dropped := atomic.AddInt64(&a.dropCount, 1)
			a.log.Warn("account streamer: dispatch channel full, dropping event",
				zap.String("type", peek.Type),
				zap.Int64("total_dropped", dropped),
			)
		}
	}
}

// dispatchLoop drains dispatchCh and calls AccountHandler methods.
// Runs in its own goroutine so handler work never blocks the receive loop.
func (a *accountStreamer) dispatchLoop(
	ctx context.Context,
	ch <-chan dispatchMsg,
	done chan<- struct{},
) {
	defer close(done)

	for msg := range ch {
		a.touchLastEvent()

		var envelope models.AccountMessage
		if err := json.Unmarshal(msg.raw, &envelope); err != nil {
			a.log.Warn("account streamer: failed to decode envelope",
				zap.String("type", msg.msgType),
				zap.Error(err),
			)
			continue
		}

		switch envelope.Type {
		case "order":
			var ev models.OrderEvent
			if err := json.Unmarshal(envelope.Data, &ev); err != nil {
				a.log.Warn("account streamer: decode OrderEvent", zap.Error(err))
				continue
			}
			a.handler.OnOrderEvent(ev)

		case "account-balance":
			var ev models.BalanceEvent
			if err := json.Unmarshal(envelope.Data, &ev); err != nil {
				a.log.Warn("account streamer: decode BalanceEvent", zap.Error(err))
				continue
			}
			a.handler.OnBalanceEvent(ev)

		case "position":
			var ev models.PositionEvent
			if err := json.Unmarshal(envelope.Data, &ev); err != nil {
				a.log.Warn("account streamer: decode PositionEvent", zap.Error(err))
				continue
			}
			a.handler.OnPositionEvent(ev)

		default:
			a.log.Debug("account streamer: unknown message type",
				zap.String("type", envelope.Type),
			)
		}
	}
}

// heartbeatLoop sends a heartbeat every 30 seconds until its context is cancelled.
func (a *accountStreamer) heartbeatLoop(
	ctx context.Context,
	conn *websocket.Conn,
	rawToken string,
	done chan<- struct{},
) {
	defer close(done)

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			hb := map[string]any{
				"action":     "heartbeat",
				"request-id": nil,
				"auth-token": rawToken,
			}
			if err := wsjson.Write(ctx, conn, hb); err != nil {
				a.log.Warn("account streamer: heartbeat send failed", zap.Error(err))
				// The receive loop will detect the dead connection and return.
				return
			}
			// Update uptime gauge: seconds since connected.
			a.statusMu.RLock()
			since := a.connectedSince
			a.statusMu.RUnlock()
			if !since.IsZero() {
				client.Metrics.StreamerUptime.WithLabelValues(accountStreamerName).
					Set(time.Since(since).Seconds())
			}
		}
	}
}

// Compile-time assertion: *accountStreamer implements Streamer.
var _ Streamer = (*accountStreamer)(nil)
