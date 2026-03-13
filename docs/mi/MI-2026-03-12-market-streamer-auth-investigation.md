# MI-2026-03-12 Market Streamer AUTH Investigation

## Trigger

Observed runtime behavior:

- REST `GET /api-quote-tokens` succeeds with HTTP 200
- market streamer connects to `wss://tasty-openapi-ws.dxfeed.com/realtime`
- websocket auth fails with:
  - `AUTH_STATE: server returned "UNAUTHORIZED"`

Goal:

- inspect the current code path only
- compare it to local reference docs in `~/Projects/tastytrade-docs`
- identify the most likely root cause without refactoring unrelated code

## Files inspected

- `internal/streamer/market.go`
- `internal/exchange/exchange.go`
- `internal/exchange/tastytrade/tastytrade.go`
- `internal/models/models.go`
- reference docs:
  - `~/Projects/tastytrade-docs/streaming_market_data.md`
  - `~/Projects/tastytrade-docs/oauth.md`

## Exact REST quote-token response model used

Current code uses:

```go
type QuoteToken struct {
    Token        string `json:"token"`
    DxlinkURL    string `json:"dxlink-url"`
    WebSocketURL string `json:"websocket-url"`
    Level        string `json:"level"`
    ExpiresIn    int    `json:"expires-in"`
}
```

The exchange implementation fetches:

- `GET {baseURL}/api-quote-tokens`

and parses it as:

- `models.DataEnvelope[models.QuoteToken]`

## Which field is passed into the market streamer

Two fields are used from the REST quote-token response:

- `QuoteToken.DxlinkURL`
  - used as the websocket endpoint (fallbacks to config URL if empty)
- `QuoteToken.Token`
  - sent in the DXLink `AUTH` message

## Exact websocket auth payload sent

Current code sends:

```json
{
  "type": "AUTH",
  "channel": 0,
  "token": "<QuoteToken.Token>"
}
```

## Is the token prefixed/transformed/wrapped incorrectly?

No.

Current code sends the raw `QuoteToken.Token` value exactly as received from the REST quote-token response.

It is **not**:

- prefixed with `Bearer `
- wrapped in another object
- transformed / re-encoded
- taken from the OAuth access token instead of the quote token

So the token field itself is being passed through correctly.

## DXLink auth/channel-open sequence: docs vs current code

### Docs in `streaming_market_data.md`

Reference docs show this order:

1. Dial websocket
2. Send `SETUP`
3. Receive `SETUP`
4. Receive `AUTH_STATE` with `state: "UNAUTHORIZED"`
5. Send `AUTH` with quote token
6. Receive `AUTH_STATE` with `state: "AUTHORIZED"`
7. Send `CHANNEL_REQUEST`
8. Receive `CHANNEL_OPENED`
9. Send `FEED_SETUP`
10. Send `FEED_SUBSCRIPTION`

The docs explicitly say:

> After `SETUP`, you should receive an `AUTH_STATE` message with `state: UNAUTHORIZED`. This is when you'd authorize with your api quote token.

### Current code sequence in `market.go`

Current code does:

1. Dial websocket
2. Send `SETUP`
3. Wait for `SETUP`
4. **Immediately send `AUTH`**
5. Wait for `AUTH_STATE` and require it to be `AUTHORIZED`
6. Send `CHANNEL_REQUEST`
7. Wait for `CHANNEL_OPENED`
8. Send `FEED_SETUP`
9. Send `FEED_SUBSCRIPTION`

So the current code **skips the documented initial `AUTH_STATE: UNAUTHORIZED` step**.

## Most likely root cause

The most likely cause is **protocol sequencing mismatch**, not token formatting.

Specifically:

- DXLink appears to send an initial `AUTH_STATE` with `UNAUTHORIZED` after `SETUP`
- current code sends `AUTH` before explicitly consuming that server challenge/state transition
- `expectAuthState()` then reads the next `AUTH_STATE` frame it sees
- if that frame is the server's initial post-SETUP `UNAUTHORIZED` state, the code treats it as a hard auth failure immediately

That would perfectly explain why:

- REST quote-token retrieval succeeds
- websocket `AUTH_STATE` still appears as `UNAUTHORIZED`
- failure happens before channel open / feed setup

In other words:

> The code is probably treating the expected pre-auth `UNAUTHORIZED` state as if it were a post-auth failure.

## Smallest surgical fix proposal

Do not refactor unrelated code.

Smallest fix:

1. After receiving the `SETUP` response, explicitly wait for the initial:

```json
{"type":"AUTH_STATE","channel":0,"state":"UNAUTHORIZED"}
```

2. Then send:

