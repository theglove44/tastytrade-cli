# internal/streamer — Phase 2

This package implements the two WebSocket streaming connections specified in
the v4 canonical document (§1.5 and §1.6).

## Planned packages

### account streamer (`account.go`)
Connects to `wss://streamer.tastytrade.com`.

Wire protocol (confirmed from spec):
- auth-token = raw `access_token` — **no** `Bearer` prefix (different from REST)
- `{"action":"connect","value":["ACCT#"],"request-id":1,"auth-token":"..."}`
- `{"action":"account-subscribe","value":["ACCT#"],"request-id":2,"auth-token":"..."}`
- `{"action":"heartbeat","request-id":null,"auth-token":"..."}`
- On reconnect: call `EnsureToken()` first, then re-send account-subscribe with fresh token

Responsibilities:
- Deliver `OnOrderFill` events → increment `Metrics.OrdersFilled` and `Metrics.OrderLatency`
- Deliver `OnPositionUpdate` events → update `Metrics.OpenPositions`
- Deliver `OnBalanceUpdate` events → update `Metrics.NLQDollars`
- Reconnect with exponential backoff (2s → 4s → 8s → 60s cap)
- Increment `Metrics.StreamerReconnects.WithLabelValues("account")` on each reconnect

### market data streamer (`market.go`)
Connects to `wss://tasty-openapi-ws.dxfeed.com/realtime` (DXLink protocol).

Requires `GET /api-quote-tokens` (unversioned — use `RequestOptions{SkipVersion: true}`)
before every connect and reconnect.

- KEEPALIVE every 30s: `{"type":"KEEPALIVE","channel":0}`
- After reconnect: re-fetch quote token, re-auth, re-subscribe all symbols
- Increment `Metrics.StreamerReconnects.WithLabelValues("market")` on each reconnect

## Interface contract (defined before implementation begins)

See `streamer.go` for the `Streamer`, `AccountHandler`, and `QuoteHandler` interfaces
that must be agreed before coding the implementations.

## Dependencies

- `nhooyr.io/websocket` — already in `go.mod`
- `internal/client` — for `EnsureToken()` and `Metrics`
- `internal/models` — for typed event structs
