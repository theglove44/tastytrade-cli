# MI-2026-03-18 Phase 7A Compare-to-Detail Operator Handoff

## Trigger / symptom

Add a tighter operator handoff from `tt submit-state compare` to the existing broker-order detail command when a canonical broker order ID is already available in a comparison result.

## Scope inspected

- `cmd/submit_state_compare.go`
- `cmd/submit_state_compare_test.go`
- `cmd/submit_state_compare_phase5a_test.go`
- `cmd/broker_orders.go`
- `docs/local-vs-broker-order-comparison.md`
- `docs/manual-reconciliation-runbook.md`
- `docs/broker-order-inspection.md`
- `docs/mi/README.md`

## Commands run

- `git status --short`
- `sed -n '1,260p' cmd/submit_state_compare.go`
- `sed -n '1,260p' cmd/broker_orders.go`
- `sed -n '1,260p' docs/local-vs-broker-order-comparison.md`
- `sed -n '1,260p' docs/manual-reconciliation-runbook.md`
- `sed -n '1,220p' docs/broker-order-inspection.md`
- `rg -n "submit-state compare|recommended_actions|broker_order_id|next_action|advisory manual" cmd/*test.go cmd/*.go`
- `sed -n '1,260p' cmd/submit_state_compare_test.go`
- `sed -n '1,220p' cmd/submit_state_compare_phase5a_test.go`

## Files inspected

- `cmd/submit_state_compare.go`
- `cmd/submit_state_compare_test.go`
- `cmd/submit_state_compare_phase5a_test.go`
- `cmd/broker_orders.go`
- `docs/local-vs-broker-order-comparison.md`
- `docs/manual-reconciliation-runbook.md`
- `docs/broker-order-inspection.md`

## Findings

- `submit-state compare` already surfaces `broker_order_id` in both human-readable and JSON output when a plausible broker-visible order is available.
- The smallest useful Phase 7A handoff is an exact broker-detail command hint derived from that existing canonical broker order ID.
- Adding the handoff only to the human-readable path preserves the current JSON schema and avoids expanding the advisory result model.
- Reusing the existing `tt broker-orders detail --id <broker-order-id>` command keeps scope inside the current read-only operator workflow.

## Direct answers / conclusions

- The handoff should be advisory only and should not introduce automation or reconciliation behavior.
- JSON should remain unchanged for this slice.
- The compare output should print the exact broker-detail command only when `broker_order_id` is already present on the comparison result.

## Proposed surgical fix

- Add a small helper that formats the broker-detail command from `broker_order_id`.
- Print that command as a `next_step=` hint in human-readable compare output only when a broker order ID is available.
- Add focused tests proving the hint appears for broker-backed results, does not appear without a broker order ID, and does not leak into JSON output.
- Update the operator docs and MI index with the new compare-to-detail handoff behavior.

## Files changed

- `cmd/submit_state_compare.go`
- `cmd/submit_state_compare_test.go`
- `cmd/submit_state_compare_phase5a_test.go`
- `docs/local-vs-broker-order-comparison.md`
- `docs/manual-reconciliation-runbook.md`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-18-phase7a-compare-to-detail-operator-handoff.md`

## Validation status

Passed.

Validation commands:

- `gofmt -w cmd/submit_state_compare.go cmd/submit_state_compare_test.go cmd/submit_state_compare_phase5a_test.go`
- `go build ./...`
- `go vet ./...`
- `go test ./...`

Validation result:

- all commands passed

## Current status / next steps

- Phase 7A compare-to-detail handoff is implemented as a human-readable `next_step=` hint only.
- No JSON change was introduced.
- No further Phase 7A work is required unless the operator workflow later needs a separate explicit JSON advisory field.
