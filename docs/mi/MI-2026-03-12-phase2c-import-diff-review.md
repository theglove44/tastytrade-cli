# MI-2026-03-12 Phase 2C Import Diff Review

## Trigger

Requested a diff review of:

- current project root
- `/Users/christaylor/Downloads/phase2c-import/tastytrade-cli`

Goal:

- summarize what changed
- identify notable additions/removals/regressions
- record findings without applying the import blindly

## Commands run

```bash
diff -qr . /Users/christaylor/Downloads/phase2c-import/tastytrade-cli || true
find /Users/christaylor/Downloads/phase2c-import/tastytrade-cli -maxdepth 2 -mindepth 1 | sort
for f in ...; do git diff --no-index --stat ...; done
for f in ...; do git diff --no-index --unified=40 ...; done
find /Users/christaylor/Downloads/phase2c-import/tastytrade-cli/internal/bus -maxdepth 2 -type f | sort
```

## High-level summary

The Phase 2C import is **not** a clean superset of the current repo.

It contains:

- new event-bus architecture work
- market streamer watchdog changes
- some command/root wiring changes
- but also several regressions against fixes already made locally, especially in auth and account parsing

So this import should **not** be copied over wholesale.

## Non-project / junk artifacts in import

Found junk / non-source artifacts:

- `cmd.test`
- brace-expansion artifact path:
  - `{cmd,internal`
  - `{cmd,internal/{client,streamer,models,store,keychain,web},config,doc}`

These should be ignored.

## Files only in current project

These are local-only additions not present in the Phase 2C import:

- `.pi/`
- `docs/`
- `cmd/account_resolver.go`
- `cmd/account_resolver_test.go`
- `cmd/login_test.go`
- `internal/client/auth_test.go`
- local built binary: `tastytrade-cli`

Meaning: the import does not include the recent MI logging setup, account resolver work, or recent auth persistence tests.

## Files only in Phase 2C import

New in import:

- `internal/bus/bus.go`
- `internal/bus/bus_test.go`
- `internal/streamer/market_watchdog_test.go`
- `internal/exchange/tastytrade/exchange_test.go`

These indicate the main substantive Phase 2C feature area is:

- brokered event-bus decoupling
- market streamer stale-connection watchdog testing

## Changed files vs current repo

Diff reported changes in:

- `cmd/handler.go`
- `cmd/handler_test.go`
- `cmd/login.go`
- `cmd/positions.go`
- `cmd/quote_handler.go`
- `cmd/root.go`
- `config/config.go`
- `internal/client/auth.go`
- `internal/client/metrics.go`
- `internal/exchange/tastytrade/tastytrade.go`
- `internal/keychain/keychain.go`
- `internal/models/models.go`
- `internal/store/store.go`
- `internal/streamer/account.go`
- `internal/streamer/market.go`
- `internal/streamer/streamer_test.go`

## Meaningful change clusters

### 1. New event-bus architecture in Phase 2C import

The biggest architectural change is around event handling.

Phase 2C import introduces:

- `internal/bus/`
- `cmd/handler.go` changes from direct side-effect handler to publisher-only adapter
- `cmd/quote_handler.go` changes from direct quote application to publisher-only adapter
- `cmd/root.go` appears to add consumer goroutines such as:
  - order consumer
  - balance consumer
  - position consumer
  - quote consumer

Interpretation:

- current repo does side effects directly in handlers
- Phase 2C import decouples streamer ingest from side effects via brokers/channels

This is a real feature/architecture delta, not a minor patch.

### 2. Market streamer stale-watchdog work in Phase 2C import

`internal/streamer/market.go` in the import adds:

- stale quote timeout logic
- `isStale(...)`
- `staleWatchdog(...)`
- per-connection context cancellation for forced reconnects
- new watchdog test file

Interpretation:

- import has stronger reconnect/staleness handling for DXLink market streaming

### 3. Root wiring expanded significantly in Phase 2C import

`cmd/root.go` has a large diff.

Observed from diff snippets:

- new consumer goroutines are wired in root
- startup orchestration is more complex
- event side effects appear moved out of handlers and into dedicated consumers

Interpretation:

- this likely depends on the new `internal/bus` package
- copying root changes without the full intended set would be risky

## Regressions present in the Phase 2C import

These are the most important findings.

### Regression A: auth request bodies reintroduce `client_id`

Import reintroduces `client_id` in both:

- `cmd/login.go`
- `internal/client/auth.go`

That is a regression versus current working auth parity, because local fixes already changed refresh-token exchange to match the Python SDK:

```json
{
  "grant_type": "refresh_token",
  "client_secret": "...",
  "refresh_token": "..."
}
```

Phase 2C import would revert that and risk bringing back:

- `invalid_grant`
- `client secret mismatch`

### Regression B: login refresh-token persistence fallback is lost

Current repo has the fix:

- if login succeeds and server omits `refresh_token`, store the refresh token entered by the user

Phase 2C import removes that helper and goes back to:

- warning only
- no stored refresh token when response omits one

That would reintroduce the runtime failure:

- `cannot load refresh_token from keychain: secret not found in keyring`

### Regression C: accounts list parser fix is missing

Current repo added:

- `AccountListItem`
- accounts parser flattening from `data.items[].account`

