# MI-2026-03-15 Phase 4B Local vs Broker Order Comparison

## Trigger

Start Phase 4B with a minimal read-only comparison workflow so operators can compare local persisted submit safety state with broker-visible order state during manual troubleshooting.

## Scope inspected

- `cmd/submit_state.go`
- `cmd/submit_idempotency.go`
- `cmd/broker_orders.go`
- `cmd/pre_submit_policy.go`
- Phase 4A broker inspection docs
- Phase 3C submit-state inspection docs

## Short plan before broad edits

Smallest viable Phase 4B slice:

- keep all Phase 3C safety behavior unchanged
- keep comparison read-only and advisory only
- reuse existing persisted submit-state inspection machinery
- reuse existing broker order client paths (`live` + `recent`)
- add a thin operator command surface under `submit-state`
- classify only a few deterministic comparison outcomes and document caveats clearly

## Files changed

- `cmd/submit_state_compare.go`
- `cmd/submit_state_compare_test.go`
- `docs/local-vs-broker-order-comparison.md`
- `docs/README.md`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-15-phase4b-local-vs-broker-order-comparison.md`

## What was added

New command:

- `tt submit-state compare --limit N`

Comparison sources:

- local persisted submit safety records
- broker live/open orders
- broker recent orders

Current advisory outcomes:

- `local_present_broker_match`
- `local_uncertain_no_broker_match`
- `broker_order_no_local_state`
- `ambiguous`

## Findings

- The existing persisted local state stores a local `order_hash` but broker-visible orders do not expose that exact local fingerprint.
- The thinnest aligned comparison is therefore a broker-side derived comparable fingerprint built from broker-visible order fields.
- This supports plausible exact-match comparison while still requiring clear documentation that non-matches can be false negatives.
- Keeping the command under `submit-state` preserves the operator mental model that local persisted safety state remains primary and broker comparison is advisory.

## Validation commands

```bash
gofmt -w cmd/submit_state_compare.go cmd/submit_state_compare_test.go
go build ./...
go vet ./...
go test ./...
```

## Validation outcome

Validation passed.

## Out of scope

- automatic reconciliation
- local state mutation
- broker mutation
- cancel/replace
- broader execution work
- any change to Phase 3C safety behavior

## Follow-on recommendation

If operators find this comparison useful, the next safe follow-on would be a slightly richer advisory summary grouped by account and outcome counts, still without introducing reconciliation or automated clearing behavior.
