# MI-2026-03-13 Phase 3A Watch Operational Heartbeat

## Trigger

Add one concise operator-facing runtime heartbeat in `tt watch` that combines the most important operational health signals into a single compact status line.

## Files changed

- `cmd/watch.go`
- `cmd/watch_test.go`
- `cmd/root.go`

## Heartbeat added

Added a compact watch heartbeat log line:

- message: `tt watch heartbeat`

It is emitted:

- once at watch startup
- periodically on the existing reconcile interval cadence

## Signals included

The heartbeat combines:

- account streamer status
- market streamer status
- reconcile status
- last reconcile run time
- reconcile mismatch count
- tracked symbols count
- open positions count
- degraded flag
- degraded reason summary when applicable

## Implementation notes

- `cmd/root.go`
  - stores runtime account and market streamer references in package globals for watch-mode inspection
- `cmd/watch.go`
  - adds `currentWatchHeartbeat(...)`
  - adds `logWatchHeartbeat(...)`
  - adds `watchStatusLoop(...)`
  - reuses `LatestResult()` from the reconciler
  - reads existing tracked-symbol/open-position gauges instead of creating duplicate state

No new subsystem or endpoint was introduced.

## Health mapping

Streamer states are surfaced compactly as:

- `up`
- `starting`
- `down`
- `n/a`

Reconcile state uses the existing structured status model:

- `ok`
- `drift_detected`
- `partial`
- `error`
- `not_yet_available`

Heartbeat degradation reason is summarized compactly, for example:

- `reconcile_drift`
- `reconcile_partial`
- `reconcile_error`
- `market`
- `account`
- combined forms like `market,reconcile_error`

## Tests added

Added focused heartbeat tests in `cmd/watch_test.go` for:

- startup / not-yet-available state
- healthy heartbeat
- drift/degraded heartbeat
- error-state heartbeat

## Validation

Ran:

```bash
gofmt -w cmd/watch.go cmd/watch_test.go cmd/root.go
go build ./...
go vet ./...
go test ./...
```

Results:

- `gofmt` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

## Follow-up suggestion

Next useful Phase 3A step:

- make the heartbeat cadence independently configurable from the full reconcile interval if operators want a shorter runtime health pulse without increasing REST reconciliation frequency.
