# MI-2026-03-15 Phase 3B Decision Gating

## Trigger

Thread the Phase 3A reconcile outcome policy into real confidence-dependent decision entry points without redesigning the reconciler or command architecture.

## Scope inspected

- `cmd/orders.go`
- `cmd/watch.go`
- `internal/client/client.go`
- `internal/reconciler/reconciler.go`
- current CLI command set in `cmd/`
- tastytrade reference docs in `/Users/christaylor/Projects/tastytrade-docs`

## Reference docs cross-checked

- `order_management.md`
  - dry-run validates order/account acceptance and returns warnings without routing
- `order_submission.md`
  - order validation and submission expectations
- `order_flow.md`
  - order lifecycle context and why pre-routing confidence matters

## Commands run

```bash
rg -n "reconcile|PolicyForResult|SuppressConfidenceActions|watch|order|submit|dry-run|candidate|strategy|plan|decision|execute" cmd internal
gofmt -w internal/reconciler/decision_gate.go internal/reconciler/decision_gate_test.go cmd/decision_gate.go cmd/decision_gate_test.go cmd/orders.go
go test ./internal/reconciler ./cmd
go build ./...
go vet ./...
go test ./...
```

## Files inspected

- `cmd/orders.go`
- `cmd/watch.go`
- `cmd/root.go`
- `internal/client/client.go`
- `internal/reconciler/reconciler.go`
- `/Users/christaylor/Projects/tastytrade-docs/order_management.md`
- `/Users/christaylor/Projects/tastytrade-docs/order_submission.md`
- `/Users/christaylor/Projects/tastytrade-docs/order_flow.md`

## Findings

- The current repo has one real confidence-dependent action entry point today: `tt dry-run`.
- Read-only paths like `tt orders`, `tt positions`, and watch/status surfaces should remain available even when reconcile policy is degraded or suppressing confidence-dependent actions.
- `internal/client.CheckOrderSafety()` already enforces kill switch and circuit breaker ordering; it does not know about reconciler state and should not be overloaded with runtime policy concerns.
- The right Phase 3B seam is therefore a small command-level confidence gate that consults the latest reconciler result immediately before consuming the orders-family dry-run endpoint.

## Change applied

Added a lightweight gating model derived directly from existing reconcile policy:

- `internal/reconciler/decision_gate.go`
  - `allowed`
  - `allowed_with_warning`
  - `blocked`
  - `GateDecisionForResult(Result)`

Mapping:

- `ok` → `allowed`
- `drift_detected` → `allowed_with_warning`
- `partial` → `allowed_with_warning`
- `error` → `blocked`

Then threaded that into the real action path:

- `cmd/decision_gate.go`
  - reads `rec.LatestResult()`
  - logs clear allow / warn / block state
  - returns a clear block error when confidence-dependent action should be suppressed
- `cmd/orders.go`
  - `runDryRun(...)` now consults the decision gate after existing order safety checks and before calling the exchange dry-run endpoint

## Operator visibility

Blocked / warned paths now emit high-signal structured logs including:

- action
- gate outcome
- reconcile status
- reconcile policy
- degraded flag
- suppress-confidence-actions flag
- reason

Blocked errors returned to the caller include explicit status/policy context, e.g.:

- `dry-run blocked by reconcile policy: status=error policy=suppress ...`

## Tests added

- `internal/reconciler/decision_gate_test.go`
  - allow under `ok`
  - warning under `drift_detected`
  - warning under `partial`
  - blocked under `error`
- `cmd/decision_gate_test.go`
  - command-level warning / block logging
  - dry-run blocked under reconcile `error`
  - dry-run still allowed under degraded warning state
  - read-only `orders` path unaffected by decision gating

## Validation status

- `gofmt -w internal/reconciler/decision_gate.go internal/reconciler/decision_gate_test.go cmd/decision_gate.go cmd/decision_gate_test.go cmd/orders.go` ✅
- `go test ./internal/reconciler ./cmd` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

## Current status / next step

Phase 3B now gives reconcile policy real operational effect at the current confidence-dependent decision entry point.

Next useful step:

- thread the same gate into the first real live-submit / strategy action path when that path is introduced, so dry-run and live execution share one decision-confidence contract.
