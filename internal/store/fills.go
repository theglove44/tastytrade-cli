package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// WriteFill persists a confirmed order fill.
// Idempotent: duplicate order_id is silently ignored (INSERT OR IGNORE).
func (s *sqliteStore) WriteFill(ctx context.Context, fill FillRecord) error {
	const q = `
		INSERT OR IGNORE INTO fills
			(order_id, account_number, symbol, action, quantity,
			 fill_price, filled_at, strategy, source)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := s.db.ExecContext(ctx, q,
		fill.OrderID,
		fill.AccountNumber,
		fill.Symbol,
		fill.Action,
		fill.Quantity,
		fill.FillPrice,
		fill.FilledAt.UTC().Format(time.RFC3339),
		fill.Strategy,
		string(fill.Source),
	)
	if err != nil {
		return fmt.Errorf("store.WriteFill: %w", err)
	}
	return nil
}

// RecentFills returns fills received after 'since', ordered by filled_at asc.
func (s *sqliteStore) RecentFills(ctx context.Context, accountID string, since time.Time) ([]FillRecord, error) {
	const q = `
		SELECT order_id, account_number, symbol, action, quantity,
		       fill_price, filled_at, strategy, source
		FROM   fills
		WHERE  account_number = ?
		AND    filled_at > ?
		ORDER  BY filled_at ASC`

	rows, err := s.db.QueryContext(ctx, q, accountID, since.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("store.RecentFills: query: %w", err)
	}
	defer rows.Close()

	var fills []FillRecord
	for rows.Next() {
		var f FillRecord
		var filledAtStr, sourceStr string
		if err := rows.Scan(
			&f.OrderID, &f.AccountNumber, &f.Symbol, &f.Action,
			&f.Quantity, &f.FillPrice, &filledAtStr, &f.Strategy, &sourceStr,
		); err != nil {
			return nil, fmt.Errorf("store.RecentFills: scan: %w", err)
		}
		t, err := time.Parse(time.RFC3339, filledAtStr)
		if err != nil {
			return nil, fmt.Errorf("store.RecentFills: parse filled_at: %w", err)
		}
		f.FilledAt = t
		f.Source = Source(sourceStr)
		fills = append(fills, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store.RecentFills: rows: %w", err)
	}
	return fills, nil
}

// rowExists is a helper used by tests to verify idempotency.
func rowExists(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, table, col, val string) (bool, error) {
	var n int
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?", table, col)
	if err := db.QueryRowContext(ctx, q, val).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}
