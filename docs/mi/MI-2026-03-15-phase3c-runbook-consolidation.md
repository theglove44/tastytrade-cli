# MI-2026-03-15 Phase 3C Runbook Consolidation

## Trigger

Consolidate the full Phase 3C live submit safety model into a single operator/developer runbook so the fail-closed execution boundary is easy to understand and operate.

## Scope inspected

- `docs/mi/*.md` Phase 3C records
- `cmd/orders.go`
- `cmd/submit_state.go`
- existing submit denial / restart / inspection semantics

## Commands run

```bash
find docs -maxdepth 2 -type f | sort
```

## Findings

- Phase 3C behavior was already documented across multiple MI files but lacked one primary operational runbook.
- The cleanest consolidation is a single primary runbook in `docs/` plus clear index links from `docs/README.md` and `docs/mi/README.md`.
- No code or policy behavior changes were required for consolidation.

## Change applied

Added primary runbook:

- `docs/live-submit-safety-runbook.md`

Added docs index:

- `docs/README.md`

Updated MI index:

- `docs/mi/README.md`

The runbook now covers:

- live trading guard
- `CheckOrderSafety()`
- decision gating
- operator confirmation
- pre-submit policy evaluation
- duplicate/idempotency protection
- approval/confirmation freshness
- denial diagnostics
- restart recovery behavior
- submit-state inspection/reset workflow
- clear distinction between local persisted safety state and broker-side order state
- concise command examples for normal submit, denied submit inspection, state inspect, and state clear

## Validation status

- docs-only change; no Go validation required

## Final Phase 3C safety model summary

The current live submit path is fail-closed at every boundary, persists only local safety state, does not automatically reconcile broker-side truth, and requires explicit operator inspection/reset when prior state is uncertain.
