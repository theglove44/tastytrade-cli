# MI-2026-03-12 Watch Command Import Review

## Trigger

Requested a surgical review of the watch-command import zip with focus on:

- `cmd/watch.go`
- `cmd/root.go`

Goal:

- inspect only the relevant watch-command changes
- identify safety issues before any import
- avoid wholesale overwrite

## Import source

Reviewed zip:

- `~/Projects/_imports/tastytrade-cli-watch.zip`

Extracted temp root used for inspection:

- `/tmp/tastytrade-watch.LnVn3T/tastytrade-cli`

## Junk paths/files to ignore

Ignored non-source and junk artifacts found in the zip:

- `tastytrade-cli/cmd.test`
- `tastytrade-cli/{cmd,internal/`
- `tastytrade-cli/{cmd,internal/{client,streamer,models,store,keychain,web},config,doc}/`

## Files diffed

Compared only:

- `cmd/watch.go`
- `cmd/root.go`

against the current repo.

## Diff summary

### `cmd/watch.go`

New file in the import.

Adds a new long-running Cobra command:

- `tt watch`

Its purpose is to:

- keep streamers, reconciler, and metrics running
- log startup state
- wait for Ctrl+C / context cancel
- block until shutdown completes

Notable implementation details:

- uses a package-level `watchShutdown` channel
- logs a startup banner with account, reconcile interval, absence threshold, metrics URL, and live-trading state
- logs kill-switch state
- blocks on `cmd.Context().Done()`
- then waits up to 15s for `watchShutdown` to be closed

### `cmd/root.go`

Import changes are small and watch-specific:

- adds `tt watch` to command tree
- adds package-level `watchShutdown chan struct{}`
- modifies shutdown flow to close `watchShutdown` after store close
- updates shutdown comments to mention watch command unblocking

No other major runtime rewiring was introduced by the watch import itself.

## Specific checks requested

### 1. Is `watchShutdown` usage safe given Cobra ordering?

Conclusion: **No, not as written.**

Reason:

- Cobra runs `PersistentPreRunE` before `RunE`
- `watchShutdown` is initialised inside `watchCmd.RunE`
- but `cmd/root.go` shutdown logic is installed during `PersistentPreRunE`
- therefore, the watch command comments claiming the channel is initialised before root runtime wiring observes it are incorrect

Why this matters:

- the imported `watch.go` comment says `watchShutdown` is set before `PersistentPreRunE`-started goroutines use it
- that is false under Cobra execution order
- at shutdown time, `watchShutdown` may still work in practice if `RunE` has already set it by then, but the lifecycle reasoning is wrong and fragile
- more importantly, the code is not safe as a design assertion because it depends on `RunE` having run before any shutdown path tries to close it

Safer pattern would be:

- initialise `watchShutdown` before or during pre-run for the watch command, not in `RunE`
- or avoid this shared channel pattern entirely

### 2. Does metrics address logging use the correct env var name?

Conclusion: **No.**

Imported `cmd/watch.go` uses:

- `METRICS_ADDR`

But the actual metrics config uses:

- `TASTYTRADE_METRICS_ADDR`

Confirmed from `internal/metrics/server.go`.

So the startup banner in imported `watch.go` would log the wrong env-var-derived address.

### 3. Is existing auth/accounts/positions/reconciler wiring preserved?

Conclusion: **Mostly yes in `cmd/root.go`, but the patch is still not safe as-is due the watch issues above.**

Confirmed preserved in the imported `cmd/root.go`:

- existing auth client construction
- exchange construction
- metrics server startup
- store open/close flow
- `--no-streamer` behavior
- Phase 2C event-bus wiring
- market streamer startup
- account streamer startup
- reconciler wiring from Phase 3A
- existing accounts/positions/reconciler runtime path structure

So the watch import does not appear to regress:

- auth flow
- accounts behavior
- positions behavior
- reconciler startup wiring

The main problems are specific to:

- `watchShutdown` lifecycle assumptions
- incorrect metrics env var name in `watch.go`

## Safety assessment

### Is the watch patch safe to merge as-is?

**No.**

Reasons:

1. `watchShutdown` lifecycle reasoning is unsafe / incorrect for Cobra order
2. metrics banner uses the wrong env var name (`METRICS_ADDR` instead of `TASTYTRADE_METRICS_ADDR`)

Because of those issues, the patch should not be copied over as-is.

## Copy commands

Not provided, because the patch is **not safe to merge as-is**.

## Validation / apply status

No code was applied from this watch-command zip during this review.

Accordingly, these steps were **not run** for this import:

- `gofmt`
- `go mod tidy`
- `go build ./...`
- `go vet ./...`
- `go test ./...`

No runtime smoke test was run for the imported watch command because it was not applied.

## Final conclusion

The watch-command import is close, but it is **not safe to merge as-is**.

### Good

- adds a useful `tt watch` command concept
- does not appear to disturb existing auth/accounts/positions/reconciler runtime wiring in root

### Not safe as written

- `watchShutdown` setup is not justified by Cobra execution order
- metrics banner reads the wrong env var name

## Corrective watch patch applied

A tiny local corrective patch was implemented instead of copying the import as-is.

### Files changed

- `cmd/watch.go`
- `cmd/root.go`

### Exact fixes made

#### 1. Safe watch shutdown lifecycle for Cobra ordering

Implemented safe pattern:

- `watchShutdown` is now initialised in `cmd/root.go` `PersistentPreRunE` when `cmd.Name() == "watch"`
- non-watch commands explicitly reset it to `nil`
- shutdown logic closes `watchShutdown` only after bus drain / reconciler exit / store close
- early-return path (`--no-streamer` or store unavailable) also closes `watchShutdown` on context cancellation so `watch` cannot hang waiting forever

This avoids relying on `RunE` initialisation before `PersistentPreRunE`.

#### 2. Correct metrics banner source

`cmd/watch.go` now uses the real metrics config path by calling:

- `internal/metrics.Addr(logger)`

This is consistent with:

- `TASTYTRADE_METRICS_ADDR`

and no longer uses the wrong `METRICS_ADDR` variable name.

## Validation after patch

Ran:

```bash
gofmt -w cmd/watch.go cmd/root.go
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

## Runtime smoke test

Built binary and exercised `tt watch` locally.

Observed successful startup signals:

- metrics server started
- store opened
- startup position seeding completed
- watch startup banner logged
- reconciler started
- account streamer connected and subscribed

Observed runtime issue during smoke:

- market streamer repeatedly reconnected with DXLink AUTH_STATE `UNAUTHORIZED`

This appears separate from the watch-command patch itself.

Observed shutdown behavior under external `SIGTERM` in this harness:

- process terminated by signal
- clean shutdown logs were not observed in the captured output

This suggests the binary may still need signal-to-context wiring at process entry for perfect graceful shutdown behavior, but that is outside the two requested corrective watch fixes.

## Current conclusion

The corrective watch patch is now safe for code merge with respect to the two identified issues:

- Cobra-safe watch shutdown channel lifecycle
- correct metrics address source

### Healthy aspects

- command builds
- command is registered
- runtime startup path appears intact
- existing auth/accounts/positions/reconciler wiring was preserved

### Caution from smoke test

- market streamer auth still showed `UNAUTHORIZED` during DXLink connect attempts
- graceful signal-driven shutdown was not fully demonstrated in this harness

So `tt watch` appears safe enough for further live validation of the watch-command wiring itself, but not fully proven end-to-end healthy in all runtime dimensions.
