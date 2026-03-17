# `tt watch` metrics

Representative application metrics exposed by `tt watch` via the Prometheus `/metrics` endpoint.

Notes:
- Values are runtime-dependent and will vary by session.
- Standard Go/process/promhttp metrics are omitted below for brevity.
- `tastytrade_last_quote_unix_seconds` can legitimately remain `0` until a live quote event is actually received.
- Phase 3A added structured reconcile status and reconcile policy metrics so operators can distinguish healthy, drift, partial, and error states without parsing logs.

## Order / API / auth metrics

- `tastytrade_orders_submitted_total`
  - labels: `strategy`
  - total submitted orders
- `tastytrade_orders_filled_total`
  - labels: `strategy`
  - total filled orders
- `tastytrade_order_latency_seconds`
  - histogram of dry-run to fill latency
- `tastytrade_api_errors_total`
  - labels: `family`, `code`
  - API error totals by request family and server code
- `tastytrade_rate_limit_hits_total`
  - total rate-limit hits
- `tastytrade_token_refresh_total`
  - labels: `outcome`
  - token refresh attempts by outcome
- `tastytrade_request_duration_seconds`
  - labels: `family`, `method`
  - HTTP request duration by request family and method

## Runtime / safety metrics

- `tastytrade_nlq_dollars`
  - current net liquidating value in USD
- `tastytrade_open_positions`
  - current open position count
- `tastytrade_circuit_breaker_state`
  - `0=normal`, `1=tripped`
- `tastytrade_kill_switch_state`
  - `0=normal`, `1=halted`

## Streamer / quote-flow metrics

- `tastytrade_streamer_reconnects_total`
  - labels: `streamer`
  - reconnect attempts by streamer
- `tastytrade_streamer_uptime_seconds`
  - labels: `streamer`
  - seconds since last successful streamer connection
- `tastytrade_quotes_received_total`
  - labels: `symbol`
  - total decoded DXLink quote events by symbol
- `tastytrade_tracked_symbols`
  - current subscribed symbol count on the market streamer
- `tastytrade_last_quote_unix_seconds`
  - Unix timestamp of the most recent quote event, or `0` if none yet
- `tastytrade_bus_dropped_events_total`
  - labels: `bus`
  - dropped internal bus events due to subscriber back-pressure

## Reconciler metrics

- `tastytrade_reconcile_runs_total`
  - total reconciliation passes attempted
- `tastytrade_reconcile_errors_total`
  - reconciliation passes that failed due to a REST error
- `tastytrade_reconcile_positions_corrected_total`
  - MarkBook entries corrected by reconciliation
- `tastytrade_reconcile_runs_by_status_total`
  - labels: `status`
  - reconciliation passes by structured outcome status (`ok`, `drift_detected`, `partial`, `error`)
- `tastytrade_reconcile_errors_by_type_total`
  - labels: `type`
  - reconciliation errors bucketed by coarse type / error text
- `tastytrade_reconcile_last_status`
  - labels: `status`
  - one-hot gauge for the latest reconcile status
- `tastytrade_reconcile_last_duration_seconds`
  - duration of the latest reconcile pass
- `tastytrade_reconcile_last_mismatch_count`
  - mismatch count from the latest reconcile pass

## Reconcile policy metrics

- `tastytrade_reconcile_policy_mode`
  - labels: `mode`
  - one-hot gauge for the current operational handling mode (`observe`, `limited_recovery`, `suppress`)
- `tastytrade_reconcile_policy_degraded`
  - `1` when the current reconciler policy marks runtime as degraded, else `0`
- `tastytrade_reconcile_policy_suppress_confidence_actions`
  - `1` when confidence-dependent actions should be suppressed, else `0`

## Representative reconcile sample

```text
# HELP tastytrade_reconcile_runs_total Total reconciliation passes attempted (success + failure).
# TYPE tastytrade_reconcile_runs_total counter
tastytrade_reconcile_runs_total 10
# HELP tastytrade_reconcile_runs_by_status_total Reconciliation passes by structured outcome status.
# TYPE tastytrade_reconcile_runs_by_status_total counter
tastytrade_reconcile_runs_by_status_total{status="ok"} 8
tastytrade_reconcile_runs_by_status_total{status="drift_detected"} 1
tastytrade_reconcile_runs_by_status_total{status="partial"} 1
tastytrade_reconcile_runs_by_status_total{status="error"} 0
# HELP tastytrade_reconcile_last_status One-hot gauge for the latest reconciliation status by label (1 = latest status, 0 = not latest).
# TYPE tastytrade_reconcile_last_status gauge
tastytrade_reconcile_last_status{status="ok"} 0
tastytrade_reconcile_last_status{status="drift_detected"} 0
tastytrade_reconcile_last_status{status="partial"} 1
tastytrade_reconcile_last_status{status="error"} 0
# HELP tastytrade_reconcile_last_duration_seconds Duration in seconds of the latest reconciliation pass.
# TYPE tastytrade_reconcile_last_duration_seconds gauge
tastytrade_reconcile_last_duration_seconds 0.118406083
# HELP tastytrade_reconcile_last_mismatch_count Mismatch count from the latest reconciliation pass.
# TYPE tastytrade_reconcile_last_mismatch_count gauge
tastytrade_reconcile_last_mismatch_count 1
# HELP tastytrade_reconcile_policy_mode One-hot gauge for the current reconciler operational handling mode.
# TYPE tastytrade_reconcile_policy_mode gauge
tastytrade_reconcile_policy_mode{mode="observe"} 1
tastytrade_reconcile_policy_mode{mode="limited_recovery"} 0
tastytrade_reconcile_policy_mode{mode="suppress"} 0
# HELP tastytrade_reconcile_policy_degraded 1 when reconciler policy currently considers runtime degraded, else 0.
# TYPE tastytrade_reconcile_policy_degraded gauge
tastytrade_reconcile_policy_degraded 1
# HELP tastytrade_reconcile_policy_suppress_confidence_actions 1 when reconciler policy currently suppresses confidence-dependent actions, else 0.
# TYPE tastytrade_reconcile_policy_suppress_confidence_actions gauge
tastytrade_reconcile_policy_suppress_confidence_actions 0
```