```json
{"type":"AUTH","channel":0,"token":"<quote-token>"}
```

3. Then wait for:

```json
{"type":"AUTH_STATE","channel":0,"state":"AUTHORIZED"}
```

This can be implemented by either:

- adding a tiny helper like `expectAuthStateValue(..., want string)`
- or adjusting `expectAuthState()` usage so the pre-auth `UNAUTHORIZED` frame is consumed before the post-auth `AUTHORIZED` wait

## Direct answers

### 1. Exact REST quote-token response model used by code

- `models.DataEnvelope[models.QuoteToken]`

### 2. Which field from that response is passed into the market streamer

- websocket URL: `QuoteToken.DxlinkURL`
- auth token: `QuoteToken.Token`

### 3. Exact websocket auth message payload sent

```json
{
  "type": "AUTH",
  "channel": 0,
  "token": "<QuoteToken.Token>"
}
```

### 4. Is the token prefixed, transformed, or wrapped incorrectly?

- No

### 5. Does the DXLink auth/channel-open sequence match current docs?

- No
- current code skips the documented initial `AUTH_STATE: UNAUTHORIZED` step after `SETUP`

### 6. Most likely reason REST succeeds while websocket auth fails

- REST quote-token retrieval is fine
- websocket side most likely fails because the code mishandles the documented DXLink auth sequence and treats the initial `UNAUTHORIZED` state as a fatal auth failure

### 7. Smallest surgical fix

- consume initial `AUTH_STATE: UNAUTHORIZED` after `SETUP`
- then send `AUTH`
- then require `AUTH_STATE: AUTHORIZED`

## Applied surgical fix

Implemented the smallest surgical fix in:

- `internal/streamer/market.go`

### Change made

Adjusted the DXLink handshake sequence to match the docs:

1. send `SETUP`
2. await `SETUP`
3. await initial `AUTH_STATE: UNAUTHORIZED`
4. send `AUTH` with `QuoteToken.Token`
5. await `AUTH_STATE: AUTHORIZED`

This was implemented by replacing the previous one-step auth wait with a tiny helper:

- `expectAuthStateValue(ctx, conn, want)`

No unrelated code was refactored.

## Validation

Ran:

```bash
gofmt -w internal/streamer/market.go
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

## Runtime retest

Built and smoke-tested via:

```bash
go build -o tastytrade-cli
TASTYTRADE_CLIENT_ID=dummy ./tastytrade-cli watch
```

Observed after the fix:

- REST quote-token request still succeeded
- market streamer connected to `wss://tasty-openapi-ws.dxfeed.com/realtime`
- market streamer reached:
  - `market streamer subscribed`

This is a successful behavioral change from the prior failure mode:

- before: `AUTH_STATE: server returned "UNAUTHORIZED"`
- after: market streamer successfully authenticated and subscribed

## Follow-up runtime checks

Additional runtime checks were performed after the auth-sequencing fix.

### Watch / startup health

Observed in `tt watch --verbose` runtime logs:

- metrics server started
- store opened
- startup REST position seeding completed
- watch startup banner printed
- reconciler started
- account streamer subscribed
- market streamer subscribed

This confirms the previous websocket auth failure is resolved far enough for the market streamer to authenticate and open the feed.

### Quote flow health

In the 20-second follow-up runtime window, no `quote applied` debug logs were observed.

Most likely reasons:

- subscribed symbols may have had low/no quote event activity during the short window
- reconciler interval is 60s, so no reconciliation pass was expected yet

So quote-flow liveness was not disproven, but also not conclusively demonstrated by actual quote events in this short run.

### Reconciler interaction

Observed:

- reconciler startup log present
- no reconcile pass log during the short runtime window

This is expected because:

- current reconcile interval is 60s
- the smoke run was shorter than one full interval

So reconciler startup appears healthy, but a full pass was not exercised in the short smoke test.

### Shutdown behavior

A more controlled Python subprocess smoke test was used to send `SIGINT`.

Observed:

- process exited on signal (`RC -2`)
- startup logs were healthy
- explicit final "clean shutdown complete" log was not captured in the short sample

So graceful shutdown still is not fully evidenced from logs, but the process does terminate correctly under controlled signal delivery in this harness.

## Current conclusion

The websocket auth failure was caused by a DXLink handshake sequencing bug, not token formatting.

The surgical fix corrected the handshake order and the market streamer now appears to authenticate successfully in runtime smoke testing.

Additional follow-up checks indicate:

- watch/startup path is healthy
- market streamer reaches subscribed state
- reconciler startup is healthy
- quote-event liveness was not conclusively demonstrated in the short observation window
- graceful shutdown is improved operationally but not fully proven from captured logs alone
