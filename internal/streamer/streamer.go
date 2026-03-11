// Package streamer implements the two WebSocket streaming connections specified
// in the v4 canonical document (§1.5 and §1.6).
//
// Phase 2A: account streamer only.
// Phase 2B: DXLink market streamer (not implemented here).
//
// Concurrency contract for handlers:
//
//	Handler methods (OnOrderEvent, OnBalanceEvent, OnPositionEvent) are called
//	from a dedicated dispatch goroutine — NOT from the WebSocket receive loop.
//	Implementations must still be fast; any blocking I/O should be done
//	asynchronously (e.g. hand off to a buffered channel).
//
// Streamer lifecycle:
//
//	Start(ctx) connects, subscribes, and runs until ctx is cancelled.
//	All reconnects are handled internally with exponential backoff.
//	Callers should run Start in a goroutine and cancel the context to stop.
package streamer

import (
	"context"
	"time"

	"github.com/theglove44/tastytrade-cli/internal/models"
)

// Streamer is the lifecycle interface for both account and market streamers.
// Start blocks until ctx is cancelled, managing all reconnects internally.
type Streamer interface {
	// Start connects, subscribes, and runs the streamer until ctx is done.
	// Returns only on context cancellation or a non-recoverable error.
	Start(ctx context.Context) error

	// Status returns a snapshot of current runtime state.
	// Safe to call from any goroutine at any time.
	Status() StreamerStatus

	// Name returns the stable label used in logs and metrics.
	// Values: "account" | "market"
	Name() string
}

// StreamerStatus is a point-in-time snapshot of streamer health.
// Exposed for health inspection; does not require locking by callers.
type StreamerStatus struct {
	// Name identifies the streamer ("account" | "market").
	Name string `json:"name"`

	// Connected is true when the WebSocket is currently established
	// and the subscription handshake has completed successfully.
	Connected bool `json:"connected"`

	// ConnectedSince is the time of the last successful subscription.
	// Zero if never successfully connected.
	ConnectedSince time.Time `json:"connected_since,omitempty"`

	// LastEventAt is the time the most recent event was dispatched.
	// Zero if no event has been received yet.
	LastEventAt time.Time `json:"last_event_at,omitempty"`

	// ReconnectCount is the total number of reconnect attempts since Start().
	// Does not include the initial connect.
	ReconnectCount int `json:"reconnect_count"`

	// LastError holds the most recent connection or subscription error,
	// or empty string if the last connect attempt succeeded.
	LastError string `json:"last_error,omitempty"`
}

// AccountHandler receives decoded account streamer events.
// All methods are called from the streamer's internal dispatch goroutine.
// Implementations must be safe for concurrent calls and must not block.
type AccountHandler interface {
	OnOrderEvent(event models.OrderEvent)
	OnBalanceEvent(event models.BalanceEvent)
	OnPositionEvent(event models.PositionEvent)
}

// QuoteHandler receives decoded DXLink quote events.
// Reserved for Phase 2B — no implementation in Phase 2A.
// Defined here so the interface set is complete and stable.
type QuoteHandler interface {
	OnQuote(event models.QuoteEvent)
}
