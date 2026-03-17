# MI-2026-03-13 Phase 3A Reconcile Outcome Policy

## Trigger

Turn reconciler observability into explicit operational handling by defining a small conservative policy for reconcile outcomes:

- `ok`
- `drift_detected`
- `partial`
- `error`

## Files changed

- `internal/reconciler/reconciler.go`
- `internal/reconciler/reconciler_test.go`
- `internal/client/metrics.go`
- `cmd/watch.go`
- `cmd/watch_test.go`

## Policy model introduced

Added in `internal/reconciler/reconciler.go`:

- `type HandlingMode string`
  - `observe`
  - `limited_recovery`
  - `suppress`
- `type Severity string`
  - `info`
  - `warn`
  - `error`
- `type OutcomePolicy struct`
  - `severity`
  - `degraded`
  - `observe_only`
  - `recovery_allowed`
  - `suppress_confidence_actions`
  - `handling`
- `func PolicyForResult(Result) OutcomePolicy`

This is intentionally a small mapping layer, not a rules engine.

## Status-to-policy mapping

### `ok`

- severity: `info`
- degraded: `false`
- observe only: `true`
- recovery allowed: `false`
- suppress confidence-dependent actions: `false`
- handling: `observe`

### `drift_detected`

- severity: `warn`
- degraded: `true`
- observe only: `true`
- recovery allowed: `false`
- suppress confidence-dependent actions: `false`
- handling: `observe`

Interpretation:

- runtime is degraded enough to surface clearly
- no new aggressive automation is introduced
- existing safe correction already performed by reconciler remains unchanged

### `partial`

- severity: `warn`
- degraded: `true`
- observe only: `true`
- recovery allowed: `false`
- suppress confidence-dependent actions: `false`
- handling: `observe`

Interpretation:

- runtime is degraded
- current codebase does not expose an additional distinct recovery path for `partial`
- in today's implementation, `partial` most concretely represents degraded side effects such as snapshot-store write failure after the normal reconcile pass has already completed
- so the honest policy is degraded observation, not implied extra recovery

### `error`

- severity: `error`
- degraded: `true`
- observe only: `false`
- recovery allowed: `false`
- suppress confidence-dependent actions: `true`
- handling: `suppress`

Interpretation:

- reconciliation confidence is not sufficient for confidence-dependent actions
- runtime should surface suppression clearly

## Runtime status changes

### Heartbeat

`tt watch heartbeat` now includes:

- `reconcile_policy`
- `suppress_confidence_actions`
- existing degraded summary continues to reflect reconcile outcomes

Examples:

- healthy: `ok / observe`
- drift: `drift_detected / observe / degraded`
- partial: `partial / observe / degraded`
- error: `error / suppress / degraded`

### Detailed watch reconcile status

`tt watch reconcile status` now includes:

- `reconcile_policy`
- `reconcile_degraded`
- `suppress_confidence_actions`

## Metrics added

Added policy-aligned metrics in `internal/client/metrics.go`:

- `tastytrade_reconcile_policy_mode{mode=...}`
- `tastytrade_reconcile_policy_degraded`
- `tastytrade_reconcile_policy_suppress_confidence_actions`

These are updated in reconciler metric recording based on `PolicyForResult(...)`.

## Logging changes

Reconciler summary logs now include:

- `policy`
- `degraded`
- `suppress_confidence_actions`

This aligns structured logs with the policy surface and heartbeat output.

## Tests added / updated

### Reconciler policy tests

Added focused unit tests for:

- `ok`
- `drift_detected`
- `partial`
- `error`

### Watch / heartbeat tests

Added / updated tests for:

- no-run / not-yet-available heartbeat state
- healthy heartbeat policy fields
- drift heartbeat policy fields
- partial heartbeat policy fields
- error heartbeat policy fields

## Validation

Ran:

```bash
gofmt -w internal/reconciler/reconciler.go internal/reconciler/reconciler_test.go internal/client/metrics.go cmd/watch.go cmd/watch_test.go
go build ./...
go vet ./...
go test ./...
```

Results:

- `gofmt` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

## Follow-up recommendation

Next useful Phase 3A step:

- thread the new suppression signal into any future confidence-dependent automation entry point so operational policy is not just visible but can also gate strategy actions in a single consistent place when strategy logic is introduced.
