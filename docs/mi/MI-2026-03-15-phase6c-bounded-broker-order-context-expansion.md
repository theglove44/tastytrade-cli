# MI-2026-03-15 Phase 6C Bounded Broker Order Context Expansion

## Trigger

Start Phase 6C by adding a small amount of extra operator-useful context to `tt broker-orders detail --id <broker-order-id>` without turning the command into a history explorer, reconciliation tool, or broader broker inspector.

## Scope inspected

- `cmd/broker_orders.go`
- `cmd/broker_orders_test.go`
- `internal/models/models.go`
- `internal/exchange/exchange.go`
- `internal/exchange/tastytrade/tastytrade.go`
- `docs/broker-order-inspection.md`

## Short plan before edits

Smallest viable Phase 6C slice:

- keep the existing single-order fetch path unchanged
- keep JSON unchanged
- extend only the human-readable detail rendering
- reuse bounded fields already present in `models.OrderLeg`
- show per-leg fill context only when the current shaped model already contains it

## Files changed

- `cmd/broker_orders.go`
- `cmd/broker_orders_test.go`
- `docs/broker-order-inspection.md`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-15-phase6c-bounded-broker-order-context-expansion.md`

## Findings

- The current order model already includes bounded per-leg fill context via `fill-quantity` and `average-fill-price`.
- Reusing those existing fields in the human-readable detail path provides real operator value for filled/partially filled orders without requiring a new endpoint or JSON redesign.
- Keeping JSON unchanged avoids widening scope and preserves current list/detail shaped output compatibility.
- The cleanest 6C slice is therefore a modest detail-rendering enhancement only.

## Validation commands

```bash
gofmt -w cmd/broker_orders.go cmd/broker_orders_test.go
go build ./...
go vet ./...
go test ./...
```

## Validation outcome

Validation passed.

## Out of scope

- status/event history
- timeline reconstruction
- local-vs-broker combined diagnostics
- reconciliation actions
- raw broker payload dumping
- JSON redesign
- additional broker endpoints
- any Phase 3C submit safety changes
