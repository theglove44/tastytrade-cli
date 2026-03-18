# MI-2026-03-18 Phase 6D Terminal-State Broker Order Reason Visibility

## Trigger

Revisit Phase 6D and verify whether broker order detail output exposes terminal-state broker context for terminal orders without widening the read-only broker inspection surface.

## Scope inspected

- `cmd/broker_orders.go`
- `cmd/broker_orders_test.go`
- `internal/models/models.go`
- `docs/broker-order-inspection.md`
- `docs/mi/README.md`
- existing Phase 6A/6B/6C MI history

## Commands run

- `git status --short`
- `git log --oneline -n 12 --decorate`
- `rg -n "terminal-state|reason visibility|order reason|reason" cmd internal docs/mi`
- `sed -n '1,240p' docs/mi/MI-2026-03-15-phase6c-bounded-broker-order-context-expansion.md`
- `sed -n '1,220p' docs/mi/README.md`
- `sed -n '1,280p' cmd/broker_orders.go`
- `sed -n '1,260p' cmd/broker_orders_test.go`
- `sed -n '1,260p' docs/broker-order-inspection.md`
- `sed -n '1,200p' internal/models/models.go`
- `sed -n '1,220p' internal/exchange/exchange.go`
- `sed -n '1,260p' internal/exchange/tastytrade/tastytrade.go`

## Files inspected

- `cmd/broker_orders.go`
- `cmd/broker_orders_test.go`
- `internal/models/models.go`
- `internal/exchange/exchange.go`
- `internal/exchange/tastytrade/tastytrade.go`
- `docs/broker-order-inspection.md`
- `docs/mi/README.md`

## Findings

- The current order model already carried timestamps and per-leg fill context, but it did not surface terminal-state broker context fields.
- The broker order detail renderer was only showing the standard order, pricing, timestamp, and leg sections.
- The JSON shape for `broker-orders detail` was stable and could absorb additional optional terminal fields without changing the lookup path.

## Direct answers / conclusions

- Phase 6D should be treated as a bounded detail-rendering extension, not a new endpoint or status-history feature.
- Terminal-state visibility is now exposed by shaping optional terminal fields through the existing order view and text renderer.
- The change remains read-only and does not affect list commands, order mutation, or reconciliation behavior.

## Proposed surgical fix

- Add optional terminal-state fields to the shaped broker order model.
- Render a compact `terminal:` section in human-readable detail output when those fields are present.
- Add tests for both the text path and JSON path.

## Files changed

- `internal/models/models.go`
- `cmd/broker_orders.go`
- `cmd/broker_orders_test.go`
- `docs/broker-order-inspection.md`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-18-phase6d-terminal-state-broker-order-reason-visibility.md`

## Validation status

Passed.

Validation commands:

- `gofmt -w cmd/broker_orders.go cmd/broker_orders_test.go internal/models/models.go`
- `go build ./...`
- `go test ./...`
- `go vet ./...`

Validation result:

- all commands passed

## Current status / next steps

- Code changes were applied for terminal-state broker context visibility with a `reason` JSON field that remains optional and broker-provided.
- No additional action required for this slice unless the API schema later confirms a different terminal reason field name.
