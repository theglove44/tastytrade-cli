# MI-2026-03-19 Phase 8B Pre-Clear Local Submit-State Target Visibility

## Trigger / symptom

Operators needed a more focused read-only way to inspect the specific local submit-state record about to be cleared.

Phase 8A made the manual clear workflow explicit, but the pre-clear local target itself was still only visible as part of the full `submit-state inspect` listing.

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
- `sed -n '1,220p' cmd/submit_state_test.go`
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

- `submit-state inspect` already exists as the local read surface, so the smallest safe improvement is to add a target filter rather than a new command.
- A targeted `--identity` filter keeps the workflow read-only, manual, and operator-controlled.
- The clear workflow can then direct operators to inspect the exact target locally before clearing it.

## Direct answers / conclusions

- Phase 8B should reuse `submit-state inspect` and add an optional identity filter.
- This improves confidence before manual use of `submit-state clear` without changing broker behavior or introducing automation.
- No new command is needed.
- No JSON churn is needed.

## Proposed surgical fix

- Add a minimal `submit-state inspect --identity <submit-identity>` filter.
- Make the human-readable output clearly say when a target identity is being inspected or when no matching local record exists.
- Update the runbooks so they tell operators to inspect the target locally before clearing it.
- Keep the explicit clear workflow manual and non-automatic.

## Files changed

- `cmd/submit_state.go`
- `cmd/submit_state_test.go`
- `docs/manual-reconciliation-runbook.md`
- `docs/live-submit-safety-runbook.md`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-19-phase8b-submit-state-target-visibility.md`

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

- Phase 8B implementation is complete.
- Keep the workflow read-only and operator-controlled.
