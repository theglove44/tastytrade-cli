# Auth Fix Review and Verification

## Imported patch location

Reviewed auth-fix zip:

- `~/Projects/_imports/tastytrade-cli-auth-fix.zip`

Extracted temporary repo root used for inspection:

- `/tmp/tastytrade-auth-fix.6lwNZZ/tastytrade-cli`

Confirmed junk brace-expansion artifact paths were present in the zip and ignored:

- `tastytrade-cli/{cmd,internal/`
- `tastytrade-cli/{cmd,internal/{client,streamer,models,store,keychain,web},config,doc}/`

These were not considered for import.

---

## Expected files reviewed

Compared only these files against the working repo:

- `internal/models/models.go`
- `internal/client/auth.go`
- `cmd/login.go`
- `config/config.go`
- `internal/models/auth_test.go`

---

## Concise diff summary

### `internal/models/models.go`

Changed `TokenResponse` to match flat OAuth JSON with underscore keys:

- `access-token` -> `access_token`
- `token-type` -> `token_type`
- `refresh-token` -> `refresh_token`
- `expires-in` -> `expires_in`

Also added comments stating that `/oauth/token` returns:

- a flat JSON object
- underscore-separated keys
- not a `DataEnvelope`

### `internal/client/auth.go`

Changed runtime refresh response parsing from:

- `models.DataEnvelope[models.TokenResponse]`

To:

- direct `models.TokenResponse`

This fixes flat OAuth parsing.

However, the request body still includes `client_id`.

### `cmd/login.go`

Changed login behavior so it no longer silently defaults to cert/sandbox.

If `TASTYTRADE_BASE_URL` is unset, login now prompts for:

- `prod`
- `sandbox`

Also changed login token response parsing from:

- `DataEnvelope[TokenResponse]`

To:

- flat `TokenResponse`

However, the login request body still includes `client_id`.

### `config/config.go`

Changed default base URL from sandbox/cert to production.

Old default:

- `https://api.cert.tastyworks.com`

New default:

- `https://api.tastytrade.com`

### `internal/models/auth_test.go`

New regression tests added for OAuth parsing.

Tests verify:

- underscore keys parse correctly
- dashed keys do not parse
- `DataEnvelope` wrapping is wrong for `/oauth/token`
- missing `refresh_token` remains safe

---

## Exact auth request bodies after patch

### Login path: `cmd/login.go`

The patch sends this JSON body to `POST /oauth/token`:

```json
{
  "grant_type": "refresh_token",
  "client_id": "...",
  "client_secret": "...",
  "refresh_token": "..."
}
```

### Runtime refresh path: `internal/client/auth.go`

The patch sends this JSON body to `POST /oauth/token`:

```json
{
  "grant_type": "refresh_token",
  "client_id": "...",
  "client_secret": "...",
  "refresh_token": "..."
}
```

---

## Direct answers

### Does login still send `client_id`?

Yes.

### Does runtime refresh still send `client_id`?

Yes.

### Does either path still parse OAuth responses through `DataEnvelope`?

No.

### Do both now parse a flat token response with underscore keys?

Yes.

---

## Comparison to known-good Python SDK behavior

Known-good Python SDK behavior uses:

- explicit prod/cert selection
- flat OAuth response
- underscore fields:
  - `access_token`
  - `refresh_token`
  - `token_type`
  - `expires_in`
- refresh-token exchange body without `client_id`

### What now matches

- safer base URL handling
- flat OAuth response parsing
- underscore OAuth fields

### Remaining mismatch

The Go patch still includes `client_id` in both refresh-token exchanges:

- login path
- runtime refresh path

So this patch is a strong partial fix, but not full parity with the Python SDK.

---

## Safety assessment

This patch was safe to apply surgically for the expected files only.

Reasons:

- changes were limited to the expected auth files
- no unrelated refactor was required
- new regression tests were included
- local validation passed cleanly

Important caveat:

- this is not a complete Python-SDK parity fix because `client_id` is still included in both refresh-token request bodies

---

## Exact surgical copy commands used

