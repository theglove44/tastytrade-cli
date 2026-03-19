# MI-2026-03-18 Phase 7B Next-Step Handoff After Compare

## Trigger / symptom

Implement the smallest safe Phase 7B slice after the Phase 7A compare-to-detail handoff by making the next broker re-inspection commands explicit for compare outcomes that do not yet have a canonical `broker_order_id`.

## Scope inspected

- `cmd/submit_state_compare.go`
- `cmd/broker_orders.go`
- `docs/local-vs-broker-order-comparison.md`
- `docs/manual-reconciliation-runbook.md`
- `docs/broker-order-inspection.md`
- Phase 6A through 6D MI notes
- Phase 7A MI note

## Commands run

- `git status --short`
- `sed -n '1,340p' cmd/submit_state_compare.go`
- `sed -n '1,220p' cmd/submit_state_compare_phase5a_test.go`
- `sed -n '1,260p' docs/local-vs-broker-order-comparison.md`
- `sed -n '1,320p' docs/manual-reconciliation-runbook.md`
- `sed -n '1,260p' docs/mi/MI-2026-03-18-phase7b-next-step-handoff-after-compare.md`

## Files inspected

- `cmd/submit_state_compare.go`
- `cmd/broker_orders.go`
- `docs/local-vs-broker-order-comparison.md`
- `docs/manual-reconciliation-runbook.md`
- `docs/broker-order-inspection.md`
- `docs/mi/README.md`
- Phase 6A/6B/6C/6D and Phase 7A MI notes

## Findings

- Phase 7A already covers the exact broker-detail handoff when `broker_order_id` exists.
- The remaining bounded operator gap is the no-match re-inspection case, especially `local_uncertain_no_broker_match`, where the compare output still relies on implicit follow-up commands.
- Reusing `tt broker-orders live` and `tt broker-orders recent --limit N` keeps the slice display-only and inside the existing read-only workflow.
- JSON should remain unchanged because these re-inspection hints are derived operator guidance, not new comparison state.

## Direct answers / conclusions

- Phase 7B is implemented as a human-readable `next_step=` re-inspection hint only for compare results without `broker_order_id`.
- The hint is intentionally narrow and currently targets the `local_uncertain_no_broker_match` path.
- Broker-order detail remains unchanged.
- JSON remains unchanged.

## Proposed surgical fix

- Add a small helper that formats re-inspection commands for compare outcomes without `broker_order_id`.
- Print `tt broker-orders live` and `tt broker-orders recent --limit N` as human-readable `next_step=` hints for `local_uncertain_no_broker_match`.
- Keep JSON unchanged and update only the affected docs and focused tests.

## Files changed

- `cmd/submit_state_compare.go`
- `cmd/submit_state_compare_phase5a_test.go`
- `docs/local-vs-broker-order-comparison.md`
- `docs/manual-reconciliation-runbook.md`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-18-phase7b-next-step-handoff-after-compare.md`

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

- Phase 7B is now implemented as a display-only compare re-inspection handoff.
- No JSON change was introduced.
- No additional Phase 7B expansion is recommended unless a later workflow slice explicitly needs it.
