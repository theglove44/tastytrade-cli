# MI-2026-03-15 Phase 3C Duplicate Submit Protection

## Trigger

Add a minimal fail-closed duplicate-submit protection layer so the same approved live order intent cannot be transmitted multiple times accidentally.

## Scope inspected

- `cmd/orders.go`
- `cmd/submit_idempotency.go`
- `cmd/submit_idempotency_test.go`
- `cmd/pre_submit_policy.go`
- tastytrade reference docs in `/Users/christaylor/Projects/tastytrade-docs`

## Reference docs cross-checked

- `order_management.md`
  - live submit transmits the approved order payload to `POST /accounts/{account_number}/orders`
- `order_submission.md`
  - approved payload semantics matter at transmit time and should not be retransmitted casually

## Commands run

```bash
gofmt -w cmd/orders.go cmd/submit_idempotency.go cmd/submit_idempotency_test.go
go build ./...
go vet ./...
go test ./...
```

## Findings

- Phase 3C Task 1 already added a fail-closed pre-submit policy boundary.
- The thinnest duplicate-submit hardening is an in-process identity registry keyed by the approved live submit state:
  - account context
  - intent ID
  - canonical order payload hash
- Minimal local persistence is now justified for restart safety semantics, but still no distributed coordination is introduced.
- On submit transport failure, safest minimal behavior is to leave the identity in `in_flight` state so retries deny by default rather than risking duplicate broker transmission.

## Change applied

Added a minimal duplicate-submit protection layer:

- `cmd/submit_idempotency.go`
  - derives a stable submit identity from account + intent ID + canonical order hash
  - reserves identity as `in_flight` before transmit
  - marks identity `submitted` after successful transmit
  - denies duplicate attempts when the same identity is already `in_flight` or `submitted`
  - fail-closes on nil/unknown/mismatched state

Structured deny reasons include:

- `duplicate_submit_unknown_state`
- `duplicate_submit_in_flight`
- `duplicate_submit_already_submitted`
- `duplicate_submit_state_mismatch`

## Files changed

- `cmd/orders.go`
- `cmd/submit_idempotency.go`
- `cmd/submit_idempotency_test.go`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-15-phase3c-duplicate-submit-protection.md`

## Validation status

- `gofmt -w cmd/orders.go cmd/submit_idempotency.go cmd/submit_idempotency_test.go` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

## Duplicate-submit invariants added

The live-submit boundary now guarantees:

- the same approved submit identity cannot be transmitted twice in-process
- a prior `in_flight` identity denies retried transmit attempts
- a prior `submitted` identity denies retried transmit attempts
- nil, unknown, or mismatched idempotency state denies by default
- duplicate protection is bound to account context, intent ID, and canonical order payload hash

## Current status / next step

Phase 3C Task 2 adds a minimal fail-closed duplicate-submit barrier without adding new execution features.

Next useful step:

- if desired later, persist duplicate-submit state across process restarts, but only once there is a clear operational need and explicit recovery semantics for in-flight uncertainty.
