# MI-2026-03-15 Phase 4A Broker Order State Inspection

## Trigger

Start Phase 4A with a minimal read-only broker-facing inspection workflow so operators can view recent/live tastytrade order state before any reconciliation or broader execution work is added.

## Scope inspected

- `cmd/orders.go`
- `cmd/account_resolver.go`
- `internal/exchange/exchange.go`
- `internal/exchange/tastytrade/tastytrade.go`
- tastytrade docs in `/Users/christaylor/Projects/tastytrade-docs/order_management.md`

## Short plan before edits

Smallest viable Phase 4A slice:

- keep existing `tt orders` behavior untouched
- add a new read-only command surface: `tt broker-orders`
- provide two subcommands:
  - `live` reusing the existing `Exchange.Orders(...)` path
  - `recent` using a thin `Exchange.RecentOrders(...)` wrapper over the search-orders endpoint
- keep output concise and operator-friendly, with optional stable JSON
- avoid any reconciliation or execution behavior changes

## Commands run

```bash
gofmt -w cmd/broker_orders.go cmd/broker_orders_test.go internal/exchange/exchange.go internal/exchange/tastytrade/tastytrade.go cmd/root.go
go build ./...
go vet ./...
go test ./...
```

## Files changed

- `cmd/broker_orders.go`
- `cmd/broker_orders_test.go`
- `cmd/root.go`
- `internal/exchange/exchange.go`
- `internal/exchange/tastytrade/tastytrade.go`
- `cmd/account_resolver_test.go`
- `cmd/decision_gate_test.go`
- `internal/reconciler/reconciler_test.go`
- `docs/broker-order-inspection.md`
- `docs/README.md`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-15-phase4a-broker-order-state-inspection.md`

## What was added

Read-only broker inspection commands:

- `tt broker-orders live`
- `tt broker-orders recent --limit N`

These surface concise broker-facing order state including:

- ID
- status
- order type
- time in force
- price / price effect
- received / updated / filled / cancelled timestamps when available
- legs

## Out of scope

- order mutation
- cancel/replace
- automatic reconciliation
- any change to Phase 3C submit safety behavior

## Follow-on recommendation

Next useful Phase 4 step:

- add a thin broker/local comparison surface only after this read-only broker inspection slice has been live-tested and operators confirm the returned fields are sufficient for manual troubleshooting.
