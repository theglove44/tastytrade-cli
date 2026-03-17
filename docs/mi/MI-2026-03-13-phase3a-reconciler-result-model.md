# MI-2026-03-13 Phase 3A Reconciler Result Model

## Trigger

Advance Phase 3A reconciler behavior so each reconciliation pass is deterministic, inspectable, and operationally useful.

Goals:

- structured reconcile result model
- clear status per run
- mismatch counts and detail categories
- improved summary logging
- improved Prometheus metrics
- focused tests for healthy, drift, partial, and error paths

## Files changed

- `internal/reconciler/reconciler.go`
- `internal/reconciler/reconciler_test.go`
- `internal/client/metrics.go`

## Result model introduced

Added in `internal/reconciler/reconciler.go`:

- `type Status string`
- statuses:
  - `ok`
  - `drift_detected`
  - `partial`
  - `error`
- `type Mismatch struct`
  - `symbol`
  - `category`
  - `action`
- `type Result struct`
  - `run_at`
  - `duration`
  - `status`
  - `positions_checked`
  - `symbols_checked`
  - `mismatch_count`
  - `mismatch_categories`
  - `recovery_triggered`
  - `action`
  - `error_text`
  - `mismatches`

`runOnce(...)` now returns `Result`.

`RunOnceForTest(...)` was updated to return the structured result as well.

## Status model semantics

- `ok`
  - REST fetch succeeded
  - no drift detected
  - no degraded side effects
- `drift_detected`
  - REST fetch succeeded
  - one or more mismatches were found and reconciled / surfaced
- `partial`
  - reconciliation logic completed but a non-fatal side effect degraded the run
  - currently used for snapshot store write failures
- `error`
  - REST positions fetch failed
  - MarkBook remains unchanged for that pass

## Mismatch categories currently surfaced

- `missing_in_markbook`
- `avg_open_drift`
- `absent_from_rest`
- `absence_pending`

## Logging changes

Each run now emits a single summary line:

- message: `reconciler: pass complete`
- fields include:
  - `status`
  - `account`
  - `positions_checked`
  - `symbols_checked`
  - `mismatches`
  - `recovery_triggered`
  - `action`
  - `duration`
  - `error` (error path only)

Behavior:

- healthy / drift / partial runs log one summary at `INFO`
- hard failure logs one summary at `WARN`
- mismatch detail logs are emitted at `DEBUG` only, one per mismatch item

This keeps healthy runs low-noise while still surfacing drift detail when needed.

## Metrics added / refined

Existing metrics retained:

- `tastytrade_reconcile_runs_total`
- `tastytrade_reconcile_errors_total`
- `tastytrade_reconcile_positions_corrected_total`

Added:

- `tastytrade_reconcile_runs_by_status_total{status=...}`
- `tastytrade_reconcile_errors_by_type_total{type=...}`
- `tastytrade_reconcile_last_status{status=...}`
  - one-hot latest status gauge
- `tastytrade_reconcile_last_duration_seconds`
- `tastytrade_reconcile_last_mismatch_count`

## Tests added / updated

Updated `internal/reconciler/reconciler_test.go` to cover:

- healthy reconcile result
- drift detected result
- error result
- partial result on store write failure
- summary log expectations
- mismatch detail debug log expectations
- metric side effects on key status paths
- existing absence threshold and patching behavior retained

## Validation

Ran:

```bash
gofmt -w internal/reconciler/reconciler.go internal/reconciler/reconciler_test.go internal/client/metrics.go
go mod tidy
go build ./...
go vet ./...
go test ./...
```

Results:

- `gofmt` ✅
- `go mod tidy` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

## Follow-up suggestion

Next useful Phase 3A step:

- expose the latest `reconciler.Result` through a lightweight in-process status surface (command status / health dump / debug endpoint) so operators can inspect the most recent mismatch details without parsing logs or scraping only aggregate metrics.
