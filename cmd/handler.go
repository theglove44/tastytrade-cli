package cmd

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/theglove44/tastytrade-cli/internal/client"
	"github.com/theglove44/tastytrade-cli/internal/models"
	"github.com/theglove44/tastytrade-cli/internal/store"
	"github.com/theglove44/tastytrade-cli/internal/streamer"
	"github.com/theglove44/tastytrade-cli/internal/valuation"
)

// accountEventHandler implements streamer.AccountHandler.
// It bridges account streamer events to:
//   - store writes (fills, balances)
//   - MarkBook updates (position-side data for mark-to-market)
//   - market streamer subscriptions (new symbols when positions open)
//   - Prometheus metrics
//
// All handler methods are called from the account streamer's dispatch goroutine.
// They must not block; slow I/O (store writes) is delegated to goroutines.
type accountEventHandler struct {
	st          store.Store
	book        *valuation.MarkBook
	mktStreamer streamer.MarketStreamer // nil when market streamer is disabled
	log         *zap.Logger
}

// newAccountEventHandler creates a handler.
// mktStreamer may be nil — all mktStreamer calls are nil-guarded.
func newAccountEventHandler(
	st store.Store,
	book *valuation.MarkBook,
	mktStreamer streamer.MarketStreamer,
	log *zap.Logger,
) *accountEventHandler {
	return &accountEventHandler{
		st:          st,
		book:        book,
		mktStreamer: mktStreamer,
		log:         log,
	}
}

// OnOrderEvent handles order status changes from the account streamer.
// Confirmed fills (Status=="Filled") are persisted to the store.
func (h *accountEventHandler) OnOrderEvent(ev models.OrderEvent) {
	if ev.Status != "Filled" {
		return
	}

	symbol := ""
	action := ""
	qty := "0"
	price := "0"
	if len(ev.Legs) > 0 {
		leg := ev.Legs[0]
		symbol = leg.Symbol
		action = leg.Action
		qty = leg.FillQuantity.String()
		price = leg.FillPrice.String()
	}

	filledAt := ev.FilledAt
	if filledAt == nil {
		h.log.Warn("OnOrderEvent: Filled status but nil FilledAt — using now",
			zap.String("order_id", ev.OrderID))
		now := clock()
		filledAt = &now
	}

	rec := store.FillRecord{
		OrderID:       ev.OrderID,
		AccountNumber: ev.AccountNumber,
		Symbol:        symbol,
		Action:        action,
		Quantity:      qty,
		FillPrice:     price,
		FilledAt:      *filledAt,
		Strategy:      "",
		Source:        store.SourceStreamer,
	}

	go func() {
		if err := h.st.WriteFill(context.Background(), rec); err != nil {
			h.log.Error("OnOrderEvent: WriteFill failed",
				zap.String("order_id", ev.OrderID),
				zap.Error(err),
			)
			return
		}
		client.Metrics.OrdersFilled.WithLabelValues("").Inc()
		h.log.Info("fill persisted",
			zap.String("order_id", ev.OrderID),
			zap.String("symbol", symbol),
		)
	}()
}

// OnBalanceEvent handles account balance updates from the account streamer.
func (h *accountEventHandler) OnBalanceEvent(ev models.BalanceEvent) {
	rec := store.BalanceRecord{
		AccountNumber:       ev.AccountNumber,
		NetLiquidatingValue: ev.NetLiquidatingValue.String(),
		BuyingPower:         ev.BuyingPower.String(),
		UpdatedAt:           ev.UpdatedAt,
		Source:              store.SourceStreamer,
	}

	go func() {
		if err := h.st.WriteBalance(context.Background(), rec); err != nil {
			h.log.Error("OnBalanceEvent: WriteBalance failed",
				zap.String("account", ev.AccountNumber),
				zap.Error(err),
			)
			return
		}
		nlq, _ := ev.NetLiquidatingValue.Float64()
		client.Metrics.NLQDollars.Set(nlq)
		h.log.Debug("balance updated",
			zap.String("nlq", ev.NetLiquidatingValue.String()),
			zap.String("buying_power", ev.BuyingPower.String()),
		)
	}()
}

// OnPositionEvent handles position changes from the account streamer.
//
// Open / Change:
//   - Loads (or updates) the position in the MarkBook so the valuation layer
//     has current quantity and cost-basis data.
//   - Subscribes the market streamer to the symbol so DXLink quotes arrive.
//     Subscribe() is idempotent — calling it for an already-subscribed symbol
//     is a no-op (deduplicated inside marketStreamer).
//
// Close:
//   - Removes the position from the MarkBook. The quote entry is also removed
//     because there is no longer a position to value against it.
//     Rationale: retaining a closed position in the MarkBook would cause
//     AllSnapshots() to include it in P&L roll-ups, giving a false picture of
//     open risk. The market streamer subscription is NOT removed — DXLink does
//     not support per-symbol unsubscribe in the current wire protocol, and a
//     stale quote for a closed symbol is harmless (no position → no P&L).
//   - Decrements the OpenPositions gauge.
func (h *accountEventHandler) OnPositionEvent(ev models.PositionEvent) {
	switch ev.Action {
	case "Open", "Change":
		// AvgOpenPrice is not carried by the account streamer PositionEvent wire
		// format. We pass decimal.Zero as a sentinel; the REST position poller
		// (startup seeding and periodic refresh) will overwrite with the real
		// basis via book.LoadPosition(). Until then, UnrealizedPnL will be
		// incorrect but the position is subscribed and mark price updates flow.
		h.book.LoadPosition(
			ev.Symbol,
			ev.AccountNumber,
			ev.Quantity.String(),
			ev.QuantityDirection,
			decimal.Zero,
		)

		if h.mktStreamer != nil {
			h.mktStreamer.Subscribe(ev.Symbol)
		}

		if ev.Action == "Open" {
			client.Metrics.OpenPositions.Inc()
		}

		h.log.Debug("position opened/changed",
			zap.String("symbol", ev.Symbol),
			zap.String("action", ev.Action),
			zap.String("qty", ev.Quantity.String()),
			zap.String("direction", ev.QuantityDirection),
		)

	case "Close":
		h.book.RemovePosition(ev.Symbol)
		client.Metrics.OpenPositions.Dec()
		h.log.Debug("position closed",
			zap.String("symbol", ev.Symbol),
		)
	}
}

// clock is a variable to allow test injection of time.Now.
var clock = func() time.Time { return time.Now() }
