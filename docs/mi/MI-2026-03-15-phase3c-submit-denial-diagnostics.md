# MI-2026-03-15 Phase 3C Submit Denial Diagnostics

## Trigger

Improve operator-facing visibility for final live-submit denials without changing execution behavior.

## Scope inspected

- `cmd/orders.go`
- `cmd/pre_submit_policy.go`
- `cmd/decision_gate_test.go`
- existing final pre-submit policy flow

## Commands run

```bash
gofmt -w cmd/orders.go cmd/pre_submit_policy.go cmd/decision_gate_test.go
go build ./...
go vet ./...
go test ./...
```

## Findings

- Final pre-submit policy denials already returned structured reason codes, but CLI output at the submit boundary was still terse.
- Manual live-path testing benefits from a compact deterministic summary that exposes the final-boundary decision context directly in stdout.
- This can be added as a rendering layer only, without changing policy logic or execution behavior.

## Change applied

Added compact operator-facing denial rendering for final pre-submit policy denials.

Surfaced fields:

- allow/deny outcome
- primary machine-readable deny reason
- intent ID
- whether approved payload hash matched current payload
- approval age / freshness state
- confirmation age / freshness state
- duplicate/idempotency state when available (or `not_checked` before duplicate check)

Example:

```text
LIVE SUBMIT DENIED
  outcome=deny primary_reason=transport_not_approved intent_id=<intent>
  payload_hash_matched=true
  approval_age=0s approval_freshness=fresh
  confirmation_age=0s confirmation_freshness=fresh
  duplicate_state=not_checked
```

## Files changed

- `cmd/orders.go`
- `cmd/pre_submit_policy.go`
- `cmd/decision_gate_test.go`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-15-phase3c-submit-denial-diagnostics.md`
- `docs/mi/MI-2026-03-15-phase3c-pre-submit-policy-hardening.md`

## Validation status

- `gofmt -w cmd/orders.go cmd/pre_submit_policy.go cmd/decision_gate_test.go` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

## Diagnostic fields surfaced

The CLI now surfaces, on final pre-submit denial:

- `outcome`
- `primary_reason`
- `intent_id`
- `payload_hash_matched`
- `approval_age`
- `approval_freshness`
- `confirmation_age`
- `confirmation_freshness`
- `duplicate_state`

## Current status / next step

This is a visibility-only hardening pass; policy behavior remains unchanged.

Next useful step:

- if desired later, emit the same compact summary in JSON form behind a debug-only flag for automated live-path harnesses, without changing normal CLI behavior.
