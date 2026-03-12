# MI-2026-03-12 Phase 3A Reconciler Import Review

## Trigger

Requested a surgical review of the imported Phase 3A zip with expected changed files only:

- `internal/reconciler/reconciler.go`
- `internal/reconciler/reconciler_test.go`
- `internal/client/metrics.go`
- `config/config.go`
- `cmd/root.go`

Goal:

- review only
- avoid wholesale overwrite
- preserve current working auth/accounts/positions behavior
- selectively merge Phase 3A reconciler functionality if safe

## Import source

Reviewed zip:

- `~/Downloads/tastytrade-cli-phase3a.zip`

Extracted temp root used for inspection:

- `/tmp/tastytrade-phase3a.DWAn20/tastytrade-cli`

## Junk paths found

Brace-expansion artifacts found in the zip and ignored:

- `tastytrade-cli/{cmd,internal/`
- `tastytrade-cli/{cmd,internal/{client,streamer,models,store,keychain,web},config,doc}/`

Also ignored non-source artifact:

- `cmd.test`

## Expected-file diff summary

### `internal/reconciler/reconciler.go`

New file in Phase 3A.

Adds a periodic REST position reconciler that:

- fetches canonical positions from `Exchange.Positions()`
- patches missing MarkBook entries
- corrects zero/stale `AvgOpenPrice`
- writes reconciliation snapshots to the store
- removes positions only after N consecutive absent REST passes
- avoids overwriting newer in-memory data
- leaves MarkBook unchanged on REST failure

### `internal/reconciler/reconciler_test.go`

New file in Phase 3A.

Adds coverage for:

- add-missing position behavior
- zero-cost-basis correction
- no-op when data is already correct
- absence threshold handling
- reset-on-reappearance logic
- REST failure isolation
- store snapshot persistence
- store write failure isolation
- lifecycle / Start cancellation
- close-price fallback
- no unnecessary reloads
- mixed multi-position reconciliation

### `internal/client/metrics.go`

Phase 3A adds reconciler metrics:

- `ReconcileRunsTotal`
- `ReconcileErrorsTotal`
- `ReconcilePositionsCorrected`

No auth-related metrics logic was changed.

### `config/config.go`

Phase 3A import adds reconciler config fields and environment parsing:

- `ReconcileInterval`
- `ReconcileAbsenceThreshold`
- `TASTYTRADE_RECONCILE_INTERVAL`
- `TASTYTRADE_RECONCILE_ABSENCE_THRESHOLD`

However, the import as-is also regressed local account behavior by removing:

- `DefaultAccountID = "5WW46136"`
- fallback `AccountID: envOr("TASTYTRADE_ACCOUNT_ID", DefaultAccountID)`

So `config/config.go` from the zip was **not safe to copy as-is**.

### `cmd/root.go`

Phase 3A import adds reconciler startup into the existing Phase 2 runtime wiring:

- imports `internal/reconciler`
- extends WaitGroup accounting
- starts the reconciler goroutine when `cfg.AccountID != ""`
- passes:
  - exchange
  - store
  - MarkBook
  - account ID
  - reconciler interval config
  - absence threshold config

It does **not** remove existing event-bus consumers, market streamer startup, or account streamer startup.

## Root wiring regression check

Specifically reviewed `cmd/root.go` for regressions against existing Phase 1/2 runtime wiring.

### Confirmed preserved

These remained intact after the selective merge:

- config load
- authenticated client construction
- exchange construction
- metrics server startup
- store open / close behavior
- `--no-streamer` short-circuit path
- event-bus construction
- market streamer startup
- consumer goroutines:
  - order consumer
  - balance consumer
  - position consumer
  - quote consumer
- account streamer startup
- shutdown ordering via WaitGroup and store close

### Added

Only the reconciler startup block plus WaitGroup accounting for it.

Conclusion:

- no existing Phase 1/2 runtime wiring was regressed in `cmd/root.go`

## Config/auth/account regression check

Specifically reviewed `config/config.go` for auth/account regressions.

### Safe reconciler additions

Added safely:

- `ReconcileInterval`
- `ReconcileAbsenceThreshold`
- env parsing for both

### Preserved local working behavior

Kept intact:

- production default base URL behavior
- existing auth-related config handling
- local default account behavior:
  - `DefaultAccountID = "5WW46136"`
  - fallback to that account when `TASTYTRADE_ACCOUNT_ID` is unset

Conclusion:

- the import's config file was **not safe as-is**
- a manual selective merge was required
- auth/account config behavior was preserved in the merged result

## Plain-English explanation of “do not overwrite newer data”

The reconciler fetches positions from REST, but streamer events may still arrive while that REST call is in flight.

The rule means:

- if a position in memory was updated after the reconciler started its REST fetch,
- the reconciler should not blindly replace that newer in-memory state with older REST-derived state.

In plain English:

> If live streamer data updated a position more recently than the REST snapshot being processed, keep the newer live version and do not roll it back.

Implementation approach used by Phase 3A:

- record `fetchedAt` before the REST call
- compare `PositionLoadedAt` in the MarkBook to `fetchedAt`
- only update entries that were loaded before the REST fetch started
- special-case: if the in-memory value is still zero cost basis, patching is allowed

## Safety assessment

### Was the Phase 3A patch safe to apply as-is?

No — not file-for-file as extracted.

Reason:

- `config/config.go` from the import would have regressed local account-default behavior

### Was a selective merge safe?

Yes.

A selective reconciliation was applied by:

- importing the two new reconciler files
- adding reconciler metrics
- wiring reconciler startup into `cmd/root.go`
- manually merging only the reconciler config additions into `config/config.go` while preserving the local account default fallback

## Exact safe copy commands from the import

These Phase 3A files were safe to copy directly from the import:

```bash
SRC=/tmp/tastytrade-phase3a.DWAn20/tastytrade-cli

mkdir -p internal/reconciler
cp "$SRC/internal/reconciler/reconciler.go" internal/reconciler/reconciler.go
cp "$SRC/internal/reconciler/reconciler_test.go" internal/reconciler/reconciler_test.go
```

The remaining two imported files were **not copied wholesale**:

- `internal/client/metrics.go`
- `config/config.go`
- `cmd/root.go`

They were merged surgically to preserve current working auth/account behavior.

## Files actually changed in the safe merge

- `internal/reconciler/reconciler.go`
- `internal/reconciler/reconciler_test.go`
- `internal/client/metrics.go`
- `config/config.go`
- `cmd/root.go`

## Validation

Ran:

```bash
gofmt -w internal/reconciler/reconciler.go internal/reconciler/reconciler_test.go internal/client/metrics.go config/config.go cmd/root.go
go mod tidy
go build ./...
go vet ./...
go test ./...
```

Results:

- `gofmt` ✅
- `go mod tidy` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

Notable passing packages:

- `cmd`
- `internal/client`
- `internal/reconciler`
- `internal/streamer`
- `internal/models`
- `internal/store`

## Final conclusion

Phase 3A merged cleanly **after selective reconciliation**, not as a blind file copy.

### Preserved working behavior

These should remain intact because their local logic was preserved:

- prod auth parity
- refresh-token persistence behavior
- accounts list parsing fix
- current accounts/positions behavior
- default account fallback to `5WW46136`

### Added functionality

- periodic REST reconciliation of positions
- safe zero-cost-basis correction
- absence-threshold-based removal
- reconciliation metrics
- reconciler lifecycle integrated into root runtime wiring
