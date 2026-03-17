package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3" // CGO SQLite driver
	"go.uber.org/zap"
)

// currentSchemaVersion is incremented whenever the schema changes.
// Migrations are applied in order from the current version to this target.
const currentSchemaVersion = 1

// sqliteStore is the production Store implementation backed by SQLite.
// Compile-time interface assertion at bottom of file.
type sqliteStore struct {
	db  *sql.DB
	log *zap.Logger
}

// Open opens (or creates) the SQLite database at the canonical path.
// The directory is created with 0700 permissions if it does not exist.
// WAL mode is enabled immediately after open.
// Schema migrations are applied idempotently.
//
// Returns a Store ready for concurrent use.
func Open(log *zap.Logger) (Store, error) {
	dir, err := dbDir()
	if err != nil {
		return nil, fmt.Errorf("store.Open: resolve dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("store.Open: mkdir: %w", err)
	}

	path := filepath.Join(dir, "tastytrade.db")
	// _busy_timeout=5000: wait up to 5s on SQLITE_BUSY before erroring.
	// _foreign_keys=on:   enforce FK constraints.
	dsn := fmt.Sprintf("file:%s?_busy_timeout=5000&_foreign_keys=on", path)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("store.Open: sql.Open: %w", err)
	}

	// Single writer connection is the recommended pattern for SQLite.
	// Readers can share the pool; writers acquire the single write slot.
	db.SetMaxOpenConns(1)

	s := &sqliteStore{db: db, log: log}

	// Enable WAL mode — must happen before migrations.
	if err := s.enableWAL(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open: WAL: %w", err)
	}

	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open: migrate: %w", err)
	}

	log.Info("store opened", zap.String("path", path))
	return s, nil
}

// dbDir returns os.UserConfigDir()/tastytrade-cli, the canonical data directory.
func dbDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "tastytrade-cli"), nil
}

// Close releases the database connection.
func (s *sqliteStore) Close() error {
	s.log.Info("store closing")
	return s.db.Close()
}

// enableWAL switches SQLite to Write-Ahead Logging mode.
// WAL allows concurrent reads alongside a write, which prevents SQLITE_BUSY
// errors when the streamer fill-writer and the position poller run concurrently.
func (s *sqliteStore) enableWAL(ctx context.Context) error {
	var mode string
	row := s.db.QueryRowContext(ctx, "PRAGMA journal_mode=WAL")
	if err := row.Scan(&mode); err != nil {
		return fmt.Errorf("enableWAL: %w", err)
	}
	if mode != "wal" {
		return fmt.Errorf("enableWAL: expected 'wal', got %q", mode)
	}
	s.log.Debug("SQLite WAL mode enabled")
	return nil
}

// migrate applies schema migrations idempotently up to currentSchemaVersion.
// Each migration step is wrapped in a transaction.
func (s *sqliteStore) migrate(ctx context.Context) error {
	// Ensure the schema_version table exists first.
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("migrate: create schema_version: %w", err)
	}

	// Read current version (0 = never migrated).
	var version int
	row := s.db.QueryRowContext(ctx, "SELECT version FROM schema_version LIMIT 1")
	if err := row.Scan(&version); err != nil {
		// No row yet → version 0.
		version = 0
	}

	if version >= currentSchemaVersion {
		s.log.Debug("schema up to date", zap.Int("version", version))
		return nil
	}

	s.log.Info("applying schema migrations",
		zap.Int("from", version),
		zap.Int("to", currentSchemaVersion),
	)

	// Apply each migration in order.
	migrations := []func(context.Context, *sql.Tx) error{
		migrationV1,
	}

	for i := version; i < currentSchemaVersion; i++ {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("migrate: begin tx v%d: %w", i+1, err)
		}
		if err := migrations[i](ctx, tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migrate: v%d: %w", i+1, err)
		}
		// Update the version counter inside the same transaction.
		if _, err := tx.ExecContext(ctx,
			"DELETE FROM schema_version"); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migrate: clear version: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO schema_version (version) VALUES (?)", i+1); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migrate: write version: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("migrate: commit v%d: %w", i+1, err)
		}
		s.log.Info("migration applied", zap.Int("version", i+1))
	}
	return nil
}

// migrationV1 creates the initial schema.
func migrationV1(ctx context.Context, tx *sql.Tx) error {
	stmts := []string{
		// fills: one row per confirmed order fill.
		// order_id is unique — duplicate fills are silently ignored via INSERT OR IGNORE.
		`CREATE TABLE IF NOT EXISTS fills (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			order_id        TEXT    NOT NULL UNIQUE,
			account_number  TEXT    NOT NULL,
			symbol          TEXT    NOT NULL,
			action          TEXT    NOT NULL,
			quantity        TEXT    NOT NULL,
			fill_price      TEXT    NOT NULL,
			filled_at       TEXT    NOT NULL,
			strategy        TEXT    NOT NULL DEFAULT '',
			source          TEXT    NOT NULL DEFAULT 'streamer'
		)`,
		`CREATE INDEX IF NOT EXISTS fills_account_filled
			ON fills (account_number, filled_at)`,

		// positions: time-series snapshots written by poller or streamer.
		`CREATE TABLE IF NOT EXISTS positions (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			account_number  TEXT    NOT NULL,
			symbol          TEXT    NOT NULL,
			instrument_type TEXT    NOT NULL,
			quantity        TEXT    NOT NULL,
			quantity_dir    TEXT    NOT NULL,
			avg_open_price  TEXT    NOT NULL,
			close_price     TEXT    NOT NULL,
			expires_at      TEXT,
			snapshotted_at  TEXT    NOT NULL,
			source          TEXT    NOT NULL DEFAULT 'streamer'
		)`,
		`CREATE INDEX IF NOT EXISTS positions_account_snap
			ON positions (account_number, snapshotted_at)`,

		// balances: only the latest balance per account is retained.
		// The UNIQUE constraint on account_number combined with INSERT OR REPLACE
		// keeps one row per account.
		`CREATE TABLE IF NOT EXISTS balances (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			account_number  TEXT    NOT NULL UNIQUE,
			nlq             TEXT    NOT NULL,
			buying_power    TEXT    NOT NULL,
			updated_at      TEXT    NOT NULL,
			source          TEXT    NOT NULL DEFAULT 'streamer'
		)`,
	}
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

// Compile-time assertion: *sqliteStore must implement Store.
var _ Store = (*sqliteStore)(nil)
