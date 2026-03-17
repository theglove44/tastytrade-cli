# MI-2026-03-15 Phase 6B Enriched Single-Order Detail Rendering

## Trigger

Start Phase 6B by improving the operator-facing human-readable rendering for `tt broker-orders detail --id <broker-order-id>` while keeping the feature read-only, minimal, and aligned with the Phase 6A single-order fetch path.

## Scope inspected

- `cmd/broker_orders.go`
- `cmd/broker_orders_test.go`
- `internal/exchange/exchange.go`
- `internal/exchange/tastytrade/tastytrade.go`
- `docs/broker-order-inspection.md`
- `docs/README.md`

## Short plan before edits

Smallest viable Phase 6B slice:

- keep the existing `broker-orders detail` command surface
- leave JSON unchanged
- improve only the human-readable rendering path
- group top-level fields more clearly
- omit absent optional fields cleanly
- improve leg readability without widening scope into a history or inspector tool

## Files changed

- `cmd/broker_orders.go`
- `cmd/broker_orders_test.go`
- `docs/broker-order-inspection.md`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-15-phase6b-enriched-single-order-detail-rendering.md`

## Findings

- The Phase 6A detail path already had the right data and fetch path; the gap was only presentation.
- The smallest aligned improvement is a single rendering helper for the detail text path.
- JSON should remain unchanged because the shaped order model already contains the needed fields.
- Omitting absent optional fields materially improves readability without changing semantics.

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

- broker status history
- timeline reconstruction
- local-vs-broker combined detail views
- reconciliation suggestions/actions
- raw broker payload dumping
- JSON redesign
- any Phase 3C safety behavior changes
