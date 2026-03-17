# MI-2026-03-15 Phase 3B Decision Gating Watch Surface

## Trigger

Extend the existing Phase 3B reconcile-policy decision gate beyond `tt dry-run` to the next real existing workflow in the repo without inventing a speculative live-submit path.

## Scope inspected

- `cmd/watch.go`
- `cmd/watch_test.go`
- `cmd/decision_gate.go`
- `cmd/orders.go`
- current CLI command set in `cmd/`
- tastytrade reference docs in `/Users/christaylor/Projects/tastytrade-docs`

## Reference docs cross-checked

- `streaming_account_data.md`
  - account/order/position state changes arrive asynchronously via the account streamer
- `order_management.md`
  - dry-run validates order/account state before routing

## Commands run

```bash
rg -n "AddCommand\(|DryRun\(|watch|decision gate|confidence-dependent" cmd internal
gofmt -w cmd/watch.go cmd/watch_test.go
go build ./...
go vet ./...
go test ./...
```

## Findings

- There is still no separate live-submit / strategy-execution command in the repo yet.
- The next best real existing workflow is `tt watch`, because it is the long-running operator workflow that keeps account/market/reconciler runtime state alive and is where suppression/warning state should be highly visible.
- Read-only commands like `tt orders` and `tt positions` should remain unaffected.
- `tt watch` should not itself be blocked by reconcile suppression, but it should surface the same gate outcome explicitly so operators can see whether confidence-dependent actions would currently be allowed, warned, or blocked.

## Change applied

Reused the existing gate contract from `cmd/decision_gate.go` inside `tt watch`:

- added `logWatchDecisionGate(...)`
- emits operator-visible structured output for:
  - `allowed`
  - `allowed_with_warning`
  - `blocked`
- includes:
  - reconcile status
  - reconcile policy
  - gate outcome
  - degraded flag
  - suppression flag
  - reason

This is called:

- once at watch startup
- on each watch status loop interval

## Tests added

Added focused watch tests covering:

- allowed path under `ok`
- warning path under `drift_detected`
- warning path under `partial`
- blocked path under `error`

Existing read-only command tests remain unchanged.

## Validation status

- `gofmt -w cmd/watch.go cmd/watch_test.go` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

## Current status / next step

Phase 3B decision gating now applies to:

- `tt dry-run` enforcement
- `tt watch` operator-surface visibility

Next useful step:

- add the same enforced gate to the first real live-submit / strategy execution path once that path lands, so watch visibility and execution enforcement stay aligned.
