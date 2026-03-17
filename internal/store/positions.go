package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// WritePositionSnapshot persists a point-in-time position snapshot.
// Multiple snapshots for the same symbol are retained (append-only time-series).
func (s *sqliteStore) WritePositionSnapshot(ctx context.Context, snap PositionSnapshot) error {
	const q = `
		INSERT INTO positions
			(account_number, symbol, instrument_type, quantity, quantity_dir,
			 avg_open_price, close_price, expires_at, snapshotted_at, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	var expiresAt *string
	if snap.ExpiresAt != nil {
		s := snap.ExpiresAt.UTC().Format(time.RFC3339)
		expiresAt = &s
	}

	_, err := s.db.ExecContext(ctx, q,
		snap.AccountNumber,
		snap.Symbol,
		snap.InstrumentType,
		snap.Quantity,
		snap.QuantityDirection,
		snap.AvgOpenPrice,
		snap.ClosePrice,
		expiresAt,
		snap.SnapshottedAt.UTC().Format(time.RFC3339),
		string(snap.Source),
	)
	if err != nil {
		return fmt.Errorf("store.WritePositionSnapshot: %w", err)
	}
	return nil
}

// WriteBalance persists a balance update for an account.
// INSERT OR REPLACE keeps only the latest balance row per account_number.
func (s *sqliteStore) WriteBalance(ctx context.Context, bal BalanceRecord) error {
	const q = `
		INSERT OR REPLACE INTO balances
			(account_number, nlq, buying_power, updated_at, source)
		VALUES (?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, q,
		bal.AccountNumber,
		bal.NetLiquidatingValue,
		bal.BuyingPower,
		bal.UpdatedAt.UTC().Format(time.RFC3339),
		string(bal.Source),
	)
	if err != nil {
		return fmt.Errorf("store.WriteBalance: %w", err)
	}
	return nil
}

// LatestBalance returns the most recently stored balance for the account.
// Returns zero-value BalanceRecord with empty AccountNumber if no record exists.
func (s *sqliteStore) LatestBalance(ctx context.Context, accountID string) (BalanceRecord, error) {
	const q = `
		SELECT account_number, nlq, buying_power, updated_at, source
		FROM   balances
		WHERE  account_number = ?
		LIMIT  1`

	var b BalanceRecord
	var updatedAtStr, sourceStr string

	err := s.db.QueryRowContext(ctx, q, accountID).Scan(
		&b.AccountNumber, &b.NetLiquidatingValue, &b.BuyingPower,
		&updatedAtStr, &sourceStr,
	)
	if err == sql.ErrNoRows {
		// Return zero-value; callers check AccountNumber == "".
		return BalanceRecord{}, nil
	}
	if err != nil {
		return BalanceRecord{}, fmt.Errorf("store.LatestBalance: %w", err)
	}

	t, err := time.Parse(time.RFC3339, updatedAtStr)
	if err != nil {
		return BalanceRecord{}, fmt.Errorf("store.LatestBalance: parse updated_at: %w", err)
	}
	b.UpdatedAt = t
	b.Source = Source(sourceStr)
	return b, nil
}

// ActivePositionSymbols returns the distinct set of symbols from the most
// recent snapshot batch for the account. Used by the market streamer to
// build its initial subscription list on connect/reconnect.
//
// "Most recent batch" = all symbols whose snapshotted_at equals the MAX
// snapshotted_at for the account. This avoids including stale symbols from
// old snapshot rounds while position count changes between snapshots.
//
// Returns an empty slice (not an error) when no snapshots exist yet.
func (s *sqliteStore) ActivePositionSymbols(ctx context.Context, accountID string) ([]string, error) {
	const q = `
		SELECT DISTINCT symbol
		FROM   positions
		WHERE  account_number = ?
		AND    snapshotted_at = (
			SELECT MAX(snapshotted_at)
			FROM   positions
			WHERE  account_number = ?
		)`

	rows, err := s.db.QueryContext(ctx, q, accountID, accountID)
	if err != nil {
		return nil, fmt.Errorf("store.ActivePositionSymbols: query: %w", err)
	}
	defer rows.Close()

	var syms []string
	for rows.Next() {
		var sym string
		if err := rows.Scan(&sym); err != nil {
			return nil, fmt.Errorf("store.ActivePositionSymbols: scan: %w", err)
		}
		syms = append(syms, sym)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store.ActivePositionSymbols: rows: %w", err)
	}
	return syms, nil
}
