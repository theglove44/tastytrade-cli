# internal/web — Phase 2

This package implements a localhost-only HTTP dashboard for monitoring
the automation pipeline without leaving the terminal.

## Security constraints (non-negotiable)

- Bind address: `127.0.0.1` only — never `0.0.0.0`
- Default port: `8080` (env: `TASTYTRADE_WEB_ADDR`)
- No TLS required (localhost only)
- No order submission from the UI without a secondary confirmation prompt
- Prometheus `/metrics` endpoint exposed here too (proxied from client.Metrics)

## Planned endpoints

| Path | Description |
|---|---|
| `GET /` | Dashboard — positions, open orders, NLQ, circuit breaker state |
| `GET /positions` | JSON positions (mirrors `tt positions --json`) |
| `GET /orders` | JSON live orders (mirrors `tt orders --json`) |
| `GET /health` | `{"ok": true, "kill_switch": false, "breaker": "OK"}` |
| `GET /metrics` | Prometheus text format (proxied from `promhttp.Handler()`) |

## Technology

- Standard `net/http` — no external web framework
- HTMX for partial page refreshes (CDN import, no build step)
- Auto-refresh every 30s via `hx-trigger="every 30s"`
- No JavaScript framework

## Startup

The web server is started by `cmd/root.go` when `--serve` flag is passed,
or unconditionally if `TASTYTRADE_WEB_ADDR` is set.

## Dependencies

- `github.com/prometheus/client_golang/prometheus/promhttp` — already in `go.mod`
- `internal/client` — for `CheckOrderSafety()`, `BreakerState()`, `Metrics`
- `internal/models` — for typed response structs
