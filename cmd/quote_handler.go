package cmd

import (
	"go.uber.org/zap"

	"github.com/theglove44/tastytrade-cli/internal/bus"
	"github.com/theglove44/tastytrade-cli/internal/models"
)

// quotePublisher implements streamer.QuoteHandler.
//
// Its only job is to forward DXLink quote events onto quoteBus.
// Side-effects (MarkBook.ApplyQuote, Prometheus metrics) live in the
// quoteConsumer goroutine wired in root.go.
//
// This keeps the market streamer's dispatch goroutine hot-path free of any
// mutex contention from store or MarkBook operations.
type quotePublisher struct {
	quoteBus *bus.Broker[models.QuoteEvent]
	log      *zap.Logger
}

// newQuotePublisher creates a quotePublisher backed by the given bus.
func newQuotePublisher(quoteBus *bus.Broker[models.QuoteEvent], log *zap.Logger) *quotePublisher {
	return &quotePublisher{quoteBus: quoteBus, log: log}
}

// OnQuote implements streamer.QuoteHandler.
// Publishes the event to quoteBus. Non-blocking.
func (p *quotePublisher) OnQuote(ev models.QuoteEvent) {
	p.quoteBus.Publish(ev)
}
