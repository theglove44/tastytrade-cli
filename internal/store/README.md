# internal/store — Phase 2

This package implements SQLite persistence for the automation pipeline.

## Storage path

`os.UserConfigDir()/tastytrade-cli/tastytrade.db`

Created automatically on first run. Directory created with `0700` permissions.

## Planned schema

### `positions` table
Point-in-time snapshots written every 5 minutes by the positions poller.

```sql
CREATE TABLE IF NOT EXISTS positions (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    account_number  TEXT    NOT NULL,
    symbol          TEXT    NOT NULL,
    instrument_type TEXT    NOT NULL,
    quantity        TEXT    NOT NULL,   -- decimal string, never float
    quantity_dir    TEXT    NOT NULL,   -- Long / Short
    avg_open_price  TEXT    NOT NULL,
    close_price     TEXT    NOT NULL,
    expires_at      TEXT,              -- RFC3339 or NULL
    snapshotted_at  TEXT    NOT NULL   -- RFC3339 UTC
);
```

### `fills` table
One row per order fill received from the account streamer.

```sql
CREATE TABLE IF NOT EXISTS fills (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    order_id        TEXT    NOT NULL,
    account_number  TEXT    NOT NULL,
    symbol          TEXT    NOT NULL,
    action          TEXT    NOT NULL,
    quantity        TEXT    NOT NULL,
    fill_price      TEXT    NOT NULL,
    filled_at       TEXT    NOT NULL,  -- RFC3339 UTC
    strategy        TEXT               -- label for P&L grouping
);
```

### `pnl_daily` table
Aggregated daily P&L per strategy, written end-of-day by a scheduled job.

## Migrations

Schema migrations are applied at startup via `db.go:migrateDB()`.
Migrations are idempotent (`CREATE TABLE IF NOT EXISTS`).
Migration version is tracked in a `schema_version` table.

## Dependencies

- `github.com/mattn/go-sqlite3` — already in `go.mod` (requires CGO)
- `internal/models` — for typed structs
