# MI-2026-03-22 Phase 8E Submit-State Clear Help/Discoverability Alignment

## Trigger / symptom

The `submit-state` command surface already had safe manual clear behavior, but the intended inspect-before-clear workflow was still not obvious enough from the command help itself.

Phase 8E aligns help text, examples, and adjacent runbook guidance so the inspect/clear relationship is clearer without changing behavior.

## Scope inspected

- `cmd/submit_state.go`
- `cmd/submit_state_test.go`
- `docs/manual-reconciliation-runbook.md`
- `docs/live-submit-safety-runbook.md`
- `docs/mi/README.md`

## Commands run

- `git branch --show-current`
- `git status --short`
- `sed -n '1,240p' cmd/submit_state.go`
- `sed -n '1,260p' cmd/submit_state_test.go`
- `sed -n '1,220p' docs/manual-reconciliation-runbook.md`
- `sed -n '1,220p' docs/live-submit-safety-runbook.md`
- `sed -n '1,220p' docs/mi/README.md`

## Files inspected

- `cmd/submit_state.go`
- `cmd/submit_state_test.go`
- `docs/manual-reconciliation-runbook.md`
- `docs/live-submit-safety-runbook.md`
- `docs/mi/README.md`

## Findings

- `submit-state inspect` and `submit-state clear` already supported the right behavior.
- The help surface needed explicit examples to make the manual workflow obvious from the command itself.
- The runbooks needed only small alignment so they matched the discoverability language in help.

## Direct answers / conclusions

- Phase 8E should stay within command help, examples, and adjacent docs.
- The smallest safe improvement is discoverability-only.
- No new command is needed.
- No JSON change is needed.

## Proposed surgical fix

- Add examples to `submit-state`, `submit-state inspect`, and `submit-state clear`.
- Keep the help text explicit about inspect-before-clear and post-verification cleanup.
- Ensure the runbooks and MI index describe the same workflow.
- Keep all behavior unchanged.

## Files changed

- `cmd/submit_state.go`
- `cmd/submit_state_test.go`
- `docs/manual-reconciliation-runbook.md`
- `docs/live-submit-safety-runbook.md`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-22-phase8e-submit-state-clear-help-alignment.md`

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

- Phase 8E implementation is complete.
- Keep the workflow manual and operator-controlled.
