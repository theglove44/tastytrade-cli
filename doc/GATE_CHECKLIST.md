# Go-Live Gate Checklist

**Version:** v4 — March 2026  
**Source:** tastytrade-api-guide-v4.docx, Part 5  
**Purpose:** Every item must be ticked and signed off before the first automated live production order.

This file lives in `/doc/` and is committed to the repository. When you complete it, add your name, date, and commit hash to the sign-off block at the bottom.

---

## Pre-conditions

Before starting this checklist, confirm all three:

- [ ] All Phase 1–9 items in the build checklist (`config/config.go` Phase Checklist) are complete
- [ ] Stage 1 (Sandbox Read-Only) test plan: all items passing
- [ ] Stage 2 (Dry-Run Orders) test plan: all items passing
- [ ] Stage 3 (Live Orders, Sandbox) test plan: all items passing

---

## Safety Controls

### Circuit Breaker

- [ ] `CircuitBreaker` initialised with `maxOrders=10, window=time.Hour` (or tighter)
- [ ] Submit test orders until the limit is hit on cert — verify trip fires
- [ ] Verify Prometheus gauge `tastytrade_circuit_breaker_state` transitions to 1 on trip
- [ ] Verify alert fires (Telegram / webhook) with strategy, count, window in message
- [ ] Verify no further orders are accepted after trip (including dry-run)
- [ ] Verify `Reset()` requires a deliberate call — does not auto-recover
- [ ] Verify `tt resume` does **not** reset the circuit breaker (they are independent)

### Kill Switch — File

- [ ] `touch ~/.config/tastytrade-cli/KILL` while bot is running → next order attempt blocked
- [ ] Log entry confirms kill switch detected with reason `kill file present: <path>`
- [ ] `rm ~/.config/tastytrade-cli/KILL` → order submission resumes (no restart needed)
- [ ] `tt kill` creates the file, `tt resume` removes it — both idempotent
- [ ] Kill file path printed on startup for operator reference

### Kill Switch — Env Var

- [ ] `TASTYTRADE_KILL_SWITCH=true` in running env → order blocked
- [ ] `tt resume` warns that env var kill is still active if TASTYTRADE_KILL_SWITCH=true
- [ ] Unsetting env var restores submission without restart (env var is read at check time, not startup)

### NLQ Floor Guard

- [ ] Set `TASTYTRADE_NLQ_FLOOR_ABSOLUTE` above current NLQ in sandbox → opening order blocked
- [ ] Verify closing order (STC/BTC action) is **permitted** while floor is breached
- [ ] Verify `tastytrade_nlq_dollars` gauge reflects live NLQ
- [ ] Restore floor below current NLQ → opening orders resume

---

## Alerting

All alerts must fire asynchronously — they must never block the order path.  
Test each in sandbox before going live.

- [ ] Live order submitted → alert received with symbol, legs, price, account, timestamp
- [ ] Order fill received → alert received with fill price, quantity
- [ ] Order rejected (422) → alert received with full `error.message`
- [ ] Circuit breaker trip → alert received with order count, window, HALT status
- [ ] Kill switch activated → alert received with source (env/file), timestamp
- [ ] NLQ floor breached → alert received with current NLQ, floor, drawdown %
- [ ] Token refresh failed → alert received with error and whether token was retained
- [ ] Streamer disconnect → alert received with streamer name, reason, reconnect count
- [ ] 5xx from API → alert received with endpoint, status, retry count
- [ ] Startup → alert received with config summary (no credentials), environment, NLQ at open
- [ ] Shutdown → alert received with reason (normal exit, panic, signal)

---

## Metrics (Prometheus / Grafana)

- [ ] `curl http://127.0.0.1:9090/metrics` returns all expected metric names
- [ ] Prometheus scrape confirmed active (no scrape errors in Prometheus UI)
- [ ] Grafana dashboard loads with all key gauges populated:
  - [ ] `tastytrade_nlq_dollars`
  - [ ] `tastytrade_open_positions`
  - [ ] `tastytrade_circuit_breaker_state`
  - [ ] `tastytrade_kill_switch_state`
  - [ ] `tastytrade_rate_limit_hits_total` (by family)
  - [ ] `tastytrade_token_refresh_total` (by outcome)
  - [ ] `tastytrade_streamer_reconnects_total` (by streamer)
  - [ ] `tastytrade_order_latency_seconds` histogram
- [ ] Metrics port NOT externally accessible (bound to 127.0.0.1 only)

---

## Production Smoke Tests

- [ ] `TASTYTRADE_BASE_URL=https://api.tastytrade.com` (prod URL confirmed, no 'cert')
- [ ] `tt accounts` returns correct live account with expected account number
- [ ] `tt positions --json` returns parseable JSON with correct live positions
- [ ] Balances match TastyTrade platform (within last-update window)
- [ ] DXLink streamer connected on prod — live quotes arriving
- [ ] Account streamer connected on prod — fill events routing to DB

### Production First Order Rules

All five must be confirmed before submitting the first automated prod order:

- [ ] `tt dry-run --file order.json --json` passes with `"ok": true` on the intended order
- [ ] Order is single-contract, single-leg equity (not a spread or multi-leg)
- [ ] Underlying is liquid (SPY, QQQ, XSP, AAPL — not micro-cap)
- [ ] Immediate cancel is staged and ready (cancel command or portal open)
- [ ] Time is during regular market hours (09:30–16:00 ET on a trading day)

- [ ] First prod order submitted, immediately cancelled before fill — no fill received
- [ ] Cancel confirmed in `tt orders --json` output

---

## Emergency Procedures

These must be documented and **tested** before going live:

- [ ] Shell alias or script staged: `touch ~/.config/tastytrade-cli/KILL && echo "HALTED"`
- [ ] TastyTrade platform URL bookmarked for manual position close: https://tastytrade.com
- [ ] api.support@tastytrade.com saved in contacts (IP block recovery, API issues)
- [ ] Broker support phone number saved
- [ ] Procedure documented: how to manually close all positions without the bot

---

## Sign-Off

All items above are checked and verified.

| Field | Value |
|-------|-------|
| Completed by | |
| Date | |
| Environment | `https://api.tastytrade.com` (production) |
| Commit hash | |
| Notes | |

> **Once signed off:** commit this file with the sign-off block filled in.  
> The commit hash becomes the audit trail for the first production deployment.
