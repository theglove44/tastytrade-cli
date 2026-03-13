# MI-2026-03-13 Phase 3A Watch Reconciler Status Output

## Trigger

Surface the latest completed reconciler result directly in the existing `tt watch` workflow so operators can inspect the latest reconciliation state at runtime without relying only on logs from the reconciler itself or aggregate Prometheus counters.

## Files changed

- `cmd/watch.go`
- `cmd/watch_test.go`
- `cmd/root.go`

## Status surface added

Added a lightweight watch-mode status log path using the already-available in-process:

```go
LatestResult() (Result, bool)
```

### Implementation

- `cmd/root.go`
  - stores the reconciler instance in package-global `rec` when watch/runtime plumbing creates it
- `cmd/watch.go`
  - adds `currentWatchReconcileStatus(...)`
  - adds `logWatchReconcileStatus(...)`
  - emits an immediate status line at watch startup
  - starts a small periodic status loop on `cfg.ReconcileInterval`

No new subsystem, endpoint, or metrics abstraction was introduced.

## Operator-facing output shape

Message:

- `tt watch reconcile status`

Before any reconcile run:

- `reconcile_status=not_yet_available`

After a completed run, fields include:

- `reconcile_status`
- `reconcile_last_run_at`
- `reconcile_duration`
- `reconcile_positions_checked`
- `reconcile_symbols_checked`
- `reconcile_mismatch_count`
- `reconcile_mismatch_categories`
- `reconcile_recovery_triggered`
- `reconcile_action`
- `reconcile_error` (if present)

Mismatch details are intentionally not dumped by default in watch status output to keep healthy and drifted snapshots readable.

## Tests added

Added focused tests in `cmd/watch_test.go` for:

- no run yet
- healthy run snapshot
- drift run status output
- error run status output

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

- combine the watch reconcile status line with a compact runtime health snapshot (account streamer, market streamer, reconciler) so `tt watch` emits one concise operational heartbeat instead of separate per-component inspection paths.