Phase 2C import removes that model change in `internal/models/models.go`.

Given the earlier bug, that strongly suggests the import does **not** contain the accounts list shape fix.

### Regression D: local account defaulting / account resolution work is absent

Current repo includes:

- `cmd/account_resolver.go`
- default account override in `config/config.go`

Phase 2C import does not include these.

It also differs in:

- `cmd/positions.go`
- `config/config.go`

So importing those files blindly would likely undo working account selection behavior.

## File-by-file concise summary

### `cmd/handler.go`
- import replaces direct side-effect handler with event-bus publisher model
- substantial architectural rewrite

### `cmd/quote_handler.go`
- import replaces MarkBook/metrics side effects with quote bus publishing
- depends on root consumer wiring and `internal/bus`

### `cmd/root.go`
- large orchestration expansion
- appears to add consumer goroutines and move business side effects here
- likely central Phase 2C wiring file

### `internal/streamer/market.go`
- import adds stale-connection watchdog / reconnect logic
- likely beneficial but non-trivial

### `internal/client/auth.go`
- import regresses auth parity by reintroducing `client_id`
- import also drops local testability hooks and local persistence helper structure

### `cmd/login.go`
- import regresses auth parity by reintroducing `client_id`
- import loses local fallback that stores entered refresh token when response omits one

### `internal/models/models.go`
- import removes local `AccountListItem` wrapper fix
- risk: accounts list bug reappears if copied blindly

### `config/config.go`
- import lacks local hard-coded default account override
- would likely change current working account behavior

### `cmd/positions.go`
- differs from current repo; likely does not include the local account resolver path

### `internal/client/metrics.go`
- import modifies metrics wiring, probably to support bus/watchdog behavior

### `internal/keychain/keychain.go`
- changed, but no critical new local finding from the quick diff beyond drift

### `internal/store/store.go`, `internal/streamer/account.go`, `internal/streamer/streamer_test.go`, `cmd/handler_test.go`
- all show supporting drift around the Phase 2C architecture move

## Direct conclusions

### Is the Phase 2C import safe to copy wholesale?

No.

### Why not?

Because it mixes:

- useful new Phase 2C architecture work
- with regressions against already-fixed local auth and account behavior

### Most important regressions to protect against

Do **not** overwrite the current versions of:

- `cmd/login.go`
- `internal/client/auth.go`
- `internal/models/models.go`
- `config/config.go`
- `cmd/positions.go`

without manually reconciling them.

## Recommended next step

If you want to import Phase 2C safely, do it as a **selective reconciliation pass**, not a full copy.

Best candidate files/features to review next:

- `internal/bus/`
- `cmd/handler.go`
- `cmd/quote_handler.go`
- `cmd/root.go`
- `internal/streamer/market.go`
- `internal/streamer/market_watchdog_test.go`

while preserving local fixes in:

- auth flow
- refresh-token persistence
- accounts parsing
- account selection/defaulting

## Selective reconciliation applied

A selective Phase 2C reconciliation was applied.

Imported / merged architecture files only:

- `internal/bus/bus.go`
- `internal/bus/bus_test.go`
- `cmd/handler.go`
- `cmd/quote_handler.go`
- `cmd/root.go`
- `internal/streamer/market.go`
- `internal/streamer/market_watchdog_test.go`

Additionally patched to support the new bus wiring:

- `internal/client/metrics.go`
  - added `BusDroppedEvents`

Deliberately preserved local working versions of:

- `cmd/login.go`
- `internal/client/auth.go`
- `internal/models/models.go`
- `config/config.go`
- `cmd/positions.go`
- `cmd/account_resolver.go`
- `cmd/account_resolver_test.go`
- `cmd/login_test.go`
- `internal/client/auth_test.go`
- local docs / `.pi` additions

## Concise merge summary

### Merged from Phase 2C

- event-bus package and tests
- publisher-only handlers for account + quote events
- root wiring for:
  - order consumer
  - balance consumer
  - position consumer
  - quote consumer
- market streamer stale-watchdog logic and tests
- bus drop metrics wiring

### Explicitly not imported

To preserve current working behavior, these Phase 2C regressions were not accepted:

- auth request-body reintroduction of `client_id`
- loss of login refresh-token persistence fallback
- removal of accounts list parser fix
- removal of current account resolver / default-account behavior

## Validation status

Ran:

```bash
gofmt -w internal/bus/bus.go internal/bus/bus_test.go cmd/handler.go cmd/quote_handler.go cmd/root.go internal/streamer/market.go internal/streamer/market_watchdog_test.go internal/client/metrics.go
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

## Current conclusion

The Phase 2C event-bus architecture and market watchdog are now merged into the repo without overwriting the local working auth/account fixes.

### Expected preserved behavior after merge

These should remain intact because their local implementations were preserved:

- production auth parity with Python SDK refresh flow
- refresh-token persistence fallback on login
- runtime refresh-token preservation / rotation behavior
- accounts list parsing fix for `data.items[].account`
- current account resolver/default-account behavior for accounts/positions flow
- local docs / MI / `.pi` additions

### Practical impact

- prod auth should remain intact
- `accounts --no-streamer` should remain intact
- `positions --no-streamer` should remain intact
- streamer-enabled paths now use the Phase 2C bus architecture and market stale-watchdog logic
