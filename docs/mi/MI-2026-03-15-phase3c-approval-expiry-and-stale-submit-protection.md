# MI-2026-03-15 Phase 3C Approval Expiry and Stale-Submit Protection

## Trigger

Add a fail-closed freshness policy so previously approved live order intents cannot be transmitted after approval or operator confirmation has gone stale.

## Scope inspected

- `cmd/orders.go`
- `cmd/pre_submit_policy.go`
- `cmd/pre_submit_policy_test.go`
- Phase 3C submit safety boundary
- tastytrade reference docs in `/Users/christaylor/Projects/tastytrade-docs`

## Reference docs cross-checked

- `order_management.md`
  - live submit transmits the approved order payload to `/accounts/{account_number}/orders`
- `order_submission.md`
  - order acceptance depends on current payload validity at transmit time

## Commands run

```bash
gofmt -w cmd/orders.go cmd/pre_submit_policy.go cmd/pre_submit_policy_test.go
go build ./...
go vet ./...
go test ./...
```

## Findings

- Phase 3C Task 1 already enforced final-boundary state integrity, but not freshness.
- The thinnest fail-closed freshness extension is to timestamp:
  - approval creation
  - operator confirmation
- Those timestamps should be evaluated only at the final pre-submit boundary, with explicit deny reasons for missing, expired, or invalid time state.

## Change applied

Extended the approved live submit state with freshness metadata:

- `SubmitApproval.ApprovedAt`
- `SubmitConfirmation.ConfirmedAt`
- final policy input `ApprovedAt` and `Now`

Added freshness enforcement in `EvaluatePreSubmitPolicy(...)`:

- deny if approval timestamp is missing
- deny if approval is expired
- deny if confirmation timestamp is missing
- deny if confirmation is expired
- deny if time metadata is zero, future-dated, inconsistent, or otherwise invalid/unknown

Structured deny reasons added / used:

- `approval_missing`
- `approval_expired`
- `confirmation_missing`
- `confirmation_expired`
- `time_state_invalid`

## Files changed

- `cmd/orders.go`
- `cmd/pre_submit_policy.go`
- `cmd/pre_submit_policy_test.go`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-15-phase3c-approval-expiry-and-stale-submit-protection.md`
- `docs/mi/MI-2026-03-15-phase3c-pre-submit-policy-hardening.md`

## Validation status

- `gofmt -w cmd/orders.go cmd/pre_submit_policy.go cmd/pre_submit_policy_test.go` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

## Freshness invariants added

The live-submit boundary now guarantees:

- no live transmit without a recorded approval timestamp
- no live transmit once approval freshness expires
- no live transmit without a recorded confirmation timestamp
- no live transmit once confirmation freshness expires
- no live transmit when time metadata is missing, zero, future-dated, inconsistent, or otherwise invalid
- ambiguity in freshness state denies by default

## Current status / next step

Phase 3C Task 3 keeps submit safety fail-closed without adding repricing, refresh, or retry workflows.

Next useful step:

- if desired later, surface approval/confirmation age in manual operator logs so stale-submit denials are easier to interpret during live-path testing.