```bash
SRC=/tmp/tastytrade-auth-fix.6lwNZZ/tastytrade-cli

cp "$SRC/internal/models/models.go" internal/models/models.go
cp "$SRC/internal/client/auth.go" internal/client/auth.go
cp "$SRC/cmd/login.go" cmd/login.go
cp "$SRC/config/config.go" config/config.go
cp "$SRC/internal/models/auth_test.go" internal/models/auth_test.go
```

---

## Validation run after apply

Commands run:

```bash
go mod tidy
go build ./...
go vet ./...
go test ./...
```

Results:

- `go mod tidy` ✅
- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

Notable passing packages:

- `cmd`
- `internal/models`
- `internal/store`
- `internal/streamer`
- `internal/valuation`

---

## Final conclusion

The imported auth patch applied cleanly and validation passed.

It fixes:

1. unsafe environment defaulting
2. incorrect OAuth response parsing via `DataEnvelope`
3. dashed OAuth field tags instead of underscore tags

It does **not** fully match the known-good Python SDK flow yet because:

- `client_id` is still included in both refresh-token exchange request bodies

So the current state is:

- safe to build
- safe to test locally
- much closer to correct auth behavior
- but still has one remaining compatibility mismatch to consider removing next

---

## Suggested next manual checks

Reasonable next commands:

```bash
go build -o tastytrade-cli
./tastytrade-cli login
```

Caveat:

- this caveat has now been addressed: `client_id` was removed from both login and runtime refresh request bodies to match the Python SDK refresh flow.

---

## Follow-up auth parity patch: removed `client_id`

A second surgical auth patch was applied to:

- `cmd/login.go`
- `internal/client/auth.go`

### What changed

Removed `client_id` from the refresh-token POST body in both places.

Final request body shape is now:

```json
{
  "grant_type": "refresh_token",
  "client_secret": "...",
  "refresh_token": "..."
}
```

This now matches the known-good Python SDK refresh flow for:

- login token exchange
- runtime token refresh

### Validation

After that parity patch:

```bash
gofmt -w cmd/login.go internal/client/auth.go
go build ./...
go vet ./...
go test ./...
```

All passed.

---

## Follow-up token persistence fix

After auth parity was fixed, a runtime/login persistence bug was isolated:

- `tt login` could succeed
- `/oauth/token` could return `access_token`, `token_type`, and `expires_in`
- but omit `refresh_token`
- login would warn and leave the keychain without any stored refresh token
- later commands would fail with:
  - `cannot load refresh_token from keychain: secret not found in keyring`

### Files changed

- `cmd/login.go`
- `internal/client/auth.go`
- `cmd/login_test.go`
- `internal/client/auth_test.go`

### Persistence rules after the fix

#### Login flow

After a successful login:

- if `/oauth/token` returns `refresh_token`, store that returned value
- if `/oauth/token` omits `refresh_token`, store the refresh token the user entered during login

This guarantees that successful login does not leave the keychain without a refresh token.

#### Runtime refresh flow

During token refresh:

- if `/oauth/token` returns a new `refresh_token`, persist it
- if `/oauth/token` omits `refresh_token`, preserve the existing stored refresh token
- never overwrite an existing refresh token with an empty value

### Tests added

Added/updated tests to verify:

- login success with missing `refresh_token` in the response still stores the entered refresh token
- runtime refresh with missing `refresh_token` preserves the existing stored token
- runtime refresh with a new `refresh_token` rotates and stores it

### Validation

After the token persistence patch:

```bash
gofmt -w cmd/login.go internal/client/auth.go cmd/login_test.go internal/client/auth_test.go
go build ./...
go vet ./...
go test ./...
```

All passed.

---

## Current auth status

The Go auth flow now matches the known-good Python SDK refresh flow on the key compatibility points:

- explicit/safe prod vs sandbox handling
- flat OAuth token parsing
- underscore OAuth fields:
  - `access_token`
  - `refresh_token`
  - `token_type`
  - `expires_in`
- refresh-token POST body contains only:
  - `grant_type`
  - `client_secret`
  - `refresh_token`
- refresh token persistence is safe when the server omits `refresh_token`
- refresh token rotation is preserved when the server returns a new one
