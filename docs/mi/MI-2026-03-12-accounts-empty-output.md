# MI-2026-03-12 Accounts Empty Output

## Trigger

Observed runtime behavior:

- accounts request returned HTTP 200
- accounts table header printed
- zero meaningful account rows displayed

## Scope inspected

Files inspected:

- `cmd/accounts.go`
- `internal/exchange/exchange.go`
- `internal/exchange/tastytrade/tastytrade.go`
- `internal/models/models.go`
- Python SDK reference:
  - `.../site-packages/tastytrade/account.py`

## Exact API path

Accounts are fetched from:

- `GET {baseURL}/customers/me/accounts`

## Current Go parsing path

`cmd/accounts.go` calls:

- `ex.Accounts(ctx)`

`internal/exchange/tastytrade/tastytrade.go` currently parses the response as:

- `models.ItemsEnvelope[models.Account]`

Current account model:

```go
type Account struct {
    AccountNumber string `json:"account-number"`
    AccountType   string `json:"account-type"`
    Nickname      string `json:"nickname"`
    IsClosed      bool   `json:"is-closed"`
    IsFirmError   bool   `json:"is-firm-error"`
}
```

## Rendering behavior

`cmd/accounts.go` does not filter rows before printing.

It simply:

- fetches `accounts`
- prints the table header
- iterates every returned row

So the command itself is not suppressing rows.

## Comparison with known-good Python SDK

Python SDK list-accounts code uses:

- `GET /customers/me/accounts`
- iterates `data["items"]`
- extracts each row from `i["account"]`

That implies the real list response shape is effectively:

```json
{
  "data": {
    "items": [
      {
        "account": {
          "account-number": "...",
          "account-type": "...",
          "nickname": "...",
          "is-closed": false
        }
      }
    ]
  }
}
```

## Findings

Most likely cause of the empty/blank output:

- the Go response model shape is wrong for the accounts list endpoint

Specifically:

- Go expects `data.items[]` to be `Account` directly
- Python SDK indicates each item is wrapped as `data.items[].account`

Likely result in Go:

- unmarshal succeeds
- nested `account` object is ignored when decoding into `models.Account`
- rows become zero-value `Account` entries
- table prints header and effectively blank rows / no useful account data

## Direct answers

### 1. Exact API path used for accounts

- `GET {baseURL}/customers/me/accounts`

### 2. Exact response model used to parse accounts

- `models.ItemsEnvelope[models.Account]`

### 3. Why is the parsed accounts slice empty / blank?

Most likely:

- response model shape is wrong

Not likely:

- API actually returned zero accounts
- command filtered rows before rendering

### 4. Should `accounts` start the market streamer?

Currently it can, because streamer startup happens in `cmd/root.go` persistent pre-run.

Operationally, `accounts` should not need the market streamer at all.

However, that is not the cause of the empty accounts output.

### 5. Smallest surgical fix

Add a tiny wrapper model for the list endpoint, for example:

```go
type AccountListItem struct {
    Account Account `json:"account"`
}
```

Then change only the accounts parser to:

- unmarshal `models.ItemsEnvelope[models.AccountListItem]`
- flatten to `[]models.Account`

## Proposed surgical fix

Minimal change set:

- `internal/models/models.go`
  - add `AccountListItem`
- `internal/exchange/tastytrade/tastytrade.go`
  - change `Accounts()` parsing only

No command-layer refactor required.

## Applied surgical fix

Applied the smallest model/parsing fix only.

Files changed:

- `internal/models/models.go`
- `internal/exchange/tastytrade/tastytrade.go`

### Patch summary

Added a wrapper model:

```go
type AccountListItem struct {
    Account Account `json:"account"`
}
```

Changed `Exchange.Accounts()` to parse:

- `models.ItemsEnvelope[models.AccountListItem]`

and then flatten:

- `[]models.Account`

No command-layer rendering logic was changed.
No streamer logic was changed.

## Validation status

Ran:

```bash
gofmt -w internal/models/models.go internal/exchange/tastytrade/tastytrade.go
go build ./...
go vet ./...
go test ./...
```

Results:

- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

## Current conclusion

The accounts empty-output issue was caused by a model mismatch on the list endpoint:

- real shape: `data.items[].account`
- old Go expectation: `data.items[]`

That mismatch has now been fixed surgically.

---

## Follow-up runtime issue: positions requires explicit account ID

### Trigger

After the accounts fix:

```bash
./tastytrade-cli accounts --no-streamer
```

worked and showed accounts, but:

```bash
./tastytrade-cli positions --no-streamer
```

returned:

- `Error: positions: TASTYTRADE_ACCOUNT_ID is not set`

### Finding

`cmd/positions.go` previously required `cfg.AccountID` unconditionally.

That meant:

- the CLI could successfully authenticate
- the CLI could successfully list accounts
- but positions still failed unless `TASTYTRADE_ACCOUNT_ID` was manually set

### Applied surgical fix

Files changed:

- `cmd/account_resolver.go`
- `cmd/positions.go`
- `cmd/account_resolver_test.go`

Behavior after fix:

- if `TASTYTRADE_ACCOUNT_ID` is set, positions uses it unchanged
- if it is not set and the API returns exactly one account, positions auto-selects that account
- if multiple accounts exist, positions fails with a clearer error listing available account numbers
- if zero accounts are returned, positions fails with a clear error

This keeps the fix surgical and scoped to account selection for positions.

### Validation

Ran:

```bash
gofmt -w cmd/account_resolver.go cmd/positions.go cmd/account_resolver_test.go
go build ./...
go vet ./...
go test ./...
```

Results:

- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

### Current status

`positions` no longer requires `TASTYTRADE_ACCOUNT_ID` when the authenticated user has exactly one account.

A similar improvement may later be desirable for:

- `orders`
- `dry-run`

---

## Follow-up operational override: hard-code target account

### Trigger

After the resolver improvement, the authenticated user still had multiple accounts and `positions` correctly returned:

- `Error: positions: multiple accounts returned; set TASTYTRADE_ACCOUNT_ID to one of: 5WX63633, 5WW46136`

Requested outcome:

- hard-code account `5WW46136`

### Applied surgical fix

File changed:

- `config/config.go`

Change made:

- added `DefaultAccountID = "5WW46136"`
- changed config loading so `AccountID` defaults to that value when `TASTYTRADE_ACCOUNT_ID` is not set

Effective behavior now:

- if `TASTYTRADE_ACCOUNT_ID` is set, it still wins
- if it is unset, CLI defaults to account `5WW46136`

### Validation

Ran:

```bash
gofmt -w config/config.go
go build ./...
go vet ./...
go test ./...
```

Results:

- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./...` ✅

### Current status

Account-scoped commands now default to account `5WW46136` unless overridden by `TASTYTRADE_ACCOUNT_ID`.

---

## Verification outcome

Confirmed after the override fix:

- `./tastytrade-cli positions --no-streamer`
  - now works
  - shows positions successfully

This closes the immediate runtime issue chain for:

- accounts list output
- positions account selection / defaulting
