# Broker Order Inspection

Phase 4A adds a minimal read-only broker-facing inspection workflow so operators can inspect tastytrade order state from the CLI before any reconciliation or broader execution capability is introduced.

For the full consolidated operator workflow, see `manual-reconciliation-runbook.md`.

## Scope

This phase is strictly read-only.

Included:

- one broker order detail inspection by canonical broker order ID
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

### One broker order in detail

```bash
tt broker-orders detail --id <broker-order-id>
```

JSON:

```bash
tt broker-orders detail --id <broker-order-id> --json
```

This uses the canonical broker order `id` already surfaced in broker order listings and documented by the tastytrade order-management APIs.
It is not intended to accept local submit identities or other local-only identifiers.

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
- account number
- status
- order type
- time in force
- price / price effect
- received / updated timestamps when available
- filled / cancelled timestamps when available
- order legs

## Human-readable detail rendering

Phase 6B keeps the same single-order fetch path and JSON shape, but makes the default text rendering easier to scan.

The detail view now groups high-signal fields into small sections such as:

- order
- pricing
- timestamps
- legs
- per-leg fill context when the current shaped order data already includes it

Behavior is intentionally conservative:

- absent timestamps are omitted cleanly
- absent instrument descriptors are omitted cleanly
- per-leg fill context is shown only when the current shaped order data already contains it
- legs are rendered in a compact numbered form
- the command still does not expose broker status history or raw broker payloads

## Limits / caveats

- `broker-orders detail` is a thin read-only single-order lookup by canonical broker order `id`; it is not a status-history explorer.
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
