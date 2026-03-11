// Package store implements SQLite persistence for the automation pipeline.
//
// Storage path: os.UserConfigDir()/tastytrade-cli/tastytrade.db
// Created automatically on first run with 0700 directory / 0600 file permissions.
//
// CGO requirement:
//
//	This package requires CGO_ENABLED=1 (default). If cross-compiling, use
//	a CGO-capable toolchain or replace the driver with a pure-Go alternative.
//
// Concurrency:
//
//	All Store methods are safe for concurrent use. WAL mode is enabled at
//	open time so reads and writes from different goroutines do not serialise.
package store

import (
	"context"
	"time"
)

// Source identifies which component wrote a record.
// This allows downstream consumers to distinguish live streamer data from
// REST snapshots or manual reconciliation runs.
type Source string

const (
	// SourceStreamer indicates the record was written by the account streamer.
	SourceStreamer Source = "streamer"
	// SourceRESTSync indicates the record was written by a REST polling pass.
	SourceRESTSync Source = "rest-sync"
	// SourceReconciliation indicates the record was written by a reconciliation job.
	SourceReconciliation Source = "reconciliation"
)

// Store is the persistence contract for the automation pipeline.
// All methods accept a context for cancellation and timeout.
// All monetary and quantity values are persisted as decimal strings — never float64.
type Store interface {
	// WriteFill persists a confirmed order fill from the account streamer.
	// Idempotent on OrderID: a duplicate fill for an existing OrderID is
	// silently ignored (returns nil).
	WriteFill(ctx context.Context, fill FillRecord) error

	// WritePositionSnapshot persists a point-in-time position snapshot.
	// Multiple snapshots for the same symbol are retained (time-series).
	WritePositionSnapshot(ctx context.Context, snap PositionSnapshot) error

	// WriteBalance persists a balance update.
	// Only the latest balance per account is retained; previous rows for the
	// same account_number are replaced.
	WriteBalance(ctx context.Context, bal BalanceRecord) error

	// LatestBalance returns the most recently stored balance for the account.
	// Returns (zero-value, nil) if no record exists — callers must check
	// BalanceRecord.AccountNumber for empty string to detect the zero case.
	LatestBalance(ctx context.Context, accountID string) (BalanceRecord, error)

	// RecentFills returns fills received after 'since', ordered by filled_at asc.
	RecentFills(ctx context.Context, accountID string, since time.Time) ([]FillRecord, error)

	// ActivePositionSymbols returns the distinct set of symbols from the most
	// recent position snapshot for the account.
	// Used by the market streamer to build its initial subscription list.
	// Returns an empty slice (not an error) if no snapshots exist yet.
	ActivePositionSymbols(ctx context.Context, accountID string) ([]string, error)

	// Close releases the database connection.
	// Must be called when the store is no longer needed (typically deferred).
	Close() error
}

// FillRecord is the persistence model for a single confirmed fill.
// Distinct from models.OrderEvent — only the fields required for persistence.
type FillRecord struct {
	// OrderID is the TastyTrade order ID. Used as the idempotency key.
	OrderID       string
	AccountNumber string
	Symbol        string
	Action        string    // e.g. "Sell to Open", "Buy to Close"
	Quantity      string    // decimal string — never float64
	FillPrice     string    // decimal string — never float64
	FilledAt      time.Time
	// Strategy is the label from the originating intent log entry.
	// Empty string when written directly from the streamer; can be
	// backfilled by a reconciliation pass that joins against the intent log.
	Strategy string
	// Source identifies what wrote this record.
	Source Source
}

// PositionSnapshot is the persistence model for a position at a point in time.
type PositionSnapshot struct {
	AccountNumber     string
	Symbol            string
	InstrumentType    string
	Quantity          string    // decimal string
	QuantityDirection string    // Long / Short
	AvgOpenPrice      string    // decimal string
	ClosePrice        string    // decimal string
	ExpiresAt         *time.Time
	SnapshottedAt     time.Time
	// Source identifies what wrote this snapshot.
	Source Source
}

// BalanceRecord is the persistence model for account balance.
type BalanceRecord struct {
	AccountNumber       string
	NetLiquidatingValue string    // decimal string
	BuyingPower         string    // decimal string
	UpdatedAt           time.Time
	// Source identifies what wrote this record.
	Source Source
}
