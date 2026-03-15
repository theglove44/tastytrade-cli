// Package exchange defines the backend-agnostic trading interface.
//
// All CLI commands depend on this interface rather than directly on the
// TastyTrade HTTP client. This enables:
//
//   - Easy unit testing with MockExchange
//   - Future alternative backends (paper trading, simulator, other brokers)
//   - Strict separation: HTTP transport lives in internal/client; domain
//     operations live in Exchange implementations
//
// The concrete TastyTrade implementation is in internal/exchange/tastytrade.
package exchange

import (
	"context"

	"github.com/theglove44/tastytrade-cli/internal/models"
)

// Exchange is the backend-agnostic contract for all trading operations.
// Implementations must be safe for concurrent use.
type Exchange interface {
	// Accounts returns all accounts for the authenticated user.
	Accounts(ctx context.Context) ([]models.Account, error)

	// Positions returns all open positions for the given account.
	Positions(ctx context.Context, accountID string) ([]models.Position, error)

	// Orders returns all live (open) orders for the given account.
	Orders(ctx context.Context, accountID string) ([]models.Order, error)

	// DryRun simulates an order submission without placing a live order.
	// idempotencyKey must be a pre-generated UUID that has already been written
	// to the intent log — this guarantees the logged key and the
	// X-Idempotency-Key HTTP header are always the same value.
	DryRun(ctx context.Context, accountID string, order models.NewOrder, idempotencyKey string) (models.DryRunResult, error)

	// Submit routes a live order to POST /accounts/{account_number}/orders.
	// idempotencyKey must be a pre-generated UUID that has already been written
	// to the intent log so the log record and request header stay identical.
	Submit(ctx context.Context, accountID string, order models.NewOrder, idempotencyKey string) (models.SubmitResult, error)

	// QuoteToken retrieves a fresh DXLink authentication token from
	// GET /api-quote-tokens. This endpoint is unversioned — the implementation
	// must use RequestOptions{SkipVersion: true}.
	// A new token must be fetched before every DXLink connect and reconnect.
	QuoteToken(ctx context.Context) (models.QuoteToken, error)
}
