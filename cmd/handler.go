package cmd

import (
	"time"

	"go.uber.org/zap"

	"github.com/theglove44/tastytrade-cli/internal/bus"
	"github.com/theglove44/tastytrade-cli/internal/models"
)

// accountPublisher implements streamer.AccountHandler.
//
// It is the sole subscriber-facing adapter between the account streamer and the
// internal event bus. Its only job is to forward decoded events onto the three
// event buses — it holds NO side-effect logic (no store writes, no MarkBook
// calls, no metric updates, no market streamer subscriptions).
//
// Side-effects live in consumer goroutines wired in root.go:
//
//	orderBus    → orderConsumer    (WriteFill, metrics)
//	balanceBus  → balanceConsumer  (WriteBalance, NLQDollars metric)
//	positionBus → positionConsumer (MarkBook, mktStreamer.Subscribe, OpenPositions metric)
//
// This separation means adding a new consumer (e.g. strategy engine, alerter)
// requires only a new Subscribe call in root.go — zero changes to handler code.
//
// All Publish calls are non-blocking (drop-on-full); the bus capacity must be
// sized to absorb bursts without dropping under normal load.
type accountPublisher struct {
	orderBus    *bus.Broker[models.OrderEvent]
	balanceBus  *bus.Broker[models.BalanceEvent]
	positionBus *bus.Broker[models.PositionEvent]
	log         *zap.Logger
}

// newAccountPublisher creates an accountPublisher backed by the given buses.
func newAccountPublisher(
	orderBus *bus.Broker[models.OrderEvent],
	balanceBus *bus.Broker[models.BalanceEvent],
	positionBus *bus.Broker[models.PositionEvent],
	log *zap.Logger,
) *accountPublisher {
	return &accountPublisher{
		orderBus:    orderBus,
		balanceBus:  balanceBus,
		positionBus: positionBus,
		log:         log,
	}
}

// OnOrderEvent implements streamer.AccountHandler.
// Publishes the event to orderBus. Non-blocking.
func (p *accountPublisher) OnOrderEvent(ev models.OrderEvent) {
	p.orderBus.Publish(ev)
}

// OnBalanceEvent implements streamer.AccountHandler.
// Publishes the event to balanceBus. Non-blocking.
func (p *accountPublisher) OnBalanceEvent(ev models.BalanceEvent) {
	p.balanceBus.Publish(ev)
}

// OnPositionEvent implements streamer.AccountHandler.
// Publishes the event to positionBus. Non-blocking.
func (p *accountPublisher) OnPositionEvent(ev models.PositionEvent) {
	p.positionBus.Publish(ev)
}

// clock is a variable to allow test injection of time.Now.
var clock = func() time.Time { return time.Now() }
