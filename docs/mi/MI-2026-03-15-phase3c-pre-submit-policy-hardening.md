# MI-2026-03-15 Phase 3C Pre-Submit Policy Hardening

## Trigger

Add a final fail-closed pre-submit execution policy layer so live broker transmission can only occur when all required live safety conditions are satisfied at the final boundary.

## Scope inspected

- `cmd/orders.go`
- `cmd/pre_submit_policy.go`
- `cmd/pre_submit_policy_test.go`
- `cmd/decision_gate.go`
- `internal/client/client.go`
- `config/config.go`
- tastytrade reference docs in `/Users/christaylor/Projects/tastytrade-docs`

## Reference docs cross-checked

- `order_management.md`
  - live submit uses `POST /accounts/{account_number}/orders`
  - submit shares payload requirements with dry-run
- `order_submission.md`
  - accepted live orders move into the broker order lifecycle with payload-level validation

## Commands run

```bash
gofmt -w cmd/orders.go cmd/pre_submit_policy.go cmd/pre_submit_policy_test.go cmd/decision_gate_test.go
go build ./...
go vet ./...
go test ./...
```

## Findings

- Existing live submit already enforced live-trading mode, order safety checks, decision gating, and operator confirmation, but the final transmit boundary still relied on those checks being scattered across the submit flow.
- Phase 3C needs one explicit fail-closed boundary that validates the complete live-submit state immediately before `ex.Submit(...)`.
- The safest minimal approach is a dedicated evaluator over captured pre-submit state, with structured deny reasons suitable for logs and tests.

## Change applied

Added `EvaluatePreSubmitPolicy(...)` in `cmd/pre_submit_policy.go`.

It now fail-closes live submit unless all of the following are true at the final boundary:

- live trading explicitly enabled
- production/live context is valid
- account ID is present and matches confirmed context
- transport is explicitly approved for live submit
- safety check result succeeded
- decision gate is available and not blocked
- approval state exists and is fresh
- explicit confirmation exists, is acknowledged, and is fresh
- confirmation is bound to the same approved intent ID
- confirmation is bound to the same canonical order payload hash
- current order payload still matches the approved payload hash

Structured deny reasons include, for example:

- `live_trading_disabled`
- `invalid_live_context`
- `transport_not_approved`
- `safety_check_failed`
- `decision_gate_unavailable`
- `decision_gate_denied`
- `confirmation_missing`
- `intent_mismatch`
- `payload_mismatch`
- `unknown_state`

## Files changed

- `cmd/orders.go`
- `cmd/pre_submit_policy.go`
- `cmd/pre_submit_policy_test.go`
- `cmd/decision_gate_test.go`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-15-phase3c-pre-submit-policy-hardening.md`

## Validation status

- `gofmt -w cmd/orders.go cmd/pre_submit_policy.go cmd/pre_submit_policy_test.go cmd/decision_gate_test.go` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

## Safety invariants added

The final live-submit boundary now guarantees:

- no live transmission without explicit live mode
- no live transmission through an unapproved/mock transport
- no live transmission without a successful safety check
- no live transmission without a successful decision gate result
- no live transmission without explicit confirmation
- no live transmission if confirmation and intent diverge
- no live transmission if the approved order payload changes before transmit
- unknown or missing pre-submit state denies by default

## Current status / next step

Phase 3C Task 1 is now a fail-closed hardening pass, not a new execution feature.

Next useful step:

- add a small operator-facing status surface for the final pre-submit policy result if you want pre-transmit denials to be easier to distinguish from earlier gate/safety denials during manual testing.
