package cmd

import (
	"go.uber.org/zap"

	"github.com/theglove44/tastytrade-cli/internal/client"
	"github.com/theglove44/tastytrade-cli/internal/models"
	"github.com/theglove44/tastytrade-cli/internal/valuation"
)

// quoteEventHandler implements streamer.QuoteHandler.
// It bridges DXLink quote events to:
//   - MarkBook.ApplyQuote() for in-memory mark-to-market state
//   - Prometheus metrics: QuotesReceived, LastQuoteTime
//
// Called from the market streamer's dispatch goroutine — must not block.
// All work is O(1) mutex operations on the MarkBook.
type quoteEventHandler struct {
	book *valuation.MarkBook
	log  *zap.Logger
}

// newQuoteEventHandler creates a handler backed by the given MarkBook.
func newQuoteEventHandler(book *valuation.MarkBook, log *zap.Logger) *quoteEventHandler {
	return &quoteEventHandler{book: book, log: log}
}

// OnQuote implements streamer.QuoteHandler.
// Applies the quote to the MarkBook and updates Prometheus metrics.
func (h *quoteEventHandler) OnQuote(ev models.QuoteEvent) {
	snap := h.book.ApplyQuote(
		ev.Symbol,
		ev.BidPrice,
		ev.AskPrice,
		ev.LastPrice,
		ev.MarkPrice,
		ev.MarkStale,
		ev.EventTime,
	)

	// Update metrics — both are non-blocking gauge/counter operations.
	client.Metrics.QuotesReceived.WithLabelValues(ev.Symbol).Inc()
	client.Metrics.LastQuoteTime.SetToCurrentTime()

	if h.log.Core().Enabled(zap.DebugLevel) {
		h.log.Debug("quote applied",
			zap.String("symbol", ev.Symbol),
			zap.String("mark", snap.MarkPrice.String()),
			zap.Bool("stale", snap.MarkStale),
			zap.String("unrealized_pnl", snap.UnrealizedPnL.String()),
		)
	}
}
