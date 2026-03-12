# MI-2026-03-11 Auth Runtime and Parity

## Summary

This MI tracks the auth investigation and subsequent surgical fixes made to align the Go CLI with the known-good Python SDK refresh flow.

## Canonical detailed notes

See:

- `docs/auth-fix-review.md`

That file contains:

- imported patch inspection
- diff summaries
- auth request body findings
- `client_id` parity mismatch
- auth parity fix
- token persistence bug and fix
- validation results

## Final state

Auth flow now matches the known-good Python SDK refresh flow on the key compatibility points:

- safe prod/sandbox handling
- flat OAuth parsing
- underscore OAuth fields
- refresh-token POST body includes only:
  - `grant_type`
  - `client_secret`
  - `refresh_token`
- login stores a refresh token even if the server omits one in the success response
- runtime refresh preserves the old refresh token when the server omits a new one
- runtime refresh rotates stored refresh tokens when the server returns a new one

## Validation status

Passed during the investigation:

- `gofmt`
- `go build ./...`
- `go vet ./...`
- `go test ./...`
