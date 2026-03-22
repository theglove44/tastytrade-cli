# MI-2026-03-22 Phase 8F Submit-State Clear No-Op Guidance

## Trigger

Phase 8F targets the `submit-state clear` no-op path so it is immediately obvious
when no matching local submit state exists to clear, and what the operator should
check next using read-only commands.

## Scope inspected

- `submit-state clear` operator-facing no-op output
- uncertain-state clear output
- inspect-before-clear runbook wording
- clear result wording in the live submit safety guide

## Commands run

```bash
git status --short --branch
sed -n '1,260p' cmd/submit_state.go
sed -n '1,260p' cmd/submit_state_test.go
sed -n '1,220p' docs/manual-reconciliation-runbook.md
sed -n '1,220p' docs/live-submit-safety-runbook.md
sed -n '1,220p' docs/mi/README.md
rg -n "nothing was cleared|No persisted live submit state record|submit-state clear|inspect --identity|explicit post-verification" docs/manual-reconciliation-runbook.md docs/live-submit-safety-runbook.md
rg -n "DuplicateSubmitUnknownState|DuplicateSubmitRestartUnknown|clear\\(" cmd internal
sed -n '240,320p' cmd/submit_idempotency.go
```

## Files inspected

- `cmd/submit_state.go`
- `cmd/submit_state_test.go`
- `cmd/submit_idempotency.go`
- `docs/manual-reconciliation-runbook.md`
- `docs/live-submit-safety-runbook.md`
- `docs/mi/README.md`

## Findings

- The clear command already distinguished between:
  - no matching local submit state
  - uncertain persisted submit state
- The existing no-op message was technically correct but did not tell the operator
  what to inspect next.
- The clear command already had the right manual workflow boundary; the missing
  piece was explicit retry guidance after a no-op.

## Direct answers / conclusions

- The smallest safe Phase 8F slice is a wording-only refinement of the no-op and
  uncertain-state clear outputs.
- The operator should be pointed back to `tt submit-state inspect --identity <submit-identity>`
  and, where relevant, manual broker verification before retrying clear.

## Proposed surgical fix

- Update `printSubmitStateClearOutcome` to add a short read-only next-step line
  for no-op and uncertain-state clear failures.
- Add/adjust tests for the missing-target and uncertain-state clear outputs.
- Update the manual reconciliation and live submit safety runbooks so they match
  the new guidance.

## Files changed

- `cmd/submit_state.go`
- `cmd/submit_state_test.go`
- `docs/manual-reconciliation-runbook.md`
- `docs/live-submit-safety-runbook.md`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-22-phase8f-submit-state-clear-noop-guidance.md`

## Validation status

- `gofmt -w cmd/submit_state.go cmd/submit_state_test.go` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

## Current status / next steps

- Phase 8F implementation is complete.
- The no-op path now points operators back to `tt submit-state inspect --identity <submit-identity>`
  and manual broker verification when the clear command finds nothing or uncertain persisted state.
