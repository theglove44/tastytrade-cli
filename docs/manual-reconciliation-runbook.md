# Manual Reconciliation Runbook

This is the primary operator/developer runbook for the current manual reconciliation workflow in `tastytrade-cli`.

It consolidates the existing read-only surfaces added across Phases 4A/4B/4C and 5A:

- local persisted submit-state inspection
- broker live/recent order inspection
- local-vs-broker advisory comparison
- comparison summaries and filters
- recommended next actions per comparison outcome

This runbook is intentionally **manual** and **read-only**.

It does **not** add or authorize:

- automatic reconciliation
- broker mutation
- cancel/replace
- automatic local-state clearing
- any Phase 3C safety behavior change

## When to inspect local submit state

Inspect local persisted submit safety state when:

- a prior submit attempt may have been interrupted or is uncertain
- submit is denied because prior state is uncertain or in-flight
- you need to understand whether the CLI still holds local duplicate-submit / restart-recovery state

Command:

```bash
tt submit-state inspect
```

JSON:

```bash
tt submit-state inspect --json
```

This tells you only what local persisted safety state exists.
It does **not** confirm broker truth.

## When to inspect broker live/recent orders

Inspect broker-visible order state when:

- you need to see what the broker currently exposes for the account
- you want to check whether a plausible broker-visible order exists for a local submit attempt
- you need a broader recent slice in addition to the broker "live orders" surface

Commands:

```bash
tt broker-orders live
tt broker-orders recent --limit 10
```

JSON:

```bash
tt broker-orders live --json
tt broker-orders recent --limit 10 --json
```

Important caveat:

- `broker-orders live` reflects the tastytrade live-orders endpoint semantics and may include orders created or updated today, not only strictly working orders.

## How to use local-vs-broker comparison

Use comparison when you want one advisory view that combines:

- local persisted submit safety state
- broker-visible live orders
- broker-visible recent orders

Command:

```bash
tt submit-state compare --limit 25
```

JSON:

```bash
tt submit-state compare --limit 25 --json
```

Helpful filters:

```bash
tt submit-state compare --account ACCT-1 --limit 25
tt submit-state compare --account ACCT-1 --outcome ambiguous --limit 25
tt submit-state compare --account ACCT-1 --outcome local_present_broker_match --limit 25 --json
```

## How to interpret summaries and filters

The comparison output includes deterministic summary counts by outcome.

Current outcomes:

- `local_present_broker_match`
- `local_uncertain_no_broker_match`
- `broker_order_no_local_state`
- `ambiguous`

Guidance for interpreting them:

- use the summary section to quickly see the shape of the current advisory result set
- use `--outcome` to narrow troubleshooting to one class of result
- use `--account` to force a specific account selection
- use `--limit` to widen or narrow the recent broker-order slice feeding the comparison

Important caveat:

- summary counts reflect the currently returned comparison result set after any outcome filter is applied

## How to use recommended next actions safely

Each comparison result may include recommended next actions.
These are intentionally advisory/manual only.

Use them as a checklist for what to inspect next, not as proof that the broker or local state is authoritative.

### Outcome: `local_present_broker_match`

Interpretation:

- a local persisted record plausibly lines up with one broker-visible order

Recommended use:

- inspect the broker order details and current status manually
- if `submit-state compare` already surfaces `broker_order_id`, use `tt broker-orders detail --id <broker-order-id>`
- keep local safety state until manual verification is complete
- only consider explicit local-state clearing later if still needed

### Outcome: `local_uncertain_no_broker_match`

Interpretation:

- local in-flight/uncertain state exists but no exact broker-visible match was found in the current inspection scope

Recommended use:

- re-check broker visibility using both live and recent broker-order inspection
- if `submit-state compare` prints `next_step=...`, run the suggested `tt broker-orders live` and `tt broker-orders recent --limit N` checks
- continue treating local state as uncertain until manually verified
- do not retry or clear local state automatically

### Outcome: `broker_order_no_local_state`

Interpretation:

- a broker-visible order exists without an exact local persisted match in the current comparison

Recommended use:

- inspect broker order details and account activity manually
- if `submit-state compare` already surfaces `broker_order_id`, use `tt broker-orders detail --id <broker-order-id>`
- confirm whether the order is expected before taking local action
- do not infer that local state should be created or cleared automatically

### Outcome: `ambiguous`

Interpretation:

- the advisory comparison could not classify the situation cleanly

Recommended use:

- inspect both local submit-state records and broker order details manually
- if `submit-state compare` already surfaces `broker_order_id`, use `tt broker-orders detail --id <broker-order-id>`
- narrow the display with `--account`, `--outcome`, and `--limit` if helpful
- do not clear local state or assume broker truth from an ambiguous result alone

## What the CLI can and cannot conclude

The current CLI can help you answer questions such as:

- "Is there local persisted submit safety state for this account?"
- "What broker-visible live or recent orders are currently exposed?"
- "Is there a plausible exact-match comparison result?"
- "What manual next checks are recommended for this advisory outcome?"

The current CLI cannot by itself prove:

- that a broker order definitely originated from a specific local submit attempt
- that no broker order exists outside the current inspection scope
- that a non-match means a submit definitely failed
- that a broker-visible order with no local match is definitely unrelated
- that local state should be cleared automatically

## When local state may be cleared

Local state may only be cleared through the already-supported explicit command after manual broker verification:

```bash
tt submit-state clear --identity <submit-identity>
```

Non-interactive acknowledgement:

```bash
tt submit-state clear --identity <submit-identity> --yes
```

Important warning:

- clearing local state does not confirm broker outcome
- clearing local state does not reconcile broker-side orders
- clearing local state should happen only after manual broker verification

Recommended before clearing:

- inspect broker truth manually with `tt broker-orders live`
- inspect a recent broker slice manually with `tt broker-orders recent --limit N`
- only then use `tt submit-state clear --identity <submit-identity>`

## Recommended manual workflow

1. inspect local persisted submit state:
   - `tt submit-state inspect`
2. inspect broker-visible order state:
   - `tt broker-orders live`
   - `tt broker-orders recent --limit N`
3. compare local vs broker state:
   - `tt submit-state compare --limit N`
4. if needed, narrow the advisory result set:
   - `tt submit-state compare --account <account> --outcome <outcome> --limit N`
5. use recommended next actions as a manual checklist
6. only after manual verification, clear local state through the explicit supported command if appropriate
7. keep clear as a local cleanup step only, after broker truth has already been checked

## Related docs

- `live-submit-safety-runbook.md`
- `broker-order-inspection.md`
- `local-vs-broker-order-comparison.md`
