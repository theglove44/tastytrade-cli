# Live Submit Safety Runbook

This is the primary operator/developer runbook for the current live submit safety model in `tastytrade-cli`.

It describes the full fail-closed boundary for:

- `tt submit`
- persisted duplicate-submit / restart-recovery state
- operator inspection/reset workflow

## Scope and intent

This runbook covers the current Phase 3C live submit safety chain only.

It does **not** add or imply:

- broker-side reconciliation automation
- retry automation
- auto-entry or strategy orchestration
- quote refresh / repricing workflows

## Safety chain overview

A live submit only reaches broker transmission after all of the following pass:

1. **Live trading guard**
   - `cfg.LiveTrading` must be true
   - production/live context must be valid
2. **`CheckOrderSafety()`**
   - kill switch
   - circuit breaker
   - future NLQ guard seam remains separate
3. **Decision gating**
   - reconcile-policy decision gate must allow submission
   - degraded states may warn
   - blocked states deny
4. **Operator confirmation**
   - human mode requires typed confirmation
   - `--json` mode requires `--yes`
5. **Pre-submit policy evaluation**
   - final fail-closed integrity check at the live boundary
   - validates account, intent, payload hash, transport, and freshness state
6. **Duplicate/idempotency protection**
   - approved live submit identity is reserved before transmit
   - duplicate in-flight/submitted identities deny by default
7. **Restart recovery semantics**
   - persisted uncertain/in-flight state denies automatic resubmit after restart
8. **Operator inspection/reset workflow**
   - local persisted state can be inspected and explicitly cleared after manual verification

## What the CLI tracks locally vs what it does not

### Local persisted safety state

The CLI persists local submit safety state for duplicate-submit and restart-recovery protection.

This local state can include:

- submit identity
- account context
- intent ID
- canonical order payload hash
- local state such as `in_flight` or `submitted`
- local timestamps used for operator inspection

### Broker-side order state

Broker-side order state is separate.

Examples:

- routed order accepted by tastytrade
- working/live broker order
- filled/canceled/rejected order

### Important distinction

**Clearing local persisted safety state does not confirm broker outcome.**

The CLI currently does **not** automatically reconcile:

- whether the broker actually received the order
- whether the order is working/live
- whether the order filled or was rejected
- whether an interrupted prior submit succeeded remotely

Manual verification is required before retry whenever submit state is uncertain.

## Current deny reasons by layer

### Early deny layers

These can block before final pre-submit policy:

- live trading not enabled
- kill switch active
- circuit breaker denial
- reconcile-policy decision gate denial
- operator confirmation declined or missing `--yes`

### Final pre-submit policy deny reasons

Examples include:

- `live_trading_disabled`
- `invalid_live_context`
- `missing_account_id`
- `account_mismatch`
- `transport_not_approved`
- `safety_check_failed`
- `decision_gate_unavailable`
- `decision_gate_denied`
- `approval_missing`
- `approval_expired`
- `confirmation_missing`
- `confirmation_declined`
- `confirmation_expired`
- `time_state_invalid`
- `intent_mismatch`
- `payload_mismatch`
- `unknown_state`

### Duplicate/restart deny reasons

Examples include:

- `duplicate_submit_in_flight`
- `duplicate_submit_already_submitted`
- `duplicate_submit_state_mismatch`
- `duplicate_submit_restart_in_flight`
- `duplicate_submit_restart_unknown`
- `duplicate_submit_unknown_state`

## Operator-visible denial diagnostics

When final pre-submit policy denies live submission, the CLI prints a compact summary such as:

```text
LIVE SUBMIT DENIED
  outcome=deny primary_reason=transport_not_approved intent_id=<intent>
  payload_hash_matched=true
  approval_age=0s approval_freshness=fresh
  confirmation_age=0s confirmation_freshness=fresh
  duplicate_state=not_checked
```

Diagnostic fields surfaced:

- outcome
- primary reason
- intent ID
- payload hash match status
- approval age / freshness
- confirmation age / freshness
- duplicate/idempotency state when available

## Freshness model

At the final live boundary, the CLI denies stale approval/confirmation state.

Current rules:

- approval timestamp must exist
- approval must still be fresh
- confirmation timestamp must exist
- confirmation must still be fresh
- time metadata must not be zero, future-dated, inconsistent, or unknown

Fail-closed rule:

- ambiguity in freshness state denies by default

## Restart recovery model

If persisted restart state indicates a prior in-flight or uncertain submit, the CLI denies automatic resubmission.

Operator meaning:

- do **not** assume the broker did not receive the prior order
- inspect broker-side order state manually first
- only clear local persisted state after manual verification

## Command examples

### Normal live submit

Human-readable:

```bash
tt submit --file order.json
```

JSON/scripted mode:

```bash
tt submit --file order.json --json --yes
```

## Denied submit inspection

If submit is denied because prior state is uncertain or in-flight, inspect local persisted state:

```bash
tt submit-state inspect
```

If final pre-submit policy denies, use the printed denial summary fields to determine whether the issue is:

- freshness
- account/intent mismatch
- transport/safety gate mismatch
- local duplicate/restart state

## Persisted submit-state inspection

Human-readable:

```bash
tt submit-state inspect
```

Targeted local inspection:

```bash
tt submit-state inspect --identity <submit-identity>
```

JSON:

```bash
tt submit-state inspect --json
```

## Persisted submit-state clear

After manual broker/order verification, clear one local persisted identity as explicit local cleanup:

```bash
tt submit-state clear --identity <submit-identity>
```

Non-interactive acknowledgement:

```bash
tt submit-state clear --identity <submit-identity> --yes
```

Important warning:

- this only clears local duplicate-submit / restart-recovery safety state
- it does not confirm broker outcome
- it should only be used after broker truth has already been checked manually
- it is helpful to verify the target locally first with `tt submit-state inspect --identity <submit-identity>`
- the confirmation step should name the exact `submit_identity` being cleared

### Important reset warning

This only clears local CLI safety state.

It does **not**:

- confirm broker acceptance
- confirm broker rejection
- confirm no order was routed
- reconcile broker-side orders automatically

## Recommended manual operator procedure

When live submit is denied due to uncertain or restart-related state:

1. inspect local state:
   - `tt submit-state inspect`
2. inspect broker-side order/account state manually
3. determine whether the prior order was:
   - not transmitted
   - transmitted and working
   - transmitted and terminal
4. only then clear local persisted state if appropriate:
   - `tt submit-state clear --identity <submit-identity>`
5. retry submit only after manual verification

## Developer notes

When changing submit behavior, preserve these invariants:

- fail closed on missing or ambiguous state
- keep local persisted safety state separate from broker truth
- do not silently retry uncertain prior submits
- keep deny reasons machine-readable and stable
- keep operator diagnostics concise and deterministic

## Related MI records

- `docs/mi/MI-2026-03-15-live-submit-minimal-path.md`
- `docs/mi/MI-2026-03-15-live-submit-operator-confirmation.md`
- `docs/mi/MI-2026-03-15-phase3c-pre-submit-policy-hardening.md`
- `docs/mi/MI-2026-03-15-phase3c-duplicate-submit-protection.md`
- `docs/mi/MI-2026-03-15-phase3c-approval-expiry-and-stale-submit-protection.md`
- `docs/mi/MI-2026-03-15-phase3c-submit-denial-diagnostics.md`
- `docs/mi/MI-2026-03-15-phase3c-restart-recovery-semantics.md`
- `docs/mi/MI-2026-03-15-phase3c-submit-state-inspection-reset.md`
