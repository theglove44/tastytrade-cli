# MI-2026-03-15 Live Submit Minimal Path

## Trigger

Start a new milestone to add the repo's first real live order submission path, reusing existing order safety and Phase 3B decision gating without broadening into orchestration or auto-entry.

## Scope inspected

- `cmd/orders.go`
- `cmd/root.go`
- `cmd/decision_gate.go`
- `internal/exchange/exchange.go`
- `internal/exchange/tastytrade/tastytrade.go`
- `internal/models/models.go`
- existing command-path tests in `cmd/`
- tastytrade reference docs in `/Users/christaylor/Projects/tastytrade-docs`

## Reference docs cross-checked

- `order_management.md`
  - `POST /accounts/{account_number}/orders` is the live submit endpoint
  - submit uses the same JSON payload shape as dry-run
- `order_submission.md`
  - validations, accepted response semantics, and warnings context
- `order_flow.md`
  - accepted live order enters submission lifecycle such as `Routed`
- `streaming_account_data.md`
  - order, position, and account state continue to change asynchronously after submit

## Commands run

```bash
gofmt -w internal/models/models.go internal/exchange/exchange.go internal/exchange/tastytrade/tastytrade.go cmd/orders.go cmd/root.go cmd/decision_gate.go cmd/decision_gate_test.go internal/reconciler/reconciler_test.go cmd/account_resolver_test.go
go build ./...
go vet ./...
go test ./...
```

## Findings

- The thinnest viable live path is a new `tt submit --file order.json [--json]` command mirroring `tt dry-run`.
- This keeps the payload contract, idempotency flow, and intent logging identical to dry-run while changing only the endpoint from `/orders/dry-run` to `/orders`.
- The command layer is the right seam for:
  - `cfg.LiveTrading` enforcement
  - `CheckOrderSafety()` reuse
  - existing Phase 3B decision gate reuse
- Read-only/status paths remain untouched.

## Change applied

Added the first minimal live-submit workflow:

- new command: `tt submit --file order.json [--json]`
- blocks unless `cfg.LiveTrading` is true
- reuses `cl.CheckOrderSafety()`
- reuses Phase 3B `enforceDecisionGate("submit", ...)`
- writes the same intent log / idempotency record shape as dry-run
- routes to `POST /accounts/{account}/orders`
- increments `tastytrade_orders_submitted_total{strategy="submit"}` on successful submit

Supporting changes:

- `internal/models/models.go`
  - added `SubmitResult`
- `internal/exchange/exchange.go`
  - added `Submit(...)`
- `internal/exchange/tastytrade/tastytrade.go`
  - implemented live submit exchange call
- `cmd/orders.go`
  - added `submitCmd`
  - added `runSubmit(...)`
  - extracted small shared helpers for order-file parsing, intent logging, and output shaping
- `cmd/root.go`
  - registered `submitCmd`

## Operator visibility

Human-readable `tt submit` now makes gate state explicit:

- warning: prints a visible degraded-confidence line before submission proceeds
- block: prints a visible blocked line before returning the blocking error

Messages include:

- reconcile status
- policy
- concise reason

JSON mode stays stable and avoids extra human chatter.

## Tests added / updated

- `cmd/decision_gate_test.go`
  - submit allowed path under `ok`
  - submit warning path under degraded reconcile state
  - submit blocked path under reconcile `error`
  - submit blocked when live trading is disabled
  - read-only `orders` path still unaffected
- interface stubs updated for new `Exchange.Submit(...)`
  - `cmd/account_resolver_test.go`
  - `internal/reconciler/reconciler_test.go`

## Validation status

- `gofmt -w ...` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

## Current status / next step

The repo now has:

- dry-run enforcement via Phase 3B decision gating
- watch-side operator visibility of gate state
- a first minimal live-submit command using the same safety and decision-confidence contract

Next useful step:

- add a thin operator confirmation / explicit acknowledgement layer for `tt submit` if desired, but keep it separate from orchestration or strategy automation.
