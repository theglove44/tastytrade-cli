# MI-2026-03-13 Last Quote Metric Stuck at Zero

## Trigger

Observed during live market hours:

- market streamer connected and stable
- tracked symbols > 0
- reconciler status = ok
- `tastytrade_last_quote_unix_seconds` remained 0

Goal:

- trace the exact metric update path
- determine whether quote events were arriving
- identify the smallest correct fix if a code bug exists

## Docs used as source of truth

Primary references read from `~/Projects/tastytrade-docs`:

- `streaming_market_data.md`
- `api_guides_instruments.md`

Key doc statement:

> To receive live market event data via DXLink, clients must convert symbols into a format that meets DxLink's requirements. For convenience, we provide these symbols via a field called `streamer-symbol`.

Equity option example from docs:

- raw OCC symbol: `SPY 230706C00370000`
- DXLink streamer symbol: `.SPY230706C370`

## Exact metric update path

`tastytrade_last_quote_unix_seconds` is updated in two places:

1. `internal/streamer/market.go`
   - `dispatchLoop(...)`
   - calls `m.touchLastEvent()`
   - `touchLastEvent()` sets `client.Metrics.LastQuoteTime`
2. `cmd/root.go`
   - `quoteConsumer(...)`
   - calls `client.Metrics.LastQuoteTime.SetToCurrentTime()` after applying a `QuoteEvent`

So the metric moves off zero only if:

- a websocket message is received
- it is a `FEED_DATA` message on the data channel
- `decodeFeedData(...)` produces one or more `models.QuoteEvent`
- at least one `QuoteEvent` reaches dispatch / quote-consumer handling

## Live market ingestion path traced

Code path:

1. `internal/streamer/market.go`
   - `receiveLoop(...)` reads websocket messages
2. message must satisfy:
   - `peek.Type == "FEED_DATA"`
   - `peek.Channel == dxlinkDataChannel`
3. `decodeFeedData(raw)` converts payload to `[]models.QuoteEvent`
4. decoded quote events are sent to `dispatchCh`
5. `dispatchLoop(...)`:
   - calls `touchLastEvent()`
   - increments `QuotesReceived`
   - calls `QuoteHandler.OnQuote(q)`
6. `cmd/quote_handler.go`
   - publishes to `quoteBus`
7. `cmd/root.go` `quoteConsumer(...)`
   - applies quote
   - increments quote metrics again
   - sets `LastQuoteTime.SetToCurrentTime()`

Conclusion:

- if `tastytrade_last_quote_unix_seconds` stays 0, no `QuoteEvent` is reaching dispatch/consumer logic

## Root cause found

The runtime subscription symbols were not in DXLink streamer-symbol format.

Evidence:

Current watch logs showed market streamer subscribing symbols like:

- `SPY   260417P00650000`
- `SPY   260417P00645000`
- `SMH   260417P00370000`
- `SMH   260417P00375000`

Those are raw tastytrade/OCC-style option symbols, not DXLink streamer symbols.

Per docs, DXLink subscriptions must use `streamer-symbol` format, e.g.:

- raw: `SPY 230706C00370000`
- streamer: `.SPY230706C370`

Because the market streamer was subscribing raw OCC/account symbols instead of DXLink symbols, the most likely runtime result is:

- websocket connected successfully
- subscription message sent successfully
- **no matching quote events arrived for those subscribed symbols**
- therefore `decodeFeedData(...)` never produced live `QuoteEvent`s for them
- therefore `tastytrade_last_quote_unix_seconds` remained 0

## What this means about event arrival

Most likely truth state:

- no usable live quote events were arriving for the subscribed symbols
- not because the websocket was down
- not because the metric setter was broken
- but because subscriptions used the wrong symbol format for DXLink

## Smallest safe fix applied

Added a narrow symbol normalization helper for the obvious/evidence-backed case:

- raw **equity option OCC symbol** → DXLink streamer symbol

New file:

- `cmd/market_data_symbol.go`

Example conversion:

- `SPY   260417P00650000` → `.SPY260417P650`

Applied this normalization in the two subscription entry points:

1. startup seed path in `seedMarkBookFromREST(...)`
2. runtime position-open/change subscription path in `positionConsumer(...)`

Non-equity-option symbols are left unchanged.

This keeps the fix scoped to the quote-subscription path.

## Notes / limitations

This fix is intentionally narrow and focused on restoring live quote ingestion for the observed option-symbol case.

It does **not** add a full instrument lookup pipeline using `streamer-symbol` from instrument endpoints for every asset class.

That broader symbol-alias harmonization is a follow-up item.

## Files inspected

- `internal/streamer/market.go`
- `internal/models/models.go`
- `cmd/quote_handler.go`
- `cmd/root.go`
- docs:
  - `~/Projects/tastytrade-docs/streaming_market_data.md`
  - `~/Projects/tastytrade-docs/api_guides_instruments.md`
- runtime evidence:
  - `/tmp/tt-watch-followup2.log`

## Files changed

- `cmd/market_data_symbol.go`
- `cmd/market_data_symbol_test.go`
- `cmd/root.go`

## Validation

Ran:

```bash
gofmt -w cmd/market_data_symbol.go cmd/market_data_symbol_test.go cmd/root.go
go build ./...
go vet ./...
go test ./...
```

Results:

- `gofmt` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

## Runtime confirmation status

Code-path diagnosis and patch are complete.

Direct live-hour runtime confirmation that `tastytrade_last_quote_unix_seconds` now moves off 0 should be performed in a manual watch run, because that depends on live market conditions and symbol activity.

## Follow-up recommendation

Next useful step:

- add a proper symbol-alias layer or instrument-driven `streamer-symbol` resolution so:
  - all asset classes subscribe with canonical DXLink symbols
  - incoming quote symbols can be mapped cleanly back to internal/position keys where needed
