# Broker Order Inspection

Phase 4A adds a minimal read-only broker-facing inspection workflow so operators can inspect tastytrade order state from the CLI before any reconciliation or broader execution capability is introduced.

## Scope

This phase is strictly read-only.

Included:

- recent broker order inspection
- live/open broker order inspection
- concise operator-friendly output
- stable JSON output via `--json`

Not included:

- order mutation
- cancel/replace
- automatic broker reconciliation
- Phase 3C safety behavior changes

## Commands

### Live/open broker orders

```bash
tt broker-orders live
```

JSON:

```bash
tt broker-orders live --json
```

This uses the current live/open orders endpoint already wired in the repo.

## Recent broker orders

```bash
tt broker-orders recent --limit 10
```

JSON:

```bash
tt broker-orders recent --limit 10 --json
```

This uses the broker/API search-orders path in descending order and returns the requested recent slice.

## Output shape

The inspection commands surface the most relevant broker-facing fields currently shaped in the CLI:

- order ID
- status
- order type
- time in force
- price / price effect
- received / updated timestamps when available
- filled / cancelled timestamps when available
- order legs

## Limits / caveats

- `broker-orders live` reflects the tastytrade "live orders" endpoint semantics, which may include orders created or updated today and is not strictly limited to only currently working orders.
- `broker-orders recent` is a thin read-only broker inspection slice, not a reconciliation workflow.
- The CLI does not yet compare broker order state with local submit safety state automatically.
- No broker-side recovery or retry logic is introduced here.

## Relationship to Phase 3C safety

These commands are inspection-only and do not change:

- live trading guard
- `CheckOrderSafety()`
- decision gating
- operator confirmation
- pre-submit policy evaluation
- duplicate/idempotency protection
- restart recovery semantics
- submit-state inspection/reset workflow
