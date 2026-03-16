# MI-2026-03-15 Phase 3C Restart Recovery Semantics

## Trigger

Define a fail-closed recovery policy for live submit attempts left in uncertain or in-flight state across process interruption or restart.

## Scope inspected

- `cmd/submit_idempotency.go`
- `cmd/submit_idempotency_test.go`
- `cmd/orders.go`
- Phase 3C duplicate-submit protection and final submit boundary

## Commands run

```bash
gofmt -w cmd/submit_idempotency.go cmd/submit_idempotency_test.go cmd/orders.go
go build ./...
go vet ./...
go test ./...
```

## Findings

- Phase 3C duplicate-submit protection was previously in-process only.
- Across restart, an interrupted live submit could leave broker state uncertain, and automatic re-submit must fail closed.
- The thinnest aligned extension is a persisted local identity registry with explicit deny semantics for:
  - prior in-flight state
  - unknown/invalid persisted state
- No automatic reconciliation or retry logic is needed yet.

## Change applied

Added persisted duplicate-submit restart semantics:

- submit identity registry now loads/saves local state under the user config directory
- a persisted prior `in_flight` identity for the same account + payload hash denies automatic retry after restart
- invalid/unknown persisted state denies by default
- clean persisted state allows normal submit flow

Structured deny reasons added / used:

- `duplicate_submit_restart_in_flight`
- `duplicate_submit_restart_unknown`

Operator-visible CLI message now explains when prior submit state is uncertain/in-flight and that manual inspection is required before retry.

## Files changed

- `cmd/submit_idempotency.go`
- `cmd/submit_idempotency_test.go`
- `cmd/orders.go`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-15-phase3c-restart-recovery-semantics.md`
- `docs/mi/MI-2026-03-15-phase3c-duplicate-submit-protection.md`

## Validation status

- `gofmt -w cmd/submit_idempotency.go cmd/submit_idempotency_test.go cmd/orders.go` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

## Restart recovery invariants added

The live-submit boundary now guarantees:

- no automatic resubmit when prior persisted state for the same account/payload is still `in_flight`
- no automatic resubmit when persisted duplicate-submit state is invalid or unknown
- clean restart state allows normal submit flow
- restart ambiguity denies by default and requires manual inspection before retry

## Current status / next step

Phase 3C Task 5 adds fail-closed restart recovery semantics without adding broker reconciliation or retry automation.

Next useful step:

- if needed later, add an operator-only inspection/reset tool for persisted in-flight identities, but only after defining explicit manual verification procedures.
