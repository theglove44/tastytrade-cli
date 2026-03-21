# MI-2026-03-20 Phase 8C Pre-Clear Confirmation Context Tightening

## Trigger / symptom

The explicit `submit-state clear` workflow from Phase 8A was already safe, but the final confirmation step could still be more specific about the exact local target being removed.

Phase 8B added read-only target visibility; Phase 8C tightens the confirmation step itself so the operator sees the exact `submit_identity` during the final clear action.

## Scope inspected

- `cmd/submit_state.go`
- `cmd/submit_state_test.go`
- `docs/manual-reconciliation-runbook.md`
- `docs/live-submit-safety-runbook.md`
- `docs/mi/README.md`

## Commands run

- `git branch --show-current`
- `git status --short`
- `sed -n '1,220p' cmd/submit_state.go`
- `sed -n '1,260p' cmd/submit_state_test.go`
- `sed -n '1,260p' docs/manual-reconciliation-runbook.md`
- `sed -n '1,260p' docs/live-submit-safety-runbook.md`
- `sed -n '1,220p' docs/mi/README.md`

## Files inspected

- `cmd/submit_state.go`
- `cmd/submit_state_test.go`
- `docs/manual-reconciliation-runbook.md`
- `docs/live-submit-safety-runbook.md`
- `docs/mi/README.md`

## Findings

- The clear workflow already required an identity and explicit confirmation.
- The target identity was visible, but the confirmation prompt itself could be more explicit about which local record was about to be cleared.
- The phase can remain wording-only and stay inside the existing clear path.

## Direct answers / conclusions

- Phase 8C should refine the confirmation prompt and success message inside `submit-state clear`.
- The prompt should name the exact `submit_identity` being cleared and reinforce that this is post-verification local cleanup.
- No new command is needed.
- No JSON change is needed.

## Proposed surgical fix

- Update the clear confirmation prompt to reference the exact `submit_identity`.
- Update the clear success message to echo the exact local target removed.
- Align the runbooks with the new confirmation wording.
- Keep the workflow manual and operator-controlled.

## Files changed

- `cmd/submit_state.go`
- `cmd/submit_state_test.go`
- `docs/manual-reconciliation-runbook.md`
- `docs/live-submit-safety-runbook.md`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-20-phase8c-submit-state-clear-confirmation-context.md`

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

- Phase 8C implementation is complete.
- Keep the clear workflow explicit, manual, and non-automatic.
