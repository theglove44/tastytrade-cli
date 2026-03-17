# Local vs Broker Order Comparison

Phase 4B adds a minimal read-only comparison workflow for manual troubleshooting.

The goal is simple: compare local persisted submit safety state from Phase 3C against broker-visible order state from Phase 4A, then surface concise advisory outcomes for operators.

## Scope

Included:

- read-only comparison of local persisted submit state against broker-visible order state
- advisory/manual comparison outcomes
- concise human-readable output
- stable JSON output with `--json`

Not included:

- automatic reconciliation
- mutation of local persisted submit state
- broker-side mutation
- cancel/replace
- execution automation
- any change to Phase 3C submit safety behavior

## Command

```bash
tt submit-state compare --limit 25
```

JSON:

```bash
tt submit-state compare --limit 25 --json
```

## What it compares

For the selected account, the CLI:

1. reads local persisted submit safety state
2. fetches broker-visible order state from:
   - live/open broker orders
   - recent broker orders
3. derives a comparable broker-side fingerprint from broker-visible order fields when possible
4. compares that fingerprint to the persisted local `order_hash`

## Advisory outcomes

The minimal comparison flow currently surfaces these labeled outcomes:

- `local_present_broker_match`
  - local persisted state exists and one plausible broker-visible match was found
- `local_uncertain_no_broker_match`
  - local `in_flight` state exists but no exact broker-visible match was found in the current broker inspection scope
- `broker_order_no_local_state`
  - a broker-visible order was found but there was no exact local persisted `order_hash` match for the selected account
- `ambiguous`
  - the comparison could not be classified cleanly, for example because multiple broker orders shared the same comparable fingerprint, multiple local records shared the same `order_hash`, or a broker order could not be converted into a comparable fingerprint

## What the comparison can conclude

This command can help operators answer questions such as:

- "Do I have a local persisted submit record that plausibly lines up with a broker-visible order?"
- "Do I still have local in-flight safety state with no exact broker-visible match in the current inspection window?"
- "Is there a broker-visible order that does not line up with any exact local persisted fingerprint for this account?"

## What the comparison cannot conclude

This command is **advisory/manual only**.

It cannot by itself prove:

- that a broker order definitely originated from a specific local submit attempt
- that no broker order exists outside the current broker inspection scope
- that a broker-visible order with no local match is truly unrelated
- that a local persisted record with no broker match definitely failed
- that local state should be cleared

## Important caveat about matching

Local persisted submit safety state stores a local canonical `order_hash` from the original submit payload.

Broker-visible orders do not expose that local hash directly, so the CLI derives a comparable broker-side fingerprint from broker-visible order fields when possible. This means:

- exact matches are only **plausible** matches
- non-matches can still be false negatives if broker-visible formatting differs from the original local payload representation
- operators must still use manual judgment before clearing local submit safety state

## Relationship to existing workflows

Use this alongside:

- `tt submit-state inspect`
- `tt broker-orders live`
- `tt broker-orders recent --limit N`

The comparison command does not replace those inspections; it only provides a thin read-only advisory layer across them.
