# MI-2026-03-15 Phase 6A Broker Order Detail Inspection

## Trigger

Start Phase 6A by adding a minimal read-only broker-facing detail inspection workflow so an operator can inspect a single tastytrade order in more depth from the CLI.

## Scope inspected

- `cmd/broker_orders.go`
- `cmd/broker_orders_test.go`
- `internal/exchange/exchange.go`
- `internal/exchange/tastytrade/tastytrade.go`
- `internal/models/models.go`
- `/Users/christaylor/Projects/tastytrade-docs/order_management.md`

## Short plan before edits

Smallest viable Phase 6A slice:

- keep the existing `broker-orders` command surface
- add one read-only detail subcommand using canonical broker order `id`
- add a thin exchange method over `GET /accounts/{account_number}/orders/{id}`
- reuse existing broker order view shaping and JSON field names
- keep output high-signal only and avoid status-history expansion
- make not-found/account-scope errors explicit

## Files changed

- `cmd/broker_orders.go`
- `cmd/broker_orders_test.go`
- `cmd/account_resolver_test.go`
- `cmd/decision_gate_test.go`
- `internal/reconciler/reconciler_test.go`
- `internal/exchange/exchange.go`
- `internal/exchange/tastytrade/tastytrade.go`
- `docs/broker-order-inspection.md`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-15-phase6a-broker-order-detail-inspection.md`

## Findings

- The tastytrade order-management docs reference fetching an individual order via `GET /accounts/{account_number}/orders/{id}`.
- The repo already had the right `broker-orders` command surface and order model for a minimal detail slice.
- Reusing the existing broker order view kept JSON/output naming consistent and avoided adding a raw API dump.
- The cleanest minimal error behavior is to render not-found/account-scope issues with explicit broker-order/account context.

## Validation commands

```bash
gofmt -w cmd/broker_orders.go cmd/broker_orders_test.go cmd/account_resolver_test.go cmd/decision_gate_test.go internal/reconciler/reconciler_test.go internal/exchange/exchange.go internal/exchange/tastytrade/tastytrade.go
go build ./...
go vet ./...
go test ./...
```

## Validation outcome

Validation passed.

## Out of scope

- broker mutation
- cancel/replace
- automatic reconciliation
- local persisted state mutation
- status-history exploration beyond the current order detail surface
- any change to Phase 3C safety behavior
