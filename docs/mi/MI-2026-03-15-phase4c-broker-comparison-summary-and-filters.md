# MI-2026-03-15 Phase 4C Broker Comparison Summary and Filters

## Trigger

Start Phase 4C by extending the existing advisory local-vs-broker comparison with concise summary counts and light filtering so operators can narrow troubleshooting output without introducing reconciliation or mutation behavior.

## Scope inspected

- `cmd/submit_state_compare.go`
- `cmd/submit_state_compare_test.go`
- `cmd/submit_state.go`
- `docs/local-vs-broker-order-comparison.md`

## Short plan before broad edits

Smallest viable Phase 4C slice:

- keep the existing `tt submit-state compare` command surface
- add deterministic summary counts by outcome
- add small optional filters only:
  - `--account`
  - `--outcome`
  - existing `--limit`
- keep JSON output stable by extending the existing comparison schema rather than creating a new command
- preserve read-only advisory behavior only

## Files changed

- `cmd/submit_state_compare.go`
- `cmd/submit_state_compare_test.go`
- `cmd/submit_state_compare_phase4c_test.go`
- `cmd/submit_state_test.go`
- `docs/local-vs-broker-order-comparison.md`
- `docs/mi/README.md`
- `docs/mi/MI-2026-03-15-phase4c-broker-comparison-summary-and-filters.md`

## What was added

Extended `tt submit-state compare` with:

- deterministic summary counts by outcome
- `--account` single-account override
- `--outcome` result filter
- existing `--limit` retained as the broker recent-order scope limiter

## Findings

- The existing comparison command was already the correct minimal surface; adding a second command was unnecessary.
- A deterministic ordered summary array fits current `--json` patterns better than a map.
- Applying the outcome filter before rendering keeps the human and JSON views aligned and predictable.
- Keeping account selection single-account avoids drifting toward multi-account reporting or reconciliation.

## Validation commands

```bash
gofmt -w cmd/submit_state_compare.go cmd/submit_state_compare_test.go cmd/submit_state_compare_phase4c_test.go cmd/submit_state_test.go
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
- multi-account reporting
- any Phase 3C safety behavior changes

## Follow-on recommendation

If operators still need more visibility later, the next safest step would be a tiny outcome-focused summary-only mode or exported report format, still read-only and still without reconciliation semantics.
