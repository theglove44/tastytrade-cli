# MI-2026-03-15 Phase 5A Manual Reconciliation Guidance

## Trigger

Start Phase 5A by adding a minimal read-only operator guidance layer that turns existing local-vs-broker comparison outcomes into recommended next actions for manual reconciliation work.

## Scope inspected

- `cmd/submit_state_compare.go`
- `cmd/submit_state_compare_test.go`
- `cmd/submit_state_compare_phase4c_test.go`
- `docs/local-vs-broker-order-comparison.md`
- `/Users/christaylor/Projects/tastytrade-docs/order_management.md`

## Short plan before edits

Smallest viable Phase 5A slice:

- reuse existing comparison outcomes exactly
- keep `tt submit-state compare` as the single command surface
- add deterministic recommended next actions per outcome
- render those actions in human-readable mode and JSON mode
- keep everything advisory-only and avoid any new reconciliation or mutation behavior

## Files changed

- `cmd/submit_state_compare.go`
- `cmd/submit_state_compare_test.go`
- `cmd/submit_state_compare_phase4c_test.go`
- `cmd/submit_state_compare_phase5a_test.go`
- `docs/local-vs-broker-order-comparison.md`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-15-phase5a-manual-reconciliation-guidance.md`

## Findings

- The existing comparison command already had the right operator context, so adding a new command was unnecessary.
- The smallest aligned guidance layer is an outcome-to-actions mapping attached directly to each comparison result.
- This stays consistent with the existing read-only/advisory design and avoids drifting into reconciliation logic.
- Since this phase mostly reuses existing broker inspection/comparison behavior, no new API transport behavior was needed; the tastytrade docs were rechecked to stay aligned with the existing broker order inspection basis.

## Validation commands

```bash
gofmt -w cmd/submit_state_compare.go cmd/submit_state_compare_test.go cmd/submit_state_compare_phase4c_test.go cmd/submit_state_compare_phase5a_test.go
go build ./...
go vet ./...
go test ./...
```

## Validation outcome

Validation passed.

## Out of scope

- automatic reconciliation
- local state mutation beyond existing explicit commands
- broker mutation
- cancel/replace
- any change to Phase 3C safety behavior

## Follow-on recommendation

The next safest follow-on would be a small docs/runbook consolidation for manual reconciliation procedures built on top of these advisory outcomes and recommended next actions, still without automation.
