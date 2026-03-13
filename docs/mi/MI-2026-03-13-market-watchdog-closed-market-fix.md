# MI-2026-03-13 Market Watchdog Closed-Market Fix

## Trigger

Observed runtime metrics during watch mode with the market closed:

- account streamer uptime healthy
- market streamer reconnects increasing
- market streamer uptime staying low
- `last_quote_unix_seconds = 0`
- `tracked_symbols > 0`

Likely hypothesis:

- stale watchdog was forcing reconnects solely because no quotes were arriving,
  even though closed-market / no-initial-quote conditions are legitimate

## Files inspected

- `internal/streamer/market.go`
- `internal/streamer/market_watchdog_test.go`
- `cmd/root.go`
- `internal/client/metrics.go`

## Findings

### Previous watchdog rule

The old `isStale(...)` logic returned true when:

- subscribed symbols > 0
- `lastEventAt` was zero, in which case it fell back to `connectedSince`
- enough time elapsed since `connectedSince`

That means a connection with:

- tracked symbols
- zero quotes ever received

would eventually be considered stale and be force-reconnected.

This causes reconnect churn during:

- closed market
- illiquid symbols
- any valid connection that has not yet seen its first quote

### Open positions metric issue

Startup position seeding loaded positions into the MarkBook but did not set:

- `tastytrade_open_positions`

So the gauge could remain zero after startup even when positions had been seeded successfully.

## Applied surgical fix

### Files changed

- `internal/streamer/market.go`
- `internal/streamer/market_watchdog_test.go`
- `cmd/root.go`

### Watchdog rule after fix

`isStale(...)` now returns true only when:

- subscribed symbols > 0
- **at least one quote has been received on this connection** (`lastEventAt != 0`)
- elapsed time since the last received quote exceeds timeout

It no longer falls back to `connectedSince` when no quote has ever been received.

In plain English:

> Do not stale-reconnect a market stream just because it has symbols subscribed and has never seen a quote yet.

### Open positions gauge fix

In `seedMarkBookFromREST(...)`, after successful position seeding, the code now sets:

- `client.Metrics.OpenPositions.Set(float64(len(positions)))`

This makes the startup gauge reflect the already-loaded MarkBook state.

## Tests updated

Updated / added watchdog tests to cover:

- zero subscriptions → no reconnect
- subscribed symbols but zero quotes ever received → no reconnect
- quote just received → no reconnect
- quote flow started and then went stale → reconnect still occurs
- boundary checks for stale timing

## Validation

Ran:

```bash
gofmt -w internal/streamer/market.go internal/streamer/market_watchdog_test.go cmd/root.go
go build ./...
go vet ./...
go test ./...
```

Results:

- `gofmt` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

## Follow-up root cause found

A second reconnect-churn path remained even after the initial watchdog rule change.

### Remaining bug

`lastEventAt` was not being reset on a fresh websocket connection.

That means:

- an older connection could receive at least one quote
- `lastEventAt` would remain populated
- a later reconnect could enter a closed-market / zero-quote state
- watchdog logic would still see the old non-zero `lastEventAt`
- elapsed time from that old quote could exceed timeout
- the new connection would be incorrectly treated as stale and force another reconnect

So the effective bug was:

> stale state was being carried across websocket connections even though the watchdog is supposed to reason about quote flow on the current connection

### Additional surgical fix applied

In `internal/streamer/market.go`:

- `setConnected(...)` now resets `lastEventAt` to zero for each new connection

This makes the stale quote clock explicitly per-connection.

### Additional test added

Added coverage in `internal/streamer/market_watchdog_test.go`:

- `TestSetConnected_ResetsQuoteClock`

This verifies that:

- an old stale quote timestamp can exist from a prior connection
- after `setConnected(...)`, the new connection starts with `lastEventAt == 0`
- the new connection is not considered stale until it has actually received a quote

## Final conclusion

### Watchdog rule before

- stale reconnect could happen with subscribed symbols even when zero quotes had ever been received
- and stale quote timestamps could leak across reconnects

### Watchdog rule after

- stale reconnect only happens after quote flow has started on the current connection and then gone stale

### Expected operational effect

- watch-mode reconnect churn should stop during closed-market / zero-initial-quote conditions
- genuine post-quote stale connections will still reconnect
- `tastytrade_open_positions` should now reflect startup-seeded positions instead of remaining zero
