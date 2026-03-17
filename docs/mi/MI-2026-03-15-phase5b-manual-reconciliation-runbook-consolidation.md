# MI-2026-03-15 Phase 5B Manual Reconciliation Runbook Consolidation

## Trigger

Start Phase 5B by consolidating the existing read-only local-state inspection, broker-order inspection, comparison, summaries/filters, and recommended next actions into one primary operator/developer runbook for manual reconciliation workflows.

## Scope inspected

- `docs/broker-order-inspection.md`
- `docs/local-vs-broker-order-comparison.md`
- `docs/live-submit-safety-runbook.md`
- `docs/README.md`
- `docs/mi/README.md`
- `cmd/submit_state.go`
- `cmd/submit_state_compare.go`

## Short plan before edits

Smallest viable Phase 5B slice:

- add one primary consolidated runbook under `docs/`
- keep existing docs for focused command/reference detail
- update help/discoverability text to point operators toward the manual reconciliation workflow
- avoid feature changes and avoid creating new command surfaces
- add only a small practical help-text test if useful

## Files changed

- `cmd/submit_state.go`
- `cmd/submit_state_compare.go`
- `cmd/submit_state_compare_phase5b_test.go`
- `docs/manual-reconciliation-runbook.md`
- `docs/README.md`
- `docs/broker-order-inspection.md`
- `docs/local-vs-broker-order-comparison.md`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-15-phase5b-manual-reconciliation-runbook-consolidation.md`

## Findings

- The repo already had the correct command surfaces; the gap was discoverability and a single consolidated procedure.
- A new runbook is the smallest aligned solution because the behavior is intentionally manual and read-only.
- Adding tiny discoverability cues in command help keeps the workflow easier to find without changing behavior.

## Validation commands

```bash
gofmt -w cmd/submit_state.go cmd/submit_state_compare.go cmd/submit_state_compare_phase5b_test.go
go build ./...
go vet ./...
go test ./...
```

## Validation outcome

Validation passed.

## Out of scope

- automatic reconciliation
- broker mutation
- new local-state mutation behavior
- cancel/replace
- Phase 3C safety behavior changes

## Follow-on recommendation

If further operator polish is needed later, the next safe step would be very small discoverability improvements in top-level CLI docs/examples, still without changing manual-only behavior.
