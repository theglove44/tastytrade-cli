# MI-2026-03-15 Live Submit Operator Confirmation

## Trigger

Add an explicit operator confirmation / acknowledgement layer to the minimal live-submit path so `tt submit` is harder to trigger casually, while preserving existing live-trading, safety, and reconcile-policy gates.

## Scope inspected

- `cmd/orders.go`
- `cmd/decision_gate_test.go`
- existing live-submit flow
- tastytrade reference docs in `/Users/christaylor/Projects/tastytrade-docs`

## Reference docs cross-checked

- `order_management.md`
  - live submit and dry-run share the same payload requirements
- `order_submission.md`
  - order structure and acceptance semantics

## Commands run

```bash
gofmt -w cmd/orders.go cmd/decision_gate_test.go
go build ./...
go vet ./...
go test ./...
```

## Findings

- Existing `tt submit` already had the right safety stack:
  - `cfg.LiveTrading`
  - `CheckOrderSafety()`
  - Phase 3B decision gate
- The thinnest additional safety layer is:
  - human mode: explicit typed confirmation prompt
  - JSON mode: explicit `--yes` acknowledgement flag
- This avoids redesigning the command while making accidental live submission less likely.

## Change applied

Added a thin confirmation layer to `tt submit`:

- human-readable mode:
  - prints a concise `LIVE ORDER SUBMISSION` summary
  - shows order type, time in force, price/effect when present, and legs
  - requires the operator to type `submit`
  - any other response aborts safely
- JSON mode:
  - no interactive prompt
  - requires `--yes`
  - missing `--yes` returns a deterministic error

Existing degraded-confidence warnings remain visible before confirmation.

## Tests added / updated

- interactive decline aborts safely
- interactive accept proceeds
- JSON mode requires `--yes`
- JSON mode with `--yes` proceeds
- existing decision-gate warn/block behavior remains intact

## Validation status

- `gofmt -w cmd/orders.go cmd/decision_gate_test.go` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

## Current status / next step

`tt submit` now requires an explicit operator acknowledgement in both human and non-interactive contexts without broadening into orchestration.

Next useful step:

- if desired, add a very small config-level safeguard for production submit defaults (for example, refusing submit when `--verbose`/dev settings are active), but keep it separate from execution workflow design.
