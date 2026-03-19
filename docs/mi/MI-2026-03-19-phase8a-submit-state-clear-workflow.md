# MI-2026-03-19 Phase 8A Explicit Post-Verification Submit-State Clear Workflow

## Trigger / symptom

The `submit-state clear` workflow was correct but a little thin on operator framing.

The goal for Phase 8A was to make the manual clear path more explicit about post-verification usage without adding any automatic clearing, broker mutation, or reconciliation behavior.

## Scope inspected

- `cmd/submit_state.go`
- `cmd/submit_state_test.go`
- `docs/manual-reconciliation-runbook.md`
- `docs/live-submit-safety-runbook.md`
- `docs/broker-order-inspection.md`

## Commands run

- `git branch --show-current`
- `git status --short`
- `sed -n '1,220p' cmd/submit_state.go`
- `sed -n '1,220p' cmd/submit_state_test.go`
- `sed -n '1,260p' docs/manual-reconciliation-runbook.md`
- `sed -n '1,260p' docs/live-submit-safety-runbook.md`
- `sed -n '1,220p' docs/broker-order-inspection.md`

## Files inspected

- `cmd/submit_state.go`
- `cmd/submit_state_test.go`
- `docs/manual-reconciliation-runbook.md`
- `docs/live-submit-safety-runbook.md`
- `docs/broker-order-inspection.md`
- `docs/mi/README.md`

## Findings

- `submit-state clear` already had the right safety boundary: it required an identity and typed confirmation unless `--yes` was provided.
- The command UX did not explicitly frame clear as post-verification cleanup strongly enough.
- The runbooks already warned that clear does not confirm broker outcome, but the operator handoff needed a clearer “verify broker truth first, then clear local state” message.

## Direct answers / conclusions

- Phase 8A should stay inside the existing `submit-state clear` command and associated runbooks.
- The safest improvement is wording only: better help text, clearer confirmation/success output, and aligned runbook guidance.
- No new command is needed.
- No JSON change is needed.

## Proposed surgical fix

- Tighten `submit-state clear` help text and human-readable output so it explicitly describes post-verification local cleanup.
- Add runbook guidance that tells operators to verify broker truth manually before clearing.
- Keep behavior read-only until the explicit clear action is invoked and do not introduce any new automation.

## Files changed

- `cmd/submit_state.go`
- `cmd/submit_state_test.go`
- `docs/manual-reconciliation-runbook.md`
- `docs/live-submit-safety-runbook.md`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-19-phase8a-submit-state-clear-workflow.md`

## Validation status

Passed.

Validation commands:

- `gofmt -w cmd/submit_state.go cmd/submit_state_test.go`
- `go build ./...`
- `go vet ./...`
- `go test ./...`

Validation result:

- all commands passed

## Current status / next steps

- Phase 8A implementation is complete.
- Keep the clear workflow manual, explicit, and non-automatic.
