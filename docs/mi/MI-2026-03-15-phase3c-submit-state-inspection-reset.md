# MI-2026-03-15 Phase 3C Submit State Inspection and Reset

## Trigger

Add a minimal operator-only workflow to inspect and explicitly clear persisted duplicate-submit / restart-recovery state after manual verification.

## Scope inspected

- `cmd/submit_state.go`
- `cmd/submit_state_test.go`
- `cmd/submit_idempotency.go`
- persisted duplicate-submit / restart-recovery state handling

## Commands run

```bash
gofmt -w cmd/submit_state.go cmd/submit_state_test.go cmd/submit_idempotency.go
go build ./...
go vet ./...
go test ./...
```

## Findings

- Restart recovery protections now persist local submit identity state, but operators lacked a supported way to inspect and explicitly clear it after manual verification.
- The thinnest safe addition is:
  - read-only inspection command
  - explicit reset command with strong confirmation
- Reset must clearly state that it only clears local safety state and does not confirm broker outcome.

## Change applied

Added operator-only commands:

- `tt submit-state inspect`
- `tt submit-state clear --identity <submit-identity>`

Inspection surfaces, where available:

- submit identity
- account context
- intent ID
- canonical payload hash
- persisted state
- created/updated timestamps
- deny reason context if present

Reset behavior:

- requires explicit confirmation (`clear`) unless `--yes` is supplied
- only clears local persisted safety state
- does not confirm broker outcome or reconcile broker-side orders
- fail-closes on missing, invalid, or ambiguous state

## Files changed

- `cmd/submit_state.go`
- `cmd/submit_state_test.go`
- `cmd/submit_idempotency.go`
- `cmd/root.go`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-15-phase3c-submit-state-inspection-reset.md`

## Validation status

- `gofmt -w cmd/submit_state.go cmd/submit_state_test.go cmd/submit_idempotency.go` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

## Inspection/reset invariants added

The operator workflow now guarantees:

- persisted live submit state can be inspected without altering execution state
- persisted state can only be cleared explicitly by operator action
- reset is strongly confirmed before local state is removed
- reset only affects local safety state and never implies broker success/failure
- invalid or ambiguous persisted state is handled fail-closed

## Current status / next step

Phase 3C Task 6 adds operator-only inspection/reset without changing live execution behavior.

Next useful step:

- if desired later, add filtering or a small summary count view for large persisted state sets, but keep it read-only and local-only.
